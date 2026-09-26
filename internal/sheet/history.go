package sheet

// MaxUndo is how many steps of undo history a sheet keeps.
const MaxUndo = 100

// History is a command log of before-images. Each step records, for every
// cell and column width it touched, the state before the step; undoing
// swaps those back in and records the state it replaced as the redo step.
//
// Steps snapshot whole cells (a shallow copy of the struct), so fields
// added to Cell later are covered without changes here. That relies on
// one rule: a stored *Cell is never modified in place except for its
// computed Value. To change a cell, build a new one and store it with
// place.
type history struct {
	undo, redo []*step
	open       *step // the step being built; nil outside change
	depth      int   // nesting of change calls
	dirty      []Addr
	lastID     int
	// mergeWidths lets the next width-only step join the top one, so a
	// live preview of a column width and its final value (or its
	// cancellation) are one step. Seal ends the run.
	mergeWidths bool
}

type step struct {
	id     int
	focus  Rect           // what the UI selects when the step is undone or redone
	cells  map[Addr]*Cell // before the step; nil for blank
	widths map[int]int    // before the step; 0 for the default width
}

func (st *step) empty() bool { return len(st.cells) == 0 && len(st.widths) == 0 }

func (c *Cell) clone() *Cell {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

// Batch runs fn as a single undo step: everything it changes is undone
// together, and formulas are recalculated once at the end. focus is the
// range to select when the step is undone or redone. Batches nest; inner
// ones join the outermost.
func (s *Sheet) Batch(focus Rect, fn func() error) error {
	var err error
	s.change(focus, func() { err = fn() })
	return err
}

// change opens a step (unless one is open), runs fn, and on the way out
// of the outermost call recalculates what changed and pushes the step.
func (s *Sheet) change(focus Rect, fn func()) {
	h := &s.hist
	if h.depth == 0 {
		h.open = &step{focus: focus, cells: map[Addr]*Cell{}, widths: map[int]int{}}
	}
	h.depth++
	defer func() {
		h.depth--
		if h.depth > 0 {
			return
		}
		st, dirty := h.open, h.dirty
		h.open, h.dirty = nil, nil
		s.recalc(dirty)
		s.push(st)
	}()
	fn()
}

// record saves the cell at a before its first change in the open step.
func (s *Sheet) record(a Addr) {
	st := s.hist.open
	if st == nil {
		return
	}
	if _, seen := st.cells[a]; !seen {
		st.cells[a] = s.cells[a].clone()
	}
	s.hist.dirty = append(s.hist.dirty, a)
}

// recordWidth saves column c's width before its first change in the open
// step.
func (s *Sheet) recordWidth(c int) {
	st := s.hist.open
	if st == nil {
		return
	}
	if _, seen := st.widths[c]; !seen {
		st.widths[c] = s.widths[c]
	}
}

// push adds a finished step to the undo stack and clears redo, dropping
// no-op changes.
func (s *Sheet) push(st *step) {
	h := &s.hist
	for c, w := range st.widths {
		if s.widths[c] == w {
			delete(st.widths, c)
		}
	}
	for a, c := range st.cells {
		if c == nil && s.cells[a] == nil {
			delete(st.cells, a)
		}
	}
	if st.empty() {
		return
	}
	h.redo = nil
	h.lastID++
	widthOnly := len(st.cells) == 0
	if top := h.top(); widthOnly && h.mergeWidths && top != nil && len(top.cells) == 0 {
		for c, w := range st.widths {
			if _, ok := top.widths[c]; !ok {
				top.widths[c] = w
			}
		}
		for c, w := range top.widths {
			if s.widths[c] == w {
				delete(top.widths, c)
			}
		}
		top.id, top.focus = h.lastID, union(top.focus, st.focus)
		if top.empty() {
			h.undo = h.undo[:len(h.undo)-1]
		}
		return
	}
	st.id = h.lastID
	h.undo = append(h.undo, st)
	if len(h.undo) > MaxUndo {
		h.undo = h.undo[len(h.undo)-MaxUndo:]
	}
	h.mergeWidths = widthOnly
}

func (h *history) top() *step {
	if len(h.undo) == 0 {
		return nil
	}
	return h.undo[len(h.undo)-1]
}

// Seal ends a run of column width changes, so the next one starts a new
// undo step. The UI calls it between user actions.
func (s *Sheet) Seal() { s.hist.mergeWidths = false }

// Undo reverts the last step and returns the range it affected.
func (s *Sheet) Undo() (Rect, bool) {
	return s.swap(&s.hist.undo, &s.hist.redo)
}

// Redo reapplies the last undone step and returns the range it affected.
func (s *Sheet) Redo() (Rect, bool) {
	return s.swap(&s.hist.redo, &s.hist.undo)
}

// CanUndo and CanRedo report whether there is a step to undo or redo.
func (s *Sheet) CanUndo() bool { return len(s.hist.undo) > 0 }
func (s *Sheet) CanRedo() bool { return len(s.hist.redo) > 0 }

// StateID identifies the sheet's contents in its undo history: undoing
// back to a saved state returns the ID it had when saved, so the UI can
// tell whether there are unsaved changes.
func (s *Sheet) StateID() int {
	if top := s.hist.top(); top != nil {
		return top.id
	}
	return 0
}

// swap pops a step from one stack, restores its before-image, and pushes
// the state it replaced onto the other stack.
func (s *Sheet) swap(from, to *[]*step) (Rect, bool) {
	if len(*from) == 0 || s.hist.open != nil {
		return Rect{}, false
	}
	st := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	// The inverse keeps the step's ID: on the redo stack, an ID names the
	// state the step leads back to.
	inv := &step{id: st.id, focus: st.focus, cells: make(map[Addr]*Cell, len(st.cells)), widths: map[int]int{}}
	changed := make([]Addr, 0, len(st.cells))
	for a, c := range st.cells {
		inv.cells[a] = s.cells[a].clone()
		s.place(a, c.clone())
		changed = append(changed, a)
	}
	for c, w := range st.widths {
		inv.widths[c] = s.widths[c]
		s.setWidth(c, w)
	}
	s.recalc(changed)
	*to = append(*to, inv)
	s.hist.mergeWidths = false
	return st.focus, true
}

// ClearHistory forgets all undo and redo steps.
func (s *Sheet) ClearHistory() {
	s.hist = history{}
}

// colRect is the range covering whole columns from..to.
func colRect(from, to int) Rect {
	return Rect{Addr{Col: from}, Addr{Col: to, Row: MaxRows - 1}}
}

// rowRect is the range covering whole rows from..to.
func rowRect(from, to int) Rect {
	return Rect{Addr{Row: from}, Addr{Col: MaxCols - 1, Row: to}}
}

func union(a, b Rect) Rect {
	return Rect{
		From: Addr{Col: min(a.From.Col, b.From.Col), Row: min(a.From.Row, b.From.Row)},
		To:   Addr{Col: max(a.To.Col, b.To.Col), Row: max(a.To.Row, b.To.Row)},
	}
}
