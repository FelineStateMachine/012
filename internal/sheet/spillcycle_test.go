package sheet

import "testing"

// Cycles through arrays settle the same typed in any order, recalculated
// or opened (spillblock.go).

// wantSettled types cells in order, then checks what the cells of want
// show, as typed, once everything is recalculated, and opened.
func wantSettled(t *testing.T, name string, order []string, cells, want map[string]string) {
	t.Helper()
	s := New()
	for _, a := range order {
		s.Set(at(a), cells[a])
	}
	check := func(how string, sh *Sheet) {
		t.Helper()
		for a, v := range want {
			if got := sh.Value(at(a)).String(); got != v {
				t.Errorf("%s, typed %v, %s: %s = %q, want %q", name, order, how, a, got, v)
			}
		}
	}
	check("typed", s)
	reopened := roundTrip(t, s)
	check("reopened", reopened)
	s.RecalcAll()
	check("recalculated", s)
}

// Two arrays reading each other's cells, one's size following what it
// reads: blocked, each shows #REF!, which the other reads, and its
// smaller array would free the first. They end blocked together.
func TestArraysGoingAroundACycle(t *testing.T) {
	cells := map[string]string{"C3": "=SORT(C13:E16)", "D13": "-5", "B14": "=SORT(C2:E5)"}
	want := map[string]string{"C3": "#REF!", "B14": "#REF!", "D3": ""}
	for _, order := range [][]string{{"C3", "D13", "B14"}, {"B14", "D13", "C3"}, {"D13", "C3", "B14"}} {
		wantSettled(t, "around", order, cells, want)
	}
}

// A formula typed last that reads an array's cells closes a cycle
// through it and another array, which is computed again for it: both
// are blocked, as when the file opens.
func TestCycleClosedThroughAnotherArray(t *testing.T) {
	cells := map[string]string{"D9": "=SORT(G15:H19)", "H19": "=SORT(D13:F16)", "D16": "=ABS($A$9)+1", "G16": `=COUNTIF(H20,">2")`}
	wantSettled(t, "closed", []string{"D9", "H19", "D16", "G16"}, cells, map[string]string{"D9": "#REF!", "H19": "#REF!"})
}

// An array blocked by a cycle whose formula gives way to a value frees
// the arrays the cycle went through.
func TestCycleGoneWithAnArray(t *testing.T) {
	s := sheetOf(t, map[string]string{"A9": "=SORT(B9:D12)"})
	s.Set(at("C9"), "=SORT(A11:C14)")
	s.Set(at("C11"), `=COUNTIF(A10:A13,">2")`)
	if v := s.Value(at("C9")); v != ErrRef {
		t.Fatalf("C9 in a cycle = %v", v)
	}
	s.Set(at("A9"), "-5")
	if sp := s.spills[at("C9")]; sp == nil || sp.why != "" {
		t.Errorf("C9 once A9 is a value: %+v", sp)
	}
	if sp := roundTrip(t, s).spills[at("C9")]; sp == nil || sp.why != "" {
		t.Errorf("C9 opened: %+v", sp)
	}
}

// An array spilling into the inputs of another, blocked by a value but
// only as wide as it is for the first's spill, and reading what the
// other needs: counted as spilling, the blocked one closes the cycle,
// so both are blocked whichever was typed first, as when opened.
func TestCycleThroughABlockedArray(t *testing.T) {
	cells := map[string]string{"C9": "1.5", "C13": "=SORT(D19:E20)", "D18": "=SORT(C9:E13)", "E19": "=SEQUENCE(3)"}
	want := map[string]string{"C13": "#REF!", "D18": "#REF!", "D13": "", "E20": "2"}
	for _, order := range [][]string{{"C9", "E19", "C13", "D18"}, {"C9", "D18", "C13", "E19"}, {"D18", "C13", "C9", "E19"}} {
		wantSettled(t, "blocked", order, cells, want)
	}
}

// A blocked array that goes, its formula computing one value, frees the
// arrays whose cycle went through the cells it needed (spill.walked):
// once Total is defined, B7 reads its own #REF! through G9.
func TestCycleGoneWithABlockedArray(t *testing.T) {
	cells := map[string]string{
		"H9": "=VLOOKUP(G8,G14:G15,1,FALSE)", "E7": "=SORT(F10:G14)", "G9": "=SUM(Total)*2", "B7": "=SORT(G7:H11)",
		"H5": "=SORT(C4:E8)", "B9": "=VLOOKUP(A13,C14:D18,1,FALSE)", "G18": "=ABS(C14)+1",
	}
	s := New()
	for _, a := range []string{"H9", "E7", "G9", "B7", "H5", "B9"} {
		s.Set(at(a), cells[a])
	}
	s.Book().DefineName("Total", s, NewRect(Addr{}, Addr{Col: 1, Row: 9}))
	s.Set(at("G18"), cells["G18"])
	wantShown(t, s, map[string]string{"B7": "#REF!", "H5": ""})
}
