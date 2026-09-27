package sheet

import (
	"math"
	"strings"
	"testing"
)

// matching lists the cells of A1:A<n> a sheet's rules style.
func matching(s *Sheet, n int) string {
	var got []string
	for row := range n {
		if s.Look(Addr{Row: row}).Styled {
			got = append(got, Addr{Row: row}.String())
		}
	}
	return strings.Join(got, " ")
}

// Top and bottom values, above and below average, duplicates and
// uniques compare a cell with the rest of the rule's cells.
func TestRankingRules(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "10", "A2": "40", "A3": "30", "A4": "40", "A5": "Apple", "A6": "apple", "A7": "pear", "A8": "",
	})
	for _, c := range []struct {
		op    RuleOp
		arg   string
		match string
	}{
		{RuleTop, "2", "A2 A4"}, // ties count
		{RuleTop, "3", "A2 A3 A4"},
		{RuleBottom, "1", "A1"},
		{RuleTopPercent, "50", "A2 A4"},
		{RuleBottomPercent, "10", "A1"}, // at least one
		{RuleTop, "99", "A1 A2 A3 A4"},
		{RuleAboveAverage, "", "A2 A4"}, // the mean is 30
		{RuleBelowAverage, "", "A1"},
		{RuleDuplicate, "", "A2 A4 A5 A6"},
		{RuleUnique, "", "A1 A3 A7"},
	} {
		s.rules = rulesState{}
		mustAdd(t, s, CondFormat{Ranges: ranges("A1:A8"), Op: c.op, Args: [2]string{c.arg}, Style: green})
		if got := matching(s, 8); got != c.match {
			t.Errorf("%s %s: matched %q, want %q", c.op, c.arg, got, c.match)
		}
	}
	for _, bad := range []CondFormat{
		{Ranges: ranges("A1:A8"), Op: RuleTop, Args: [2]string{"0"}, Style: green},
		{Ranges: ranges("A1:A8"), Op: RuleTopPercent, Args: [2]string{"101"}, Style: green},
		{Ranges: ranges("A1:A8"), Op: RuleBottom, Args: [2]string{"x"}, Style: green},
	} {
		if bad.Check() == nil {
			t.Errorf("%s %q passed", bad.Op, bad.Args[0])
		}
	}
	f := CondFormat{Ranges: ranges("A1"), Op: RuleTopPercent, Args: [2]string{"10"}, Style: green}
	if f.Summary() != "Top 10%" {
		t.Errorf("summary %q", f.Summary())
	}
}

// Date rules take periods: this, last or next week, month or year, and
// the past week, month or year. 2026-09-26 is a Saturday.
func TestDatePeriods(t *testing.T) {
	fixedNow(t)
	s := sheetOf(t, map[string]string{
		"A1": "2026-09-19", "A2": "2026-09-20", "A3": "2026-09-26", "A4": "2026-09-27", "A5": "2026-10-15",
		"A6": "2026-08-31", "A7": "2025-12-31", "A8": "2026-01-01",
	})
	for _, c := range []struct {
		op    RuleOp
		arg   string
		match string
	}{
		{RuleDateIs, "this week", "A2 A3"},
		{RuleDateIs, "last week", "A1"},
		{RuleDateIs, "next week", "A4"},
		{RuleDateIs, "this month", "A1 A2 A3 A4"},
		{RuleDateIs, "Next  month", "A5"},
		{RuleDateIs, "last month", "A6"},
		{RuleDateIs, "this year", "A1 A2 A3 A4 A5 A6 A8"},
		{RuleDateIs, "last year", "A7"},
		{RuleDateIs, "past week", "A2 A3"},
		{RuleDateIs, "in the past month", "A1 A2 A3 A6"},
		{RuleDateBefore, "this week", "A1 A6 A7 A8"},
		{RuleDateAfter, "this month", "A5"},
		{RuleDateIs, "today", "A3"},
	} {
		s.rules = rulesState{}
		mustAdd(t, s, CondFormat{Ranges: ranges("A1:A8"), Op: c.op, Args: [2]string{c.arg}, Style: green})
		if got := matching(s, 8); got != c.match {
			t.Errorf("%s %q: matched %q, want %q", c.op, c.arg, got, c.match)
		}
	}
	if !IsPeriod("last month") || IsPeriod("2026-09-01") || IsPeriod("soon") {
		t.Error("IsPeriod")
	}
	if (CondFormat{Ranges: ranges("A1"), Op: RuleDateIs, Args: [2]string{"soon"}, Style: green}).Check() == nil {
		t.Error("a period that isn't one passed")
	}
}

// A data bar is as long as its number is far from the shortest point,
// zero for positive numbers, to the longest; an icon set gives each
// number the icon of the last threshold it reaches.
func TestDataBarsAndIcons(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "0", "A2": "25", "A3": "50", "A4": "100", "A5": "x"})
	mustAdd(t, s, CondFormat{Ranges: ranges("A2:A5"), Bar: ColorBlue,
		Scale: []ScalePoint{{Kind: PointMin}, {Kind: PointMax}}})
	for cell, want := range map[string]float64{"A2": 0.25, "A3": 0.5, "A4": 1} {
		l := s.Look(at(cell))
		if !l.Bar || l.BarColor != ColorBlue || math.Abs(l.BarLen-want) > 1e-9 {
			t.Errorf("%s: %+v, want a bar %v long", cell, l, want)
		}
	}
	if s.Look(at("A5")).Bar {
		t.Error("text has a bar")
	}
	s.rules = rulesState{}
	icons := CondFormat{Ranges: ranges("A1:A4"), Icons: IconsArrows,
		Scale: []ScalePoint{{Kind: PointPercent, Value: "33"}, {Kind: PointPercent, Value: "67"}}}
	mustAdd(t, s, icons)
	for cell, want := range map[string]string{"A1": "↓", "A2": "↓", "A3": "→", "A4": "↑"} {
		if l := s.Look(at(cell)); l.Icon != want {
			t.Errorf("%s: icon %q, want %q", cell, l.Icon, want)
		}
	}
	if l := s.Look(at("A4")); l.IconColor != ColorGreen || l.ValueHidden {
		t.Errorf("A4: %+v", l)
	}
	icons.Reverse, icons.BarOnly = true, true
	s.SetCondFormat(0, icons)
	if l := s.Look(at("A4")); l.Icon != "↓" || l.IconColor != ColorRed || !l.ValueHidden {
		t.Errorf("reversed A4: %+v", l)
	}
	for _, bad := range []CondFormat{
		{Ranges: ranges("A1"), Icons: IconsSymbols, Scale: []ScalePoint{{Kind: PointPercent, Value: "50"}}},
		{Ranges: ranges("A1"), Icons: IconsArrows, Scale: []ScalePoint{{Kind: PointPercent, Value: "150"}, {Kind: PointNumber, Value: "2"}}},
		{Ranges: ranges("A1"), Bar: ColorRed, Scale: []ScalePoint{{Kind: PointMin}}},
	} {
		if bad.Check() == nil {
			t.Errorf("%+v passed", bad)
		}
	}
}

// Data bars and icon sets are lines of the file as the other rules are.
func TestBarAndIconJSON(t *testing.T) {
	for _, line := range []string{
		`{"ranges":"B2:B9","dataBar":{"color":"blue","min":{"type":"num","value":"0"},"max":{"type":"percentile","value":"90"},"barOnly":true}}`,
		`{"ranges":"C2:C9","iconSet":{"icons":"rating","points":[{"type":"percent","value":"20"},{"type":"percent","value":"40"},{"type":"percent","value":"60"},{"type":"percent","value":"80"}],"reverse":true,"iconOnly":true}}`,
		`{"ranges":"D2:D9","condition":"top_percent","values":["10"],"fill":"green"}`,
		`{"ranges":"E2:E9","condition":"date_is","values":["last month"],"bold":true}`,
	} {
		f, err := ParseCondFormat(line)
		if err != nil {
			t.Errorf("%s: %v", line, err)
			continue
		}
		if got := f.JSON(); got != line {
			t.Errorf("read and written:\n%s\nwant\n%s", got, line)
		}
	}
	for _, bad := range []string{
		`{"ranges":"B2","dataBar":{"color":"","min":{"type":"min"},"max":{"type":"max"}}}`,
		`{"ranges":"B2","iconSet":{"icons":"stars","points":[]}}`,
		`{"ranges":"B2","iconSet":{"icons":"symbols","points":[{"type":"percent","value":"50"}]}}`,
	} {
		if _, err := ParseCondFormat(bad); err == nil {
			t.Errorf("%s read", bad)
		}
	}
}

// A checkbox may hold values of its own: checked when a cell shows the
// first, unchecked for the second or a blank, invalid otherwise.
func TestCustomCheckbox(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "Yes", "A2": "no", "A3": "", "A4": "TRUE", "A5": "maybe"})
	v := Validation{Ranges: ranges("A1:A5"), Kind: ValidCheckbox, Items: []string{"Yes", "No"}}
	if err := s.AddValidation(v); err != nil {
		t.Fatal(err)
	}
	for cell, want := range map[string][2]bool{"A1": {true, false}, "A2": {false, false}, "A3": {false, false}, "A4": {false, true}, "A5": {false, true}} {
		l := s.Look(at(cell))
		if !l.Checkbox || l.Checked != want[0] || l.Invalid != want[1] {
			t.Errorf("%s: %+v, want checked %v invalid %v", cell, l, want[0], want[1])
		}
	}
	if s.CheckboxInput(at("A1"), true) != "Yes" || s.CheckboxInput(at("A1"), false) != "No" {
		t.Error("toggled values")
	}
	if bad := s.CheckEntry(at("A3"), "maybe"); bad == nil || bad.Help != "Input must be Yes or No" {
		t.Errorf("entry check %+v", bad)
	}
	for _, bad := range []Validation{
		{Ranges: ranges("B1"), Kind: ValidCheckbox, Items: []string{"", "No"}},
		{Ranges: ranges("B1"), Kind: ValidCheckbox, Items: []string{"x", "X"}},
		{Ranges: ranges("B1"), Kind: ValidCheckbox, Items: []string{"a", "b", "c"}},
	} {
		if bad.Check() == nil {
			t.Errorf("%q passed", bad.Items)
		}
	}
	for _, line := range []string{
		`{"ranges":"A1:A5","criteria":"checkbox","items":["Yes","No"]}`,
		`{"ranges":"B1:B5","criteria":"list","items":["a","b"],"display":"chip"}`,
		`{"ranges":"C1:C5","criteria":"range","source":"A1:A5","display":"plain"}`,
	} {
		v, err := ParseValidation(line)
		if err != nil || v.JSON() != line {
			t.Errorf("%s read as %s, %v", line, v.JSON(), err)
		}
	}
	if _, err := ParseValidation(`{"ranges":"A1","criteria":"list","items":["a"],"display":"bubble"}`); err == nil {
		t.Error("an unknown display read")
	}
	s.AddValidation(Validation{Ranges: ranges("B1"), Kind: ValidList, Items: []string{"a"}, Display: DropChip})
	if l := s.Look(at("B1")); !l.Dropdown || l.Display != DropChip {
		t.Errorf("chip look %+v", l)
	}
}
