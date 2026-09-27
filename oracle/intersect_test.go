package oracle

import (
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// intersections are formulas that use a range where one value is wanted,
// each in the cell it is written in: the range reads as the cell in the
// formula's row (a column) or column (a row), or #VALUE! when there is
// none. Rent is the named range Sheet1!B2:B4; Other!B2:B4 and B9:D9 hold
// other numbers. excelize reads the first cell of any range instead, so
// it agrees only where that is the intersection, or where a function
// takes the range; the other cases are skipped against excelize and give
// what Excel and Sheets do (excel), which 012 is checked against. A
// range that is a cell's whole formula spills in Sheets and Excel 365,
// so a unary + makes it one value where the intersection is tested.
var intersections = []struct{ cell, formula, excel string }{
	// The intersection is the first cell, or a function takes the range.
	{"F2", "=B2:B4+1", ""}, {"F2", "=Rent*2", ""}, {"B8", "=B6:D6*3", ""},
	{"F2", "=Other!B2:B4*10", ""}, {"B1", "=Other!B9:D9", ""},
	{"H3", "=SUM(Rent)", ""}, {"H4", "=MATCH(1500,Rent,0)", ""}, {"H2", `=COUNTIF(Rent,">1420")`, ""},

	// excelize reads the first cell where Excel and Sheets intersect.
	{"F3", "=Rent*2", "2900"}, {"F4", "=Rent*2", "3000"}, {"F5", "=Rent*2", "#VALUE!"},
	{"G3", "=+B2:B4", "1450"}, {"G4", "=ABS(-Rent)", "1500"}, {"G3", `=Rent&"x"`, "1450x"},
	{"C8", "=B6:D6*3", "60"}, {"E8", "=B6:D6*3", "#VALUE!"}, {"A9", "=+B2:C4", "#VALUE!"},
	{"F3", "=Other!B2:B4*10", "20"}, {"C1", "=+Other!B9:D9", "5"}, {"F1", "=+Other!B2:B4", "#VALUE!"},
}

func TestImplicitIntersection(t *testing.T) {
	for _, decimal := range []bool{false, true} {
		s, x := buildIntersect(t)
		s.SetDecimal(decimal)
		for _, tc := range intersections {
			key := tc.cell + " " + tc.formula
			if err := s.Set(addr(tc.cell), tc.formula); err != nil {
				t.Fatalf("012 rejects %s: %v", key, err)
			}
			ours := s.Value(addr(tc.cell))
			s.Set(addr(tc.cell), "")
			if tc.excel != "" {
				t.Logf("skip %s: excelize reads the range's first cell; Excel and Sheets intersect", key)
				if !agree(ours, tc.excel) {
					t.Errorf("decimal %v: %s: 012 %s, Excel and Sheets %s", decimal, key, show(ours), tc.excel)
				}
				continue
			}
			if theirs := calcAt(t, x, tc.cell, tc.formula); !agree(ours, theirs) {
				t.Errorf("decimal %v: %s: 012 %s, excelize %q", decimal, key, show(ours), theirs)
			}
		}
	}
}

// buildIntersect sets up the cells and the name Rent in both engines.
func buildIntersect(t *testing.T) (*sheet.Sheet, *excelize.File) {
	t.Helper()
	s := sheet.New()
	o, err := s.Book().AddSheet("Other", 1)
	if err != nil {
		t.Fatal(err)
	}
	x := excelize.NewFile()
	if _, err := x.NewSheet("Other"); err != nil {
		t.Fatal(err)
	}
	put := func(on *sheet.Sheet, name string, cells map[string]string) {
		for a, in := range cells {
			if err := on.Set(addr(a), in); err != nil {
				t.Fatal(err)
			}
			if err := x.SetCellValue(name, a, on.Value(addr(a)).Num); err != nil {
				t.Fatal(err)
			}
		}
	}
	put(s, "Sheet1", map[string]string{
		"B2": "1400", "B3": "1450", "B4": "1500", "B6": "10", "C6": "20", "D6": "30", "C2": "7", "C3": "8", "C4": "9",
	})
	put(o, "Other", map[string]string{"B2": "1", "B3": "2", "B4": "3", "B9": "4", "C9": "5", "D9": "6"})
	if err := s.DefineName("Rent", sheet.Rect{From: addr("B2"), To: addr("B4")}); err != nil {
		t.Fatal(err)
	}
	if err := x.SetDefinedName(&excelize.DefinedName{Name: "Rent", RefersTo: "Sheet1!$B$2:$B$4"}); err != nil {
		t.Fatal(err)
	}
	return s, x
}

// calcAt is excelize's result for the formula f in cell on Sheet1, which
// is emptied again afterwards.
func calcAt(t *testing.T, x *excelize.File, cell, f string) string {
	t.Helper()
	if err := x.SetCellFormula("Sheet1", cell, toExcel(f)); err != nil {
		t.Fatal(err)
	}
	theirs, err := x.CalcCellValue("Sheet1", cell, excelize.Options{RawCellValue: true})
	if err != nil && (theirs == "" || theirs[0] != '#') {
		theirs = err.Error()
	}
	if err := x.SetCellValue("Sheet1", cell, nil); err != nil {
		t.Fatal(err)
	}
	return theirs
}
