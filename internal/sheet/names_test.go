package sheet

import (
	"strings"
	"testing"
)

func rect(s string) Rect {
	r, _ := ParseRange(s)
	return r
}

func TestValidName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string // start of the error, "" for valid
	}{
		{"Sales", ""},
		{"_tax.rate", ""},
		{"Q1_2026", ""},
		{"R2D2", ""},
		{"CAT", ""},
		{"", "Enter a name"},
		{"1st", "A name must start"},
		{"net profit", "A name can only"},
		{"$A", "A name must start"},
		{"true", "TRUE and FALSE"},
		{"A1", "A name can't look like a cell"},
		{"xfd1048576", "A name can't look like a cell"},
		{"R1C1", "A name can't look like a cell"},
		{"rc", "A name can't look like a cell"},
		{"C", "A name can't look like a cell"},
		{strings.Repeat("a", 251), "A name can be at most"},
	} {
		err := ValidName(tc.name)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%q: %v", tc.name, err)
		case tc.want != "" && (err == nil || !strings.HasPrefix(err.Error(), tc.want)):
			t.Errorf("%q: got %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestNamesInFormulas(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"B1": "10", "B2": "20", "B3": "30", "D1": "0.5",
		"C1": "=SUM(sales)", "C2": "=Rate*2", "C3": "=AVERAGE(Sales)*Rate",
	})
	if v := s.Value(at("C1")); v != ErrName {
		t.Fatalf("undefined name: %v", v)
	}
	if err := s.DefineName("Sales", rect("B1:B3")); err != nil {
		t.Fatal(err)
	}
	if err := s.DefineName("Rate", rect("D1")); err != nil {
		t.Fatal(err)
	}
	check := func(when string, want map[string]float64) {
		t.Helper()
		for a, n := range want {
			if v := s.Value(at(a)); v.Kind != Number || v.Num != n {
				t.Errorf("%s: %s = %v, want %v", when, a, v, n)
			}
		}
	}
	check("defined", map[string]float64{"C1": 60, "C2": 1, "C3": 10})
	s.Set(at("B2"), "50")
	s.Set(at("D1"), "1")
	check("inputs changed", map[string]float64{"C1": 90, "C2": 2, "C3": 30})

	if err := s.EditName("sales", "Sales", rect("B1:B2")); err != nil {
		t.Fatal(err)
	}
	check("range edited", map[string]float64{"C1": 60})
	if s.Value(at("B3")).Num != 30 {
		t.Error("B3 changed")
	}
	if err := s.DeleteName("RATE"); err != nil {
		t.Fatal(err)
	}
	if v := s.Value(at("C2")); v != ErrName {
		t.Errorf("deleted name: C2 = %v", v)
	}
	if err := s.DefineName("SALES", rect("A1")); err == nil || err.Error() != "Sales already names B1:B2" {
		t.Errorf("duplicate: %v", err)
	}
}

func TestRenameRewritesFormulas(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "4", "B1": "=Old*2+SUM(old)", "B2": "=Other"})
	s.DefineName("Old", rect("A1"))
	if err := s.EditName("Old", "Fresh", rect("A1")); err != nil {
		t.Fatal(err)
	}
	if got := s.Cell(at("B1")).Input; got != "=Fresh*2+SUM(Fresh)" {
		t.Errorf("B1 = %q", got)
	}
	if s.Value(at("B1")).Num != 12 || s.Cell(at("B2")).Input != "=Other" {
		t.Errorf("B1 %v B2 %q", s.Value(at("B1")), s.Cell(at("B2")).Input)
	}
	if _, ok := s.LookupName("old"); ok {
		t.Error("old name kept")
	}
	s.Undo()
	if got := s.Cell(at("B1")).Input; got != "=Old*2+SUM(old)" || s.Value(at("B1")).Num != 12 {
		t.Errorf("undo: B1 = %q %v", got, s.Value(at("B1")))
	}
}

func TestNamesFollowStructureChanges(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   func(s *Sheet)
		want string // the name's range after the change
	}{
		{"insert rows above", func(s *Sheet) { s.InsertRows(0, 2) }, "B4:B6"},
		{"insert rows inside", func(s *Sheet) { s.InsertRows(2, 1) }, "B2:B5"},
		{"insert columns left", func(s *Sheet) { s.InsertCols(0, 1) }, "C2:C4"},
		{"delete rows inside", func(s *Sheet) { s.DeleteRows(2, 1) }, "B2:B3"},
		{"delete all rows", func(s *Sheet) { s.DeleteRows(1, 3) }, "#REF!"},
		{"delete the column", func(s *Sheet) { s.DeleteCols(1, 1) }, "#REF!"},
		{"move the range", func(s *Sheet) { s.Move(rect("B2:B4"), at("D5")) }, "D5:D7"},
		{"move part of it", func(s *Sheet) { s.Move(rect("B2:B3"), at("D5")) }, "B2:B4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sheetOf(t, map[string]string{"B2": "1", "B3": "2", "B4": "3", "A10": "=SUM(Data)"})
			s.DefineName("Data", rect("B2:B4"))
			tc.op(s)
			n, _ := s.LookupName("data")
			if n.Ref() != tc.want {
				t.Fatalf("range %s, want %s", n.Ref(), tc.want)
			}
			// The formula still reads the name and follows it.
			for _, a := range s.Addrs() {
				if c := s.Cell(a); c.IsFormula() && c.Input != "=SUM(Data)" {
					t.Errorf("formula %s", c.Input)
				}
			}
			if n.Lost {
				if v := s.Value(s.Addrs()[len(s.Addrs())-1]); v != ErrRef {
					t.Errorf("lost name evaluates to %v", v)
				}
			}
			s.Undo()
			if n, _ := s.LookupName("Data"); n.Ref() != "B2:B4" {
				t.Errorf("undo: %s", n.Ref())
			}
		})
	}
}

func TestNamesUndoRedo(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "5", "B1": "=Price"})
	s.DefineName("Price", rect("A1"))
	if s.Value(at("B1")).Num != 5 {
		t.Fatalf("B1 = %v", s.Value(at("B1")))
	}
	c, ok := s.Undo()
	if !ok || c.Label != "name A1 Price" || s.Value(at("B1")) != ErrName || len(s.Names()) != 0 {
		t.Fatalf("undo: %v %v B1=%v names %v", c, ok, s.Value(at("B1")), s.Names())
	}
	s.Redo()
	if s.Value(at("B1")).Num != 5 {
		t.Fatalf("redo: B1 = %v", s.Value(at("B1")))
	}
	s.DeleteName("price")
	s.Undo()
	if n, ok := s.LookupName("PRICE"); !ok || n.Name != "Price" || s.Value(at("B1")).Num != 5 {
		t.Errorf("undo delete: %v %v", n, s.Value(at("B1")))
	}
}

func TestNameCycles(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "=SUM(Loop)", "A2": "1"})
	s.DefineName("Loop", rect("A1:A2"))
	if !s.Book().Circular || s.Value(at("A1")) != ErrRef {
		t.Errorf("circular %v A1 %v", s.Book().Circular, s.Value(at("A1")))
	}
}

func TestNamesFileRoundTrip(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "3", "A2": "4", "B1": "=SUM(Pair)", "B2": "=Gone"})
	s.DefineName("Pair", rect("A1:A2"))
	s.DefineName("Gone", rect("C5"))
	s.DeleteCols(2, 1)
	var b strings.Builder
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{`"version": 3`, `"names": {"Gone": "#REF!", "Pair": "A1:A2"}`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	r, err := Read(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if nameRefs(r) != nameRefs(s) || r.Value(at("B1")).Num != 7 || r.Value(at("B2")) != ErrRef {
		t.Errorf("read back %v B1=%v B2=%v", r.Names(), r.Value(at("B1")), r.Value(at("B2")))
	}
	if r.CanUndo() {
		t.Error("loading is undoable")
	}

	for _, bad := range []string{
		`{"version": 3, "names": {"A1": "B2"}, "cells": {}}`,
		`{"version": 3, "names": {"x": "B2:"}, "cells": {}}`,
		`{"version": 3, "names": {"x": "B2", "X": "B3"}, "cells": {}}`,
	} {
		if _, err := Read(strings.NewReader(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestPrecedentsAndDependents(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "1", "A2": "2", "B1": "3",
		"C1": "=A1+SUM(A1:A2)+Both+A1", "C2": "=A2*2", "C3": "=C1", "D1": "text",
	})
	s.DefineName("Both", rect("A2:B2"))
	if got, want := targets(s, s.Precedents(at("C1"))), "A1 A1:A2 A2:B2"; got != want {
		t.Errorf("precedents of C1 = %v, want %v", got, want)
	}
	if got := s.Precedents(at("D1")); got != nil {
		t.Errorf("precedents of text: %v", got)
	}
	if got, want := targets(s, s.Dependents(at("A2"))), "C1 C2"; got != want {
		t.Errorf("dependents of A2 = %v, want %v", got, want)
	}
	if got, want := targets(s, s.Dependents(at("B2"))), "C1"; got != want {
		t.Errorf("dependents of B2 through a name = %v, want %v", got, want)
	}
	if got := s.Dependents(at("B1")); len(got) != 0 {
		t.Errorf("dependents of B1 = %v", got)
	}
}

// targets writes traced ranges as seen from s, e.g. "A1 Data!B2:B3".
func targets(s *Sheet, ts []Target) string {
	var out []string
	for _, t := range ts {
		ref := t.Range.String()
		if t.Sheet != s {
			ref = Qualified(t.Sheet.Name(), t.Range)
		}
		out = append(out, ref)
	}
	return strings.Join(out, " ")
}

// nameRefs lists a workbook's names and what they stand for, to compare
// names across workbooks.
func nameRefs(s *Sheet) string {
	var b strings.Builder
	for _, n := range s.Names() {
		b.WriteString(n.Name + "=" + n.Ref() + " ")
	}
	return b.String()
}
