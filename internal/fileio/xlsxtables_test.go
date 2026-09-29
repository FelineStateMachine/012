package fileio

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Tables go out as Excel's tables, with their filters and style, and
// formulas reading them in Excel's syntax; excelize and 012 read them
// back.
func TestXLSXTables(t *testing.T) {
	src := build(t, map[string]string{
		"A1": "Region", "B1": "2024", "C1": "Amount",
		"A2": "North", "B2": "2", "C2": "10",
		"A3": "South", "B3": "3", "C3": "20",
		"E1": "=SUM(Sales[Amount])", "D2": "=Sales[@Amount]*2",
	})
	if err := src.CreateTable("Sales", rect(t, "A1:C3")); err != nil {
		t.Fatal(err)
	}
	src.Book().SetTableStyle("Sales", false, true)
	src.CreateFilter(rect(t, "A1:C3"))
	src.FilterColumn(0, sheet.Criteria{Hidden: []string{"South"}})
	name := filepath.Join(t.TempDir(), "tables.xlsx")
	if _, err := Export(context.Background(), name, XLSX, SnapBook(src), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := x.GetTables("Sheet1")
	if err != nil || len(ts) != 1 || ts[0].Name != "Sales" || ts[0].Range != "A1:C3" || ts[0].StyleName != "TableStyleMedium2" {
		t.Errorf("excelize tables %+v, %v", ts, err)
	}
	if f, _ := x.GetCellFormula("Sheet1", "D2"); f != "Sales[[#This Row],[Amount]]*2" {
		t.Errorf("formula %q", f)
	}
	if v, _ := x.GetCellValue("Sheet1", "B1"); v != "2024" {
		t.Errorf("header %q", v)
	}
	x.Close()
	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet
	tb, ok := s.TableAt(addr(t, "B2"))
	if !ok || tb.Name != "Sales" || tb.Range != rect(t, "A1:C3") || tb.Banded || !tb.Header || strings.Join(tb.Cols, ",") != "Region,2024,Amount" {
		t.Fatalf("table read back %+v", tb)
	}
	if r, ok := s.FilterRange(); !ok || r != rect(t, "A1:C3") || !s.RowHidden(2) {
		t.Errorf("filter %v %v, row 3 hidden %v", r, ok, s.RowHidden(2))
	}
	for a, want := range map[string]string{"E1": "30", "D2": "20"} {
		if v := s.Value(addr(t, a)).String(); v != want {
			t.Errorf("%s = %s, want %s", a, v, want)
		}
	}
}

// A formula reading a table the file doesn't hold, a notebook output's,
// is written as its value, as Excel would refuse it.
func TestXLSXUnknownTable(t *testing.T) {
	src := build(t, map[string]string{"A1": "=SUM(app[ms])+1"})
	name := filepath.Join(t.TempDir(), "outputs.xlsx")
	res, err := Export(context.Background(), name, XLSX, SnapBook(src), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(res.Notes, "; "), "saved as values") {
		t.Errorf("notes %q", res.Notes)
	}
	x, _ := excelize.OpenFile(name)
	defer x.Close()
	if f, _ := x.GetCellFormula("Sheet1", "A1"); f != "" {
		t.Errorf("formula kept: %q", f)
	}
}

// Tables Excel makes, a totals row and all, come in as tables.
func TestXLSXTablesFromExcelize(t *testing.T) {
	x := excelize.NewFile()
	for a, v := range map[string]any{"A1": "Item", "B1": "Cost", "A2": "Rent", "B2": 1450, "A3": "Food", "B3": 300, "A4": "Total", "B4": 1750} {
		x.SetCellValue("Sheet1", a, v)
	}
	x.SetCellFormula("Sheet1", "D1", "SUM(Costs[Cost])")
	stripes := true
	if err := x.AddTable("Sheet1", &excelize.Table{Range: "A1:B3", Name: "Costs", StyleName: "TableStyleLight9", ShowRowStripes: &stripes}); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "excelize.xlsx")
	if err := x.SaveAs(name); err != nil {
		t.Fatal(err)
	}
	x.Close()
	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet
	tb, ok := s.TableAt(addr(t, "A2"))
	if !ok || tb.Name != "Costs" || !tb.Banded || !tb.Header || strings.Join(tb.Cols, ",") != "Item,Cost" {
		t.Fatalf("table %+v", tb)
	}
	if v := s.Value(addr(t, "D1")).String(); v != "1750" {
		t.Errorf("D1 = %s", v)
	}
}

// A table's totals row stays cells below it.
func TestXLSXTableTotalsRow(t *testing.T) {
	b, err := readParts(t, tableParts(), defaultXLSXLimits)
	if err != nil {
		t.Fatal(err)
	}
	s := b.Sheet
	tb, ok := s.TableAt(addr(t, "A1"))
	if !ok || tb.Range != rect(t, "A1:B2") || !tb.Banded || !tb.Header {
		t.Fatalf("table %+v", tb)
	}
	if v := s.Value(addr(t, "C2")).String(); v != "10" {
		t.Errorf("C2 = %s", v)
	}
}

func rect(t *testing.T, s string) sheet.Rect {
	t.Helper()
	r, ok := sheet.ParseRange(s)
	if !ok {
		t.Fatalf("bad range %s", s)
	}
	return r
}
