package ui

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Go to takes a region's name as formulas write it and selects its
// table, header row included, on whichever sheet holds it.
func TestGotoRegion(t *testing.T) {
	m := newModel()
	nb, err := m.book().AddNotebook("Shell", m.sheet)
	if err != nil {
		t.Fatal(err)
	}
	if err := nb.AddRegion(sheet.Region{Name: "totals", Command: "ls"}); err != nil {
		t.Fatal(err)
	}
	d := &sheet.RegionData{Rows: 3, Cols: 2, Values: make([]sheet.Value, 6)}
	for i := range d.Values {
		d.Values[i] = sheet.Value{Kind: sheet.Text, Str: "x"}
	}
	if err := nb.ShowRegion("totals", d); err != nil {
		t.Fatal(err)
	}
	want, ok := nb.RegionTable("totals")
	if !ok {
		t.Fatal("no table")
	}
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
