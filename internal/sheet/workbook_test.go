package sheet

import (
	"bytes"
	"strings"
	"testing"
)

// page is one sheet of a test workbook: its name and cell inputs.
type page struct {
	name  string
	cells map[string]string
}

// bookOf builds a workbook from pages, without undo history.
func bookOf(t *testing.T, pages ...page) *Workbook {
	t.Helper()
	w := emptyBook()
	for _, p := range pages {
		w.insert(w.newSheet(p.name), len(w.sheets))
	}
	for i, p := range pages {
		for a, in := range p.cells {
			if err := w.sheets[i].put(at(a), in); err != nil {
				t.Fatalf("%s!%s: %v", p.name, a, err)
			}
		}
	}
	w.RecalcAll()
	return w
}

// show is what a cell shows, for comparing in tests: its value, or its
// input with "input:" in front of the address.
func show(t *testing.T, w *Workbook, ref string) string {
	t.Helper()
	sheet, rest := SplitSheet(ref)
	s := w.Lookup(sheet)
	if s == nil {
		t.Fatalf("no sheet %q", sheet)
	}
	if in, ok := strings.CutPrefix(rest, "input:"); ok {
		if c := s.Cell(at(in)); c != nil {
			return c.Input
		}
		return ""
	}
	return s.Value(at(rest)).String()
}

func expect(t *testing.T, w *Workbook, want map[string]string) {
	t.Helper()
	for ref, v := range want {
		if got := show(t, w, ref); got != v {
			t.Errorf("%s = %q, want %q", ref, got, v)
		}
	}
}

func TestParseSheetRefs(t *testing.T) {
	tests := []struct{ in, out, err string }{
		{in: "=Sheet2!A1", out: "=Sheet2!A1"},
		{in: "=sheet2!$a$1+1", out: "=sheet2!$A$1+1"},
		{in: "='My Sheet'!B2:C9", out: "='My Sheet'!B2:C9"},
		{in: "=SUM('Bob''s'!A1:A3)", out: "=SUM('Bob''s'!A1:A3)"},
		{in: "=Data!B3:Data!A1", out: "=Data!A1:B3"},
		{in: "='2026'!A1", out: "='2026'!A1"},
		{in: "='A1'!A1", out: "='A1'!A1"},
		{in: "=Q3.plan!A1..B2", out: "=Q3.plan!A1:B2"},
		{in: "=Sheet2!Sales", err: "Expected a cell after Sheet2!"},
		{in: "=Sheet2!A1:Other!B2", err: "A range can't span sheets"},
		{in: "='Open!A1", err: "Expected ! after a quoted sheet name"},
		{in: "='x'A1", err: "Expected ! after a quoted sheet name"},
	}
	for _, tt := range tests {
		n, err := Parse(tt.in)
		switch {
		case tt.err != "":
			if err == nil || err.Error() != tt.err {
				t.Errorf("%s: error %v, want %q", tt.in, err, tt.err)
			}
		case err != nil:
			t.Errorf("%s: %v", tt.in, err)
		case formulaText(n) != tt.out:
			t.Errorf("%s prints as %s, want %s", tt.in, formulaText(n), tt.out)
		}
	}
}

func TestQuoteSheet(t *testing.T) {
	for name, want := range map[string]string{
		"Sheet1": "Sheet1", "Q3_plan": "Q3_plan", "My Sheet": "'My Sheet'", "Bob's": "'Bob''s'",
		"2026": "'2026'", "AB12": "'AB12'", "R1C1": "'R1C1'", "Café": "'Café'",
	} {
		if got := quoteSheet(name); got != want {
			t.Errorf("quoteSheet(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestSheetNames(t *testing.T) {
	w := NewBook()
	tests := []struct{ name, err string }{
		{"Budget", ""},
		{"Sheet1", "There's already a sheet named Sheet1"},
		{"sheet1", "There's already a sheet named Sheet1"},
		{"a/b", `A sheet name can't contain : \ / ? * [ or ]`},
		{"'quoted", "A sheet name can't start or end with '"},
		{strings.Repeat("x", 32), "A sheet name can be at most 31 characters"},
	}
	for _, tt := range tests {
		_, err := w.AddSheet(tt.name, w.Len())
		if got := errText(err); got != tt.err {
			t.Errorf("AddSheet(%q): %q, want %q", tt.name, got, tt.err)
		}
	}
	if s, _ := w.AddSheet("", w.Len()); s.Name() != "Sheet3" {
		t.Errorf("next default name %s, want Sheet3", s.Name())
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestCrossSheetValues(t *testing.T) {
	w := bookOf(t,
		page{"Sheet1", map[string]string{
			"A1": "=Data!A1*2", "A2": "=SUM('Q3 plan'!A1:A3)", "A3": "=Nowhere!A1",
			"A4": "=Sales", "A5": "=Data!A1+A1", "A6": "=VLOOKUP(2, 'Q3 plan'!A1:B3, 2, FALSE)",
		}},
		page{"Data", map[string]string{"A1": "21", "B1": "=Sheet1!A1+1"}},
		page{"Q3 plan", map[string]string{"A1": "1", "A2": "2", "A3": "3", "B2": "two"}},
	)
	if err := w.DefineName("Sales", w.Lookup("Q3 plan"), NewRect(at("A1"), at("A3"))); err != nil {
		t.Fatal(err)
	}
	w.Sheet(0).Set(at("A4"), "=SUM(Sales)")
	expect(t, w, map[string]string{
		"Sheet1!A1": "42", "Sheet1!A2": "6", "Sheet1!A3": "#REF!", "Sheet1!A4": "6",
		"Sheet1!A5": "63", "Sheet1!A6": "two", "Data!B1": "43",
	})
	if got := w.Sheet(0).ExplainError(at("A3")); got != "Unresolved sheet name Nowhere" {
		t.Errorf("explain A3: %q", got)
	}
	// Changing a cell on one sheet recalculates the others, in order.
	w.Lookup("Data").Set(at("A1"), "5")
	w.Lookup("Q3 plan").Set(at("A3"), "30")
	expect(t, w, map[string]string{"Sheet1!A1": "10", "Data!B1": "11", "Sheet1!A2": "33", "Sheet1!A4": "33", "Sheet1!A5": "15"})
	// A cycle across sheets is circular.
	w.Lookup("Data").Set(at("A1"), "=Sheet1!A1")
	if !w.Circular || show(t, w, "Sheet1!A1") != "#REF!" {
		t.Errorf("circular %v, A1 %s", w.Circular, show(t, w, "Sheet1!A1"))
	}
	if got := w.Sheet(0).ExplainError(at("A1")); got != "Circular reference: A1 → Data!A1 → A1" {
		t.Errorf("explain cycle: %q", got)
	}
}

func TestRenameSheetRewritesFormulas(t *testing.T) {
	w := bookOf(t,
		page{"Sheet1", map[string]string{"A1": "=Data!A1+data!A2", "A2": "=SUM(Data!A1:A2)", "A3": "=Other!A1"}},
		page{"Data", map[string]string{"A1": "1", "A2": "2", "A3": "=Data!A1*10"}},
	)
	data := w.Lookup("Data")
	if err := w.RenameSheet(data, "Q3 plan"); err != nil {
		t.Fatal(err)
	}
	expect(t, w, map[string]string{
		"Sheet1!input:A1": "='Q3 plan'!A1+'Q3 plan'!A2", "Sheet1!input:A2": "=SUM('Q3 plan'!A1:A2)",
		"Sheet1!input:A3": "=Other!A1", "Q3 plan!input:A3": "='Q3 plan'!A1*10",
		"Sheet1!A1": "3", "Q3 plan!A3": "10",
	})
	// Renaming to a name formulas already use makes them resolve.
	if err := w.RenameSheet(data, "Other"); err != nil {
		t.Fatal(err)
	}
	expect(t, w, map[string]string{"Sheet1!A3": "1", "Sheet1!input:A1": "=Other!A1+Other!A2"})
	w.Undo()
	w.Undo()
	expect(t, w, map[string]string{"Data!A3": "10", "Sheet1!input:A1": "=Data!A1+data!A2", "Sheet1!A3": "#REF!"})
	if data.Name() != "Data" {
		t.Errorf("undo left the name %s", data.Name())
	}
}

func TestDeleteSheet(t *testing.T) {
	w := bookOf(t,
		page{"Sheet1", map[string]string{"A1": "=Data!A1*2", "A2": "=Total"}},
		page{"Data", map[string]string{"A1": "21"}},
	)
	data := w.Lookup("Data")
	w.DefineName("Total", data, NewRect(at("A1"), at("A1")))
	if err := w.DeleteSheet(data); err != nil {
		t.Fatal(err)
	}
	expect(t, w, map[string]string{"Sheet1!A1": "#REF!", "Sheet1!A2": "#REF!", "Sheet1!input:A1": "=Data!A1*2"})
	if n, _ := w.LookupName("Total"); n.Ref() != "#REF!" {
		t.Errorf("name on a deleted sheet: %s", n.Ref())
	}
	if err := w.DeleteSheet(w.Sheet(0)); errText(err) != "A spreadsheet needs at least one sheet" {
		t.Errorf("deleting the last sheet: %v", err)
	}
	// Changes to the deleted sheet's old cells don't reach anyone.
	c, ok := w.Undo()
	if !ok || c.Label != "delete sheet Data" || c.Sheet != data || w.Len() != 2 {
		t.Fatalf("undo: %+v %v, %d sheets", c, ok, w.Len())
	}
	expect(t, w, map[string]string{"Sheet1!A1": "42", "Sheet1!A2": "21"})
	w.Redo()
	// A new sheet with the name brings the formulas back.
	s, _ := w.AddSheet("data", 1)
	s.Set(at("A1"), "4")
	expect(t, w, map[string]string{"Sheet1!A1": "8"})
}

func TestStructureOnOneSheetLeavesOthers(t *testing.T) {
	w := bookOf(t,
		page{"Sheet1", map[string]string{"A1": "1", "A2": "=Data!A2", "A3": "=A1", "A4": "=SUM(Data!A1:A3)"}},
		page{"Data", map[string]string{"A1": "10", "A2": "20", "A3": "30", "B1": "=Sheet1!A1", "B2": "=A2"}},
	)
	one, data := w.Sheet(0), w.Lookup("Data")
	// Rows inserted on Data move references to Data, wherever they are.
	if err := data.InsertRows(0, 1); err != nil {
		t.Fatal(err)
	}
	expect(t, w, map[string]string{
		"Sheet1!input:A2": "=Data!A3", "Sheet1!input:A4": "=SUM(Data!A2:A4)", "Sheet1!A2": "20",
		"Data!input:B2": "=Sheet1!A1", "Data!input:B3": "=A3",
	})
	// Rows inserted on Sheet1 leave its references to Data alone.
	if err := one.InsertRows(0, 2); err != nil {
		t.Fatal(err)
	}
	expect(t, w, map[string]string{
		"Sheet1!input:A4": "=Data!A3", "Sheet1!input:A5": "=A3", "Data!input:B2": "=Sheet1!A3", "Sheet1!A4": "20",
	})
	// Deleting referenced rows on Data leaves #REF! in Sheet1.
	data.DeleteRows(2, 1)
	expect(t, w, map[string]string{"Sheet1!input:A4": "=#REF!", "Sheet1!input:A6": "=SUM(Data!A2:A3)", "Sheet1!A6": "40"})
	w.Undo()
	expect(t, w, map[string]string{"Sheet1!input:A4": "=Data!A3", "Sheet1!A6": "60"})
}

func TestCopyBetweenSheets(t *testing.T) {
	w := bookOf(t,
		page{"Sheet1", map[string]string{"A1": "5", "B1": "=A1*2", "C1": "=Data!A1+$A$1"}},
		page{"Data", map[string]string{"A1": "100", "A2": "7"}},
	)
	one, data := w.Sheet(0), w.Lookup("Data")
	clip := one.Copy(NewRect(at("B1"), at("C1")))
	if _, err := data.Paste(clip, NewRect(at("B2"), at("B2")), false); err != nil {
		t.Fatal(err)
	}
	// References without a sheet now read Data; relative parts shifted.
	expect(t, w, map[string]string{"Data!input:B2": "=A2*2", "Data!B2": "14", "Data!input:C2": "=Data!A2+$A$1", "Data!C2": "107"})
}

func TestMoveBetweenSheets(t *testing.T) {
	w := bookOf(t,
		page{"Sheet1", map[string]string{"A1": "5", "A2": "=A1*2", "A3": "=B9", "B1": "=A2+1", "C1": "=Data!Z1"}},
		page{"Data", map[string]string{"B1": "old", "Z1": "=Sheet1!A2", "Z2": "=B1"}},
		page{"Other", map[string]string{"A1": "=Sheet1!A1+Sheet1!A2"}},
	)
	one, data := w.Sheet(0), w.Lookup("Data")
	d, err := one.MoveTo(data, NewRect(at("A1"), at("A3")), at("B1"))
	if err != nil || d != NewRect(at("B1"), at("B3")) {
		t.Fatalf("MoveTo: %v %v", d, err)
	}
	expect(t, w, map[string]string{
		// The moved formulas read what they read, naming Sheet1 for B9.
		"Data!input:B2": "=B1*2", "Data!input:B3": "=Sheet1!B9", "Data!B2": "10",
		// Formulas elsewhere follow the cells to Data.
		"Sheet1!input:B1": "=Data!B2+1", "Sheet1!B1": "11", "Data!input:Z1": "=B2",
		"Other!input:A1": "=Data!B1+Data!B2", "Other!A1": "15",
		// Z2 read the cell the move overwrote.
		"Data!input:Z2": "=#REF!", "Sheet1!input:A1": "",
	})
	c, _ := w.Undo()
	if c.Sheet != data {
		t.Errorf("undo shows %v", c.Sheet)
	}
	expect(t, w, map[string]string{"Sheet1!input:A2": "=A1*2", "Data!B1": "old", "Other!A1": "15", "Data!input:Z2": "=B1"})
}

func TestSheetUndo(t *testing.T) {
	w := NewBook()
	one := w.Sheet(0)
	one.Set(at("A1"), "=Two!A1")
	two, _ := w.AddSheet("Two", 1)
	two.Set(at("A1"), "7")
	dup, _ := w.DuplicateSheet(two)
	w.MoveSheet(dup, 0)
	w.RenameSheet(two, "Deux")
	order := func() string {
		var names []string
		for _, s := range w.Sheets() {
			names = append(names, s.Name())
		}
		return strings.Join(names, ",")
	}
	if got := order(); got != "Copy of Two,Sheet1,Deux" || show(t, w, "Sheet1!A1") != "7" {
		t.Fatalf("order %s, A1 %s", got, show(t, w, "Sheet1!A1"))
	}
	steps := []string{"Copy of Two,Sheet1,Two", "Sheet1,Two,Copy of Two", "Sheet1,Two", "Sheet1,Two", "Sheet1"}
	for i, want := range steps {
		w.Undo()
		if got := order(); got != want {
			t.Errorf("undo %d: %s, want %s", i+1, got, want)
		}
	}
	if show(t, w, "Sheet1!A1") != "#REF!" {
		t.Errorf("A1 %s after undoing the sheet", show(t, w, "Sheet1!A1"))
	}
	for range steps {
		w.Redo()
	}
	if got := order(); got != "Copy of Two,Sheet1,Deux" || show(t, w, "Sheet1!A1") != "7" || show(t, w, "Copy of Two!A1") != "7" {
		t.Errorf("redo: %s, A1 %s", order(), show(t, w, "Sheet1!A1"))
	}
}

func TestDuplicateIsIndependent(t *testing.T) {
	w := bookOf(t, page{"Sheet1", map[string]string{"A1": "2", "A2": "=A1*3"}})
	one := w.Sheet(0)
	one.SetFrozen(1, 0)
	cp, err := w.DuplicateSheet(one)
	if err != nil || cp.Name() != "Copy of Sheet1" || w.Index(cp) != 1 {
		t.Fatalf("duplicate %v %v", cp, err)
	}
	cp.Set(at("A1"), "10")
	if r, _ := cp.Frozen(); r != 1 {
		t.Error("frozen rows not copied")
	}
	expect(t, w, map[string]string{"Sheet1!A2": "6", "Copy of Sheet1!A2": "30"})
	if again, _ := w.DuplicateSheet(one); again.Name() != "Copy of Sheet1 2" {
		t.Errorf("second copy named %s", again.Name())
	}
}

func TestWorkbookFileRoundTrip(t *testing.T) {
	w := bookOf(t,
		page{"Budget", map[string]string{"A1": "Rent", "B1": "1450", "B2": "='Q3 plan'!A1"}},
		page{"Q3 plan", map[string]string{"A1": "=Budget!B1*2"}},
	)
	w.Sheet(1).SetColWidth(0, 14)
	w.Sheet(1).SetFrozen(1, 0)
	w.DefineName("Rent", w.Sheet(0), NewRect(at("B1"), at("B1")))
	w.SetActive(w.Sheet(1))
	var b bytes.Buffer
	if err := w.Write(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{`"version": 4,`, `"names": {"Rent": "Budget!B1"},`, `"active": 1,`,
		`      "name": "Q3 plan",` + "\n" + `      "widths": {"A": 14},`, `        "B2": "='Q3 plan'!A1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	r, err := ReadBook(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if r.Len() != 2 || r.Active() != 1 || nameRefs(r.Sheet(0)) != "Rent=Budget!B1 " {
		t.Fatalf("read %d sheets, active %d, names %s", r.Len(), r.Active(), nameRefs(r.Sheet(0)))
	}
	expect(t, r, map[string]string{"Budget!B2": "2900", "Q3 plan!A1": "2900"})
	if rows, _ := r.Sheet(1).Frozen(); rows != 1 || r.Sheet(1).ColWidth(0) != 14 || r.CanUndo() {
		t.Error("sheet settings not read back, or loading is undoable")
	}
	var again bytes.Buffer
	r.Write(&again)
	if again.String() != out {
		t.Errorf("second write differs:\n%s", again.String())
	}
}

// A workbook of one sheet keeps writing the single-sheet format, so older
// builds open it; a renamed sheet adds a field they ignore.
func TestSingleSheetFileStaysCompatible(t *testing.T) {
	w := bookOf(t, page{"Sheet1", map[string]string{"A1": "1"}})
	write := func() string {
		var b bytes.Buffer
		w.Write(&b)
		return b.String()
	}
	if got := write(); got != "{\n  \"version\": 2,\n  \"cells\": {\n    \"A1\": \"1\"\n  }\n}\n" {
		t.Errorf("plain sheet:\n%s", got)
	}
	w.RenameSheet(w.Sheet(0), "Budget")
	out := write()
	if !strings.HasPrefix(out, "{\n  \"version\": 2,\n  \"name\": \"Budget\",\n") {
		t.Errorf("renamed sheet:\n%s", out)
	}
	r, err := Read(strings.NewReader(out))
	if err != nil || r.Name() != "Budget" {
		t.Errorf("read back %v %v", r, err)
	}
	// A formula naming a sheet, even an unknown one, needs version 4.
	w.Sheet(0).Set(at("A2"), "=Other!A1")
	if out := write(); !strings.Contains(out, `"version": 4`) {
		t.Errorf("cross-sheet formula:\n%s", out)
	}
	for _, bad := range []string{
		`{"version": 4, "sheets": []}`,
		`{"version": 4, "sheets": [{"name": "A", "cells": {}}, {"name": "a", "cells": {}}]}`,
		`{"version": 4, "sheets": [{"name": "x/y", "cells": {}}]}`,
		`{"version": 4, "names": {"N": "Missing!A1"}, "sheets": [{"name": "A", "cells": {}}]}`,
		`{"version": 5, "cells": {}}`,
	} {
		if _, err := ReadBook(strings.NewReader(bad)); err == nil {
			t.Errorf("read %s without error", bad)
		}
	}
}

// Decimal arithmetic is a workbook setting: one undo step for every
// sheet, saved once at the top of the file.
func TestDecimalIsWorkbookWide(t *testing.T) {
	w := bookOf(t,
		page{"Sheet1", map[string]string{"A1": "=0.1+0.2"}},
		page{"Data", map[string]string{"A1": "=Sheet1!A1=0.3"}},
	)
	w.SetDecimal(true)
	expect(t, w, map[string]string{"Sheet1!A1": "0.3", "Data!A1": "TRUE"})
	var b bytes.Buffer
	w.Write(&b)
	if out := b.String(); !strings.Contains(out, "{\n  \"version\": 4,\n  \"arithmetic\": \"decimal\",\n") {
		t.Errorf("file:\n%s", out)
	}
	r, err := ReadBook(&b)
	if err != nil || !r.Decimal() {
		t.Fatalf("read back: decimal %v, %v", r.Decimal(), err)
	}
	c, _ := w.Undo()
	if w.Decimal() || c.Label != "turn off decimal arithmetic" && c.Label != "turn on decimal arithmetic" {
		t.Errorf("undo: %v %q", w.Decimal(), c.Label)
	}
	expect(t, w, map[string]string{"Data!A1": "FALSE"})
	// A version 3 file keeps the setting at the top, as before.
	s, err := Read(strings.NewReader(`{"version": 3, "arithmetic": "decimal", "cells": {"A1": "=0.1+0.2"}}`))
	if err != nil || !s.Decimal() || s.Value(Addr{}).String() != "0.3" {
		t.Errorf("version 3: %v %v", s, err)
	}
}

func TestSplitSheet(t *testing.T) {
	tests := []struct{ in, sheet, rest string }{
		{"A1", "", "A1"},
		{"Sheet2!A1:B3", "Sheet2", "A1:B3"},
		{"'Q3 plan'!B2", "Q3 plan", "B2"},
		{"'Bob''s'!C1", "Bob's", "C1"},
		{" Data!Z9 ", "Data", "Z9"},
	}
	for _, tt := range tests {
		if sheet, rest := SplitSheet(tt.in); sheet != tt.sheet || rest != tt.rest {
			t.Errorf("SplitSheet(%q) = %q, %q", tt.in, sheet, rest)
		}
	}
}
