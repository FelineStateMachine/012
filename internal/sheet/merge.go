package sheet

import (
	"errors"
	"fmt"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Merged cells, as Sheets' Format > Merge cells: a merge joins a range
// into one cell that shows its top-left cell's contents and formatting.
// The merges are part of the sheet's view state (view.go): a list
// replaced whole on every change, so an undo step keeps it as it was,
// that follows inserted and deleted lines as range references do and is
// saved in the file. Finding the merge holding a cell goes through an
// index built when the list changes.

// MergeKind is how Format > Merge cells joins a range.
type MergeKind uint8

const (
	MergeAll          MergeKind = iota // the whole range, one cell
	MergeHorizontally                  // each row of it
	MergeVertically                    // each column of it
)

// maxMerges caps the merges one change makes: merging a range's rows or
// columns makes one per line.
const maxMerges = 10000

var (
	// ErrMergeOne is why a single cell can't be merged.
	ErrMergeOne = errors.New("Select more than one cell to merge")
	// ErrMergeMany is why merging a whole column's rows is refused.
	ErrMergeMany = fmt.Errorf("That would make more than %d merged cells", maxMerges)
	// ErrSortMerged is why a range holding merged cells isn't sorted.
	ErrSortMerged = errors.New("Can't sort a range with merged cells; unmerge them first")
)

// mergeIndex finds merges by the cells they cover, for the list it was
// built from (compared by identity: the list is replaced, never changed
// in place).
type mergeIndex struct {
	of  []Rect
	idx rangeIndex
	at  map[Addr]Rect // each merge by its top-left cell
}

// sameList reports whether a and b are the same slice.
func sameList(a, b []Rect) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func (s *Sheet) merges() *mergeIndex {
	x := &s.mergeIdx
	if !sameList(x.of, s.view.merges) || x.at == nil {
		*x = mergeIndex{of: s.view.merges, at: make(map[Addr]Rect, len(s.view.merges))}
		for _, r := range s.view.merges {
			x.at[r.From] = r
			x.idx.add(r.From, []Rect{r})
		}
	}
	return x
}

// Merges returns the merged ranges, in the order they were made.
func (s *Sheet) Merges() []Rect { return slices.Clone(s.view.merges) }

// MergeAt returns the merged range holding a, and false when a isn't
// merged.
func (s *Sheet) MergeAt(a Addr) (Rect, bool) {
	if len(s.view.merges) == 0 {
		return Rect{}, false
	}
	x := s.merges()
	var out Rect
	found := false
	x.idx.readers(a, func(tl Addr) { out, found = x.at[tl], true })
	return out, found
}

// MergesIn returns the merges that overlap r.
func (s *Sheet) MergesIn(r Rect) []Rect {
	var out []Rect
	for _, m := range s.view.merges {
		if _, ok := intersectRect(m, r); ok {
			out = append(out, m)
		}
	}
	return out
}

// Grow returns r grown to hold every merge it overlaps, as a selection
// in Sheets takes in the merged cells it touches.
func (s *Sheet) Grow(r Rect) Rect {
	if len(s.view.merges) == 0 {
		return r
	}
	for grown := true; grown; {
		grown = false
		for _, m := range s.MergesIn(r) {
			if u := union(r, m); u != r {
				r, grown = u, true
			}
		}
	}
	return r
}

// mergeAreas are the ranges merging r with k makes.
func mergeAreas(r Rect, k MergeKind) ([]Rect, error) {
	var out []Rect
	switch k {
	case MergeHorizontally:
		if r.To.Row-r.From.Row >= maxMerges {
			return nil, ErrMergeMany
		}
		for row := r.From.Row; row <= r.To.Row && r.From.Col < r.To.Col; row++ {
			out = append(out, Rect{From: Addr{Col: r.From.Col, Row: row}, To: Addr{Col: r.To.Col, Row: row}})
		}
	case MergeVertically:
		if r.To.Col-r.From.Col >= maxMerges {
			return nil, ErrMergeMany
		}
		for c := r.From.Col; c <= r.To.Col && r.From.Row < r.To.Row; c++ {
			out = append(out, Rect{From: Addr{Col: c, Row: r.From.Row}, To: Addr{Col: c, Row: r.To.Row}})
		}
	default:
		if r.From != r.To {
			out = []Rect{r}
		}
	}
	if len(out) == 0 {
		return nil, ErrMergeOne
	}
	return out, nil
}

// MergeLoses returns the first cell merging r with k would clear: one
// with contents that isn't the top-left of its merge. Sheets warns that
// only the top-left value is kept.
func (s *Sheet) MergeLoses(r Rect, k MergeKind) (Addr, bool) {
	areas, err := mergeAreas(r, k)
	if err != nil {
		return Addr{}, false
	}
	var lost []Addr
	for a, c := range s.cells.anyInRange(r) {
		if !c.Blank() && !c.Spilled() && !isTopLeft(areas, a) {
			lost = append(lost, a)
		}
	}
	if len(lost) == 0 {
		return Addr{}, false
	}
	sortAddrs(lost)
	return lost[0], true
}

// isTopLeft reports whether a is the top-left cell of one of areas, which
// all lie along r's rows or columns, or a isn't in any.
func isTopLeft(areas []Rect, a Addr) bool {
	for _, m := range areas {
		if m.Contains(a) {
			return a == m.From
		}
	}
	return true
}

// Merge joins r into merged cells as k says, as one undo step: each
// keeps its top-left cell's contents and clears the rest, keeping their
// formatting and notes. Merges it overlaps are replaced.
func (s *Sheet) Merge(r Rect, k MergeKind) error {
	areas, err := mergeAreas(r, k)
	if err != nil {
		return err
	}
	s.change("merge "+r.String(), r, func() {
		for a, c := range s.cells.anyInRange(r) {
			if !c.Blank() && !isTopLeft(areas, a) {
				s.place(a, c.leftover())
			}
		}
		next, span := s.mergesOutside(r)
		s.setMerges(append(next, areas...), span)
	})
	return nil
}

// MergeAcross returns a merge the line after the first n rows (or
// columns) would cut through, as freezing them would, and whether there
// is one.
func (s *Sheet) MergeAcross(rows bool, n int) (Rect, bool) {
	for _, m := range s.view.merges {
		from, to := m.From.Col, m.To.Col
		if rows {
			from, to = m.From.Row, m.To.Row
		}
		if n > 0 && from < n && to >= n {
			return m, true
		}
	}
	return Rect{}, false
}

// Unmerge splits the merges overlapping r back into cells, as one undo
// step, and returns how many there were.
func (s *Sheet) Unmerge(r Rect) int {
	next, span := s.mergesOutside(r)
	n := len(s.view.merges) - len(next)
	if n > 0 {
		s.change("unmerge "+r.String(), r, func() { s.setMerges(next, span) })
	}
	return n
}

// mergesOutside returns the merges that don't overlap r, and the range
// holding r and those that do: where arrays blocked by them may spill
// once they go.
func (s *Sheet) mergesOutside(r Rect) ([]Rect, Rect) {
	span := r
	next := slices.DeleteFunc(slices.Clone(s.view.merges), func(m Rect) bool {
		_, overlaps := intersectRect(m, r)
		if overlaps {
			span = union(span, m)
		}
		return overlaps
	})
	return next, span
}

// setMerges replaces the merges, recording them for undo, and has the
// arrays spilling over the range that changed spill again: a merge
// blocks a spill.
func (s *Sheet) setMerges(next []Rect, changed Rect) {
	if len(next) == 0 {
		next = nil
	}
	s.recordView()
	s.view.merges = next
	s.version++
	s.respill(changed)
}

// respill marks the anchors spilling, or blocked, over r to recalculate.
func (s *Sheet) respill(r Rect) {
	for a, sp := range s.spills {
		if _, ok := intersectRect(sp.area, r); ok {
			sp.stale = true
			s.wb.markDirty(loc{s, a})
		}
	}
}

// LoadMerge adds a merge as a loader does, without recording undo; one
// cell or one overlapping another is left out.
func (s *Sheet) LoadMerge(r Rect) {
	if r.From == r.To || !r.To.Valid() || len(s.MergesIn(r)) > 0 {
		return
	}
	s.view.merges = append(slices.Clone(s.view.merges), r)
	s.version++
}

// shiftMerges moves merges with inserted or deleted rows or columns, as
// a range reference moves; one left a single cell goes.
func shiftMerges(ms []Rect, rows bool, sp formula.Span) []Rect {
	if len(ms) == 0 {
		return ms
	}
	_, rng := formula.AxisMaps(rows, sp)
	var out []Rect
	for _, m := range ms {
		if r, ok := rng(m); ok && r.From != r.To {
			out = append(out, r)
		}
	}
	return out
}

// mergesWithin is the merges wholly inside r, moved by its top-left corner,
// for a copy to carry.
func (s *Sheet) mergesWithin(r Rect) []Rect {
	var out []Rect
	for _, m := range s.view.merges {
		if r.Contains(m.From) && r.Contains(m.To) {
			out = append(out, Rect{From: subAddr(m.From, r.From), To: subAddr(m.To, r.From)})
		}
	}
	return out
}

// pasteMerges merges what a copy carried at each tile of p, if anything,
// replacing the merges the paste lands over.
func (s *Sheet) pasteMerges(carried []Rect, p pasteLayout) {
	if len(carried) == 0 {
		return
	}
	// The merges it replaces may reach past it: arrays they blocked
	// there spill again too.
	next, span := s.mergesOutside(p.dst)
	for tr := range p.down {
		for tc := range p.across {
			at := Addr{Col: p.dst.From.Col + tc*p.tw, Row: p.dst.From.Row + tr*p.th}
			for _, m := range carried {
				if r := (Rect{From: addAddr(m.From, at), To: addAddr(m.To, at)}); r.To.Valid() && len(next) < maxMerges {
					next = append(next, r)
				}
			}
		}
	}
	s.setMerges(next, span)
}

// moveMerges moves the merges wholly inside src on from to dst on s, as
// cutting and pasting them does, replacing those dst overlaps.
func (s *Sheet) moveMerges(from *Sheet, src, dst Rect) {
	carried := from.mergesWithin(src)
	if len(carried) > 0 {
		left := slices.DeleteFunc(slices.Clone(from.view.merges), func(m Rect) bool {
			return src.Contains(m.From) && src.Contains(m.To)
		})
		from.setMerges(left, src)
	}
	p := pasteLayout{dst: dst, across: 1, down: 1,
		tw: dst.To.Col - dst.From.Col + 1, th: dst.To.Row - dst.From.Row + 1}
	s.pasteMerges(carried, p)
}

func subAddr(a, b Addr) Addr { return Addr{Col: a.Col - b.Col, Row: a.Row - b.Row} }
func addAddr(a, b Addr) Addr { return Addr{Col: a.Col + b.Col, Row: a.Row + b.Row} }
