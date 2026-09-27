package sheet

import (
	"math"
	"testing"
)

// fixture is a small sheet the function tests read from.
//
//	   A       B       C   D       E    F            G   H      I
//	1  10      north   1   Apple   1.5  9/26/2026    1   -1000  30
//	2  20      south   2   Banana  2.5  2026-01-31   2   300    20
//	3  30      north   3   Cherry  3.5  14:30        2   400    10
//	4  apple   east    4   date    x    2/29/2024    3   500
//	5          north   5   Apple   9    12/31/2025       5
//	6  TRUE    ""      6
//	7  -5      Ab*d    7
func fixture(t *testing.T) *Sheet {
	t.Helper()
	fixedNow(t)
	s := New()
	for a, in := range map[string]string{
		"A1": "10", "A2": "20", "A3": "30", "A4": "apple", "A6": "TRUE", "A7": "-5",
		"B1": "north", "B2": "south", "B3": "north", "B4": "east", "B5": "north", "B6": `=""`, "B7": "Ab*d",
		"C1": "1", "C2": "2", "C3": "3", "C4": "4", "C5": "5", "C6": "6", "C7": "7",
		"D1": "Apple", "D2": "Banana", "D3": "Cherry", "D4": "date", "D5": "Apple",
		"E1": "1.5", "E2": "2.5", "E3": "3.5", "E4": "x", "E5": "9",
		"F1": "9/26/2026", "F2": "2026-01-31", "F3": "14:30", "F4": "2/29/2024", "F5": "12/31/2025",
		"G1": "1", "G2": "2", "G3": "2", "G4": "3",
		"H1": "-1000", "H2": "300", "H3": "400", "H4": "500",
		"I1": "30", "I2": "20", "I3": "10",
	} {
		if err := s.Set(at(a), in); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	return s
}

type fnCase struct {
	in   string
	want Value
}

// checkFormulas evaluates each formula in Z1 and compares, allowing
// rounding differences in the 12th significant digit.
func checkFormulas(t *testing.T, s *Sheet, tests []fnCase) {
	t.Helper()
	for _, tt := range tests {
		if err := s.Set(at("Z1"), tt.in); err != nil {
			t.Errorf("Set(%q): %v", tt.in, err)
			continue
		}
		got := s.Value(at("Z1"))
		if got.Kind == Number && tt.want.Kind == Number {
			if d := math.Abs(got.Num - tt.want.Num); d > 1e-9*math.Max(1, math.Abs(tt.want.Num)) {
				t.Errorf("%s = %v, want %v", tt.in, got.Num, tt.want.Num)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}
