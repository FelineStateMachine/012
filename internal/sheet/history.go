package sheet

import (
	"maps"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// MaxUndo is how many steps of undo history a workbook keeps.
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
	// mergeWidths lets the next width-only step join the top one, so a
	// live preview of a column width and its final value (or its
	// cancellation) are one step. Seal ends the run.
	mergeWidths bool
}

type step struct {
	id     int
	label  string           // what the step did, e.g. "clear B3:B5"
	sheet  *Sheet           // the sheet the UI shows when the step is undone or redone
	focus  Rect             // what the UI selects there
	cells  map[loc]*Cell    // before the step; nil for blank
	widths map[colKey]int   // before the step; 0 for the default width
	names  map[string]*Name // before the step, by key; nil for undefined
	views  map[*Sheet]*viewState
	// charts holds each touched sheet's charts before the step.
	charts map[*Sheet][]Chart
	// sheets is the sheet list before the step, when it changed.
	sheets *sheetList
	// decimal is the arithmetic setting before the step, when it changed.
	decimal *bool
	// macros is the macro list before the step, when it changed.
	macros *[]Macro
}

// colKey is a column of a sheet.
type colKey struct {
	s   *Sheet
	col int
}

// sheetList is the order and names of the sheets.
type sheetList struct {
	order []*Sheet
	names map[*Sheet]string
}

func newStep(label string, s *Sheet, focus Rect) *step {
	return &step{label: label, sheet: s, focus: focus, cells: map[loc]*Cell{}, widths: map[colKey]int{},
		names: map[string]*Name{}, views: map[*Sheet]*viewState{}, charts: map[*Sheet][]Chart{}}
}

func (st *step) empty() bool {
	return len(st.cells) == 0 && len(st.widths) == 0 && st.widthOnly()
}

// widthOnly reports whether the step changed nothing but column widths.
func (st *step) widthOnly() bool {
	return len(st.cells) == 0 && len(st.names) == 0 && len(st.views) == 0 && len(st.charts) == 0 && st.sheets == nil &&
		st.decimal == nil && st.macros == nil
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
		st.cells[l] = s.cells.get(a).clone()
	}
	s.wb.hist.dirty = append(s.wb.hist.dirty, l)
}

// recordWidth saves column c's width before its first change in the open
// step.
func (s *Sheet) recordWidth(c int) {
	st := s.wb.hist.open
	if st == nil {
		return
	}
	k := colKey{s, c}
	if _, seen := st.widths[k]; !seen {
		st.widths[k] = s.widths[c]
	}
}

// recordName saves the named range with key k before its first change in
// the open step.
func (w *Workbook) recordName(k string) {
	st := w.hist.open
	if st == nil {
		return
	}
	if _, seen := st.names[k]; !seen {
		st.names[k] = w.namePtr(k)
	}
}

// recordCharts saves the sheet's charts before their first change in the
// open step. Charts are few, so the step keeps them all.
func (s *Sheet) recordCharts() {
	if st := s.wb.hist.open; st != nil {
		if _, seen := st.charts[s]; !seen {
			st.charts[s] = slices.Clone(s.charts)
		}
	}
}

// recordSheets saves the sheet list before its first change in the open
// step, and has the step recalculate everything when it ends.
func (w *Workbook) recordSheets() {
	w.structural = true
	if st := w.hist.open; st != nil && st.sheets == nil {
		st.sheets = w.sheetList()
	}
}

// recordDecimal saves the arithmetic setting before its first change in
// the open step.
func (w *Workbook) recordDecimal() {
	if st := w.hist.open; st != nil && st.decimal == nil {
		d := w.decimal
		st.decimal = &d
	}
}

func (w *Workbook) sheetList() *sheetList {
	l := &sheetList{order: slices.Clone(w.sheets), names: map[*Sheet]string{}}
	for _, s := range w.sheets {
		l.names[s] = s.name
	}
	return l
}

// setSheets restores a sheet list: sheets not in it are detached, the
// rest renamed and attached in its order.
func (w *Workbook) setSheets(l *sheetList) {
	for _, s := range w.sheets {
		if !slices.Contains(l.order, s) {
			w.detach(s)
		}
	}
	w.byKey = map[string]*Sheet{}
	w.sheets = slices.Clone(l.order)
	for _, s := range w.sheets {
		s.name = l.names[s]
		if s.live {
			w.byKey[formula.SheetKey(s.name)] = s
		} else {
			w.attach(s)
		}
	}
	w.structural = true
}

func (l *sheetList) equal(m *sheetList) bool {
	return slices.Equal(l.order, m.order) && maps.Equal(l.names, m.names)
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
	h.undo = append(h.undo, st)
	if len(h.undo) > MaxUndo {
		h.undo = h.undo[len(h.undo)-MaxUndo:]
	}
	h.mergeWidths = widthOnly
}

// dropUnchanged removes from st what ended the step as it began.
func (w *Workbook) dropUnchanged(st *step) {
	for k, width := range st.widths {
		if k.s.widths[k.col] == width {
			delete(st.widths, k)
		}
	}
	for l, c := range st.cells {
		if c == nil && l.s.cells.get(l.a) == nil {
			delete(st.cells, l)
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
	if st.sheets != nil && st.sheets.equal(w.sheetList()) {
		st.sheets = nil
	}
	if st.decimal != nil && *st.decimal == w.decimal {
		st.decimal = nil
	}
	if st.macros != nil && slices.Equal(*st.macros, w.macros) {
		st.macros = nil
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
// top, so a live preview of a column width and its final value (or its
// cancellation) are one step.
func (h *history) joinWidths(top, st *step) {
	for k, width := range st.widths {
		if _, ok := top.widths[k]; !ok {
			top.widths[k] = width
		}
	}
	for k, width := range top.widths {
		if k.s.widths[k.col] == width {
			delete(top.widths, k)
		}
	}
	top.id, top.focus = h.lastID, union(top.focus, st.focus)
	if top.empty() {
		h.undo = h.undo[:len(h.undo)-1]
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
func (w *Workbook) Undo() (Change, bool) { return w.swap(&w.hist.undo, &w.hist.redo) }

// Redo reapplies the last undone step and describes it.
func (w *Workbook) Redo() (Change, bool) { return w.swap(&w.hist.redo, &w.hist.undo) }

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

// swap pops a step from one stack, restores its before-image, and pushes
// the state it replaced onto the other stack.
func (w *Workbook) swap(from, to *[]*step) (Change, bool) {
	if len(*from) == 0 || w.hist.open != nil {
		return Change{}, false
	}
	st := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
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
		inv.cells[l] = l.s.cells.get(l.a).clone()
		l.s.place(l.a, c.clone())
		changed = append(changed, l)
	}
	for k, width := range st.widths {
		inv.widths[k] = k.s.widths[k.col]
		k.s.setWidth(k.col, width)
	}
	for k, n := range st.names {
		inv.names[k] = w.namePtr(k)
		changed = append(changed, w.putName(k, n)...)
	}
	for s, charts := range st.charts {
		inv.charts[s] = s.charts
		s.charts = slices.Clone(charts)
	}
	if st.decimal != nil {
		cur := w.decimal
		inv.decimal, w.decimal = &cur, *st.decimal
		w.structural = true // every formula computes differently
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
	}
	if w.structural {
		w.structural = false
		w.recalcAll()
	} else {
		w.recalc(changed)
	}
	*to = append(*to, inv)
	w.hist.mergeWidths = false
	macrosOnly := st.macros != nil && len(st.cells) == 0 && len(st.names) == 0 && len(st.charts) == 0 && st.sheets == nil
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
