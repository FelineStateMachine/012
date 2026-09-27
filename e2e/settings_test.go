package e2e

import "testing"

// money types a few amounts where binary floating point is off by a
// hair: whole cents of $4.35 come to 434, $0.10 plus $0.20 isn't 0.3,
// and the difference shows in General format.
func money(s *session) {
	s.keys("Coffee", "<tab>", "$4.35", "<enter>")
	s.keys("Tip", "<tab>", "$0.10", "<enter>")
	s.keys("Fee", "<tab>", "$0.20", "<enter>")
	s.keys("Cents", "<tab>", "=INT(B1*100)", "<enter>")
	s.keys("Is 0.30?", "<tab>", "=B2+B3=0.3", "<enter>")
	s.keys("Residue", "<tab>", "=0.1+0.2-0.3", "<enter>")
	s.waitForBar("A7", "")
}

// Decimal arithmetic from the palette recalculates at once, shows on the
// status line, and is saved with the file.
func TestDecimalArithmeticSetting(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	money(s)
	s.waitForLine(gridRow1+3, "    4  Cents      $434.00")
	s.waitForLine(gridRow1+4, "    5  Is 0.30?   FALSE")
	s.waitForLine(gridRow1+5, "    6  Residue  5.551E-17")
	s.keys("<ctrl+k>", "decimal", "<enter>")
	s.waitFor("Decimal arithmetic on")
	s.waitForLine(gridRow1+3, "    4  Cents      $435.00")
	s.waitForLine(gridRow1+4, "    5  Is 0.30?    TRUE")
	s.waitForLine(gridRow1+5, "    6  Residue          0")
	s.waitFor("modified  decimal")

	s.keys("<ctrl+s>", "money", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - money.012" })
	s.keys("<ctrl+q>")
	s.waitExit()

	r := start(t, dir, "money.012")
	r.waitForLine(gridRow1+3, "    4  Cents      $435.00")
	r.waitFor("money.012  decimal")
}
