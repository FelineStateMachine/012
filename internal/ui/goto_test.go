package ui

import (
	"strconv"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// withRegion adds a notebook sheet after the one shown with region name
// showing a table of a header row and two rows of numbers, and returns
// the sheet and the table.
func withRegion(t *testing.T, m *Model, name string) (*sheet.Sheet, sheet.Rect) {
	t.Helper()
	nb, err := m.book().AddNotebook("Shell", m.sheet)
	if err != nil {
		t.Fatal(err)
	}
	if err := nb.AddRegion(sheet.Region{Name: name, Command: "ls"}); err != nil {
		t.Fatal(err)
	}
	d := &sheet.RegionData{Rows: 3, Cols: 2, Values: []sheet.Value{
		{Kind: sheet.Text, Str: "name"}, {Kind: sheet.Text, Str: "size"},
		{Kind: sheet.Text, Str: "a"}, {Kind: sheet.Number, Num: 1},
		{Kind: sheet.Text, Str: "b"}, {Kind: sheet.Number, Num: 2},
	}}
	if err := nb.ShowRegion(name, d); err != nil {
		t.Fatal(err)
	}
	table, ok := nb.RegionTable(name)
	if !ok {
		t.Fatal("no table")
	}
	return nb, table
}

// Go to takes a region's name as formulas write it and selects its
// table, header row included, on whichever sheet holds it.
func TestGotoRegion(t *testing.T) {
	m := newModel()
	nb, want := withRegion(t, m, "totals")
	press(t, m, "<ctrl+g>", "NU.Totals", "<enter>")
	if m.mode != modeReady || m.sheet != nb {
		t.Fatalf("mode %v, sheet %q, err %q", m.mode, m.sheet.Name(), m.errMsg)
	}
	if got := m.selection(); got != want {
		t.Errorf("selected %v, want the table %v", got, want)
	}
	press(t, m, "<f5>", "nu.nothing", "<enter>")
	if m.mode != modeError {
		t.Errorf("an unknown region: mode %v", m.mode)
	}
}

// From a cell of a region, or its label, the table around it is the
// region's table, not the block of data that takes in the label line;
// Insert > Chart and a pivot table read it.
func TestTableAroundRegion(t *testing.T) {
	m := newModel()
	nb, table := withRegion(t, m, "r1")
	m.showSheet(nb)
	label := sheet.Addr{Col: table.From.Col, Row: table.From.Row - 1}
	for _, at := range []sheet.Addr{table.To, label} {
		m.clearSelection()
		m.cur = at
		if got := m.dataRange(); got != table {
			t.Errorf("from %v: %v, want the table %v", at, got, table)
		}
		if got, ok := m.pivotData(); !ok || got != table {
			t.Errorf("from %v: pivot reads %v %v, want %v", at, got, ok, table)
		}
	}
	m.cur = table.To
	m.runCommand("insert.chart")
	press(t, m, "<enter>")
	if cs := nb.Charts(); len(cs) != 1 || cs[0].Data != table {
		t.Fatalf("charts %+v, want one of %v", cs, table)
	}
	// Ordinary data is still the block around the cell.
	m.showSheet(m.book().Sheets()[0])
	for r := range 3 {
		m.sheet.Set(sheet.Addr{Row: r}, strconv.Itoa(r))
	}
	m.cur = sheet.Addr{Row: 1}
	if got := m.dataRange(); got != (sheet.Rect{To: sheet.Addr{Row: 2}}) {
		t.Errorf("ordinary data: %v", got)
	}
}
