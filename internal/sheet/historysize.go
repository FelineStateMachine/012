package sheet

import "slices"

// MaxUndoBytes caps the memory the undo history's before-images hold, as
// estimated by step.size. When a new step takes the history past it, the
// oldest steps are dropped first; the newest step is always kept, however
// large, so any single change can be undone. 256 MB keeps MaxUndo steps
// that each rewrite a whole column (about 250 MB, see docs/contributing/limits.md).
const MaxUndoBytes = 256 << 20

// undoBudget is MaxUndoBytes; tests lower it.
var undoBudget int64 = MaxUndoBytes

// Estimated heap held by the parts of a step. A before-image is a shallow
// copy of a Cell (216 B) plus its map entry and boxed literal; a formula
// also keeps its parsed tree and reference lists alive, which grow with
// its text. The constants match the heap measured by the stress
// benchmarks (BenchmarkHistoryFull) within about 10%.
const (
	stepBytes      = 512 // the step and its empty maps
	entryBytes     = 64  // a map entry: the key and a pointer
	cellBytes      = 240 // a copied Cell and its boxed literal
	formulaBytes   = 256 // a parsed formula's tree and references, plus perFormulaByte per byte of text
	perFormulaByte = 16
	chartBytes     = 128
	ruleBytes      = 256 // a rule with its ranges and values
)

// keep records c as the before-image of the cell at l and counts it.
func (st *step) keep(l loc, c *Cell) {
	st.cells[l] = c
	st.cellBytes += entryBytes + cellSize(c)
}

// size estimates the heap a finished step holds: its cells, counted as
// they were recorded, and the rest, which is small.
func (st *step) size() int64 {
	n := stepBytes + st.cellBytes
	n += int64(len(st.widths)+len(st.lines)+len(st.names)+len(st.views)) * entryBytes
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
			n += int64(s.Len()) * (entryBytes + cellBytes)
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
