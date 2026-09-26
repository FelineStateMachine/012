package sheet

// This file is a stand-in for the undo history on main
// (internal/sheet/history.go there), with the same change, record, clone,
// Batch and ClearHistory but no undo. Every mutation in this branch goes
// through change and place as it does on main, so formatting changes
// become undo steps once merged. When merging, take main's version of
// this file.

type history struct {
	depth int    // nesting of change calls
	dirty []Addr // cells changed in the open step
}

// Change describes a step: what it did, in lower case for use in a
// sentence ("format B3:B5 as currency"), and the range it affected.
type Change struct {
	Label string
	Focus Rect
}

// Batch runs fn as a single step, recalculating once at the end. Batches
// nest; inner ones join the outermost.
func (s *Sheet) Batch(c Change, fn func() error) error {
	var err error
	s.change(c.Label, c.Focus, func() { err = fn() })
	return err
}

// change runs fn and, on the way out of the outermost call, recalculates
// what changed.
func (s *Sheet) change(_ string, _ Rect, fn func()) {
	h := &s.hist
	h.depth++
	defer func() {
		h.depth--
		if h.depth > 0 {
			return
		}
		dirty := h.dirty
		h.dirty = nil
		s.recalc(dirty)
	}()
	fn()
}

// record notes that the cell at a is about to change.
func (s *Sheet) record(a Addr) {
	if s.hist.depth > 0 {
		s.hist.dirty = append(s.hist.dirty, a)
	}
}

func (c *Cell) clone() *Cell {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

// ClearHistory forgets all steps.
func (s *Sheet) ClearHistory() { s.hist = history{} }
