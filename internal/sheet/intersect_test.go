package sheet

import (
	"strings"
	"testing"
)

// A range where one value is wanted reads as the cell in the formula's
// row (a column) or column (a row), as in Sheets and Excel; functions
// that take ranges still read all of them.
func TestImplicitIntersection(t *testing.T) {
	for _, decimal := range []bool{false, true} {
		s := sheetOf(t, map[string]string{
			"A1": "Rent", "B1": "Amount",
			"B2": "1400", "B3": "1450", "B4": "1500",
			"B6": "10", "C6": "20", "D6": "30",
			"F2": "=B2:B4*2", "F3": "=Rent*2", "F4": "=Rent*2", "F5": "=Rent*2",
			"G3": "=B2:B4", "G4": "=ABS(-Rent)", "G5": "=B:B+1",
			"H3": "=SUM(Rent)", "H4": "=SUMPRODUCT(Rent)", "H2": "=MATCH(1450,Rent,0)",
			"H5": "=SUM(Rent*2)", "H6": "=SUMPRODUCT(Rent*2)",
			"C7": "=B6:D6+1", "E7": "=B6:D6", "A8": "=B2:C4",
			"J3": "=IF(TRUE,Rent)", "K3": "=Rent&\"!\"", "L3": "=COUNTIF(Rent,\">1420\")",
		})
		s.SetDecimal(decimal)
		if err := s.DefineName("Rent", rect("B2:B4")); err != nil {
			t.Fatal(err)
		}
		for a, want := range map[string]string{
			"F2": "2800", "F3": "2900", "F4": "3000", "F5": "#VALUE!",
			"G3": "1450", "G4": "1500", "G5": "1",
			"H3": "4350", "H4": "4350", "H2": "2",
			"H5": "8700", "H6": "8700", // array arguments: computed over the whole range
			"C7": "21", "E7": "#VALUE!", "A8": "#VALUE!",
			"J3": "1450", "K3": "1450!", "L3": "2",
		} {
			if got := s.Value(at(a)).String(); got != want {
				t.Errorf("decimal %v: %s = %s, want %s", decimal, a, got, want)
			}
		}
		s.Set(at("B3"), "1475") // the intersected cell is a dependency
		if got := s.Value(at("F3")).String(); got != "2950" {
			t.Errorf("decimal %v: F3 after B3 changed = %s", decimal, got)
		}
	}
}

// Ranges on other sheets intersect with the formula's own row or column.
func TestImplicitIntersectionOtherSheet(t *testing.T) {
	s := New()
	w := s.Book()
	data, err := w.AddSheet("Q3 plan", 1)
	if err != nil {
		t.Fatal(err)
	}
	for a, in := range map[string]string{"C2": "5", "C3": "6", "C4": "7", "B9": "1", "C9": "2", "D9": "3"} {
		if err := data.Set(at(a), in); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.DefineName("Plan", data, rect("C2:C4")); err != nil {
		t.Fatal(err)
	}
	for a, in := range map[string]string{
		"A3": "='Q3 plan'!C2:C4*10", "A4": "=Plan+1", "A5": "=Plan",
		"C1": "='Q3 plan'!B9:D9", "E1": "='Q3 plan'!B9:D9",
	} {
		if err := s.Set(at(a), in); err != nil {
			t.Fatal(err)
		}
	}
	for a, want := range map[string]string{"A3": "60", "A4": "8", "A5": "#VALUE!", "C1": "2", "E1": "#VALUE!"} {
		if got := s.Value(at(a)).String(); got != want {
			t.Errorf("%s = %s, want %s", a, got, want)
		}
	}
	data.Set(at("C3"), "9")
	if got := s.Value(at("A3")).String(); got != "90" {
		t.Errorf("A3 after 'Q3 plan'!C3 changed = %s", got)
	}
}

// An error read through an intersected range is traced to its cell.
func TestImplicitIntersectionExplain(t *testing.T) {
	s := sheetOf(t, map[string]string{"B2": "1", "B3": "=1/0", "B4": "3", "D3": "=B2:B4*2"})
	if got := s.ExplainError(at("D3")); !strings.HasPrefix(got, "From B3") {
		t.Errorf("D3 explained as %q", got)
	}
}
