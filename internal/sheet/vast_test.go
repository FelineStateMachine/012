package sheet

import "testing"

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
