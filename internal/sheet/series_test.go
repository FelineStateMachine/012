package sheet

import (
	"reflect"
	"testing"
)

func TestFillSeries(t *testing.T) {
	tests := []struct {
		name string
		src  []string // A1 down
		n    int      // cells to fill below
		want []string
	}{
		{"counting", []string{"1", "2"}, 3, []string{"3", "4", "5"}},
		{"even numbers", []string{"2", "4", "6"}, 2, []string{"8", "10"}},
		{"decimals without float noise", []string{"0.1", "0.2"}, 2, []string{"0.3", "0.4"}},
		{"linear trend", []string{"1", "2", "4", "5"}, 1, []string{"6.5"}},
		{"counting down", []string{"10", "7"}, 2, []string{"4", "1"}},
		{"a single number is copied", []string{"5"}, 2, []string{"5", "5"}},
		{"currency keeps its format", []string{"$100", "$150"}, 1, []string{"$200"}},
		{"months", []string{"Jan", "Feb"}, 2, []string{"Mar", "Apr"}},
		{"months wrap around", []string{"Nov"}, 3, []string{"Dec", "Jan", "Feb"}},
		{"long months, every other", []string{"January", "March"}, 1, []string{"May"}},
		{"May reads as long or short", []string{"Apr", "May"}, 1, []string{"Jun"}},
		{"days keep their case", []string{"MON", "TUE"}, 2, []string{"WED", "THU"}},
		{"long days", []string{"friday"}, 2, []string{"saturday", "sunday"}},
		{"text with a number", []string{"Item 1"}, 2, []string{"Item 2", "Item 3"}},
		{"text with a step", []string{"Q1", "Q3"}, 1, []string{"Q5"}},
		{"zero padding", []string{"Run 08", "Run 09"}, 2, []string{"Run 10", "Run 11"}},
		{"dates by day", []string{"9/26/2026"}, 2, []string{"9/27/2026", "9/28/2026"}},
		{"dates by week", []string{"9/26/2026", "10/3/2026"}, 1, []string{"10/10/2026"}},
		{"different days of the month go on by days", []string{"1/31/2026", "2/28/2026"}, 1, []string{"3/28/2026"}},
		{"dates on the same day by month", []string{"1/15/2026", "3/15/2026"}, 2, []string{"5/15/2026", "7/15/2026"}},
		{"plain text is copied in turn", []string{"a", "b"}, 3, []string{"a", "b", "a"}},
		{"mixed is copied", []string{"1", "x"}, 2, []string{"1", "x"}},
		{"formulas are copied with their references", []string{"=B1", "=B2*2"}, 2, []string{"=B3", "=B4*2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			for i, in := range tt.src {
				s.Set(Addr{Row: i}, in)
			}
			s.ClearHistory()
			src := Rect{To: Addr{Row: len(tt.src) - 1}}
			dst := Rect{To: Addr{Row: len(tt.src) + tt.n - 1}}
			got, err := s.FillSeries(src, dst)
			if err != nil || got != dst {
				t.Fatalf("FillSeries = %v, %v", got, err)
			}
			if got := column(s, "A", len(tt.src)+1, len(tt.src)+tt.n); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filled %q, want %q", got, tt.want)
			}
			s.Undo()
			if s.CanUndo() || s.Len() != len(tt.src) {
				t.Errorf("fill isn't one undo step: %v", inputs(s))
			}
		})
	}
}

func TestFillSeriesDirections(t *testing.T) {
	s := sheetOf(t, map[string]string{"C3": "5", "C4": "6", "D3": "Mon", "D4": "Tue"})
	// Up continues backwards.
	s.FillSeries(Rect{From: at("C3"), To: at("D4")}, Rect{From: at("C1"), To: at("D4")})
	if got := column(s, "C", 1, 2); !reflect.DeepEqual(got, []string{"3", "4"}) {
		t.Errorf("up: C = %q", got)
	}
	if got := column(s, "D", 1, 2); !reflect.DeepEqual(got, []string{"Sat", "Sun"}) {
		t.Errorf("up: D = %q", got)
	}
	// Right and left, per row.
	s = sheetOf(t, map[string]string{"B1": "1", "C1": "3", "B2": "x"})
	s.FillSeries(Rect{From: at("B1"), To: at("C2")}, Rect{From: at("A1"), To: at("C2")})
	s.FillSeries(Rect{From: at("A1"), To: at("C2")}, Rect{From: at("A1"), To: at("E2")})
	want := map[string]string{"A1": "-1", "B1": "1", "C1": "3", "D1": "5", "E1": "7", "A2": "", "B2": "x", "D2": "", "E2": "x"}
	for a, in := range want {
		if got := input(s, a); got != in {
			t.Errorf("%s = %q, want %q", a, got, in)
		}
	}
}

func input(s *Sheet, a string) string {
	if c := s.Cell(at(a)); c != nil {
		return c.Input
	}
	return ""
}

func TestFillDownContinuesSeries(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
		fill  string
		want  []string
	}{
		{"a series with blanks below continues", map[string]string{"A1": "1", "A2": "2"}, "A1:A5", []string{"1", "2", "3", "4", "5"}},
		{"one row is copied, as Sheets' Ctrl+D", map[string]string{"A1": "Item 1"}, "A1:A3", []string{"Item 1", "Item 1", "Item 1"}},
		{"filled cells below: copy the top row", map[string]string{"A1": "1", "A2": "2", "A4": "x"}, "A1:A4", []string{"1", "1", "1", "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := sheetOf(t, tt.cells)
			r, _ := ParseRange(tt.fill)
			if _, err := s.FillDown(r); err != nil {
				t.Fatal(err)
			}
			if got := column(s, "A", 1, len(tt.want)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("A = %q, want %q", got, tt.want)
			}
		})
	}
	s := sheetOf(t, map[string]string{"A1": "Jan", "B1": "Feb"})
	s.FillRight(Rect{From: at("A1"), To: at("D1")})
	if got := input(s, "D1"); got != "Apr" {
		t.Errorf("fill right: D1 = %q", got)
	}
}
