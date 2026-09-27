package sheet

import "slices"

// MaxUndo is how many steps of undo history a workbook keeps, at most;
// MaxUndoBytes also bounds it.
const MaxUndo = 100

// History is a command log of before-images, shared by every sheet of a
// workbook as in Sheets. Each step records, for every cell, column width,
// named range, view, chart list and the sheet list it touched, the state
// before the step; undoing swaps those back in and records the state it
// replaced as the redo step.
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
	dirty      []loc
	lastID     int
	bytes      int64 // estimated heap held by the undo steps; see historysize.go
	// mergeWidths lets the next width-only step join the top one, so a
	// live preview of a column width or row height and its final value
	// (or its cancellation) are one step. Seal ends the run.
	mergeWidths bool
}

type step struct {
	id    int
	bytes int64 // estimated heap held, set when pushed on the undo stack
	// cellBytes is the estimated heap held by cells, kept as they are
	// recorded so sizing a step doesn't walk them again.
	cellBytes int64
	label     string              // what the step did, e.g. "clear B3:B5"
	sheet     *Sheet              // the sheet the UI shows when the step is undone or redone
	focus     Rect                // what the UI selects there
	cells     map[loc]*Cell       // before the step; nil for blank
	widths    map[colKey]int      // before the step; 0 for the default width
	heights   map[rowKey]int      // rows' heights before the step, 0 for none; nil until one changes
	lines     map[lineKey]lineFmt // column and row formats before the step
	names     map[string]*Name    // before the step, by key; nil for undefined
	views     map[*Sheet]*viewState
	// charts holds each touched sheet's charts before the step.
	charts map[*Sheet][]Chart
	// pivots holds each touched sheet's pivot definition before the step.
	pivots map[*Sheet]*Pivot
	// rules holds each touched sheet's conditional formats and data
	// validation before the step.
	rules map[*Sheet]rulesState
	// sheets is the sheet list before the step, when it changed.
	sheets *sheetList
	// settings are the workbook's settings before the step, when they
	// changed.
	settings *settings
	// macros is the macro list before the step, when it changed.
	macros *[]Macro
}

// colKey is a column of a sheet.
type colKey struct {
	s   *Sheet
	col int
}

func newStep(label string, s *Sheet, focus Rect) *step {
	return &step{label: label, sheet: s, focus: focus, cells: map[loc]*Cell{}, widths: map[colKey]int{}, lines: map[lineKey]lineFmt{},
		names: map[string]*Name{}, views: map[*Sheet]*viewState{}, charts: map[*Sheet][]Chart{}, pivots: map[*Sheet]*Pivot{}, rules: map[*Sheet]rulesState{}}
}

func (st *step) empty() bool {
	return len(st.cells) == 0 && len(st.widths) == 0 && len(st.heights) == 0 && st.widthOnly()
}

// widthOnly reports whether the step changed nothing but column widths
// and row heights.
func (st *step) widthOnly() bool {
	return len(st.cells) == 0 && len(st.lines) == 0 && len(st.names) == 0 && len(st.views) == 0 && len(st.charts) == 0 && len(st.pivots) == 0 && len(st.rules) == 0 && st.sheets == nil && st.settings == nil &&
		st.macros == nil
}

func (c *Cell) clone() *Cell {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

// Change describes an undo step: what it did, in lower case for use in a
// sentence ("clear B3:B5"), the sheet it happened on and the range it
// affected there.
type Change struct {
	Label string
	Focus Rect
	Sheet *Sheet
	// Tabs is set when the step added, deleted, renamed or moved sheets;
	// Focus means nothing then.
	Tabs bool
	// Macros is set when the step changed nothing but the macros; Focus
	// means nothing then either.
	Macros bool
}

// Batch runs fn as a single undo step: everything it changes is undone
// together, and formulas are recalculated once at the end. Batches nest;
// inner ones join the outermost.
func (s *Sheet) Batch(c Change, fn func() error) error {
	var err error
	s.change(c.Label, c.Focus, func() { err = fn() })
	return err
}

// change runs fn as an undo step on this sheet; see Workbook.change.
func (s *Sheet) change(label string, focus Rect, fn func()) {
	s.wb.change(s, label, focus, fn)
}

// change opens a step (unless one is open), runs fn, and on the way out
// of the outermost call recalculates what changed and pushes the step.
func (w *Workbook) change(s *Sheet, label string, focus Rect, fn func()) {
	w.begin(s, label, focus)
	defer w.finish()
	fn()
}

// begin opens a step unless one is open, and nests into it.
func (w *Workbook) begin(s *Sheet, label string, focus Rect) {
	h := &w.hist
	if h.depth == 0 {
		h.open = newStep(label, s, focus)
		w.structural = false
	}
	h.depth++
}

// finish leaves a nesting level; the outermost recalculates what changed
// and pushes the step.
func (w *Workbook) finish() {
	h := &w.hist
	h.depth--
	if h.depth > 0 {
		return
	}
	st, dirty := h.open, h.dirty
	h.open, h.dirty = nil, nil
	if w.structural {
		w.structural = false
		w.recalcAll()
	} else {
		w.recalc(dirty)
	}
	w.push(st)
}

// record saves the cell at a before its first change in the open step.
func (s *Sheet) record(a Addr) {
	st := s.wb.hist.open
	if st == nil {
		return
	}
	l := loc{s, a}
	if _, seen := st.cells[l]; !seen {
		st.keep(l, s.cells.copyOf(a))
	}
	s.wb.hist.dirty = append(s.wb.hist.dirty, l)
}

// push adds a finished step to the undo stack and clears redo, dropping
// no-op changes.
func (w *Workbook) push(st *step) {
	w.dropUnchanged(st)
	if st.empty() {
		return
	}
	h := &w.hist
	h.redo = nil
	h.lastID++
	widthOnly := st.widthOnly()
	if top := h.top(); widthOnly && h.mergeWidths && top != nil && top.widthOnly() {
		h.joinWidths(top, st)
		return
	}
	st.id = h.lastID
	w.pushUndo(st)
	h.mergeWidths = widthOnly
}

// dropUnchanged removes from st what ended the step as it began.
func (w *Workbook) dropUnchanged(st *step) {
	st.dropUnchangedLines()
	st.dropUnchangedRules()
	for l, c := range st.cells {
		if c == nil && !l.s.cells.has(l.a) {
			delete(st.cells, l)
			st.cellBytes -= entryBytes
		}
	}
	for k, n := range st.names {
		if sameName(n, w.namePtr(k)) {
			delete(st.names, k)
		}
	}
	for s, charts := range st.charts {
		if slices.Equal(charts, s.charts) {
			delete(st.charts, s)
		}
	}
	for s, v := range st.views {
		if v.equal(s.view) {
			delete(st.views, s)
		}
	}
	for s, p := range st.pivots {
		if samePivot(p, s.pivot.def) {
			delete(st.pivots, s)
		}
	}
	if st.sheets != nil && st.sheets.equal(w.sheetList()) {
		st.sheets = nil
	}
	if st.settings != nil && *st.settings == w.settings {
		st.settings = nil
	}
	if st.macros != nil && slices.Equal(*st.macros, w.macros) {
		st.macros = nil
	}
}

// dropUnchangedLines removes the column widths, row heights and line
// formats that ended the step as they began.
func (st *step) dropUnchangedLines() {
	for k, width := range st.widths {
		if k.s.widths[k.col] == width {
			delete(st.widths, k)
		}
	}
	for k, h := range st.heights {
		if k.s.heights[k.row] == h {
			delete(st.heights, k)
		}
	}
	for k, l := range st.lines {
		if k.s.line(k.row, k.n) == l {
			delete(st.lines, k)
		}
	}
}

// dropUnchangedRules removes the sheets' rules that ended the step as
// they began.
func (st *step) dropUnchangedRules() {
	for s, r := range st.rules {
		if r.equal(s.rules) {
			delete(st.rules, s)
		}
	}
}

// sameName reports whether two named ranges (nil for undefined) are the
// same.
func sameName(a, b *Name) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// joinWidths folds the width-only step st into the width-only step on
// top, so a live preview of a column width or row height and its final
// value (or its cancellation) are one step.
func (h *history) joinWidths(top, st *step) {
	for k, width := range st.widths {
		if _, ok := top.widths[k]; !ok {
			top.widths[k] = width
		}
	}
	for k, h := range st.heights {
		if _, ok := top.heights[k]; !ok {
			top.keepHeight(k, h)
		}
	}
	top.dropUnchangedLines()
	top.id, top.focus = h.lastID, union(top.focus, st.focus)
	h.bytes -= top.bytes
	top.bytes = top.size()
	h.bytes += top.bytes
	if top.empty() {
		h.popUndo()
	}
}

func (h *history) top() *step {
	if len(h.undo) == 0 {
		return nil
	}
	return h.undo[len(h.undo)-1]
}

// Seal ends a run of column width changes, so the next one starts a new
// undo step. The UI calls it between user actions.
func (w *Workbook) Seal() { w.hist.mergeWidths = false }

// Undo reverts the last step and describes it.
func (w *Workbook) Undo() (Change, bool) { return w.swap(true) }

// Redo reapplies the last undone step and describes it.
func (w *Workbook) Redo() (Change, bool) { return w.swap(false) }

// UndoLabel describes the step Undo would revert, e.g. "clear B3:B5",
// or "" when there is none.
func (w *Workbook) UndoLabel() string {
	if top := w.hist.top(); top != nil {
		return top.label
	}
	return ""
}

// CanUndo and CanRedo report whether there is a step to undo or redo.
func (w *Workbook) CanUndo() bool { return len(w.hist.undo) > 0 }
func (w *Workbook) CanRedo() bool { return len(w.hist.redo) > 0 }

// StateID identifies the workbook's contents in its undo history: undoing
// back to a saved state returns the ID it had when saved, so the UI can
// tell whether there are unsaved changes.
func (w *Workbook) StateID() int {
	if top := w.hist.top(); top != nil {
		return top.id
	}
	return 0
}

// ClearHistory forgets all undo and redo steps.
func (w *Workbook) ClearHistory() { w.hist = history{} }

// The sheet's history methods are the workbook's, for callers that work
// with one sheet at a time.

func (s *Sheet) Seal()                { s.wb.Seal() }
func (s *Sheet) Undo() (Change, bool) { return s.wb.Undo() }
func (s *Sheet) Redo() (Change, bool) { return s.wb.Redo() }
func (s *Sheet) CanUndo() bool        { return s.wb.CanUndo() }
func (s *Sheet) CanRedo() bool        { return s.wb.CanRedo() }
func (s *Sheet) StateID() int         { return s.wb.StateID() }
func (s *Sheet) ClearHistory()        { s.wb.ClearHistory() }

// swap pops a step from the undo stack (or the redo stack), restores its
// before-image, and pushes the state it replaced onto the other stack.
func (w *Workbook) swap(undo bool) (Change, bool) {
	h := &w.hist
	from := &h.redo
	if undo {
		from = &h.undo
	}
	if len(*from) == 0 || h.open != nil {
		return Change{}, false
	}
	var st *step
	if undo {
		st = h.popUndo()
	} else {
		st = h.redo[len(h.redo)-1]
		h.redo = h.redo[:len(h.redo)-1]
	}
	// The inverse keeps the step's ID: on the redo stack, an ID names the
	// state the step leads back to.
	inv := newStep(st.label, st.sheet, st.focus)
	inv.id = st.id
	// The sheet list goes first, so cells land on attached sheets.
	if st.sheets != nil {
		inv.sheets = w.sheetList()
		w.setSheets(st.sheets)
	}
	changed := make([]loc, 0, len(st.cells))
	for l, c := range st.cells {
		inv.keep(l, l.s.cells.copyOf(l.a))
		l.s.place(l.a, c.clone())
		changed = append(changed, l)
	}
	for k, width := range st.widths {
		inv.widths[k] = k.s.widths[k.col]
		k.s.setWidth(k.col, width)
	}
	for k, h := range st.heights {
		inv.keepHeight(k, k.s.heights[k.row])
		k.s.setHeight(k.row, h)
	}
	for k, l := range st.lines {
		inv.lines[k] = k.s.line(k.row, k.n)
		k.s.setLine(k.row, k.n, l)
		changed = append(changed, k.s.lineChanged(k.row, k.n)...)
	}
	for k, n := range st.names {
		inv.names[k] = w.namePtr(k)
		changed = append(changed, w.putName(k, n)...)
	}
	for s, charts := range st.charts {
		inv.charts[s] = s.charts
		s.charts = slices.Clone(charts)
	}
	for s, p := range st.pivots {
		inv.pivots[s] = s.pivot.def
		s.pivot.def, s.pivot.stale = p, true
	}
	for s, r := range st.rules {
		inv.rules[s] = s.rules
		s.rules = r
		s.looks.reset()
	}
	if st.settings != nil {
		cur := w.settings
		inv.settings, w.settings = &cur, *st.settings
		// Every formula computes differently in the other arithmetic.
		w.structural = w.structural || cur.decimal != w.decimal
	}
	if st.macros != nil {
		cur := w.macros
		inv.macros, w.macros = &cur, slices.Clone(*st.macros)
	}
	for s, v := range st.views {
		cur := s.view
		inv.views[s] = &cur
		s.view = *v
		s.hidden.valid = false
		if !slices.Equal(cur.merges, v.merges) {
			s.version++
			s.respill(Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}})
		}
	}
	w.recalcSwapped(changed)
	if undo {
		h.redo = append(h.redo, inv)
	} else {
		w.pushUndo(inv)
	}
	h.mergeWidths = false
	macrosOnly := st.macros != nil && len(st.cells) == 0 && len(st.lines) == 0 && len(st.names) == 0 && len(st.charts) == 0 && len(st.pivots) == 0 && len(st.rules) == 0 && st.sheets == nil
	return Change{Label: st.label, Focus: st.focus, Sheet: st.sheet, Tabs: st.sheets != nil, Macros: macrosOnly}, true
}

// colRect is the range covering whole columns from..to.
func colRect(from, to int) Rect {
	return Rect{From: Addr{Col: from}, To: Addr{Col: to, Row: MaxRows - 1}}
}

// rowRect is the range covering whole rows from..to.
func rowRect(from, to int) Rect {
	return Rect{From: Addr{Row: from}, To: Addr{Col: MaxCols - 1, Row: to}}
}

func union(a, b Rect) Rect {
	return Rect{
		From: Addr{Col: min(a.From.Col, b.From.Col), Row: min(a.From.Row, b.From.Row)},
		To:   Addr{Col: max(a.To.Col, b.To.Col), Row: max(a.To.Row, b.To.Row)},
	}
}
