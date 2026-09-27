package e2e

import (
	"strings"
	"testing"
)

func TestTypeDateAndApplyCurrency(t *testing.T) {
	s := start(t, "")
	s.keys("9/26/2026", "<enter>", "2026-09-27", "<enter>", "14:30", "<enter>")
	s.waitForLine(gridRow1, numRow(1, "9/26/2026"))
	s.waitForLine(gridRow1+1, numRow(2, "2026-09-27"))
	s.waitForLine(gridRow1+2, numRow(3, "14:30:00"))

	// Dates are numbers: the day after is one more.
	s.keys("=A1+1", "<enter>")
	s.waitForLine(gridRow1+3, numRow(4, "9/27/2026"))

	s.keys("1450", "<tab>", "612.4", "<enter>", "<up>", "<shift+right>", "<ctrl+shift+4>")
	s.waitForLine(gridRow1+4, numRow(5, "$1,450.00", "$612.40"))
	s.waitFor("Currency format for A5:B5")

	// Too wide next to a filled cell: #s, as in Sheets.
	s.keys("<ctrl+shift+5>", "<ctrl+b>")
	s.waitForLine(gridRow1+4, numRow(5, "#########", "61240.00%"))
	s.waitFor("Bold on for A5:B5")
	if !strings.Contains(s.html(), "font-weight:bold") {
		t.Errorf("no bold cells:\n%s", s.html())
	}
}

func TestFormatsSurviveSaveAndReopen(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	s.keys("1450", "<enter>", "<up>", "<ctrl+shift+1>", "<ctrl+i>")
	s.waitForLine(gridRow1, numRow(1, "1,450.00"))
	s.keys("<ctrl+s>", "formats", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - formats.012" })
	s.keys("<ctrl+q>")
	s.waitExit()

	r := start(t, dir, "formats.012")
	r.waitForLine(gridRow1, numRow(1, "1,450.00"))
	if !strings.Contains(r.html(), "font-style:italic") {
		t.Errorf("italic lost after reopening:\n%s", r.html())
	}
}

// A column's format travels when the column is copied, and formulas
// reading its blanks take it at once.
func TestColumnFormatTravelsWithCopy(t *testing.T) {
	s := start(t, "")
	s.keys("5", "<enter>", "7", "<enter>", "<ctrl+home>", "<right>", "=C9*2", "<enter>")
	s.waitForLine(gridRow1, numRow(1, "5", "0"))
	s.keys("<ctrl+home>", "<ctrl+space>", "<ctrl+shift+4>", "<ctrl+c>")
	s.waitFor("Copied A:A")
	s.keys("<esc>", "<right>", "<right>", "<ctrl+v>")
	s.waitFor("Pasted 1 column at C:C")
	s.waitForLine(gridRow1, numRow(1, "$5.00", "$0.00", "$5.00"))
	s.keys("<esc>", "<down>", "<down>", "<down>", "<down>", "3", "<enter>")
	s.waitForLine(gridRow1+4, numRow(5, "", "", "$3.00"))
}
