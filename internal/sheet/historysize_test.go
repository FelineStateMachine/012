package sheet

import (
	"strconv"
	"testing"
)

// fillColumn writes v into rows 0..n-1 of column A as one step.
func fillColumn(t *testing.T, s *Sheet, n int, v string) {
	t.Helper()
	col := NewRect(Addr{}, Addr{Row: n - 1})
	if err := s.FillEntry(col, col.From, v); err != nil {
		t.Fatal(err)
	}
}

// The history drops its oldest steps once it holds more than the byte
// budget, keeps a running total that matches its steps, and always keeps
// the newest step.
func TestUndoByteBudget(t *testing.T) {
	s := New()
	fillColumn(t, s, 1000, "x")
	w := s.Book()
	w.ClearHistory()
	fillColumn(t, s, 1000, "0") // before-images of 1000 cells, slots and one string
	one := w.HistoryBytes()
	if one < 1000*slotBytes || one > 1000*slotBytes+8<<10 {
		t.Fatalf("a 1000-cell step is estimated at %d bytes", one)
	}
	defer func(b int64) { undoBudget = b }(undoBudget)
	undoBudget = 5 * one

	for i := 1; i < 20; i++ {
		fillColumn(t, s, 1000, strconv.Itoa(i))
	}
	if w.HistoryBytes() > undoBudget {
		t.Errorf("history holds %d bytes, over the budget of %d", w.HistoryBytes(), undoBudget)
	}
	if got := historySum(w); got != w.HistoryBytes() {
		t.Errorf("running total %d, steps add up to %d", w.HistoryBytes(), got)
	}
	n := 0
	for s.CanUndo() {
		s.Undo()
		n++
	}
	if n < 3 || n > 5 {
		t.Errorf("kept %d steps under a budget of about 5", n)
	}
	// The oldest steps went first: undoing everything kept lands on the
	// value written n steps before the last.
	if want := strconv.Itoa(19 - n); inputs(s)["A1"] != want {
		t.Errorf("after undoing all: A1 %q, want %q", inputs(s)["A1"], want)
	}
	if w.HistoryBytes() != 0 {
		t.Errorf("empty undo stack holds %d bytes", w.HistoryBytes())
	}
	// Redoing brings the steps back onto the undo stack, counted again.
	for s.CanRedo() {
		s.Redo()
	}
	if got := historySum(w); got != w.HistoryBytes() || got == 0 || got > undoBudget {
		t.Errorf("after redo: running total %d, steps %d", w.HistoryBytes(), got)
	}

	// A single step bigger than the budget is still kept.
	undoBudget = one / 2
	fillColumn(t, s, 1000, "big")
	if !s.CanUndo() {
		t.Fatal("the newest step was dropped")
	}
	s.Undo()
	if s.CanUndo() {
		t.Error("older steps survived a budget smaller than one step")
	}
	if inputs(s)["A1"] != "19" {
		t.Errorf("undo big: A1 %q", inputs(s)["A1"])
	}
}

// Formulas are estimated larger than numbers: their parsed trees stay
// alive in the before-image.
func TestUndoBytesFormulas(t *testing.T) {
	nums, forms := New(), New()
	fillColumn(t, nums, 100, "1")
	fillColumn(t, forms, 100, "=SUM(B1:B10)*2")
	nums.Book().ClearHistory()
	forms.Book().ClearHistory()
	fillColumn(t, nums, 100, "")
	fillColumn(t, forms, 100, "")
	if n, f := nums.Book().HistoryBytes(), forms.Book().HistoryBytes(); f < n+100*formulaBytes {
		t.Errorf("clearing 100 numbers %d bytes, 100 formulas %d", n, f)
	}
}

// A deleted sheet keeps its cells for undo, and the step that deleted it
// counts them.
func TestUndoBytesCountDeletedSheet(t *testing.T) {
	s := New()
	w := s.Book()
	data, err := w.AddSheet("Data", 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 500 {
		data.Set(Addr{Row: i}, "x")
	}
	w.ClearHistory()
	if err := w.DeleteSheet(data); err != nil {
		t.Fatal(err)
	}
	if got := w.HistoryBytes(); got < 500*slotBytes {
		t.Errorf("deleting a 500-cell sheet is estimated at %d bytes", got)
	}
}

// historySum adds up the undo steps' sizes, checking each step's
// running count of its cells against a walk over them.
func historySum(w *Workbook) int64 {
	var n int64
	for _, st := range w.hist.undo {
		var cells int64
		for _, img := range st.cells {
			cells += img.walkSize()
		}
		if cells != st.cellBytes {
			return -1
		}
		n += st.bytes
	}
	return n
}

// walkSize is what keep counts for the image, found by walking it.
func (img *image) walkSize() int64 {
	t := img.tables()
	n := int64(imageBytes + t.stored.n*slotBytes)
	for _, ic := range img.small {
		if !ic.blank {
			n += slotBytes
		}
	}
	n += int64(t.stored.blocks()+img.blank.blocks()) * blockBytes
	for _, rc := range t.rich {
		n += richBytes + cellSize(rc.c)
	}
	for _, s := range t.strs.strs[min(1, len(t.strs.strs)):] {
		n += strBytes + int64(len(s))
	}
	return n
}
