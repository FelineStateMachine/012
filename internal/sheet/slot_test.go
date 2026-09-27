package sheet

import (
	"reflect"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/formula"
)

// literalValue is what recalculation computes for an entry that isn't a
// formula (evaluate.go).
func literalValue(c *Cell) Value {
	switch e := c.expr.(type) {
	case formula.Num:
		return num(e.V)
	case formula.Bool:
		return boolean(e.V)
	}
	switch {
	case c.Input == "":
		return Value{}
	case c.Format.Kind == FmtText:
		return Value{Kind: Text, Str: c.Input}
	}
	return Value{Kind: Text, Str: strings.TrimPrefix(c.Input, "'")}
}

// roundTripCells are cells of every kind: entries of each sort under
// formats, styles and notes, with the values recalculation gives them.
func roundTripCells() []*Cell {
	inputs := []string{
		"1", "1.50", "-0", "0.1", "1e5", "007", "1.0000000000000000001", "123456789012345678901234567890",
		"$1,200", "12%", "9/26/2026", "2026-09-26", "13:45", "TRUE", "true", "FALSE", "hello", "'123", "'",
		"=A1", "=5", "=TRUE", "+5", "-5", "+A1", " 7 ", "ünïcode", "#N/A", "1e400", "",
	}
	formats := []Format{{}, Preset(FmtCurrency), {Kind: FmtText}, {Kind: FmtDate, Pattern: "yyyy-mm-dd"}}
	styles := []Style{{}, {Bold: true}, {Align: AlignRight, own: true}}
	var out []*Cell
	for i := range len(inputs) * len(formats) * len(styles) * 4 {
		input, f := inputs[i%len(inputs)], formats[i/len(inputs)%len(formats)]
		sty, rest := styles[i/len(inputs)/len(formats)%len(styles)], i/len(inputs)/len(formats)/len(styles)
		c, err := newCell(input, f, sty, rest&1 == 1)
		if rest&2 == 2 {
			c = c.withNote("a note")
		}
		if err != nil || c == nil {
			continue
		}
		c.Value = literalValue(c)
		if c.expr != nil && !c.literal() {
			c.Value = Value{} // a formula: only recalculation sets it
		}
		out = append(out, c)
	}
	return out
}

// Every cell comes back from the store as it went in, whether it is kept
// as a plain slot or whole: its entry exactly as typed, its value, format,
// style, note and expression.
func TestSlotRoundTrip(t *testing.T) {
	st := newCellStore()
	a := Addr{Col: 3, Row: 1500}
	cells := roundTripCells()
	for _, c := range cells {
		want := c.clone()
		st.set(a, c)
		checkSame(t, st.get(a), want)
		checkSame(t, st.copyOf(a), want)
		if v := st.value(a); v != want.Value {
			t.Errorf("%q: value %v, want %v", want.Input, v, want.Value)
		}
		if got := st.filledAt(a); got != !want.Blank() {
			t.Errorf("%q: filled %v", want.Input, got)
		}
		if !st.holds(a, st.get(a)) {
			t.Errorf("%q: doesn't hold its own view", want.Input)
		}
	}
	st.delete(a)
	if st.len() != 0 || len(st.strs.index) != 0 || len(st.rich) != len(st.richFree) {
		t.Errorf("after %d cells: %d stored, %d strings, %d rich", len(cells), st.len(), len(st.strs.index), len(st.rich)-len(st.richFree))
	}
}

// literal reports whether c's expression is a typed number or boolean,
// which recalculation doesn't evaluate as a formula.
func (c *Cell) literal() bool {
	switch c.expr.(type) {
	case formula.Num, formula.Bool:
		return !strings.HasPrefix(c.Input, "=")
	}
	return false
}

func checkSame(t *testing.T, got, want *Cell) {
	t.Helper()
	if got.Input != want.Input || got.Value != want.Value || got.Format != want.Format || got.Style != want.Style ||
		got.Note != want.Note || got.auto != want.auto || got.derived != want.derived || got.spilled != want.spilled ||
		!reflect.DeepEqual(got.expr, want.expr) || got.IsFormula() != want.IsFormula() {
		t.Errorf("stored %+v\ncame back %+v", want, got)
	}
}

// Cells sharing a string keep it while any of them uses it, and a cell
// turning from plain to rich and back keeps its place.
func TestSlotStrings(t *testing.T) {
	st := newCellStore()
	a1, a2 := Addr{}, Addr{Row: 1}
	st.set(a1, &Cell{Input: "same"})
	st.set(a2, &Cell{Input: "same"})
	if len(st.strs.index) != 1 {
		t.Fatalf("%d strings for one text", len(st.strs.index))
	}
	st.delete(a1)
	if c := st.get(a2); c.Input != "same" || c.Value.Str != "same" {
		t.Errorf("the other cell is %+v", c)
	}
	rich := &Cell{Input: "same", Note: "n"}
	st.set(a2, rich)
	if st.get(a2) != rich || st.richAt(a2) != rich || len(st.strs.index) != 0 {
		t.Errorf("rich cell not kept whole, %d strings", len(st.strs.index))
	}
	num, _ := newCell("12.50", Format{}, Style{}, true)
	st.set(a2, num)
	if c := st.get(a2); c.Input != "12.50" || c.Value.Num != 12.5 || st.richAt(a2) != nil {
		t.Errorf("back to plain: %+v", c)
	}
	if len(st.rich) != len(st.richFree) {
		t.Errorf("%d rich cells left", len(st.rich)-len(st.richFree))
	}
}

// A plain number costs its slot: no string, no Cell.
func TestSlotCompact(t *testing.T) {
	s := New()
	for row := range 2048 {
		s.Load(Addr{Row: row}, []string{"1", "2.5", "3.10", "-4"}[row%4], Format{}, Style{})
	}
	if n := len(s.cells.rich) + len(s.cells.strs.strs); n != 0 {
		t.Errorf("%d rich cells and strings for plain numbers", n)
	}
}
