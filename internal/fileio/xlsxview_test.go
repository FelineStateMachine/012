package fileio

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// months is a small table with a header row: months, amounts and notes.
func months(t *testing.T) *sheet.Sheet {
	return build(t, map[string]string{
		"A1": "Month", "B1": "Amount", "C1": "Note",
		"A2": "Jan", "B2": "$1,200", "C2": "rent*",
		"A3": "Feb", "B3": "80", "C3": "food",
		"A4": "Mar", "B4": "1450", "C4": "",
		"A5": "Apr", "B5": "=B4*2", "C5": "Fees",
	})
}

// exportImport writes s as XLSX and reads it back.
func exportImport(t *testing.T, s *sheet.Sheet) (string, *Result) {
	t.Helper()
	name := filepath.Join(t.TempDir(), "view.xlsx")
	if _, err := Export(context.Background(), name, XLSX, SnapBook(s), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return name, got
}

func hiddenRows(s *sheet.Sheet) []int {
	var out []int
	for r := range 10 {
		if s.RowHidden(r) {
			out = append(out, r)
		}
	}
	return out
}

// Frozen panes and a filter go out as Excel's and come back the same.
func TestXLSXFrozenAndFilterRoundTrip(t *testing.T) {
	src := months(t)
	src.SetFrozen(1, 2)
	src.CreateFilter(sheet.NewRect(addr(t, "A1"), addr(t, "C5")))
	src.FilterColumn(0, sheet.Criteria{Hidden: []string{"Feb"}})
	src.FilterColumn(1, sheet.Criteria{Cond: sheet.Condition{Op: sheet.CondLess, Arg: "2000"}})
	want := []int{2, 4} // Feb, and Apr's 2900
	if got := hiddenRows(src); !slices.Equal(got, want) {
		t.Fatalf("hidden before export: %v", got)
	}
	name, got := exportImport(t, src)

	// excelize reads what Excel would.
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if p, _ := x.GetPanes("Sheet1"); !p.Freeze || p.XSplit != 2 || p.YSplit != 1 || p.TopLeftCell != "C2" || p.ActivePane != "bottomRight" {
		t.Errorf("panes %+v", p)
	}
	for row, shown := range map[int]bool{1: true, 2: true, 3: false, 4: true, 5: false} {
		if v, _ := x.GetRowVisible("Sheet1", row); v != shown {
			t.Errorf("row %d visible %v in Excel", row, v)
		}
	}
	db := false
	for _, dn := range x.GetDefinedName() {
		db = db || dn.Name == "_xlnm._FilterDatabase" && dn.RefersTo == "Sheet1!$A$1:$C$5" && dn.Scope == "Sheet1"
	}
	if !db {
		t.Errorf("no _FilterDatabase name: %+v", x.GetDefinedName())
	}
	afs, _, _, err := referenceAutoFilters(name)
	if err != nil {
		t.Fatal(err)
	}
	wantAF := &xlsxAutoFilter{ref: "A1:C5", cols: []xlsxFilterColumn{
		{col: 0, filters: true, values: []string{"Apr", "Jan", "Mar"}},
		{col: 1, custom: []xlsxCustomFilter{{op: "lessThan", val: "2000"}}},
	}}
	if !reflect.DeepEqual(afs[0], wantAF) {
		t.Errorf("autoFilter %+v, want %+v", afs[0], wantAF)
	}

	s := got.Sheet
	if r, c := s.Frozen(); r != 1 || c != 2 {
		t.Errorf("frozen %d rows, %d cols", r, c)
	}
	if !reflect.DeepEqual(s.Filter(), src.Filter()) {
		t.Errorf("filter %+v, want %+v", s.Filter(), src.Filter())
	}
	if h := hiddenRows(s); !slices.Equal(h, want) {
		t.Errorf("hidden after import: %v", h)
	}
	if s.Book().CanUndo() {
		t.Error("import left an undo step")
	}
}

// Each condition goes out as Excel's custom filter (or its blanks
// filter) and comes back hiding the same rows.
func TestXLSXFilterConditions(t *testing.T) {
	for _, c := range []sheet.Condition{
		{Op: sheet.CondEmpty}, {Op: sheet.CondNotEmpty},
		{Op: sheet.CondContains, Arg: "E"}, {Op: sheet.CondContains, Arg: "t*"}, {Op: sheet.CondNotContains, Arg: "oo"},
		{Op: sheet.CondStartsWith, Arg: "rent"}, {Op: sheet.CondEndsWith, Arg: "s"}, {Op: sheet.CondExactly, Arg: "FOOD"},
		{Op: sheet.CondGreater, Arg: "100"}, {Op: sheet.CondGreaterEq, Arg: "$1,450"}, {Op: sheet.CondLess, Arg: "1200"},
		{Op: sheet.CondLessEq, Arg: "80"}, {Op: sheet.CondEqual, Arg: "1450"}, {Op: sheet.CondNotEqual, Arg: "80"},
	} {
		col := 2 // text conditions on the notes, the others on the amounts
		if c.Op >= sheet.CondGreater {
			col = 1
		}
		src := months(t)
		src.CreateFilter(sheet.NewRect(addr(t, "A1"), addr(t, "C5")))
		src.FilterColumn(col, sheet.Criteria{Cond: c})
		want := hiddenRows(src)
		_, got := exportImport(t, src)
		s := got.Sheet
		cr := s.Filter().Cols[col]
		if cr.Cond.Op != c.Op || len(cr.Hidden) > 0 {
			t.Errorf("%s %q came back as %+v", c.Op, c.Arg, cr)
		}
		if h := hiddenRows(s); !slices.Equal(h, want) {
			t.Errorf("%s %q hides %v, want %v", c.Op, c.Arg, h, want)
		}
		if len(got.Notes) > 0 {
			t.Errorf("%s: notes %q", c.Op, got.Notes)
		}
	}
}

// A values list with a condition goes out as the values both let
// through; criteria Excel has and 012 hasn't are left out with a note.
func TestXLSXFilterLimits(t *testing.T) {
	src := months(t)
	src.CreateFilter(sheet.NewRect(addr(t, "A1"), addr(t, "C5")))
	src.FilterColumn(0, sheet.Criteria{Hidden: []string{"Feb"}, Cond: sheet.Condition{Op: sheet.CondStartsWith, Arg: "a"}})
	_, got := exportImport(t, src)
	if cr := got.Sheet.Filter().Cols[0]; !slices.Equal(cr.Hidden, []string{"Feb", "Jan", "Mar"}) {
		t.Errorf("values and condition came back as %+v", cr)
	}

	dir := t.TempDir()
	x := excelize.NewFile()
	for i, row := range [][]any{{"Name", "Score"}, {"a", 1}, {"b", 7}, {"c", 3}} {
		x.SetSheetRow("Sheet1", "A"+itoa(i+1), &row)
	}
	x.AutoFilter("Sheet1", "A1:B4", []excelize.AutoFilterOptions{{Column: "B", Expression: "x > 1 and x < 5"}, {Column: "A", Expression: "x == a?"}})
	name := filepath.Join(dir, "limits.xlsx")
	if err := x.SaveAs(name); err != nil {
		t.Fatal(err)
	}
	res, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if f := res.Sheet.Filter(); f == nil || len(f.Cols) != 0 || f.Range != sheet.NewRect(addr(t, "A1"), addr(t, "B4")) {
		t.Errorf("filter %+v", f)
	}
	if n := strings.Join(res.Notes, "; "); n != `2 criteria of a filter 012 can't apply left out, e.g. column B on Sheet1 (two conditions)` {
		t.Errorf("notes %q", n)
	}
}
