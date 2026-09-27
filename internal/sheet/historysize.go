package sheet

import "slices"

// MaxUndoBytes caps the memory the undo history's before-images hold, as
// estimated by step.size. When a new step takes the history past it, the
// oldest steps are dropped first; the newest step is always kept, however
// large, so any single change can be undone (a step past MaxStepBytes
// asks first). 256 MB holds a step clearing a full max-cells sheet of
// numbers, about 195 MB, and MaxUndo steps rewriting a whole column of
// 8192 take 16 MB (see docs/contributing/limits.md).
const MaxUndoBytes = 256 << 20

// undoBudget is MaxUndoBytes; tests lower it.
var undoBudget int64 = MaxUndoBytes

// Estimated heap held by the parts of a step. A plain cell's
// before-image is a slot (slotBytes) and its text, once per image (see
// historyimage.go); a rich one is also a Cell (216 B) with its input,
// and a formula keeps its parsed tree and reference lists alive, which
// grow with its text. The constants match the heap measured by the
// stress benchmarks (BenchmarkHistoryFull, BenchmarkHistoryWide,
// BenchmarkClearMax) within about 10%.
const (
	stepBytes      = 512 // the step and its empty maps
	entryBytes     = 64  // a map entry: the key and a pointer
	imageBytes     = 256 // a sheet's image, empty
	cellBytes      = 256 // a Cell, its input's header and its boxed literal
	formulaBytes   = 256 // a parsed formula's tree and references, plus perFormulaByte per byte of text
	perFormulaByte = 16
	chartBytes     = 128
	ruleBytes      = 256 // a rule with its ranges and values
)

// keep records the cell at a of s as it stands, unless the step holds
// it already, and counts it.
func (st *step) keep(s *Sheet, a Addr) {
	img := st.cells[s]
	if img == nil {
		img = &image{}
		st.cells[s] = img
		st.cellBytes += imageBytes
	}
	if !img.seen(a) {
		st.cellBytes += img.keep(&s.cells, a)
	}
}

// size estimates the heap a finished step holds: its cells, counted as
// they were recorded, and the rest, which is small.
func (st *step) size() int64 {
	n := stepBytes + st.cellBytes
	n += int64(len(st.widths)+len(st.heights)+len(st.lines)+len(st.names)+len(st.views)) * entryBytes
	for _, charts := range st.charts {
		n += entryBytes + int64(len(charts))*chartBytes
	}
	for _, r := range st.rules {
		n += entryBytes + int64(len(r.formats)+len(r.validations))*ruleBytes
	}
	if st.sheets != nil {
		n += int64(len(st.sheets.order)) * 2 * entryBytes
	}
	return n
}

// cellSize estimates the heap one before-image holds; nil stands for a
// blank cell.
func cellSize(c *Cell) int64 {
	if c == nil {
		return 0
	}
	n := int64(cellBytes + len(c.Input))
	if c.IsFormula() {
		n += formulaBytes + perFormulaByte*int64(len(c.Input))
	}
	return n
}

// deletedSize estimates the cells held by sheets a step's sheet list
// has that the workbook no longer does: a deleted sheet keeps its cells
// so undo can bring it back.
func (w *Workbook) deletedSize(st *step) int64 {
	if st.sheets == nil {
		return 0
	}
	var n int64
	for _, s := range st.sheets.order {
		if !slices.Contains(w.sheets, s) {
			n += s.cells.size()
		}
	}
	return n
}

// pushUndo adds st to the undo stack with its size, then drops the
// oldest steps while the history holds more than MaxUndo steps or more
// than the byte budget.
func (w *Workbook) pushUndo(st *step) {
	h := &w.hist
	st.bytes = st.size() + w.deletedSize(st)
	h.undo = append(h.undo, st)
	h.bytes += st.bytes
	drop := 0
	for len(h.undo)-drop > MaxUndo || h.bytes > undoBudget && len(h.undo)-drop > 1 {
		h.bytes -= h.undo[drop].bytes
		drop++
	}
	if drop > 0 {
		clear(h.undo[:drop])
		h.undo = h.undo[drop:]
	}
}

// popUndo removes and returns the top of the undo stack.
func (h *history) popUndo() *step {
	st := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.bytes -= st.bytes
	return st
}

// HistoryBytes estimates the memory the undo steps hold.
func (w *Workbook) HistoryBytes() int64 { return w.hist.bytes }

// MaxStepBytes is the most undo history one change may hold without
// asking: the UI asks before a change its UndoCost puts over it, and runs
// it WithoutUndo if told to go on. Plain cells cost about 20 B each, so
// only millions of formulas or of distinct long texts come near it.
const MaxStepBytes = 1 << 30

// UndoCost estimates the undo history a change to every cell of r would
// hold: a slot a cell, the rich cells in r whole, and r's share of the
// sheet's strings. The UI asks it before every command that edits, so
// it costs the fewer of r's cells and the sheet's rich cells.
func (s *Sheet) UndoCost(r Rect) int64 {
	st := &s.cells
	cells := 0
	for _, c := range st.stored.colsIn(r.From.Col, r.To.Col) {
		cells += st.stored.countIn(c, r.From.Row, r.To.Row)
	}
	if cells == 0 {
		return 0
	}
	n := int64(cells)*slotBytes + st.strs.bytes*int64(cells)/int64(st.len())
	add := func(c *Cell) { n += richBytes + cellSize(c) }
	if cells < len(st.rich)-len(st.richFree) { // the cheaper way to find r's rich cells
		for a := range st.anyKeysIn(r) {
			if c := st.richAt(a); c != nil {
				add(c)
			}
		}
		return n
	}
	for _, rc := range st.rich {
		if rc.c != nil && r.Contains(rc.a) {
			add(rc.c)
		}
	}
	return n
}

// WithoutUndo runs fn, a change too large to undo (see MaxStepBytes),
// recording no before-images, and forgets the undo history, which could
// no longer be undone past it. Inside a Batch it runs fn as part of it.
func (w *Workbook) WithoutUndo(fn func()) {
	h := &w.hist
	if h.depth > 0 {
		fn()
		return
	}
	h.off = true
	defer func() {
		id := h.lastID + 1
		w.hist = history{lastID: id, base: id}
	}()
	fn()
}
