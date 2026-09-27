package sheet

import (
	"bytes"
	"strings"
	"testing"
)

// A derived cell of every value, format, inferred format and mark comes
// back from the store as it went in, kept in a slot unless it has a
// note, and the store counts the pivot's results it holds.
func TestSlotDerived(t *testing.T) {
	st := newCellStore()
	a := Addr{Col: 2, Row: 700}
	values := []Value{num(1.5), num(-0.25), boolean(true), boolean(false), {Kind: Text, Str: "north"},
		{Kind: Text, Str: "=not a formula"}, {Kind: Text, Str: "12"}, ErrRef, ErrDiv0, {Kind: Text}}
	for _, v := range values {
		for _, pivot := range []bool{true, false} {
			for _, note := range []string{"", "a note"} {
				c := &Cell{Input: derivedInput(v), Value: v, Format: Preset(FmtCurrency), Note: note, derived: pivot, spilled: !pivot}
				if !pivot {
					c.auto = Preset(FmtPercent)
				}
				c.Style.Bold = pivot
				checkDerived(t, &st, a, c)
			}
		}
	}
	st.delete(a)
	if st.len() != 0 || st.pivots != 0 || len(st.strs.index) != 0 || len(st.rich) != len(st.richFree) {
		t.Errorf("left: %d stored, %d pivot cells, %d strings, %d rich", st.len(), st.pivots, len(st.strs.index), len(st.rich)-len(st.richFree))
	}
}

// checkDerived stores the derived cell c at a and reads it back.
func checkDerived(t *testing.T, st *cellStore, a Addr, c *Cell) {
	t.Helper()
	want := c.clone()
	st.set(a, c)
	checkSame(t, st.get(a), want)
	if rich := st.richAt(a) != nil; rich != (want.Note != "") {
		t.Errorf("%+v: rich %v", want, rich)
	}
	if got := st.filledAt(a); got != !want.Blank() {
		t.Errorf("%+v: filled %v", want, got)
	}
	if !st.holds(a, st.get(a)) {
		t.Errorf("%+v: doesn't hold its own view", want)
	}
	if n := len(collect(st.pivotKeys())); n != st.pivots || (n == 1) != want.derived {
		t.Errorf("%+v: %d pivot cells found, %d counted", want, n, st.pivots)
	}
}

func collect(seq func(func(Addr) bool)) []Addr {
	var out []Addr
	seq(func(a Addr) bool { out = append(out, a); return true })
	return out
}

// Spilled cells are slots, and read as they did as Cells: values of each
// kind, the anchor's inferred format, values when copied, skipped by
// Replace and left out of the file.
func TestSpillCompact(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "$5", "A2": "$6", "A3": "x",
		"B1": "=ARRAYFORMULA(A1:A2*2)",
		"C1": `=ARRAYFORMULA(IF(SEQUENCE(3)=2, "none", SEQUENCE(3)))`,
		"D1": "=SEQUENCE(2)>1",
		"E1": "=1/(SEQUENCE(2)-2)",
	})
	wantShown(t, s, map[string]string{"B2": "12", "C2": "none", "C3": "3", "D2": "TRUE", "E2": "#DIV/0!"})
	for _, a := range []string{"B2", "C2", "C3", "D2", "E2"} {
		if s.cells.richAt(at(a)) != nil {
			t.Errorf("%s is a whole Cell", a)
		}
		if c := s.Cell(at(a)); !c.Spilled() || c.Input != derivedInput(c.Value) {
			t.Errorf("%s = %+v", a, c)
		}
	}
	if f := s.DisplayFormat(at("B2")); f.Kind != FmtCurrency {
		t.Errorf("B2's format %v, want the anchor's currency", f)
	}
	found, err := s.Find("none", FindOptions{})
	if err != nil || len(found) != 1 || found[0] != at("C2") {
		t.Errorf("Find found %v %v, want the spilled C2", found, err)
	}
	if changed, err := s.Replace(at("C2"), "none", "x", FindOptions{}); changed || err != nil {
		t.Errorf("Replace changed a spilled cell: %v %v", changed, err)
	}
	clip := s.Copy(rect("E1:E2"))
	if _, err := s.Paste(clip, rect("G1:G1"), false); err != nil {
		t.Fatal(err)
	}
	if c := s.Cell(at("G2")); c.Spilled() || c.Value.String() != "#DIV/0!" {
		t.Errorf("pasted spilled error = %+v", c)
	}
	var buf bytes.Buffer
	if err := s.Book().Write(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"B2"`) || strings.Contains(buf.String(), `"D2"`) {
		t.Errorf("spilled cells saved:\n%s", buf.String())
	}
}

// A pivot's results are slots, found again when they shrink.
func TestPivotCompact(t *testing.T) {
	src := sheetOf(t, map[string]string{"A1": "Region", "B1": "Sales", "A2": "north", "B2": "10", "A3": "south", "B3": "5", "A4": "east", "B4": "7"})
	r := rect("A1:B4")
	p := NewPivot(src, r)
	p.Rows = []PivotGroup{{Col: 0}}
	p.Values = []PivotValue{{Col: 1, Summarize: SumBy}}
	pv, err := src.Book().CreatePivot(src, r, "", p)
	if err != nil {
		t.Fatal(err)
	}
	n := pv.cells.pivots
	if n == 0 || len(pv.cells.rich) != len(pv.cells.richFree) {
		t.Fatalf("%d pivot cells, %d rich", n, len(pv.cells.rich)-len(pv.cells.richFree))
	}
	if err := src.Set(at("A4"), "north"); err != nil {
		t.Fatal(err)
	}
	if pv.cells.pivots >= n {
		t.Errorf("%d pivot cells after a group went, had %d", pv.cells.pivots, n)
	}
	if got := len(collect(pv.cells.pivotKeys())); got != pv.cells.pivots {
		t.Errorf("%d pivot cells found, %d counted", got, pv.cells.pivots)
	}
}
