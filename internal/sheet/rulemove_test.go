package sheet

import "testing"

// Rules move with cut and paste: the cut cells take them along, the
// cells they land on lose theirs, and relative formulas stay right for
// the cells that stayed and those that moved.
func TestRulesMoveWithCells(t *testing.T) {
	s := New()
	for r := range 10 {
		s.Set(Addr{Row: r}, "5")
	}
	mustAdd(t, s, CondFormat{Ranges: ranges("A1:A10"), Op: RuleFormula, Args: [2]string{"=A1>4"}, Style: green})
	mustAdd(t, s, CondFormat{Ranges: ranges("A1:A10"), Op: RuleGreater, Args: [2]string{"3"}, Style: green})
	if err := s.AddValidation(Validation{Ranges: ranges("A1:A10"), Kind: ValidNumber, Op: RuleBetween, Args: [2]string{"1", "9"}, Reject: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddValidation(Validation{Ranges: ranges("C1:C3"), Kind: ValidList, Items: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Move(rect("A1:A3"), at("C2")); err != nil {
		t.Fatal(err)
	}
	fs, vs := s.CondFormats(), s.Validations()
	if len(fs) != 3 {
		t.Fatalf("formats %+v", fs)
	}
	// The formula rule splits: A4:A10 reads its own cells, and the moved
	// cells read theirs, at their new places.
	if RangesText(fs[0].Ranges) != "A4:A10" || fs[0].Args[0] != "=A4>4" {
		t.Errorf("kept part: %s %s", RangesText(fs[0].Ranges), fs[0].Args[0])
	}
	if RangesText(fs[1].Ranges) != "C2:C4" || fs[1].Args[0] != "=C2>4" {
		t.Errorf("moved part: %s %s", RangesText(fs[1].Ranges), fs[1].Args[0])
	}
	if RangesText(fs[2].Ranges) != "A4:A10,C2:C4" {
		t.Errorf("a plain rule keeps one list: %s", RangesText(fs[2].Ranges))
	}
	// The list rule loses the cells landed on; the number rule reaches them.
	if len(vs) != 2 || RangesText(vs[0].Ranges) != "A4:A10,C2:C4" || RangesText(vs[1].Ranges) != "C1" {
		t.Errorf("validations %s, %s", RangesText(vs[0].Ranges), RangesText(vs[len(vs)-1].Ranges))
	}
	if !s.Look(at("C3")).Styled || s.Look(at("A2")).Styled {
		t.Error("looks don't follow the move")
	}
	s.Undo()
	if fs := s.CondFormats(); len(fs) != 2 || RangesText(fs[0].Ranges) != "A1:A10" || fs[0].Args[0] != "=A1>4" {
		t.Errorf("undo: %+v", fs)
	}
}

// A paste or fill is checked after the fact: InvalidIn lists what fails,
// and Discard takes the change back without leaving it to redo.
func TestInvalidInAndDiscard(t *testing.T) {
	s := New()
	if err := s.AddValidation(Validation{Ranges: ranges("B1:B9"), Kind: ValidNumber, Op: RuleBetween, Args: [2]string{"1", "10"}, Reject: true}); err != nil {
		t.Fatal(err)
	}
	s.Set(at("A1"), "5")
	s.Set(at("A2"), "50")
	s.Set(at("A3"), "x")
	if _, err := s.Paste(s.Copy(rect("A1:A3")), rect("B1"), false); err != nil {
		t.Fatal(err)
	}
	bad := s.InvalidIn(rect("B1:B3"))
	if len(bad) != 2 || bad[0].Addr != at("B2") || bad[1].Addr != at("B3") || !bad[0].Reject {
		t.Fatalf("invalid %+v", bad)
	}
	if !s.Discard() || s.Value(at("B2")).Kind != Empty || s.CanRedo() {
		t.Errorf("discard: B2 %v, redo %v", s.Value(at("B2")), s.CanRedo())
	}
	if len(s.InvalidIn(rect("B1:B3"))) != 0 {
		t.Error("still invalid after discarding")
	}
}
