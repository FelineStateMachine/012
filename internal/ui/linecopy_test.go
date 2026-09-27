package ui

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Copying a whole column and pasting it at the top of another carries
// its column format; cutting one moves it, one undo step each.
func TestCopyColumnCarriesItsFormat(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "5", "<enter>", "<up>", "<ctrl+space>")
	m.runCommand("format.currency")
	press(t, m, "<ctrl+c>", "<esc>", "<right>", "<right>", "<ctrl+v>")
	if got := line(m, 2); got != "Pasted 1 column at D:D" {
		t.Errorf("context line %q", got)
	}
	if f := m.sheet.DisplayFormat(sheet.Addr{Col: 3, Row: 500}); f.Kind != sheet.FmtCurrency {
		t.Errorf("D501 shows %v, want column B's currency", f.Kind)
	}
	press(t, m, "<ctrl+z>")
	if f := m.sheet.DisplayFormat(sheet.Addr{Col: 3, Row: 500}); !f.IsZero() {
		t.Errorf("after undo D501 shows %v", f.Kind)
	}

	// Cut column B and paste it at F1: its format goes along.
	press(t, m, "<ctrl+home>", "<right>", "<ctrl+space>", "<ctrl+x>")
	m.clearSelection()
	m.cur = sheet.Addr{Col: 5}
	press(t, m, "<ctrl+v>")
	if got := line(m, 2); got != "Moved B:B to F:F" {
		t.Errorf("context line %q", got)
	}
	if input(m, "F1") != "5" || m.sheet.DisplayFormat(sheet.Addr{Col: 5, Row: 9}).Kind != sheet.FmtCurrency ||
		!m.sheet.DisplayFormat(sheet.Addr{Col: 1, Row: 9}).IsZero() {
		t.Errorf("F1 %q, F10 %v, B10 %v", input(m, "F1"), m.sheet.DisplayFormat(sheet.Addr{Col: 5, Row: 9}).Kind,
			m.sheet.DisplayFormat(sheet.Addr{Col: 1, Row: 9}).Kind)
	}
}
