package sheet

import "testing"

// A criterion's walk over a whole column reads its cells right the first
// time, when its buffer grows between chunks.
func TestCriteriaWalkGrowingBuffer(t *testing.T) {
	s := New()
	for i := range 12 {
		lv := "info"
		if i == 5 {
			lv = "error"
		}
		s.Set(Addr{Col: 1, Row: i}, lv)
	}
	s.Set(Addr{Col: 5}, `=COUNTIF(B:B,"error")`)
	s.Set(Addr{Col: 5, Row: 1}, `=MATCH("error",B:B,0)`)
	if got := s.Value(Addr{Col: 5}); got.Num != 1 {
		t.Errorf("COUNTIF = %v, want 1", got)
	}
	if got := s.Value(Addr{Col: 5, Row: 1}); got.Num != 6 {
		t.Errorf("MATCH = %v, want 6", got)
	}
}
