package sheet

import "testing"

// Regions meeting each other show the same whichever order their rows
// arrive in, as a file reopened sends them in another order than they
// arrived in (regionclash.go).

// sendBoth is a sheet with outputs a and b at the cells aAt and bAt,
// sent opA and opB in that order, or the other with bFirst.
func sendBoth(t *testing.T, aAt, bAt string, opA, opB LiveOp, bFirst bool) *Sheet {
	t.Helper()
	s := New()
	sent(t, s, "a", aAt)
	sent(t, s, "b", bAt)
	s.Set(at("H1"), "=SUM(nu.a)")
	s.Set(at("H2"), "=SUM(nu.b)")
	ops := []LiveOp{opA, opB}
	if bFirst {
		ops = []LiveOp{opB, opA}
	}
	for _, op := range ops {
		apply(t, s, op)
	}
	return s
}

// A region's anchor is in the way of another's table, whether or not
// its own rows have arrived.
func TestRegionAnchorBlocksAnother(t *testing.T) {
	opA := LiveOp{Region: "a", Reset: true, Header: liveRow("x", "y"), Rows: []LiveRow{liveRow("1", "2")}}
	opB := LiveOp{Region: "b", Reset: true, Header: liveRow("p", "q"), Rows: []LiveRow{liveRow("3", "4")}}
	for _, bFirst := range []bool{false, true} {
		s := sendBoth(t, "A1", "B2", opA, opB, bFirst)
		wantShown(t, s, map[string]string{"A1": "#REF!", "A2": "", "B2": "p", "C3": "4", "H1": "#REF!", "H2": "7"})
	}
}

// When two tables need the same cells, the region whose anchor comes
// first, row by row, gets them, whichever arrived first.
func TestRegionFirstAnchorWins(t *testing.T) {
	// a needs B1:C3 and b A2:C2: neither holds the other's anchor.
	opA := LiveOp{Region: "a", Reset: true, Header: liveRow("x", "y"), Rows: []LiveRow{liveRow("1", "2"), liveRow("3", "4")}}
	opB := LiveOp{Region: "b", Reset: true, Header: liveRow("5", "6", "7")}
	for _, bFirst := range []bool{false, true} {
		s := sendBoth(t, "B1", "A2", opA, opB, bFirst)
		wantShown(t, s, map[string]string{"B1": "x", "B2": "1", "C3": "4", "A2": "#REF!", "H1": "10", "H2": "#REF!"})
	}
}

// A region an array spills over gives way, and the formulas naming it
// read #REF! until it's sent again.
func TestRegionGivingWayToAnArray(t *testing.T) {
	s := New()
	sent(t, s, "a", "B1")
	s.Set(at("H1"), "=SUM(nu.a)")
	apply(t, s, LiveOp{Region: "a", Reset: true, Header: liveRow("x"), Rows: []LiveRow{liveRow("5")}})
	wantShown(t, s, map[string]string{"H1": "5"})
	s.Set(at("A1"), "=SEQUENCE(1,2)")
	wantShown(t, s, map[string]string{"A1": "1", "B1": "2", "B2": "", "H1": "#REF!"})
}

// An array spilling over a region's table isn't blocked by a formula
// naming the region that it reads: the region gives way, so the array
// doesn't depend on it, whichever came first.
func TestArrayOverRegionItReadsByName(t *testing.T) {
	for _, arrayFirst := range []bool{false, true} {
		s := New()
		sent(t, s, "a", "B1")
		s.Set(at("D5"), "=SUM(nu.a)")
		op := LiveOp{Region: "a", Reset: true, Header: liveRow("x"), Rows: []LiveRow{liveRow("5")}}
		if !arrayFirst {
			apply(t, s, op)
		}
		s.Set(at("A1"), "=IF(ISERROR(D5),SEQUENCE(1,2),SEQUENCE(1,2))") // over B1
		if arrayFirst {
			apply(t, s, op)
		}
		wantShown(t, s, map[string]string{"A1": "1", "B1": "2", "D5": "#REF!"})
		if _, ok := s.SpillAnchor(at("B1")); !ok {
			t.Errorf("array first %v: the array doesn't spill over the region", arrayFirst)
		}
	}
}

// A paste into a blank cell of a region's table has the region find it
// in its way when its rows are sent again, as they are when the file is
// opened.
func TestPasteIntoARegionsBlankCell(t *testing.T) {
	s := New()
	sent(t, s, "a", "A1")
	op := LiveOp{Region: "a", Reset: true, Header: liveRow("x", "y"), Rows: []LiveRow{liveRow("1", "")}}
	apply(t, s, op)
	s.Set(at("E1"), "9")
	if _, err := s.Paste(s.Copy(Rect{From: at("E1"), To: at("E1")}), Rect{From: at("B2"), To: at("B2")}, false); err != nil {
		t.Fatal(err)
	}
	if got := s.Book().StaleOutputs(); len(got) != 1 {
		t.Fatalf("stale outputs %v, want a", got)
	}
	apply(t, s, op)
	wantShown(t, s, map[string]string{"A1": "#REF!", "A2": "", "B2": "9"})
}

// An array that would spill over a region's table whose cells it reads
// is a circular dependency: spilling, it would block the region and
// read its #REF!, come out smaller and free it. It shows #REF! and the
// region its table, whichever came first, and opened again.
func TestArrayOverRegionItReadsTheCellsOf(t *testing.T) {
	op := LiveOp{Region: "a", Reset: true, Header: liveRow("x", "y"), Rows: []LiveRow{liveRow("1", "2")}}
	feeds := map[string]LiveOp{"a": op}
	want := map[string]string{"I9": "#REF!", "H10": "x", "I11": "2"}
	for _, arrayFirst := range []bool{false, true} {
		s := New()
		sent(t, s, "a", "H10")
		if !arrayFirst {
			apply(t, s, op)
		}
		s.Set(at("I9"), "=SORT(H9:H11)")
		if arrayFirst {
			apply(t, s, op)
		}
		feedStale(s.Book(), feeds)
		if stale := s.Book().StaleOutputs(); len(stale) > 0 {
			t.Errorf("array first %v: still stale %v", arrayFirst, stale)
		}
		wantShown(t, s, want)
		opened := roundTrip(t, s)
		feedAll(opened.Book(), feeds)
		feedStale(opened.Book(), feeds)
		wantShown(t, opened, want)
	}
}

// A region blocked by a formula and an array reading its cells, the formula
// cleared, is blocked by the array alone, which then finds the cycle.
func TestRegionFreedButForAnArrayReadingIt(t *testing.T) {
	op := LiveOp{Region: "a", Reset: true, Header: liveRow("x", "y", "z"), Rows: []LiveRow{liveRow("1", "2", "3")}}
	s := New()
	sent(t, s, "a", "F10")
	s.Set(at("F11"), "=SEQUENCE(1)")
	s.Set(at("H9"), "=SORT(F6:F10)")
	apply(t, s, op)
	s.Set(at("F11"), "")
	feedStale(s.Book(), map[string]LiveOp{"a": op})
	wantShown(t, s, map[string]string{"H9": "#REF!", "F10": "x", "H11": "3"})
}
