package sheet

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/FelineStateMachine/012/internal/formula"
)

// A filter over whole columns tests the rows with data and treats the
// blank rows past them as one: hiding them, counting them and stepping
// over them cost nothing per row.
func TestFilterWholeColumns(t *testing.T) {
	s := New()
	for i, in := range []string{"h", "1", "", "3", "x"} {
		if in != "" {
			s.Set(Addr{Row: i}, in)
		}
	}
	s.CreateFilter(Rect{To: Addr{Col: 1, Row: MaxRows - 1}})
	s.FilterColumn(0, Criteria{Cond: Condition{CondNotEmpty, ""}})
	if n := s.HiddenRows(); n != MaxRows-4 {
		t.Errorf("HiddenRows = %d, want %d", n, MaxRows-4)
	}
	if r, ok := s.NextShownRow(3, 1); !ok || r != 4 {
		t.Errorf("NextShownRow(3, 1) = %d, %v", r, ok)
	}
	if r, ok := s.NextShownRow(4, 1); ok {
		t.Errorf("NextShownRow(4, 1) = %d, want none", r)
	}
	if r, ok := s.NextShownRow(MaxRows, -1); !ok || r != 4 {
		t.Errorf("NextShownRow from the bottom = %d, %v", r, ok)
	}
	if got := s.Edge(at("A5"), 0, 1); got != at("A5") {
		t.Errorf("Edge down from the last shown row = %v", got)
	}
	if got := s.Edge(at("B1"), 0, 1); got != at("B5") {
		t.Errorf("Edge down column B = %v, want B5", got)
	}
	vals := s.FilterValues(1)
	if len(vals) != 1 || vals[0].Text != "" || vals[0].Count != 3 {
		t.Errorf("FilterValues(B) = %+v", vals)
	}
	s.FilterColumn(0, Criteria{})
	if vals := s.FilterValues(0); vals[len(vals)-1].Text != "" || vals[len(vals)-1].Count != MaxRows-4 {
		t.Errorf("blanks in FilterValues(A) = %+v", vals[len(vals)-1])
	}
}

// Sorting moves only the rows with cells, and lands every row where a
// stable sort of all the range's rows (blank rows included) would.
func TestSortMatchesDenseSort(t *testing.T) {
	for seed := range 20 {
		rng := rand.New(rand.NewPCG(uint64(seed), 3))
		s := New()
		fill := []string{"", "", "", "3", "1", "b", "A", "TRUE", "=1/0", "=A1+1", `=""`}
		for row := range 30 {
			if rng.IntN(4) == 0 {
				continue // a blank row
			}
			for col := range 3 {
				if in := fill[rng.IntN(len(fill))]; in != "" {
					s.Set(Addr{Col: col, Row: row}, in)
				}
			}
		}
		r := Rect{From: Addr{Row: 2}, To: Addr{Col: 2, Row: MaxRows - 1}}
		keys := []SortKey{{Col: rng.IntN(3), Desc: seed%2 == 1}, {Col: rng.IntN(3)}}
		want := denseSort(s, r, keys)
		s.SortRange(r, keys)
		if got := inputs(s); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("seed %d, keys %v:\n got %v\nwant %v", seed, keys, got, want)
		}
	}
}

// denseSort is what sorting did before it went by rows with data: a
// stable sort of every row of r (clipped to the used range), returning
// the inputs it leaves.
func denseSort(s *Sheet, r Rect, keys []SortKey) map[string]string {
	used, _ := s.UsedRange()
	r.To.Row, r.To.Col = min(r.To.Row, used.To.Row), min(r.To.Col, used.To.Col)
	order := make([]int, r.To.Row-r.From.Row+1)
	for i := range order {
		order[i] = r.From.Row + i
	}
	slices.SortStableFunc(order, func(i, j int) int { return s.compareRows(keys, i, j) })
	out := map[string]string{}
	for a, c := range s.cells.all() {
		if !r.Contains(a) {
			out[a.String()] = c.Input
		}
	}
	for k, src := range order {
		for col := r.From.Col; col <= r.To.Col; col++ {
			if c := s.cells.get(Addr{Col: col, Row: src}); c != nil {
				to := Addr{Col: col, Row: r.From.Row + k}
				out[to.String()] = c.rewritten(formula.Shift(0, to.Row-src)).Input
			}
		}
	}
	return out
}
