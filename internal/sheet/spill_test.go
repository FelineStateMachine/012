package sheet

import (
	"bytes"
	"strings"
	"testing"
)

func wantShown(t *testing.T, s *Sheet, want map[string]string) {
	t.Helper()
	for a, w := range want {
		if got := s.Value(at(a)).String(); got != w {
			t.Errorf("%s = %q, want %q", a, got, w)
		}
	}
}

func TestSpillSequence(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "3", "B2": "=SEQUENCE(A1, 2)"})
	wantShown(t, s, map[string]string{"B2": "1", "C2": "2", "B3": "3", "C3": "4", "B4": "5", "C4": "6", "B5": ""})
	if a, ok := s.SpillAnchor(at("C3")); !ok || a != at("B2") {
		t.Errorf("C3's anchor = %v %v, want B2", a, ok)
	}
	if _, ok := s.SpillAnchor(at("B2")); ok {
		t.Error("the anchor is its own cell, not spilled")
	}
	if r, ok := s.SpillArea(at("B2")); !ok || r != rect("B2:C4") {
		t.Errorf("area %v %v, want B2:C4", r, ok)
	}
	// Growing and shrinking rewrite the spilled cells.
	s.Set(at("A1"), "4")
	wantShown(t, s, map[string]string{"B5": "7", "C5": "8"})
	s.Set(at("A1"), "1")
	wantShown(t, s, map[string]string{"B2": "1", "C2": "2", "B3": "", "C5": ""})
	if s.Cell(at("B3")) != nil {
		t.Error("a cell the spill left is still stored")
	}
	// Undo brings the bigger array back.
	s.Book().Undo()
	wantShown(t, s, map[string]string{"B5": "7", "C5": "8"})
}

func TestSpillBlocked(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SEQUENCE(3)", "A3": "note"})
	wantShown(t, s, map[string]string{"A1": "#REF!", "A2": "", "A3": "note"})
	if got := s.ExplainError(at("A1")); got != "Array result was not expanded because it would overwrite data in A3" {
		t.Errorf("A1 explained as %q", got)
	}
	// Clearing what's in the way lets it spill.
	if err := s.Set(at("A3"), ""); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"A1": "1", "A2": "2", "A3": "3"})
	// Spilled cells can't be typed into.
	if err := s.Set(at("A2"), "x"); err != ErrSpillEdit {
		t.Errorf("Set in a spill: %v, want ErrSpillEdit", err)
	}
	if a, ok := s.InSpill(rect("A2:B9")); !ok || a != at("A2") {
		t.Errorf("InSpill(A2:B9) = %v %v", a, ok)
	}
	if _, ok := s.InSpill(rect("A1:A3")); ok {
		t.Error("a range holding the anchor can be edited as a whole")
	}
	// The sheet's edge blocks too.
	s.Set(at("A1048575"), "=SEQUENCE(5)")
	if got := s.ExplainError(at("A1048575")); !strings.Contains(got, "edge of the sheet") {
		t.Errorf("at the edge: %q", got)
	}
}

func TestSpillDependents(t *testing.T) {
	w := NewBook()
	s := w.Sheet(0)
	other, err := w.AddSheet("Other", 1)
	if err != nil {
		t.Fatal(err)
	}
	for a, in := range map[string]string{"A1": "5", "A2": "3", "A3": "8", "C1": "=SORT(A1:A3)", "E1": "=C3*10", "E2": "=SUM(C1:C9)"} {
		if err := s.Set(at(a), in); err != nil {
			t.Fatal(err)
		}
	}
	if err := other.Set(at("A1"), "=Sheet1!C2+1"); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"C1": "3", "C2": "5", "C3": "8", "E1": "80", "E2": "16"})
	wantShown(t, other, map[string]string{"A1": "6"})
	s.Set(at("A2"), "9")
	wantShown(t, s, map[string]string{"C1": "5", "C2": "8", "C3": "9", "E1": "90", "E2": "22"})
	wantShown(t, other, map[string]string{"A1": "9"})
	// A spill reading a spill.
	s.Set(at("G1"), "=FILTER(C1:C3, C1:C3>5)")
	wantShown(t, s, map[string]string{"G1": "8", "G2": "9", "G3": ""})
	s.Set(at("A1"), "1")
	wantShown(t, s, map[string]string{"C1": "1", "G1": "8", "G2": "9"})
	// Deleting the anchor clears its spill, and what read it follows.
	s.Set(at("C1"), "")
	wantShown(t, s, map[string]string{"C2": "", "C3": "", "E1": "0", "E2": "0", "G1": "#N/A"})
	wantShown(t, other, map[string]string{"A1": "1"})
	w.Undo()
	wantShown(t, s, map[string]string{"C3": "9", "E1": "90", "G1": "8"})
}

func TestSpillNotSaved(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SEQUENCE(2)", "B1": "x"})
	var buf bytes.Buffer
	if err := s.Book().Write(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"A2"`) {
		t.Errorf("spilled cell saved:\n%s", buf.String())
	}
	w, err := ReadBook(&buf)
	if err != nil {
		t.Fatal(err)
	}
	wantShown(t, w.Sheet(0), map[string]string{"A1": "1", "A2": "2"})
}

func TestSpillStructure(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SEQUENCE(3)", "C1": "=SUM(A1:A3)", "A5": "end"})
	if err := s.InsertRows(1, 1); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"A1": "1", "A2": "2", "A3": "3", "A4": "", "A6": "end", "C1": "6"})
	s.DeleteRows(0, 1) // the anchor goes
	wantShown(t, s, map[string]string{"A1": "", "A2": "", "A5": "end"})
	if len(s.spills) != 0 {
		t.Errorf("spills left: %v", s.spills)
	}
	s.Book().Undo()
	s.Book().Undo()
	wantShown(t, s, map[string]string{"A1": "1", "A3": "3", "A5": "end", "C1": "6"})
}

func TestSpillFormattingKept(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SEQUENCE(3)"})
	s.SetStyle(rect("A2:A2"), func(st *Style) { st.Bold = true })
	wantShown(t, s, map[string]string{"A2": "2"})
	if c := s.Cell(at("A2")); !c.Spilled() || !c.Style.Bold {
		t.Errorf("A2 = %+v, want spilled and bold", c)
	}
	s.Set(at("A1"), "=SEQUENCE(1)")
	if c := s.Cell(at("A2")); c == nil || c.Spilled() || !c.Style.Bold || !c.Blank() {
		t.Errorf("A2 after the spill shrank = %+v, want blank and bold", c)
	}
}

func TestSpillCopiesValues(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SEQUENCE(3)"})
	clip := s.Copy(rect("A1:A3"))
	if _, err := s.Paste(clip, rect("C1:C1"), false); err != nil {
		t.Fatal(err)
	}
	if got := s.Cell(at("C2")).Input; got != "2" {
		t.Errorf("pasted spilled cell = %q, want its value", got)
	}
}
