package sheet

import (
	"fmt"
	"os"
	"runtime/debug"
	"testing"
)

// withEvalDepth lowers maxEvalDepth for a test.
func withEvalDepth(t *testing.T, n int) {
	t.Helper()
	old := maxEvalDepth
	maxEvalDepth = n
	t.Cleanup(func() { maxEvalDepth = old })
}

// chain fills column col, rows 1 to n, with =A(row-1)+1 below a 1, so
// the last is n.
func chain(s *Sheet, col, n int) {
	for row := range n {
		in := "1"
		if row > 0 {
			in = "=" + Addr{Col: col, Row: row - 1}.String() + "+1"
		}
		s.Load(Addr{Col: col, Row: row}, in, Format{}, Style{})
	}
}

// Evaluation past maxEvalDepth puts cells off and comes back to them:
// values come out as if evaluated in one go.
func TestDeepChains(t *testing.T) {
	withEvalDepth(t, 10)
	s := New()
	chain(s, 0, 1000)
	// Deep formulas in a chain: each cell nests 8 levels.
	for row := range 300 {
		in := "=1"
		if row > 0 {
			in = "=((((((((" + Addr{Col: 1, Row: row - 1}.String() + "+1))))))))"
		}
		s.Load(Addr{Col: 1, Row: row}, in, Format{}, Style{})
	}
	s.RecalcAll()
	if got := s.Value(Addr{Row: 999}); got != num(1000) {
		t.Errorf("A1000 = %+v", got)
	}
	if got := s.Value(Addr{Col: 1, Row: 299}); got != num(300) {
		t.Errorf("B300 = %+v", got)
	}
	if s.Book().Circular {
		t.Error("a chain was taken for a cycle")
	}
	// An edit at the top recalculates the chain the same way.
	s.Set(at("A1"), "5")
	if got := s.Value(Addr{Row: 999}); got != num(1004) {
		t.Errorf("after an edit, A1000 = %+v", got)
	}
}

// A cycle longer than maxEvalDepth is still a cycle.
func TestDeepCycle(t *testing.T) {
	withEvalDepth(t, 10)
	s := New()
	chain(s, 0, 500)
	s.Set(at("A1"), "=A500+1")
	if !s.Book().Circular {
		t.Error("a cycle of 500 cells wasn't found")
	}
	if got := s.Value(at("A250")); got.Kind != Error {
		t.Errorf("A250, in the cycle, = %+v", got)
	}
	s.Set(at("A1"), "1")
	if s.Book().Circular || s.Value(at("A500")) != num(500) {
		t.Errorf("after breaking the cycle: circular %v, A500 = %+v", s.Book().Circular, s.Value(at("A500")))
	}
}

// A chain through every cell of a full sheet fits in a bounded stack.
// Slow (2 M cells, 1 GB): set SHEET_FULL_CHAIN=1 to run it.
func TestFullSheetChain(t *testing.T) {
	if os.Getenv("SHEET_FULL_CHAIN") == "" {
		t.Skip("set SHEET_FULL_CHAIN=1 to evaluate a chain through all 2 M cells")
	}
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))
	s := New()
	for col := range MaxCols {
		for row := range MaxRows {
			in := "1"
			switch {
			case row > 0:
				in = "=" + Addr{Col: col, Row: row - 1}.String() + "+1"
			case col > 0:
				in = "=" + Addr{Col: col - 1, Row: MaxRows - 1}.String() + "+1"
			}
			s.Load(Addr{Col: col, Row: row}, in, Format{}, Style{})
		}
	}
	s.RecalcAll()
	if got, want := s.Value(Addr{Col: MaxCols - 1, Row: MaxRows - 1}), num(MaxCols*MaxRows); got != want {
		t.Errorf("IV8192 = %+v, want %v", got, fmt.Sprint(want.Num))
	}
}
