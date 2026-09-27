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

// Cut on one sheet and pasted on another, the cells take their rules
// along: they leave the source's rules, the cells they land on lose
// theirs, and a moved formula reads the cells it read, naming the sheet
// it came from.
func TestRulesMoveToAnotherSheet(t *testing.T) {
	s := New()
	for r := range 4 {
		s.Set(Addr{Row: r}, "5")
	}
	mustAdd(t, s, CondFormat{Ranges: ranges("A1:A4"), Op: RuleGreater, Args: [2]string{"2"}, Style: green})
	mustAdd(t, s, CondFormat{Ranges: ranges("A1:A4"), Op: RuleFormula, Args: [2]string{"=$B1>0"}, Style: green})
	if err := s.AddValidation(Validation{Ranges: ranges("A2:A3"), Kind: ValidList, Items: []string{"5"}}); err != nil {
		t.Fatal(err)
	}
	other, err := s.Book().AddSheet("Other", 1)
	if err != nil {
		t.Fatal(err)
	}
	mustAdd(t, other, CondFormat{Ranges: ranges("C1:C2"), Op: RuleEmpty, Style: green})
	mustAdd(t, other, CondFormat{Ranges: ranges("D1"), Op: RuleEqual, Args: [2]string{"=C1"}, Style: green})
	if _, err := s.MoveTo(other, rect("A2:A3"), at("C1")); err != nil {
		t.Fatal(err)
	}
	fs := s.CondFormats()
	if len(fs) != 2 || RangesText(fs[0].Ranges) != "A1,A4" || RangesText(fs[1].Ranges) != "A1,A4" || fs[1].Args[0] != "=$B1>0" {
		t.Errorf("source formats %+v", fs)
	}
	if len(s.Validations()) != 0 {
		t.Errorf("source validations %+v", s.Validations())
	}
	ofs := other.CondFormats()
	if len(ofs) != 3 {
		t.Fatalf("destination formats %+v", ofs)
	}
	if RangesText(ofs[0].Ranges) != "D1" || RangesText(ofs[1].Ranges) != "C1:C2" || ofs[1].Op != RuleGreater {
		t.Errorf("destination %s %s", RangesText(ofs[0].Ranges), RangesText(ofs[1].Ranges))
	}
	if ofs[2].Args[0] != "=Sheet1!$B2>0" {
		t.Errorf("moved formula %q", ofs[2].Args[0])
	}
	if vs := other.Validations(); len(vs) != 1 || RangesText(vs[0].Ranges) != "C1:C2" {
		t.Errorf("destination validations %+v", vs)
	}
	if !other.Look(at("C1")).Styled {
		t.Error("the moved rule doesn't draw")
	}
	s.Book().Undo()
	if len(s.CondFormats()) != 2 || RangesText(s.CondFormats()[0].Ranges) != "A1:A4" || len(other.CondFormats()) != 2 || len(s.Validations()) != 1 {
		t.Errorf("undo: %+v %+v", s.CondFormats(), other.CondFormats())
	}
}
