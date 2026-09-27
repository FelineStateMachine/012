package sheet

import (
	"reflect"
	"testing"
)

// column returns the inputs of rows from..to of column col.
func column(s *Sheet, col string, from, to int) []string {
	var out []string
	for r := from; r <= to; r++ {
		c := s.Cell(at(col + itoa(r)))
		if c == nil {
			out = append(out, "")
			continue
		}
		out = append(out, c.Input)
	}
	return out
}

func itoa(n int) string { return Addr{Row: n - 1}.String()[1:] }

func TestSortRange(t *testing.T) {
	data := map[string]string{
		"A1": "Name", "B1": "Qty",
		"A2": "pear", "B2": "3",
		"A3": "Apple", "B3": "10",
		"A4": "fig", "B4": "",
		"A5": "banana", "B5": "3",
		"A6": "7", "B6": "TRUE",
	}
	tests := []struct {
		name string
		keys []SortKey
		a, b []string
	}{
		{"by name A to Z: numbers, then text ignoring case", []SortKey{{Col: 0}},
			[]string{"7", "Apple", "banana", "fig", "pear"}, []string{"TRUE", "10", "3", "", "3"}},
		{"by name Z to A", []SortKey{{Col: 0, Desc: true}},
			[]string{"pear", "fig", "banana", "Apple", "7"}, []string{"3", "", "3", "10", "TRUE"}},
		{"by qty: stable ties, booleans after numbers, blanks last", []SortKey{{Col: 1}},
			[]string{"pear", "banana", "Apple", "7", "fig"}, []string{"3", "3", "10", "TRUE", ""}},
		{"by qty Z to A keeps blanks last", []SortKey{{Col: 1, Desc: true}},
			[]string{"7", "Apple", "pear", "banana", "fig"}, []string{"TRUE", "10", "3", "3", ""}},
		{"by qty then name Z to A", []SortKey{{Col: 1}, {Col: 0, Desc: true}},
			[]string{"pear", "banana", "Apple", "7", "fig"}, []string{"3", "3", "10", "TRUE", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := sheetOf(t, data)
			s.SortRange(Rect{From: at("A2"), To: at("B6")}, tt.keys)
			if got := column(s, "A", 2, 6); !reflect.DeepEqual(got, tt.a) {
				t.Errorf("A = %q, want %q", got, tt.a)
			}
			if got := column(s, "B", 2, 6); !reflect.DeepEqual(got, tt.b) {
				t.Errorf("B = %q, want %q", got, tt.b)
			}
			if got := s.Cell(at("A1")).Input; got != "Name" {
				t.Errorf("header moved: %q", got)
			}
			// One undo step restores everything.
			s.Undo()
			if got := column(s, "A", 2, 6); !reflect.DeepEqual(got, []string{"pear", "Apple", "fig", "banana", "7"}) {
				t.Errorf("after undo A = %q", got)
			}
			if s.CanUndo() {
				t.Error("sort took more than one undo step")
			}
		})
	}
}

func TestSortMovesFormulasWithTheirRows(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "3", "B1": "=A1*2", "C1": "=$A$1",
		"A2": "1", "B2": "=A2*2", "C2": "=$A$1",
		"A3": "2", "B3": "=A3*2", "C3": "=$A$1",
		"E1": "=A1", // outside the sort: left pointing at A1
	})
	s.SortRange(Rect{From: at("A1"), To: at("C3")}, []SortKey{{Col: 0}})
	want := map[string]string{"A1": "1", "B1": "=A1*2", "B2": "=A2*2", "B3": "=A3*2", "C1": "=$A$1", "E1": "=A1"}
	for a, in := range want {
		if got := s.Cell(at(a)).Input; got != in {
			t.Errorf("%s = %q, want %q", a, got, in)
		}
	}
	if v := s.Value(at("B3")).Num; v != 6 {
		t.Errorf("B3 = %v, want 6", v)
	}
	if v := s.Value(at("E1")).Num; v != 1 {
		t.Errorf("E1 = %v, want 1", v)
	}
}

func TestSortClampsToData(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "b", "A2": "a"})
	// A whole-column selection sorts only as far as the data goes.
	s.SortRange(Rect{From: at("A1"), To: Addr{Col: 0, Row: MaxRows - 1}}, []SortKey{{Col: 0}})
	if got := column(s, "A", 1, 3); !reflect.DeepEqual(got, []string{"a", "b", ""}) {
		t.Errorf("A = %q", got)
	}
}

func TestRegionForSortAndFilter(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"B2": "x", "C2": "y", "B3": "1", "D4": "diagonal",
		"G7": "island",
	})
	tests := []struct{ from, want string }{
		{"B2", "B2:D4"},
		{"D4", "D4"},    // reaches nothing: only B2:C3 reaches it
		{"C3", "B2:D4"}, // blank, next to data
		{"A1", "B2:D4"}, // blank, touching B2 diagonally
		{"G7", "G7"},
		{"J10", "J10"},
	}
	for _, tt := range tests {
		if got := s.Region(at(tt.from)).String(); got != tt.want {
			t.Errorf("Region(%s) = %s, want %s", tt.from, got, tt.want)
		}
	}
}
