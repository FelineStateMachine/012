package oracle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
)

// tableFormulas read the table Sales (A1:C5: Region, Units, Amount) by
// structured references, each in the cell it is written in. excel is
// what Excel gives where excelize can't compute a structured reference
// (it reads tables but its calculation engine doesn't resolve them); 012
// is checked against it there, and against excelize elsewhere.
var tableFormulas = []struct{ cell, formula, excel string }{
	{"E1", "SUM(Sales[Amount])", "100"},
	{"E2", "SUM(Sales[[Units]:[Amount]])", "113"},
	{"E3", "ROWS(Sales[#All])", "5"},
	{"E4", "ROWS(Sales)", "4"},
	{"E5", "Sales[[#Headers],[Units]]", "Units"},
	{"E6", `COUNTIF(Sales[Region],"North")`, "2"},
	{"D3", "Sales[[#This Row],[Amount]]*2", "40"},
	{"E7", "SUM(C2:C5)", ""},
}

// TestTablesFromExcel builds a workbook with a table in excelize, as
// Excel would save it, and reads it with 012's XLSX reader: the table
// comes in, and formulas reading it compute what Excel computes.
func TestTablesFromExcel(t *testing.T) {
	x := excelize.NewFile()
	rows := [][]any{{"Region", "Units", "Amount"}, {"North", 2, 10}, {"South", 3, 20}, {"North", 1, 30}, {"East", 7, 40}}
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := x.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.AddTable("Sheet1", &excelize.Table{Range: "A1:C5", Name: "Sales"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range tableFormulas {
		if err := x.SetCellFormula("Sheet1", tc.cell, tc.formula); err != nil {
			t.Fatal(err)
		}
	}
	name := filepath.Join(t.TempDir(), "tables.xlsx")
	if err := x.SaveAs(name); err != nil {
		t.Fatal(err)
	}
	got, err := fileio.Import(context.Background(), name, fileio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet
	if tb, ok := s.TableAt(addr("B2")); !ok || tb.Name != "Sales" || tb.Range.String() != "A1:C5" {
		t.Fatalf("table %+v", tb)
	}
	for _, tc := range tableFormulas {
		ours := s.Value(addr(tc.cell))
		if tc.excel != "" {
			t.Logf("skip %s: excelize doesn't compute structured references", tc.formula)
			if !agree(ours, tc.excel) {
				t.Errorf("%s: 012 %s, Excel %s", tc.formula, show(ours), tc.excel)
			}
			continue
		}
		theirs, _ := x.CalcCellValue("Sheet1", tc.cell, excelize.Options{RawCellValue: true})
		if !agree(ours, theirs) {
			t.Errorf("%s: 012 %s, excelize %q", tc.formula, show(ours), theirs)
		}
	}
	x.Close()
}
