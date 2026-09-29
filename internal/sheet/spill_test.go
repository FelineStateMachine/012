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

// A note on a blank cell an array spills into stays with the cell, as its
// formatting does, but spilled cells take no new notes.
func TestSpillNotes(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SEQUENCE(1)"})
	if err := s.SetNote(at("A2"), "check"); err != nil {
		t.Fatal(err)
	}
	s.Set(at("A1"), "=SEQUENCE(3)")
	if c := s.Cell(at("A2")); !c.Spilled() || c.Note != "check" || s.Value(at("A2")).String() != "2" {
		t.Errorf("A2 under the spill = %+v", c)
	}
	if err := s.SetNote(at("A3"), "x"); err != ErrSpillEdit {
		t.Errorf("note on a spilled cell: %v, want ErrSpillEdit", err)
	}
	var buf bytes.Buffer
	if err := s.Book().Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"A2": {"note":"check"}`) {
		t.Errorf("the note wasn't saved on its own:\n%s", buf.String())
	}
	s.Set(at("A1"), "")
	if c := s.Cell(at("A2")); c == nil || c.Spilled() || c.Note != "check" {
		t.Errorf("A2 after the spill went = %+v", c)
	}
}

// Rules over a spill see its values: a conditional format colors spilled
// cells and a validation rule marks spilled values it doesn't accept,
// but neither stops the array from spilling, as rules only ever judge
// what's typed.
func TestSpillRules(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SEQUENCE(4)"})
	if err := s.AddCondFormat(CondFormat{Ranges: []Rect{rect("A1:A9")}, Op: RuleGreater, Args: [2]string{"2"}, Style: RuleStyle{Bold: true}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddValidation(Validation{Ranges: []Rect{rect("A1:A9")}, Kind: ValidNumber, Op: RuleLessEq, Args: [2]string{"3"}, Reject: true}); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"A4": "4"})
	if l := s.Look(at("A3")); !l.Styled || l.Invalid {
		t.Errorf("A3's look = %+v, want styled and valid", l)
	}
	if l := s.Look(at("A4")); !l.Styled || !l.Invalid {
		t.Errorf("A4's look = %+v, want styled and invalid", l)
	}
}

// When two arrays need the same cells, the one anchored first, row by
// row, gets them, whichever was typed first, even where the other's
// cells are blank; reopening the file agrees.
func TestOverlappingArraysFirstAnchorWins(t *testing.T) {
	cases := []struct {
		first, second [2]string
		win, lose     string
	}{
		{[2]string{"C9", "=SORT(E4:G8)"}, [2]string{"D8", "=SEQUENCE(2)"}, "D8", "C9"},
		{[2]string{"D8", "=SEQUENCE(2)"}, [2]string{"C9", "=SORT(E4:G8)"}, "D8", "C9"},
		{[2]string{"E5", "=SEQUENCE(3,2)"}, [2]string{"F4", "=SEQUENCE(3)"}, "F4", "E5"},
		{[2]string{"F4", "=SEQUENCE(3)"}, [2]string{"E5", "=SEQUENCE(3,2)"}, "F4", "E5"},
	}
	for _, c := range cases {
		s := sheetOf(t, map[string]string{"E4": "5", "G8": "1"})
		s.Set(at(c.first[0]), c.first[1])
		s.Set(at(c.second[0]), c.second[1])
		for name, sh := range map[string]*Sheet{"typed": s, "reopened": roundTrip(t, s)} {
			if v := sh.Value(at(c.lose)); v != ErrRef {
				t.Errorf("%v then %v, %s: %s = %v", c.first, c.second, name, c.lose, v)
			}
			if _, ok := sh.SpillArea(at(c.win)); !ok {
				t.Errorf("%v then %v, %s: %s doesn't spill", c.first, c.second, name, c.win)
			}
		}
		// Taking the second back frees the first to spill.
		s.Undo()
		if _, ok := s.SpillArea(at(c.first[0])); !ok {
			t.Errorf("%v then %v, undone: %s doesn't spill", c.first, c.second, c.first[0])
		}
	}
}

// A formula reading an array blocked by that formula's own cell reads
// the anchor's #REF!, settling rather than going back and forth as the
// formula's own array comes and goes.
func TestReadingABlockedArray(t *testing.T) {
	s := New()
	s.Set(at("A18"), "=SORT(A16:C16)")
	s.Set(at("A16"), "=SEQUENCE(3)")
	for name, sh := range map[string]*Sheet{"typed": s, "reopened": roundTrip(t, s)} {
		for _, a := range []string{"A16", "A18"} {
			if v := sh.Value(at(a)); v != ErrRef {
				t.Errorf("%s: %s = %v", name, a, v)
			}
		}
		if sh.Book().Circular {
			t.Errorf("%s: arrays kept moving", name)
		}
	}
}

// An array whose formula reads its own cells through another formula
// is a circular dependency too: #REF! for it and what reads it, typed,
// recalculated or opened, without going back and forth. Here the
// formula reads the array's anchor too (VLOOKUP's range holds C19), a
// cycle of formulas, found whether or not VLOOKUP reads that far.
func TestSpillIntoItsInputThroughAFormula(t *testing.T) {
	s := New()
	s.Set(at("F11"), "=VLOOKUP(F14,C17:D19,1,FALSE)")
	s.Set(at("C19"), "=SORT(E9:F11)")
	s.Set(at("H15"), "=MAX(E:C)")
	check := func(name string, sh *Sheet) {
		t.Helper()
		for _, a := range []string{"C19", "H15"} {
			if v := sh.Value(at(a)); v != ErrRef {
				t.Errorf("%s: %s = %v", name, a, v)
			}
		}
		if got := sh.ExplainError(at("C19")); !strings.Contains(got, "Circular") {
			t.Errorf("%s: %q", name, got)
		}
		if sh.Filled(at("C20")) {
			t.Errorf("%s: the array spilled", name)
		}
	}
	check("typed", s)
	check("reopened", roundTrip(t, s))
	s.RecalcAll()
	check("recalculated", s)
}

// An array that would spill into the cells its formula reads is a
// circular dependency: #REF!, whether typed or opened, rather than a
// result that depends on what was computed first.
func TestSpillIntoItsInput(t *testing.T) {
	s := sheetOf(t, map[string]string{"C8": "3", "A9": "=SORT(B8:D9)"})
	for name, sh := range map[string]*Sheet{"typed": s, "reopened": roundTrip(t, s)} {
		if v := sh.Value(at("A9")); v != ErrRef {
			t.Errorf("%s: A9 = %v", name, v)
		}
		if sh.Filled(at("B9")) || sh.Filled(at("B10")) {
			t.Errorf("%s: spilled into its input", name)
		}
		if got := sh.ExplainError(at("A9")); !strings.Contains(got, "Circular dependency") {
			t.Errorf("%s: %q", name, got)
		}
	}
}

// An array isn't left blocked by a cycle through the cells of an array
// that stopped spilling in the same recalculation: deleting a row moves
// the array at A3 to A2, and puts at A3 a formula reading B2 whose
// array spilled over C3, which G1 reads, until it's computed again.
func TestNoCycleThroughAnArrayGone(t *testing.T) {
	s := sheetOf(t, map[string]string{"A3": "=D11:F11", "A4": "=B3", "G2": "=C4", "F11": "=G2"})
	if sp := s.spills[at("A3")]; sp == nil || sp.why != "" {
		t.Fatalf("A3's array: %+v", sp)
	}
	s.DeleteRows(0, 1)
	if sp := s.spills[at("A2")]; sp == nil || sp.why != "" {
		t.Fatalf("A2's array after deleting a row: %+v", sp)
	}
}
