package sheet

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// Formulas read ranges sparsely (only the cells that hold something),
// clip positional reads to the data and share running aggregates across a
// recalculation. Each formula here gives exactly what reading every
// address of its ranges gives, on sheets with blanks, text, errors and
// empty strings scattered through the data.
func TestRangeReadsMatchDense(t *testing.T) {
	formulas := []string{
		"=SUM(A1:A2000)", "=SUM(A1:E2000)", "=AVERAGE(B3:B1500)", "=COUNT(A:A)", "=COUNTA(A1:E3000)",
		"=MIN(C1:C2000)", "=MAX(A1:C2000)", "=PRODUCT(D1:D40)", "=SUM(A1:A5, 1.5, A6:A900)",
		`=SUMIF(A1:A2000, "")`, `=SUMIF(A1:A2000, ">5", C1:C2000)`, `=SUMIF(B1:B2000, "<>x", C1)`,
		`=COUNTIF(A1:A2000, "")`, `=COUNTIF(B1:B2000, "<>b")`, `=COUNTIF(A1:C2000, ">3")`,
		"=COUNTBLANK(A1:E2000)", `=COUNTIFS(A1:A2000, "", B1:B2000, "")`, `=SUMIFS(C1:C2000, A1:A2000, "<>")`,
		`=AVERAGEIF(A1:A2000, ">0")`, `=AVERAGEIFS(C1:C2000, B1:B2000, "")`,
		"=SUMPRODUCT(A1:A2000, C1:C2000)", "=SUMPRODUCT(A1:B2000, C1:D2000)",
		"=MATCH(7, A1:A2000, 0)", "=MATCH(Z99, A1:A2000, 0)", "=MATCH(Z99, A1:A2000)", "=MATCH(1E9, C1:C2000)",
		"=MATCH(Z99, A1:A2000, -1)", "=MATCH(-1, C1:C2000, -1)",
		"=VLOOKUP(Z99, A1:C2000, 3, FALSE)", "=VLOOKUP(5, A1:C2000, 2)", "=HLOOKUP(Z99, A1:E2000, 2, FALSE)",
		"=XLOOKUP(Z99, A1:A2000, C1:C2000)", "=XLOOKUP(Z99, A1:A2000, C1:C2000, , 0, -1)",
		"=XLOOKUP(3, A1:A2000, C1:C2000, , 1)", "=XLOOKUP(3, A1:A2000, C1:C2000, , -1, -1)",
		`=TEXTJOIN(",", FALSE, B1:B60)`, `=LEN(TEXTJOIN("-", FALSE, B1:C2000))`, `=TEXTJOIN(",", TRUE, B1:B2000)`,
		"=CONCATENATE(B1:B80)", "=MEDIAN(A1:A2000)", "=RANK(C5, C1:C2000)", "=ROWS(A:A)", "=COLUMNS(1:1)",
		"=INDEX(A:A, 70000)", "=MATCH(Z99, F1:F2000, 0)", "=MATCH(Z99, F1:F2000)", "=MATCH(Z99, F1:F2000, -1)",
		"=XLOOKUP(Z99, F1:F2000, C1:C2000)", "=XLOOKUP(Z99, F1:F2000, C1:C2000, , 0, -1)", "=XLOOKUP(Z99, F1:F2000, C1:C2000, , 1)",
		"=VLOOKUP(Z99, F1:G2000, 2, FALSE)", "=VLOOKUP(Z99, F1:G2000, 2)", `=COUNTIF(F1:F2000, "")`, "=COUNTBLANK(F1:F2000)",
		`=LEN(TEXTJOIN(",", FALSE, F1:F2000))`, `=SUMIF(F1:F2000, "", G1:G2000)`, `=SUMIF(F1:F2000, "<>1", A1:A2000)`, "=SUM(Nope!A1:A2000)", "=COUNTBLANK(Nope!A1:A20)", `=COUNTIF(Nope!A1:A20, "")`,
	}
	for seed := range 6 {
		rng := rand.New(rand.NewPCG(uint64(seed), 7))
		s := New()
		fill := []string{"", "", "", "1", "2.5", "-3", "7", "x", "b", "TRUE", `=""`, "=1/0", "'5"}
		if seed%2 == 1 {
			fill = fill[:10] // no errors: aggregates get past them
		}
		for row := range 300 {
			for col := range 5 {
				if in := fill[rng.IntN(len(fill))]; in != "" {
					s.Set(Addr{Col: col, Row: row * (1 + seed%3)}, in)
				}
			}
		}
		for row := range 60 {
			s.Set(Addr{Col: 5, Row: row}, fill[3+rng.IntN(len(fill)-3)])
			s.Set(Addr{Col: 6, Row: row + 1}, "4")
		}
		for i, f := range formulas {
			s.Set(Addr{Col: 7 + i/200, Row: i % 200}, f)
		}
		// Running totals, down a column and over itself.
		for row := range 120 {
			s.Set(Addr{Col: 11, Row: row}, fmt.Sprintf("=SUM($A$1:A%d)", row+1))
			s.Set(Addr{Col: 12, Row: row}, fmt.Sprintf("=SUM($M$1:M%d)+1", row))
			s.Set(Addr{Col: 13, Row: row}, fmt.Sprintf("=COUNTA($A$1:$C%d)+MAX($C$3:C%d)", row+20, row+20))
		}
		s.Set(Addr{Col: 12, Row: 0}, "1")
		s.RecalcAll()
		sparse := snapshotValues(s)
		denseReads = true
		s.RecalcAll()
		dense := snapshotValues(s)
		denseReads = false
		for a, v := range dense {
			if sparse[a] != v {
				t.Errorf("seed %d: %s %q = %+v, reading every cell gives %+v", seed, a, s.Cell(a).Input, sparse[a], v)
			}
		}
	}
}

func snapshotValues(s *Sheet) map[Addr]Value {
	out := map[Addr]Value{}
	for a, c := range s.cells.all() {
		out[a] = c.Value
	}
	return out
}

// A range whose running aggregate is being read by the formulas it
// contains still gives each formula its own value.
func TestRunningAggregateReentry(t *testing.T) {
	s := New()
	s.Set(at("A1"), "1")
	for row := 2; row <= 40; row++ {
		s.Set(Addr{Col: 0, Row: row - 1}, fmt.Sprintf("=SUM($A$1:A%d)", row-1))
	}
	// A(n) = 2^(n-2) for n >= 2.
	if got := s.Value(at("A40")).Num; got != 1<<38 {
		t.Errorf("A40 = %v, want %v", got, 1<<38)
	}
	s.Set(at("A1"), "2")
	if got := s.Value(at("A40")).Num; got != 1<<39 {
		t.Errorf("after the edit A40 = %v, want %v", got, 1<<39)
	}
}
