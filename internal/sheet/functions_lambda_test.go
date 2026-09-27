package sheet

import "testing"

func TestLetAndLambda(t *testing.T) {
	s := fixture(t)
	if err := s.DefineName("x", rect("A1:A1")); err != nil {
		t.Fatal(err)
	}
	checkArrays(t, s, []arrCase{
		{"=LET(x, 5, x*2)", []string{"10"}},
		{"=LET(a, 2, b, a+1, a*b)", []string{"6"}},
		{"=LET(r, C1:C5, SUM(r))", []string{"15"}},
		{"=LET(s, SEQUENCE(3), s*s)", []string{"1", "4", "9"}},
		{"=x+1", []string{"11"}}, // the named range, outside LET
		{"=LET(x, 1, x) + x", []string{"11"}},
		{"=LAMBDA(v, v*3)(4)", []string{"12"}},
		{"=LAMBDA(a, b, a-b)(10, 4)", []string{"6"}},
		{"=LAMBDA(a, b, a-b)(10)", []string{"#N/A"}},
		{"=LET(double, LAMBDA(v, v*2), double(21))", []string{"42"}},
		{"=LAMBDA(v, v)", []string{"#VALUE!"}},
		{"=MAP(C1:C3, LAMBDA(v, v*10))", []string{"10", "20", "30"}},
		{"=MAP(C1:C2, G1:G2, LAMBDA(a, b, a+b))", []string{"2", "4"}},
		{"=MAP(C1:C3, LAMBDA(a, b, a))", []string{"#N/A"}},
		{"=REDUCE(0, C1:C5, LAMBDA(acc, v, acc+v))", []string{"15"}},
		{"=SCAN(0, C1:C4, LAMBDA(acc, v, acc+v))", []string{"1", "3", "6", "10"}},
		{"=BYROW(C1:E2, LAMBDA(row, SUM(row)))", []string{"2.5", "4.5"}},
		{"=BYCOL(C1:E2, LAMBDA(col, COUNT(col)))", []string{"2|0|2"}},
		{"=BYROW(C1:C2, LAMBDA(row, SEQUENCE(2)))", []string{"#VALUE!", "#VALUE!"}},
		{"=MAKEARRAY(2, 3, LAMBDA(r, c, r*c))", []string{"1|2|3", "2|4|6"}},
	})
}
