package e2e

import "testing"

// A million rows through a real terminal: whole-column formulas, a jump
// to the last row and three-letter columns, with row numbers widening to
// fit.
func TestMillionRows(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.keys("5", "<enter>", "7", "<enter>", "<right>", "<up>", "<up>", "=SUM(A:A)", "<enter>")
	s.waitFor("12")
	s.keys("<up>")
	s.waitForBar("B1", "=SUM(A:A)")

	// Ctrl+Down from B1, with nothing below, lands on the last row.
	s.keys("<ctrl+down>")
	s.waitForBar("B1048576", "")
	s.waitFor(" 1048576")
	s.keys("=A1*2", "<enter>")
	s.keys("<f5>", "A1", "<enter>", "100", "<enter>")
	s.keys("<f5>", "B1048576", "<enter>")
	s.waitForBar("B1048576", "=A1*2")
	s.waitFor("200")

	// Three-letter columns, to the last.
	s.keys("<f5>", "AAA9", "<enter>", "=ROWS(C:C)", "<enter>")
	s.keys("<f5>", "AAA9", "<enter>")
	s.waitForBar("AAA9", "=ROWS(C:C)")
	s.waitFor("1048576")
	s.keys("<ctrl+right>")
	s.waitForBar("XFD9", "")
}
