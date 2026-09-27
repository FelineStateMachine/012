package ui

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Vim's yy and dd copy whole rows, so p and P carry their row formats,
// as a whole-row paste does: the pasted row is bold far past its data.
func TestVimPutCarriesRowFormats(t *testing.T) {
	m := vimModel(t)
	press(t, m, "2G", "<shift+space>")
	m.runCommand("format.bold")
	press(t, m, "<esc>", "yy", "G", "p")
	far := sheet.Addr{Col: 400, Row: 6} // row 7, the pasted one
	if input(m, "A7") != "A2" || !m.sheet.CellStyle(far).Bold {
		t.Fatalf("p: A7 %q, row 7 bold %v", input(m, "A7"), m.sheet.CellStyle(far).Bold)
	}
	if m.sheet.CellStyle(sheet.Addr{Col: 400, Row: 5}).Bold {
		t.Error("row 6 turned bold")
	}
	press(t, m, "u")
	if m.sheet.CellStyle(far).Bold {
		t.Error("undo left row 7 bold")
	}
	// dd then P: the row format moves with the row.
	press(t, m, "2G", "dd", "gg", "P")
	if input(m, "A1") != "A2" || !m.sheet.CellStyle(sheet.Addr{Col: 400}).Bold || m.sheet.CellStyle(sheet.Addr{Col: 400, Row: 1}).Bold {
		t.Errorf("dd P: A1 %q, row 1 bold %v, row 2 bold %v", input(m, "A1"),
			m.sheet.CellStyle(sheet.Addr{Col: 400}).Bold, m.sheet.CellStyle(sheet.Addr{Col: 400, Row: 1}).Bold)
	}
}

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
