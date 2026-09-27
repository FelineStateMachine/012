package sheet

import (
	"fmt"
	"math"
	"testing"
)

// bruteStats sums r cell by cell, as RangeStats did before the index.
func bruteStats(s *Sheet, r Rect) Stats {
	var st Stats
	for row := r.From.Row; row <= r.To.Row; row++ {
		for col := r.From.Col; col <= r.To.Col; col++ {
			if c := s.Cell(Addr{Col: col, Row: row}); !c.Blank() {
				st.add(c.Value)
			}
		}
	}
	return st
}

func sameStats(a, b Stats) bool {
	return a.Count == b.Count && a.Nums == b.Nums && math.Abs(a.Sum-b.Sum) <= 1e-9*math.Max(1, math.Abs(b.Sum))
}

// TestRangeStatsIndex checks the block index against summing cell by
// cell, over selections that start and end inside blocks and on their
// edges, as cells are set, recalculated, deleted and undone.
func TestRangeStatsIndex(t *testing.T) {
	s := New()
	for row := range 2000 {
		for col := range 10 {
			s.Load(Addr{Col: col, Row: row}, fmt.Sprint(row*10+col), Format{}, Style{})
		}
	}
	s.Load(at("L1"), "=SUM(A1:A2000)", Format{}, Style{})
	s.Load(at("K5"), "text", Format{}, Style{})
	s.RecalcAll()
	rects := []Rect{
		NewRect(at("A1"), at("L8192")),
		NewRect(at("A1"), at("IV8192")),
		NewRect(at("B2"), at("K1999")),
		NewRect(at("A64"), at("C129")),  // rows 63..128: one whole block
		NewRect(at("A65"), at("C128")),  // rows 64..127: exactly one block
		NewRect(at("C10"), at("F100")),  // no whole block
		NewRect(at("A1"), at("A8192")),  // one column
		NewRect(at("A100"), at("J120")), // small: cell by cell
	}
	check := func(when string) {
		t.Helper()
		for _, r := range rects {
			if got, want := s.RangeStats(r), bruteStats(s, r); !sameStats(got, want) {
				t.Errorf("%s: RangeStats(%s) = %+v, want %+v", when, r, got, want)
			}
		}
	}
	check("built")
	if !s.cells.statsUsed {
		t.Fatal("the statistics index wasn't built")
	}
	s.Set(at("A1"), "1000000") // recalculates L1 as well
	check("after an edit")
	s.Set(at("C70"), "")
	check("after clearing a cell")
	s.Set(at("Z8000"), "7")
	check("after a cell outside the data")
	s.Undo()
	s.Undo()
	check("after undo")
	s.Redo()
	check("after redo")
}

func TestRangeStatsSparse(t *testing.T) {
	s := New()
	s.Set(at("A1"), "2")
	s.Set(at("IV8192"), "3")
	if got := s.RangeStats(NewRect(at("A1"), at("IV8192"))); got != (Stats{Sum: 5, Count: 2, Nums: 2}) {
		t.Errorf("whole sparse sheet = %+v", got)
	}
	if s.cells.statsUsed {
		t.Error("a sparse sheet built the statistics index")
	}
}

// On the full grid the index takes memory only where there is data: a
// whole-sheet selection over cells scattered down a million rows and
// across 16,384 columns sums as cell by cell would, after edits too.
func TestRangeStatsFullGrid(t *testing.T) {
	s := New()
	for i := range 20000 {
		s.Load(Addr{Col: (i * 7919) % MaxCols, Row: (i * 104729) % MaxRows}, fmt.Sprint(i%97), Format{}, Style{})
	}
	s.RecalcAll()
	brute := func(r Rect) Stats {
		var st Stats
		for a, c := range s.cells.all() {
			if r.Contains(a) && !c.Blank() {
				st.add(c.Value)
			}
		}
		return st
	}
	all := Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}}
	for _, r := range []Rect{all, {From: Addr{Col: 3, Row: 70}, To: Addr{Col: 9000, Row: 900000}}} {
		if got, want := s.RangeStats(r), brute(r); !sameStats(got, want) {
			t.Errorf("RangeStats(%s) = %+v, want %+v", r, got, want)
		}
	}
	s.Set(Addr{Col: 5, Row: 5}, "1000")
	s.Set(Addr{Col: (7 * 7919) % MaxCols, Row: (7 * 104729) % MaxRows}, "")
	if got, want := s.RangeStats(all), brute(all); !sameStats(got, want) {
		t.Errorf("after edits RangeStats = %+v, want %+v", got, want)
	}
	if !s.cells.statsUsed {
		t.Error("the index wasn't used")
	}
}
