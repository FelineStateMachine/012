package sheet

import "testing"

// Range users are found by column; a formula must follow a change in any
// column its ranges cover, and stop following a range it no longer has.
func TestRangeUsersByColumn(t *testing.T) {
	tests := []struct {
		name   string
		setup  []string // cell=input pairs, in order
		change string   // cell=input
		cell   string
		want   float64
	}{
		{"middle column of a block", []string{"A1=1", "B2=2", "C3=3", "E1==SUM(A1:C3)"}, "B3=10", "E1", 16},
		{"two ranges", []string{"A1=1", "D1=4", "E1==SUM(A1:A2,D1:D2)"}, "D2=5", "E1", 10},
		{"range replaced by another", []string{"A1=1", "B1=2", "E1==SUM(A1:A5)", "E1==SUM(B1:B5)"}, "B2=3", "E1", 5},
		{"through a chain", []string{"A1=1", "B1==SUM(A1:A3)", "C1==B1*2"}, "A3=4", "C1", 10},
		{"row range", []string{"A1=1", "E2==SUM(A1:D1)"}, "D1=2", "E2", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			set := func(pair string) {
				t.Helper()
				for i := range pair {
					if pair[i] == '=' {
						if err := s.Set(at(pair[:i]), pair[i+1:]); err != nil {
							t.Fatal(err)
						}
						return
					}
				}
			}
			for _, p := range tt.setup {
				set(p)
			}
			set(tt.change)
			if got := s.Value(at(tt.cell)); got.Num != tt.want {
				t.Errorf("%s = %v, want %v", tt.cell, got, tt.want)
			}
		})
	}
	// Replacing E1's formula leaves nothing indexed under column A.
	s := New()
	s.Set(at("E1"), "=SUM(A1:A5)")
	s.Set(at("E1"), "=SUM(B1:B5)")
	if iv := s.rangeUsers.byCol[0]; iv != nil {
		t.Errorf("%d stale range users in column A", len(iv.users))
	}
}
