package sheet

import (
	"bytes"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func ranges(s string) []Rect {
	rs, ok := ParseRanges(s)
	if !ok {
		panic(s)
	}
	return rs
}

func mustAdd(t *testing.T, s *Sheet, f CondFormat) {
	t.Helper()
	if err := s.AddCondFormat(f); err != nil {
		t.Fatal(err)
	}
}

var green = RuleStyle{Fill: ColorGreen}

func TestCondFormatTests(t *testing.T) {
	fixedNow(t)
	s := sheetOf(t, map[string]string{
		"A1": "150", "A2": "50", "A3": "", "A4": "apple pie", "A5": "Pineapple", "A6": "2026-09-25",
		"A7": "2026-09-27", "A8": "12", "B1": "=A1*2",
	})
	s.SetFormat(rng("A3"), Format{Kind: FmtNumber, Decimals: 2}) // formatted but blank
	cases := []struct {
		op    RuleOp
		args  [2]string
		match string // cells of A1:A8 that match
	}{
		{RuleGreater, [2]string{"100"}, "A1 A6 A7"},
		{RuleLessEq, [2]string{"50"}, "A2 A8"},
		{RuleBetween, [2]string{"60", "10"}, "A2 A8"},
		{RuleNotBetween, [2]string{"10", "60"}, "A1 A6 A7"},
		{RuleEmpty, [2]string{}, "A3"},
		{RuleNotEmpty, [2]string{}, "A1 A2 A4 A5 A6 A7 A8"},
		{RuleContains, [2]string{"APPLE"}, "A4 A5"},
		{RuleStartsWith, [2]string{"apple"}, "A4"},
		{RuleEndsWith, [2]string{"pie"}, "A4"},
		{RuleExactly, [2]string{"pineapple"}, "A5"},
		{RuleDateBefore, [2]string{"today"}, "A1 A2 A6 A8"},
		{RuleDateIs, [2]string{"tomorrow"}, "A7"},
		{RuleDateAfter, [2]string{"2026-09-26"}, "A7"},
		{RuleEqual, [2]string{"=A$8"}, "A8"},
		{RuleFormula, [2]string{"=$A1>=150"}, "A1 A4 A5 A6 A7"}, // text sorts after numbers,
		{RuleFormula, [2]string{"=B1=300"}, "A1"},
	}
	for _, c := range cases {
		s.rules = rulesState{}
		mustAdd(t, s, CondFormat{Ranges: ranges("A1:A8"), Op: c.op, Args: c.args, Style: green})
		var got []string
		for row := range 8 {
			if s.Look(Addr{Row: row}).Styled {
				got = append(got, Addr{Row: row}.String())
			}
		}
		if strings.Join(got, " ") != c.match {
			t.Errorf("%s %v: matched %v, want %s", c.op, c.args, got, c.match)
		}
	}
}

func TestCondFormatOrderAndScale(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "0", "A2": "5", "A3": "10", "A4": "x"})
	red := RuleStyle{Text: ColorRed, Bold: true}
	mustAdd(t, s, CondFormat{Ranges: ranges("A1:A4"), Op: RuleGreater, Args: [2]string{"7"}, Style: red})
	mustAdd(t, s, CondFormat{Ranges: ranges("A1:A4"), Scale: []ScalePoint{
		{Kind: PointMin, Color: ColorRed}, {Kind: PointPercentile, Value: "50", Color: ColorYellow}, {Kind: PointMax, Color: ColorGreen}}})
	if l := s.Look(at("A3")); !l.Styled || l.Style != red || l.Scaled {
		t.Errorf("the first rule wins: %+v", l)
	}
	l := s.Look(at("A2"))
	if !l.Scaled || l.From != ColorRed || l.To != ColorYellow || l.Pos != 1 {
		t.Errorf("midpoint: %+v", l)
	}
	if l := s.Look(at("A1")); !l.Scaled || l.From != ColorRed || l.Pos != 0 {
		t.Errorf("min: %+v", l)
	}
	if l := s.Look(at("A4")); l.Scaled || l.Styled {
		t.Errorf("text on a scale: %+v", l)
	}
	s.MoveCondFormat(1, 0)
	if l := s.Look(at("A3")); !l.Scaled || l.Pos != 1 || l.To != ColorGreen {
		t.Errorf("after moving the scale first: %+v", l)
	}
	// The looks follow edits: the scale reads the new values.
	s.Set(at("A3"), "20")
	if l := s.Look(at("A2")); l.Pos != 1 || l.To != ColorYellow || math.Abs(s.Look(at("A3")).Pos-1) > 1e-9 {
		t.Errorf("after an edit: %+v", l)
	}
	if l := s.Look(at("A1")); l.From != ColorRed || l.To != ColorYellow {
		t.Errorf("min segment: %+v", l)
	}
}

func TestCondFormatCheck(t *testing.T) {
	bad := []CondFormat{
		{Op: RuleGreater, Args: [2]string{"1"}, Style: green},
		{Ranges: ranges("A1"), Op: RuleGreater, Style: green},
		{Ranges: ranges("A1"), Op: RuleGreater, Args: [2]string{"1"}},
		{Ranges: ranges("A1"), Op: RuleFormula, Args: [2]string{"A1>1"}, Style: green},
		{Ranges: ranges("A1"), Op: RuleFormula, Args: [2]string{"=A1>"}, Style: green},
		{Ranges: ranges("A1"), Op: RuleDateIs, Args: [2]string{"soon"}, Style: green},
		{Ranges: ranges("A1"), Scale: []ScalePoint{{Kind: PointMin, Color: ColorRed}}},
		{Ranges: ranges("A1"), Scale: []ScalePoint{{Kind: PointMin, Color: ColorRed}, {Kind: PointPercent, Value: "150", Color: ColorRed}}},
	}
	for _, f := range bad {
		if f.Check() == nil {
			t.Errorf("accepted %+v", f)
		}
	}
	s := New()
	if err := s.AddCondFormat(bad[2]); err == nil || s.CanUndo() {
		t.Error("a bad rule was added")
	}
}

func TestRulesUndoAndFile(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "1"})
	mustAdd(t, s, CondFormat{Ranges: ranges("A1:A9,C1:C9"), Op: RuleFormula, Args: [2]string{"=$B1>100"}, Style: RuleStyle{Text: ColorBlue, Italic: true}})
	mustAdd(t, s, CondFormat{Ranges: ranges("B1:B9"), Scale: []ScalePoint{{Kind: PointNumber, Value: "0", Color: ColorRed}, {Kind: PointMax, Color: ColorGreen}}})
	if err := s.AddValidation(Validation{Ranges: ranges("D1:D9"), Kind: ValidList, Items: []string{"Yes", "No"}, Reject: true, Help: "Pick one"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddValidation(Validation{Ranges: ranges("E1:E9"), Kind: ValidNumber, Op: RuleBetween, Args: [2]string{"1", "10"}}); err != nil {
		t.Fatal(err)
	}
	if s.Book().UndoLabel() != "add data validation E1:E9" {
		t.Errorf("label %q", s.Book().UndoLabel())
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"conditionalFormats": [`,
		`{"ranges":"A1:A9,C1:C9","condition":"formula","values":["=$B1\u003e100"],"text":"blue","italic":true}`,
		`{"ranges":"B1:B9","scale":[{"type":"num","value":"0","color":"red"},{"type":"max","color":"green"}]}`,
		`{"ranges":"D1:D9","criteria":"list","items":["Yes","No"],"reject":true,"help":"Pick one"}`,
		`{"ranges":"E1:E9","criteria":"number","condition":"between","values":["1","10"]}`,
		`"version": 2,`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("file lacks %s:\n%s", want, buf.String())
		}
	}
	back, err := Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !back.rules.equal(s.rules) {
		t.Errorf("round trip:\n%+v\n%+v", back.rules, s.rules)
	}
	for range 4 {
		s.Undo()
	}
	if !s.rules.empty() {
		t.Errorf("undo left %+v", s.rules)
	}
	s.Redo()
	if len(s.CondFormats()) != 1 {
		t.Errorf("redo: %+v", s.rules)
	}
}

func TestRulesFollowInsertsAndRenames(t *testing.T) {
	w := NewBook()
	s := w.Sheet(0)
	lists, _ := w.AddSheet("Lists", 1)
	mustAdd(t, s, CondFormat{Ranges: ranges("B2:B9"), Op: RuleFormula, Args: [2]string{"=$C2>Lists!$A$1"}, Style: green})
	if err := s.AddValidation(Validation{Ranges: ranges("D2:D9"), Kind: ValidRange, Source: "Lists!A1:A5"}); err != nil {
		t.Fatal(err)
	}
	s.InsertRows(0, 2)
	s.InsertCols(0, 1)
	lists.InsertRows(0, 1)
	f, v := s.CondFormats()[0], s.Validations()[0]
	if RangesText(f.Ranges) != "C4:C11" || f.Args[0] != "=$D4>Lists!$A$2" {
		t.Errorf("format after inserts: %s %s", RangesText(f.Ranges), f.Args[0])
	}
	if RangesText(v.Ranges) != "E4:E11" || v.Source != "Lists!A2:A6" {
		t.Errorf("validation after inserts: %s %s", RangesText(v.Ranges), v.Source)
	}
	if err := w.RenameSheet(lists, "Choices"); err != nil {
		t.Fatal(err)
	}
	if f := s.CondFormats()[0]; f.Args[0] != "=$D4>Choices!$A$2" || s.Validations()[0].Source != "Choices!A2:A6" {
		t.Errorf("after rename: %s %s", f.Args[0], s.Validations()[0].Source)
	}
	s.DeleteCols(2, 1)
	if len(s.CondFormats()) != 0 {
		t.Error("a rule whose cells were deleted stays")
	}
	w.Undo()
	if len(s.CondFormats()) != 1 {
		t.Error("undo didn't bring the rule back")
	}
}

func TestValidation(t *testing.T) {
	fixedNow(t)
	s := sheetOf(t, map[string]string{"A1": "Red", "A2": "Blue", "A3": "red", "A4": "", "A5": "Green", "B1": "Blue"})
	add := func(v Validation) {
		t.Helper()
		if err := s.AddValidation(v); err != nil {
			t.Fatal(err)
		}
	}
	add(Validation{Ranges: ranges("B1:B9"), Kind: ValidRange, Source: "$A$1:$A$9", Reject: true})
	add(Validation{Ranges: ranges("C1:C9"), Kind: ValidList, Items: []string{"Yes", "No"}})
	add(Validation{Ranges: ranges("D1:D9"), Kind: ValidNumber, Op: RuleBetween, Args: [2]string{"1", "10"}})
	add(Validation{Ranges: ranges("E1:E9"), Kind: ValidDate, Op: RuleGreater, Args: [2]string{"2026-01-01"}})
	add(Validation{Ranges: ranges("F1:F9"), Kind: ValidLength, Op: RuleLessEq, Args: [2]string{"3"}})
	add(Validation{Ranges: ranges("G1:G9"), Kind: ValidFormula, Args: [2]string{"=G1<>A1"}})
	add(Validation{Ranges: ranges("H1:H9"), Kind: ValidCheckbox})
	if got := strings.Join(s.DropdownItems(at("B2")), ","); got != "Red,Blue,Green" {
		t.Errorf("items %q", got)
	}
	cases := []struct {
		cell, input string
		ok          bool
	}{
		{"B1", "green", true}, {"B1", "Purple", false}, {"C1", "yes", true}, {"C1", "maybe", false},
		{"D1", "10", true}, {"D1", "11", false}, {"D1", "=2*3", true}, {"D1", "ten", false},
		{"E1", "2026-03-01", true}, {"E1", "2025-12-31", false}, {"E1", "45000", false},
		{"F1", "abc", true}, {"F1", "abcd", false}, {"G1", "Red", false}, {"G1", "Pink", true},
		{"H1", "TRUE", true}, {"H1", "yes", false}, {"Z1", "anything", true}, {"D1", "", true},
	}
	for _, c := range cases {
		err := s.CheckEntry(at(c.cell), c.input)
		if (err == nil) != c.ok {
			t.Errorf("%s %q: %v", c.cell, c.input, err)
		}
	}
	err := s.CheckEntry(at("B1"), "Purple")
	if err == nil || !err.Reject || err.Help != "Input must fall within specified range" {
		t.Errorf("reject: %+v", err)
	}
	if err := s.CheckEntry(at("D1"), "0"); err == nil || err.Reject || err.Help != "Input must be a number between 1 and 10" {
		t.Errorf("warn: %+v", err)
	}
	s.Set(at("D2"), "42")
	s.Set(at("H2"), "TRUE")
	if l := s.Look(at("D2")); !l.Invalid {
		t.Errorf("invalid mark: %+v", l)
	}
	if l := s.Look(at("H2")); !l.Checkbox || !l.Checked {
		t.Errorf("checkbox: %+v", l)
	}
	if l := s.Look(at("H3")); !l.Checkbox || l.Checked || l.Invalid {
		t.Errorf("blank checkbox: %+v", l)
	}
	if l := s.Look(at("C3")); !l.Dropdown {
		t.Errorf("dropdown: %+v", l)
	}
	// A rule over cells of another takes them.
	add(Validation{Ranges: ranges("C5:D6"), Kind: ValidCheckbox})
	if got := RangesText(s.Validations()[1].Ranges); got != "C1:C4,C7:C9" {
		t.Errorf("list rule's cells: %s", got)
	}
	if got := RangesText(s.Validations()[2].Ranges); got != "D1:D4,D7:D9" {
		t.Errorf("number rule's cells: %s", got)
	}
	s.ClearValidations(rng("A1:XFD1048576"))
	if len(s.Validations()) != 0 {
		t.Errorf("clear left %+v", s.Validations())
	}
}

func TestRuleJSON(t *testing.T) {
	lines := []string{
		`{"ranges":"A1:B2","condition":"between","values":["1","2"],"fill":"yellow","underline":true}`,
		`{"ranges":"A:A","scale":[{"type":"min","color":"red"},{"type":"percentile","value":"50","color":"yellow"},{"type":"max","color":"green"}]}`,
	}
	for _, l := range lines {
		f, err := ParseCondFormat(l)
		if err != nil || f.JSON() != l {
			t.Errorf("%s: %v %s", l, err, f.JSON())
		}
	}
	v, err := ParseValidation(`{"ranges":"C1:C9","criteria":"date","condition":"lt","values":["2026-12-31"],"reject":true}`)
	if err != nil || v.Kind != ValidDate || v.Op != RuleLess || !v.Reject {
		t.Errorf("%+v %v", v, err)
	}
	if _, err := ParseValidation(`{"ranges":"C1","criteria":"list"}`); err == nil {
		t.Error("a list without items")
	}
	if _, err := ParseCondFormat(`{"ranges":"C1","condition":"gt","values":["1"],"fill":"mauve"}`); err == nil {
		t.Error("an unknown color")
	}
}

// Percentiles by selection agree with sorting, duplicates and all.
func TestPercentileBySelection(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for _, n := range []int{1, 2, 3, 10, 101, 1000} {
		vals := make([]float64, n)
		for i := range vals {
			vals[i] = float64(r.IntN(20))
		}
		sorted := slices.Sorted(slices.Values(vals))
		for _, p := range []float64{0, 0.1, 0.25, 0.5, 0.9, 1} {
			x := p * float64(n-1)
			i := int(x)
			want := sorted[i]
			if i < n-1 {
				want += (x - float64(i)) * (sorted[i+1] - sorted[i])
			}
			if got := percentile(slices.Clone(vals), p); math.Abs(got-want) > 1e-9 {
				t.Errorf("n %d p %v: %v, want %v", n, p, got, want)
			}
		}
	}
}

func TestSubtract(t *testing.T) {
	got := RangesText(subtract(rng("A1:E5"), rng("B2:C3")))
	if got != "A1:E1,A4:E5,A2:A3,D2:E3" {
		t.Errorf("%s", got)
	}
	if got := RangesText(subtract(rng("A1:A5"), rng("C1:C5"))); got != "A1:A5" {
		t.Errorf("disjoint: %s", got)
	}
	if got := subtract(rng("B2:B3"), rng("A1:C9")); len(got) != 0 {
		t.Errorf("covered: %v", got)
	}
}
