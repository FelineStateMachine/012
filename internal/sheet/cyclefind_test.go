package sheet

import (
	"maps"
	"strings"
	"testing"
)

// Cycles evaluation doesn't walk are cycles all the same, #REF! in every
// cell, typed in any order or opened (cyclefind.go).
func TestCyclesFoundAsWritten(t *testing.T) {
	for _, tc := range []cycleCase{
		{"an IF branch not taken", map[string]string{"A1": "=IF(FALSE,B1,1)", "B1": "=A1+1"}, []string{"A1", "B1"}, nil},
		{"a range read cut short by an error", map[string]string{"A1": "=SUM(B1:B3)", "B1": "=1/0", "B3": "=A1"}, []string{"A1", "B3"}, map[string]string{"B1": "#DIV/0!"}},
		{"INDEX reading one cell", map[string]string{"A1": "=INDEX(A2:A5,1)", "A2": "7", "A4": "=A1"}, []string{"A1", "A4"}, nil},
		{"itself, behind a branch", map[string]string{"C3": "=IF(TRUE,1,C3)", "D3": "=IFERROR(C3,5)"}, []string{"C3"}, map[string]string{"D3": "5"}},
		{"another sheet", map[string]string{"A1": "=IF(FALSE,Data!A1,1)", "Data!A1": "=Sheet1!A1"}, []string{"A1", "Data!A1"}, nil},
		{"a name", map[string]string{"A1": "=IF(FALSE,SUM(Near),1)", "B2": "=A1"}, []string{"A1", "B2"}, nil},
	} {
		orders := [][]string{nil, nil}
		for a := range tc.cells {
			orders[0] = append(orders[0], a)
			orders[1] = append([]string{a}, orders[1]...)
		}
		for _, order := range orders {
			tc.check(t, order)
		}
	}
}

// cycleCase is cells to type, those of the cycle they make, and what
// others show.
type cycleCase struct {
	name   string
	cells  map[string]string
	cycle  []string
	others map[string]string
}

// check types the cells in order, on a workbook with a sheet Data and a
// name Near for A1:B2, and checks what they show, and show once opened.
func (tc cycleCase) check(t *testing.T, order []string) {
	t.Helper()
	s := New()
	w := s.Book()
	w.AddSheet("Data", 1)
	w.DefineName("Near", s, rect("A1:B2"))
	for _, a := range order {
		setIn(t, w, a, tc.cells[a])
	}
	reopened := roundTrip(t, s).Book()
	if !reopened.Circular {
		t.Errorf("%s: not circular", tc.name)
	}
	want := map[string]string{}
	maps.Copy(want, tc.others)
	for _, a := range tc.cycle {
		want[a] = "#REF!"
	}
	for _, sh := range []*Workbook{w, reopened} {
		for a, v := range want {
			if got := show(t, sh, qualified(a)); got != v {
				t.Errorf("%s, typed %v: %s = %s, want %s", tc.name, order, a, got, v)
			}
		}
	}
}

// setIn types in into the cell a names, on Sheet1 unless it names a
// sheet.
func setIn(t *testing.T, w *Workbook, a, in string) {
	t.Helper()
	s := w.Sheet(0)
	if sheet, cell, ok := strings.Cut(a, "!"); ok {
		s, a = w.Lookup(sheet), cell
	}
	if err := s.Set(at(a), in); err != nil {
		t.Fatal(err)
	}
}

func qualified(a string) string {
	if strings.Contains(a, "!") {
		return a
	}
	return "Sheet1!" + a
}

// Formulas reading each other without a cycle aren't taken for one:
// a table's formulas reading their own row, running totals over a
// column of formulas, and a total above the column it adds up.
func TestNoCycleWithout(t *testing.T) {
	s := salesSheet(t)
	s.Set(at("D1"), "Total")
	w := s.Book()
	w.ResizeTable("Sales", rect("A1:D4"))
	for row := 2; row <= 4; row++ {
		s.Set(Addr{Col: 3, Row: row - 1}, "=Sales[@Units]*Sales[@Amount]")
	}
	for row := 1; row <= 30; row++ {
		s.Set(Addr{Col: 6, Row: row}, "=F"+itoa(row+1)+"*2")
		s.Set(Addr{Col: 7, Row: row}, "=SUM(G$2:G"+itoa(row+1)+")")
	}
	s.Set(at("G1"), "=SUM(G2:G31)")
	for _, sh := range []*Workbook{w, roundTrip(t, s).Book()} {
		if sh.Circular {
			t.Error("circular")
		}
		if got := show(t, sh, "Sheet1!D4"); got != "150" {
			t.Errorf("D4 = %s", got)
		}
		if got := show(t, sh, "Sheet1!H31"); got != "0" {
			t.Errorf("H31 = %s", got)
		}
	}
}
