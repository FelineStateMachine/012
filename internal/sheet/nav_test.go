package sheet

import "testing"

func TestEdge(t *testing.T) {
	s := New()
	for _, a := range []string{"A1", "A2", "A3", "A6", "A7"} {
		s.Set(at(a), "1")
	}
	tests := []struct {
		from   string
		dc, dr int
		want   string
	}{
		{"A1", 0, 1, "A3"},       // inside a block: to its end
		{"A3", 0, 1, "A6"},       // end of a block: to the next block
		{"A4", 0, 1, "A6"},       // from a blank: to the next filled cell
		{"A7", 0, 1, "A1048576"}, // nothing ahead: sheet edge
		{"A6", 0, -1, "A3"},
		{"A1", 0, -1, "A1"}, // already at the edge
		{"A1", 1, 0, "XFD1"},
		{"C1", -1, 0, "A1"},
	}
	for _, tt := range tests {
		if got := s.Edge(at(tt.from), tt.dc, tt.dr); got != at(tt.want) {
			t.Errorf("Edge(%s, %d, %d) = %s, want %s", tt.from, tt.dc, tt.dr, got, tt.want)
		}
	}
}

func TestUsedRangeAndStats(t *testing.T) {
	s := New()
	if _, ok := s.UsedRange(); ok {
		t.Error("empty sheet has a used range")
	}
	s.Set(at("B2"), "10")
	s.Set(at("B3"), "32")
	s.Set(at("D1"), "label")
	if r, _ := s.UsedRange(); r.String() != "A1:D3" {
		t.Errorf("UsedRange = %s", r)
	}
	want := Stats{Sum: 42, Count: 3, Nums: 2}
	if got := s.RangeStats(NewRect(at("A1"), at("D3"))); got != want {
		t.Errorf("RangeStats small = %+v", got)
	}
	// Whole columns take the sparse path.
	if got := s.RangeStats(NewRect(at("A1"), at("D8192"))); got != want {
		t.Errorf("RangeStats large = %+v", got)
	}
	// The cached result follows edits, recalculation and undo.
	r := NewRect(at("A1"), at("D3"))
	s.Set(at("C1"), "=B2*2")
	if got := s.RangeStats(r); got != (Stats{Sum: 62, Count: 4, Nums: 3}) {
		t.Errorf("after an edit = %+v", got)
	}
	s.Set(at("B2"), "1")
	if got := s.RangeStats(r); got != (Stats{Sum: 35, Count: 4, Nums: 3}) {
		t.Errorf("after a recalc = %+v", got)
	}
	s.Undo()
	if got := s.RangeStats(r); got != (Stats{Sum: 62, Count: 4, Nums: 3}) {
		t.Errorf("after undo = %+v", got)
	}
}
