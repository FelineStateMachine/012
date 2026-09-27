package sheet

import (
	"reflect"
	"strings"
	"testing"
)

// sheetOf builds a sheet from cell inputs, without undo history.
func sheetOf(t *testing.T, cells map[string]string) *Sheet {
	t.Helper()
	s := New()
	for a, in := range cells {
		if err := s.put(at(a), in); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	s.RecalcAll()
	return s
}

// inputs returns every cell's input by address.
func inputs(s *Sheet) map[string]string {
	out := map[string]string{}
	for a, c := range s.cells {
		out[a.String()] = c.Input
	}
	return out
}

func TestUndoRedo(t *testing.T) {
	s := New()
	s.Set(at("A1"), "1")
	s.Set(at("A2"), "=A1*10")
	s.Set(at("A1"), "2")
	s.EraseRange(NewRect(at("A1"), at("A2")))
	if s.Len() != 0 || s.StateID() != 4 {
		t.Fatalf("len %d state %d", s.Len(), s.StateID())
	}
	c, ok := s.Undo()
	if !ok || c != (Change{"clear A1:A2", NewRect(at("A1"), at("A2")), s, false}) || s.Value(at("A2")).Num != 20 {
		t.Fatalf("undo erase: %v %v A2=%+v", c, ok, s.Value(at("A2")))
	}
	s.Undo()
	if s.Value(at("A2")).Num != 10 || s.StateID() != 2 {
		t.Errorf("undo set: A2=%+v state %d", s.Value(at("A2")), s.StateID())
	}
	s.Redo()
	if s.Value(at("A2")).Num != 20 || s.StateID() != 3 {
		t.Errorf("redo: A2=%+v state %d", s.Value(at("A2")), s.StateID())
	}
	// A new change drops the redo stack.
	s.Set(at("B1"), "x")
	if s.CanRedo() {
		t.Error("redo survived a new change")
	}
	for s.CanUndo() {
		s.Undo()
	}
	if s.Len() != 0 || s.StateID() != 0 {
		t.Errorf("undo all: %v", inputs(s))
	}
	// A parse error changes nothing and adds no step.
	s.Set(at("A1"), "=1+")
	if s.CanUndo() {
		t.Error("failed Set added a step")
	}
}

func TestUndoKeepsWholeCells(t *testing.T) {
	s := New()
	s.Set(at("A1"), "=1+1")
	orig := s.Cell(at("A1"))
	s.Set(at("A1"), "x")
	s.Undo()
	got := s.Cell(at("A1"))
	if got == orig || !reflect.DeepEqual(*got, *orig) {
		t.Errorf("restored %+v, want a copy of %+v", got, orig)
	}
}

func TestBatchGroupsAndHistoryCap(t *testing.T) {
	s := New()
	s.Batch(Change{Label: "type", Focus: NewRect(at("A1"), at("A3"))}, func() error {
		for i := range 3 {
			s.Set(Addr{Row: i}, "1")
		}
		s.SetColWidth(0, 20)
		return nil
	})
	s.Undo()
	if s.Len() != 0 || s.ColWidth(0) != DefaultWidth || s.CanUndo() {
		t.Errorf("batch undo left %v width %d", inputs(s), s.ColWidth(0))
	}
	for i := range MaxUndo + 10 {
		s.Set(at("A1"), strings.Repeat("x", i+1))
	}
	n := 0
	for s.CanUndo() {
		s.Undo()
		n++
	}
	if n != MaxUndo || inputs(s)["A1"] != strings.Repeat("x", 10) {
		t.Errorf("undid %d steps, A1 %q", n, inputs(s)["A1"])
	}
}

func TestWidthPreviewMerges(t *testing.T) {
	s := New()
	s.Set(at("A1"), "1")
	// A live preview over two columns, then the final value: one step.
	for w := 11; w <= 13; w++ {
		s.SetColWidth(0, w)
		s.SetColWidth(1, w)
	}
	s.Undo()
	if s.ColWidth(0) != DefaultWidth || s.ColWidth(1) != DefaultWidth || s.Len() != 1 {
		t.Errorf("widths %d %d len %d", s.ColWidth(0), s.ColWidth(1), s.Len())
	}
	// A preview that is cancelled leaves no step at all.
	s.Redo()
	id := s.StateID()
	s.Seal()
	s.SetColWidth(0, 20)
	s.SetColWidth(0, 13)
	if s.StateID() != id {
		t.Errorf("cancelled preview left a step")
	}
	// Separate actions are separate steps.
	s.Seal()
	s.SetColWidth(2, 5)
	s.Seal()
	s.SetColWidth(3, 5)
	s.Undo()
	if s.ColWidth(2) != 5 || s.ColWidth(3) != DefaultWidth {
		t.Errorf("sealed width changes merged")
	}
}

func TestLoadIsNotUndoable(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "1"})
	s.SetColWidth(0, 12)
	s.RecalcAll()
	if s.CanUndo() || s.StateID() != 0 {
		t.Error("RecalcAll kept history")
	}
}
