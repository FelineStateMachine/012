package sheet

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/FelineStateMachine/012/internal/functions"
)

// Spills, as Sheets' array results: a formula computing an array (its
// anchor) shows the first value and spills the rest into the cells to
// its right and below. Those are spilled cells, derived like a pivot's
// results: owned by the anchor, rewritten outside the undo history
// whenever it recomputes, refused by Set, constants to formulas reading
// them, values to copies and exports, and never saved (the file keeps
// the anchor's formula). Formatting a spilled cell keeps the formatting
// and the anchor spills its value into it again.
//
// Spills are written after each evaluation pass: the formulas reading
// spilled cells that changed are then recalculated, which may spill
// again (a FILTER of a SORT), up to maxSpillPasses. When the cells an
// array needs aren't empty, the anchor shows #REF! and says which cell
// is in the way; changing that cell has the anchor try again.

// spill is what an anchor spills, or would.
type spill struct {
	area Rect   // the cells it covers, the anchor first; when blocked, those it needs
	why  string // why the anchor shows #REF! instead, or ""
}

// ErrSpillEdit is what typing into a spilled cell says: its value
// belongs to the formula that spilled it.
var ErrSpillEdit = errors.New("That cell holds part of an array result: edit the formula it spills from")

// maxSpillPasses bounds the passes that spill arrays and recalculate what
// reads them, so arrays reading each other can't loop.
const maxSpillPasses = 64

// spillWork is the arrays computed in an evaluation pass, by anchor,
// with the value each anchor's formula computed.
type spillWork map[loc]pendingSpill

type pendingSpill struct {
	arr *functions.Array // nil when the formula computed one value
	v   Value            // what the formula computed, before spilling
}

// noteSpill records what the formula at l computed, when it spills or
// did.
func (w *Workbook) noteSpill(l loc, arr *functions.Array, v Value) {
	if arr == nil && l.s.spills[l.a] == nil {
		return
	}
	if w.spillWork == nil {
		w.spillWork = spillWork{}
	}
	w.spillWork[l] = pendingSpill{arr, v}
}

// settleSpills writes the arrays evaluated, then recalculates what reads
// the cells that changed, until nothing spills anew. It returns the
// sheets whose pivots to refresh: stale, and those whose source a spill
// changed.
func (w *Workbook) settleSpills(stale []*Sheet) []*Sheet {
	for pass := 0; len(w.spillWork) > 0; pass++ {
		var changed, anchors []loc
		for _, l := range w.spillOrder() {
			cells, moved := l.s.applySpill(l.a, w.spillWork[l])
			changed = append(changed, cells...)
			if moved {
				anchors = append(anchors, l)
			}
		}
		w.spillWork = nil
		if len(changed) == 0 && len(anchors) == 0 {
			break
		}
		if pass == maxSpillPasses {
			w.Circular = true
			break
		}
		w.affectedBy(changed, anchors, false)
		for _, s := range w.stalePivots() {
			if !slices.Contains(stale, s) {
				stale = append(stale, s)
			}
		}
		w.evaluate()
	}
	return stale
}

// spillOrder is the anchors of spillWork in the order they spill: by
// sheet, then row by row, so when two arrays need the same cells, the
// first one written in that order gets them.
func (w *Workbook) spillOrder() []loc {
	order := slices.Collect(maps.Keys(w.spillWork))
	slices.SortFunc(order, func(x, y loc) int {
		return cmp.Or(cmp.Compare(w.Index(x.s), w.Index(y.s)), cmp.Compare(x.a.Row, y.a.Row), cmp.Compare(x.a.Col, y.a.Col))
	})
	return order
}

// applySpill writes what the anchor at a computed, returning the cells
// that changed and whether the anchor's own value did (it shows #REF!
// when blocked).
func (s *Sheet) applySpill(a Addr, p pendingSpill) ([]loc, bool) {
	c := s.cells.get(a)
	if c == nil || !c.IsFormula() || p.arr == nil {
		return s.dropSpill(a), false
	}
	area, why := s.spillArea(a, p.arr)
	if why != "" {
		changed := s.dropSpill(a)
		s.setSpill(a, &spill{area: area, why: why})
		c.Value = ErrRef
		return changed, p.v != ErrRef
	}
	var changed []loc
	if old := s.spills[a]; old != nil && old.why == "" {
		changed = s.clearSpilled(old.area, area)
	}
	s.setSpill(a, &spill{area: area})
	for r := range area.To.Row - a.Row + 1 {
		for col := range area.To.Col - a.Col + 1 {
			if r == 0 && col == 0 {
				continue
			}
			at := Addr{Col: a.Col + col, Row: a.Row + r}
			if s.writeSpilled(at, p.arr.At(r, col), c.auto) {
				changed = append(changed, loc{s, at})
			}
		}
	}
	return changed, false
}

// spillArea is where the array computed at a goes, or why it can't: past
// the sheet's edge, past max-cells, or over a cell that isn't empty.
// Blank entries past the array's data aren't spilled, so a whole column
// read as an array spills what it holds.
func (s *Sheet) spillArea(a Addr, arr *functions.Array) (Rect, string) {
	rows, cols := arr.Rows, arr.Cols
	if arr.Fill.Kind == Empty {
		rows, cols = min(rows, max(arr.DRows, 1)), min(cols, max(arr.DCols, 1))
	}
	area := Rect{From: a, To: Addr{Col: a.Col + cols - 1, Row: a.Row + rows - 1}}
	switch {
	case area.To.Row >= MaxRows || area.To.Col >= MaxCols:
		area.To = Addr{Col: min(area.To.Col, MaxCols-1), Row: min(area.To.Row, MaxRows-1)}
		return area, "Array result was not expanded because it would go past the edge of the sheet"
	case rows*cols > MaxCells():
		return area, fmt.Sprintf("Array result was not expanded because it would write more cells than max-cells allows (%d)", MaxCells())
	}
	old := s.spills[a]
	for at, c := range s.cells.anyInRange(area) {
		if at == a || c.Blank() || c.spilled && old != nil && old.why == "" && old.area.Contains(at) {
			continue
		}
		return area, "Array result was not expanded because it would overwrite data in " + at.String()
	}
	return area, ""
}

// writeSpilled makes the cell at a show v as a spilled cell, keeping its
// formatting, and reports whether it changed.
func (s *Sheet) writeSpilled(a Addr, v Value, auto Format) bool {
	old := s.cells.get(a)
	var f Format
	var st Style
	if old != nil {
		f, st = old.Format, old.Style
	}
	if v.Kind == Empty {
		if old == nil || !old.spilled {
			return false
		}
		s.setDerived(a, formattingOnly(f, st))
		return true
	}
	c := &Cell{Input: derivedInput(v), Value: v, Format: f, Style: st, auto: auto, spilled: true}
	if old != nil && old.spilled && old.Value == c.Value && old.Input == c.Input && old.auto == c.auto {
		return false
	}
	s.setDerived(a, c)
	return true
}

// clearSpilled turns the spilled cells of area outside keep back into
// what they were before: blank, or formatting only.
func (s *Sheet) clearSpilled(area, keep Rect) []loc {
	var changed []loc
	var gone []Addr
	for at, c := range s.cells.anyInRange(area) {
		if c.spilled && !keep.Contains(at) {
			gone = append(gone, at)
		}
	}
	for _, at := range gone {
		c := s.cells.get(at)
		s.setDerived(at, formattingOnly(c.Format, c.Style))
		changed = append(changed, loc{s, at})
	}
	return changed
}

// dropSpill forgets the anchor at a's spill and clears its cells,
// returning those that changed.
func (s *Sheet) dropSpill(a Addr) []loc {
	old := s.spills[a]
	if old == nil {
		return nil
	}
	s.setSpill(a, nil)
	if old.why != "" {
		return nil
	}
	return s.clearSpilled(old.area, Rect{From: a, To: a})
}

// setSpill records the anchor at a's spill (nil for none), indexing
// its area so that changing a cell there recalculates the anchor.
func (s *Sheet) setSpill(a Addr, sp *spill) {
	if old := s.spills[a]; old != nil {
		s.spillAt.remove(a, []Rect{old.area})
		delete(s.spills, a)
	}
	if sp == nil {
		return
	}
	if s.spills == nil {
		s.spills = map[Addr]*spill{}
	}
	s.spills[a] = sp
	s.spillAt.add(a, []Rect{sp.area})
}

// spillTouched notes that the cell at a is being changed by hand (or by
// a paste, a sort or undo): the anchors whose spill covers or needs it
// recompute, and an anchor losing its formula drops its spill.
func (s *Sheet) spillTouched(a Addr, c *Cell) {
	if s.spills == nil {
		return
	}
	if s.spills[a] != nil && (c == nil || !c.IsFormula()) {
		for _, l := range s.dropSpill(a) {
			s.wb.markDirty(l)
		}
	}
	s.spillAt.readers(a, func(anchor Addr) {
		if anchor != a {
			s.wb.markDirty(loc{s, anchor})
		}
	})
}

// markDirty has the step being made or undone recalculate the cell at
// l, without recording it for undo.
func (w *Workbook) markDirty(l loc) {
	w.hist.dirty = append(w.hist.dirty, l)
}

// SpillAnchor returns the cell whose array result the cell at a shows,
// when a is a spilled cell (not the anchor itself).
func (s *Sheet) SpillAnchor(a Addr) (Addr, bool) {
	var found Addr
	ok := false
	s.spillAt.readers(a, func(anchor Addr) {
		if sp := s.spills[anchor]; anchor != a && sp.why == "" {
			found, ok = anchor, true
		}
	})
	return found, ok
}

// SpillArea returns the cells the formula at a spills into, the anchor
// first, when it spills.
func (s *Sheet) SpillArea(a Addr) (Rect, bool) {
	sp := s.spills[a]
	if sp == nil || sp.why != "" {
		return Rect{}, false
	}
	return sp.area, true
}

// InSpill returns a spilled cell in r whose anchor isn't in r, which an
// edit of r can't change: editing spilled cells is refused, while
// clearing or replacing the anchor with its spill is fine.
func (s *Sheet) InSpill(r Rect) (Addr, bool) {
	for anchor, sp := range s.spills {
		if sp.why != "" || r.Contains(anchor) || sp.area == (Rect{From: anchor, To: anchor}) {
			continue
		}
		if overlap, ok := intersectRect(sp.area, r); ok {
			return overlap.From, true // r doesn't hold the anchor: spilled cells
		}
	}
	return Addr{}, false
}

// intersectRect is the overlap of two ranges, if any.
func intersectRect(a, b Rect) (Rect, bool) {
	r := Rect{
		From: Addr{Col: max(a.From.Col, b.From.Col), Row: max(a.From.Row, b.From.Row)},
		To:   Addr{Col: min(a.To.Col, b.To.Col), Row: min(a.To.Row, b.To.Row)},
	}
	return r, r.From.Col <= r.To.Col && r.From.Row <= r.To.Row
}

// saved is what the file keeps of the cell, or nil: a pivot's results
// are computed, not saved, and a spilled cell keeps its formatting, not
// the array's value.
func (c *Cell) saved() *Cell {
	switch {
	case c.derived:
		return nil
	case c.spilled:
		return formattingOnly(c.Format, c.Style)
	}
	return c
}

// spillError says why the anchor at a shows #REF!, or "".
func (s *Sheet) spillError(a Addr) string {
	if sp := s.spills[a]; sp != nil {
		return sp.why
	}
	return ""
}
