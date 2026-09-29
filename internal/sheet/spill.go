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

// A spilled cell's note, like its formatting, belongs to the cell: one
// that had a note before an array spilled into it keeps it, but SetNote
// refuses spilled cells, as it refuses a pivot's results.

// spill is what an anchor spills, or would.
type spill struct {
	area Rect   // the cells it covers, the anchor first; when blocked, those it needs
	why  string // why the anchor shows #REF! instead, or ""
	// circular is set when it's blocked by a cycle through its cells
	// (spillblock.go).
	circular bool
	// arr and auto are the array written and its format, so the same
	// array computed again isn't written again, unless stale says a cell
	// of area changed since.
	arr   *functions.Array
	auto  Format
	stale bool
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
	v   Value            // what the anchor showed formulas read meanwhile
	top Value            // what the formula computed, the array's first value
}

// noteSpill records what the formula of c, at a on s, computed (top,
// and arr, nil for one value), when it spills or did. An array blocked
// before keeps showing #REF! until spilling finds it free, so formulas
// reading it don't see it come and go while it stays blocked.
//
//go:noinline
func (w *Workbook) noteSpill(s *Sheet, a Addr, c *Cell, arr *functions.Array) {
	l, top := loc{s, a}, c.Value
	sp := s.spills[a]
	if arr == nil && sp == nil {
		return
	}
	if arr != nil && sp != nil && sp.why != "" {
		c.Value = ErrRef
	}
	if w.spillWork == nil {
		w.spillWork = spillWork{}
	}
	w.spillWork[l] = pendingSpill{arr, c.Value, top}
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
	area, why, later := s.spillArea(a, p.arr)
	var cycle []loc
	circ := false
	if why == "" {
		// Checked each time the anchor is computed: a formula typed since
		// it spilled may lead from its cells back to it.
		why, cycle = s.spillsIntoItself(a, area)
		circ = why != ""
	}
	old := s.spills[a]
	if why == "" && len(later) == 0 && old != nil && old.why == "" && !old.stale && old.auto == c.auto && sameArray(old.arr, p.arr) {
		return nil, false // written already, and nothing has changed in its way
	}
	var changed []loc
	if circ != (old != nil && old.circular) {
		changed = s.wb.recheckArrays(loc{s, a}) // a cycle through arrays began or ended here
	}
	if why != "" {
		changed = append(changed, s.dropSpill(a)...)
		s.setSpill(a, &spill{area: area, why: why, circular: circ})
		c.Value = ErrRef
		return append(changed, blockCircular(cycle)...), p.v != ErrRef
	}
	for _, b := range later {
		// b's array gives way, and b is computed again to find itself
		// blocked by this one.
		changed = append(append(changed, s.dropSpill(b)...), loc{s, b})
	}
	changed = append(changed, s.regionsGiveWay(area)...)
	if old != nil && old.why == "" {
		changed = append(changed, s.clearSpilled(old.area, area)...)
	}
	s.setSpill(a, &spill{area: area, arr: p.arr, auto: c.auto})
	c.Value = p.top
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
	return changed, p.v != p.top
}

// spillArea is where the array computed at a goes, or why it can't: past
// the sheet's edge, past max-cells, or over a cell that isn't empty.
// Blank entries past the array's data aren't spilled, so a whole column
// read as an array spills what it holds.
//
// When two arrays need the same cells, the one whose anchor comes first,
// row by row, gets them, whichever spilled first: later lists the
// anchors whose arrays give way to this one. So what spills doesn't
// depend on the order formulas were typed or computed in, and a file
// opens as it was saved.
func (s *Sheet) spillArea(a Addr, arr *functions.Array) (area Rect, why string, later []Addr) {
	rows, cols := arr.Rows, arr.Cols
	if arr.Fill.Kind == Empty {
		rows, cols = min(rows, max(arr.DRows, 1)), min(cols, max(arr.DCols, 1))
	}
	area = Rect{From: a, To: Addr{Col: a.Col + cols - 1, Row: a.Row + rows - 1}}
	switch {
	case area.To.Row >= MaxRows || area.To.Col >= MaxCols:
		area.To = Addr{Col: min(area.To.Col, MaxCols-1), Row: min(area.To.Row, MaxRows-1)}
		return area, "Array result was not expanded because it would go past the edge of the sheet", nil
	case rows*cols > MaxCells():
		return area, fmt.Sprintf("Array result was not expanded because it would write more cells than max-cells allows (%d)", MaxCells()), nil
	}
	if ms := s.MergesIn(area); len(ms) > 0 {
		return area, "Array result was not expanded because it would overwrite merged cells in " + ms[0].String(), nil
	}
	if at, blocked := s.arraysInTheWay(a, area, &later); blocked {
		return area, "Array result was not expanded because it would overwrite data in " + at.String(), nil
	}
	old := s.spills[a]
	for at := range s.cells.anyKeysIn(area) {
		if at == a || !s.cells.filledAt(at) {
			continue
		}
		if s.cells.derivedAt(at) == slotSpill {
			if old != nil && old.why == "" && old.area.Contains(at) {
				continue
			}
			if _, ok := s.SpillAnchor(at); ok || s.regionCell(at) {
				continue // an array's, found above, or a region's, which gives way
			}
		}
		return area, "Array result was not expanded because it would overwrite data in " + at.String(), nil
	}
	return area, "", later
}

// arraysInTheWay finds the other arrays spilled over area, where the
// array at a would go, blank cells of theirs included, as Sheets keeps
// an array's whole range: it reports a cell of one whose anchor comes
// before a, which blocks it, and adds to later those whose anchors come
// after.
func (s *Sheet) arraysInTheWay(a Addr, area Rect, later *[]Addr) (Addr, bool) {
	if n := len(s.spills); n == 0 || n == 1 && s.spills[a] != nil {
		return Addr{}, false
	}
	var at Addr
	blocked := false
	for row := area.From.Row; row <= area.To.Row && !blocked; row++ {
		for col := area.From.Col; col <= area.To.Col && !blocked; col++ {
			cell := Addr{Col: col, Row: row}
			s.spillAt.readers(cell, func(b Addr) {
				switch sp := s.spills[b]; {
				case b == a || sp == nil || sp.why != "" || blocked:
				case b.Row < a.Row || b.Row == a.Row && b.Col < a.Col:
					at, blocked = cell, true
				case !slices.Contains(*later, b):
					*later = append(*later, b)
				}
			})
		}
	}
	return at, blocked
}

// writeSpilled makes the cell at a show v as a spilled cell, keeping its
// formatting and note, and reports whether it changed.
func (s *Sheet) writeSpilled(a Addr, v Value, auto Format) bool {
	was, l, kind := s.cells.derivedOf(a)
	spilled := kind == slotSpill
	if v.Kind == Empty {
		if !spilled {
			return false
		}
		s.setDerived(a, s.cells.get(a).leftover())
		return true
	}
	if spilled && was == v && l.auto == auto {
		return false // its input is its value's
	}
	if s.cells.setSpilled(a, v, auto) { // formatting kept, no formula to unlink
		s.version++
		return true
	}
	c := &Cell{Input: derivedInput(v), Value: v, Format: l.f, Style: l.st, auto: auto, spilled: true}
	if old := s.cells.richAt(a); old != nil {
		c.Note = old.Note
	}
	s.setDerived(a, c)
	return true
}

// clearSpilled turns the spilled cells of area outside keep back into
// what they were before: blank, or formatting only.
func (s *Sheet) clearSpilled(area, keep Rect) []loc {
	var changed []loc
	var gone []Addr
	for at := range s.cells.anyKeysIn(area) {
		if s.cells.derivedAt(at) == slotSpill && !keep.Contains(at) && !s.regionCell(at) {
			gone = append(gone, at)
		}
	}
	for _, at := range gone {
		s.setDerived(at, s.cells.get(at).leftover())
		changed = append(changed, loc{s, at})
	}
	return append(changed, s.wakeBlocked(area, keep)...)
}

// wakeBlocked returns the anchors of arrays blocked from spilling over
// the cells of area outside keep, which an array leaving them freed, to
// compute again, and marks the regions blocked there to be sent again.
// The cells it left may have been blank, which changing
// wouldn't find.
func (s *Sheet) wakeBlocked(area, keep Rect) []loc {
	var out []loc
	freed := func(r Rect) bool {
		x, ok := intersectRect(r, area)
		return ok && !(keep.Contains(x.From) && keep.Contains(x.To))
	}
	for b, sp := range s.spills {
		if sp.why != "" && freed(sp.area) {
			sp.stale = true
			out = append(out, loc{s, b})
		}
	}
	for _, me := range s.regionMeta {
		if me.why != "" && freed(me.need) {
			me.stale = true // sent again, as a region blocked by a cell cleared is
		}
	}
	return out
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
			s.spills[anchor].stale = true
			s.wb.markDirty(loc{s, anchor})
		}
	})
}

// sameArray reports whether two arrays hold the same values.
func sameArray(x, y *functions.Array) bool {
	if x == y {
		return true
	}
	if x == nil || y == nil || x.Rows != y.Rows || x.Cols != y.Cols || x.DRows != y.DRows || x.DCols != y.DCols || x.Fill != y.Fill {
		return false
	}
	for i, v := range x.V {
		if y.V[i] != v {
			return false
		}
	}
	return true
}

// recalcSwapped recalculates what an undo or redo changed, and the
// anchors of the spills it touched (markDirty).
func (w *Workbook) recalcSwapped(changed []loc) {
	changed = append(changed, w.hist.dirty...)
	w.hist.dirty = nil
	if w.structural {
		w.structural = false
		w.recalcAll()
		return
	}
	w.recalc(changed)
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

// HasSpills reports whether a formula on the sheet spills or would, or a
// region shows a table (whose cells are spilled cells too), so what
// draws the sheet can skip looking for spilled cells.
func (s *Sheet) HasSpills() bool { return len(s.spills) > 0 || len(s.regions.list) > 0 }

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
// are computed, not saved, and a spilled cell keeps its formatting and
// note, not the array's value.
func (c *Cell) saved() *Cell {
	switch {
	case c.derived:
		return nil
	case c.spilled:
		return c.leftover()
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
