package oracle

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// arrayFormulas compute over arrays; each is compared by the value its
// cell shows (an array's first). Those excelize can't compute have the
// value Sheets gives and the reason, and 012 is checked against that.
var arrayFormulas = []struct{ formula, sheets, why string }{
	{`=UNIQUE(G1:G4)`, "", ""},
	{`=TRANSPOSE(C1:D2)`, "", ""},
	{`=INDEX(TRANSPOSE(C1:D2), 2, 1)`, "", ""},
	{`=TEXTJOIN(",", TRUE, UNIQUE(D1:D5))`, "", ""},
	{`=SUM({1,2,3})`, "", ""},
	{`=SUMPRODUCT((B1:B5="north")*C1:C5)`, "9", "excelize reads a range in an expression as its first cell"},
	{`=SUMPRODUCT(C1:C3*G1:G3)`, "11", "excelize reads a range in an expression as its first cell"},
	{`=SUMPRODUCT(--(C1:C7>3))`, "4", "excelize doesn't negate a comparison of ranges"},
	{`=ROWS(UNIQUE(D1:D5))`, "4", "excelize's ROWS takes only a reference"},
	{`=INDEX({10,20,30}, 2)`, "20", "excelize's INDEX takes only a reference"},
	{`=SUM(C1:C3*2)`, "12", "excelize doesn't compute an expression over ranges inside SUM"},
	{`=MAX(LEN(D1:D5))`, "6", "excelize doesn't apply LEN to each cell of a range inside MAX"},
	{`=SEQUENCE(3, 2, 10)`, "10", "excelize has no SEQUENCE"},
	{`=FILTER(D1:D5, C1:C5>3)`, "date", "excelize has no FILTER"},
	{`=SORT(I1:I3)`, "10", "excelize has no SORT"},
	{`=SORTN(C1:C7, 1, 0, 1, FALSE)`, "7", "excelize has no SORTN, which only Sheets has"},
	{`=FLATTEN(C1:D2)`, "1", "excelize has no FLATTEN, which only Sheets has"},
	{`=CHOOSECOLS(C1:E2, 2)`, "Apple", "excelize has no CHOOSECOLS"},
	{`=CHOOSEROWS(C1:D3, -1)`, "3", "excelize has no CHOOSEROWS"},
	{`=ARRAYFORMULA(SUM(C1:C3*G1:G3))`, "11", "excelize has no ARRAYFORMULA, which only Sheets has"},
	{`=LET(x, C3, x*x)`, "9", "excelize has no LET"},
	{`=LAMBDA(v, v+1)(C2)`, "3", "excelize has no LAMBDA"},
	{`=REDUCE(0, C1:C5, LAMBDA(a, v, a+v))`, "15", "excelize has no REDUCE"},
	{`=MAP(C1:C3, LAMBDA(v, v*v))`, "1", "excelize has no MAP"},
	{`=REGEXEXTRACT("order 42", "[0-9]+")`, "42", "excelize has no REGEXEXTRACT"},
	{`=REGEXREPLACE("a-b", "-", "+")`, "a+b", "excelize has no REGEXREPLACE"},
	{`=REGEXMATCH("Cherry", "^Ch")`, "TRUE", "excelize has no REGEXMATCH, which only Sheets has"},
	{`=SPLIT("x,y", ",")`, "x", "excelize has no SPLIT, which only Sheets has"},
}

func TestArraysAgainstExcelize(t *testing.T) {
	s, x := build(t)
	const cell = "Z1"
	for _, tc := range arrayFormulas {
		if err := s.Set(addr(cell), tc.formula); err != nil {
			t.Errorf("012 rejects %s: %v", tc.formula, err)
			continue
		}
		ours := s.Value(addr(cell))
		s.Set(addr(cell), "")
		if tc.why != "" {
			t.Logf("skip %s: %s", tc.formula, tc.why)
			if !agree(ours, tc.sheets) {
				t.Errorf("%s: 012 %s, Sheets %s", tc.formula, show(ours), tc.sheets)
			}
			continue
		}
		if err := x.SetCellFormula("Sheet1", cell, toExcel(tc.formula)); err != nil {
			t.Fatal(err)
		}
		theirs, err := x.CalcCellValue("Sheet1", cell, excelize.Options{RawCellValue: true})
		if err != nil && !strings.HasPrefix(theirs, "#") {
			theirs = err.Error()
		}
		if !agree(ours, theirs) {
			t.Errorf("%s: 012 %s, excelize %q", tc.formula, show(ours), theirs)
		}
	}
}
