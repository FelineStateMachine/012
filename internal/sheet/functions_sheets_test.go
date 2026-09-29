package sheet

import "testing"

// Where Sheets and Excel differ, 012 follows Sheets (the Sheets corpus in
// internal/fileio checks these against Sheets itself).
func TestSheetsSemantics(t *testing.T) {
	s := fixture(t)
	for a, in := range map[string]string{"J1": "'3", "J2": "=1/0", "J3": "FALSE", "J4": "=NA()"} {
		if err := s.Set(at(a), in); err != nil {
			t.Fatal(err)
		}
	}
	checkFormulas(t, s, []fnCase{
		// Text.
		{`=UPPER("straße")`, txt("STRASSE")},
		{`=UPPER("ǆ")`, txt("Ǆ")},
		{`=TEXT(TRUE, "0")`, txt("TRUE")},
		{`=TEXT(FALSE, "0.00")`, txt("FALSE")},
		{`=TEXT(0.75, "# ?/?")`, txt("3/4")},
		{`=TEXT(2.5, "# ?/?")`, txt("2 1/2")},
		// Numbers compare at the 15 digits a cell shows.
		{"=0.1+0.2=0.3", boolean(true)},
		{"=(1-0.9)=0.1", boolean(true)},
		{"=SUM(0.1, 0.2, 0.3)=0.6", boolean(true)},
		{"=0.1+0.2<0.3", boolean(false)},
		{"=0.1+0.2>=0.3", boolean(true)},
		{"=0.1+0.2-0.3=0", boolean(false)},
		{"=1.00000000000001=1", boolean(false)},
		{`=COUNTIF(E1:E3, 0.1+0.2+1.2)`, num(1)},
		{"=123456789012345678", num(123456789012345000)},
		{"=123456789012345678=123456789012345000", boolean(true)},
		// ROUND is half away from zero on the shortest decimal, as in
		// Sheets and Excel; exact inputs stay exact.
		{"=ROUND(2.675, 2)=2.68", boolean(true)},
		{"=ROUND(-1.005, 2)=-1.01", boolean(true)},
		{"=ROUND(1.5, 14)-1.5=0", boolean(true)},
		{"=ROUND(0.1+0.2, 15)-0.3=0", boolean(true)},
		{"=ROUND(-2.5)", num(-3)},
		// Errors and conditions.
		{"=IFERROR(NA())", Value{}},
		{`=ISBLANK(IFERROR(NA()))`, boolean(true)},
		{`=IF("TRUE", 1, 2)`, num(1)},
		{`=IF("false", 1, 2)`, num(2)},
		{`=IF("", 1, 2)`, num(2)},
		{`=IF("abc", 1, 2)`, ErrValue},
		{`=NOT("")`, boolean(true)},
		{`=NOT("TRUE")`, boolean(false)},
		{`=AND("TRUE", 1)`, boolean(true)},
		{"=COUNT(J1:J4)", num(0)},
		{"=COUNTA(J1:J4)", num(4)},
		{"=COUNT(1/0, 2)", num(1)},
		{"=SUM(J1:J4)", ErrDiv0},
		{"=COUNTIF(J1:J4, 3)", num(1)},
		{`=COUNTIF(J1:J4, "<>3")`, num(3)},
		{`=COUNTIF(J1:J4, ">2")`, num(0)},
		// Positions out of range, and whole rows and columns.
		{`=CHOOSE(0, "a")`, ErrNum},
		{"=INDEX(A1:A3, 5)", ErrNum},
		{"=ROWS(INDEX(A1:C3, 0, 1))", num(3)},
		{"=COLUMNS(INDEX(A1:C3, 2, 0))", num(3)},
		{"=SUM(INDEX(C1:E3, 0, 1))", num(6)},
		{"=ROWS(SEQUENCE(0))", ErrNum},
		{"=ROWS(1/0)", ErrDiv0},
		// Arrays and names.
		{"=SUMPRODUCT(C1:C3>1)", num(2)},
		{"=SUMPRODUCT(C1:C3>1, C1:C3)", num(5)},
		{"=LET(x, 1/0, 5)", num(5)},
		{"=LET(x, 1/0, x+1)", ErrDiv0},
	})
}
