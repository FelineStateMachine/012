package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// Screen layout: panel lines 0-2, column header 3, grid row 1 on line 4.
const gridRow1 = 4

// numRow is how a grid row of numbers in default-width (9) columns looks:
// a 6-wide row header, then each number right-aligned with one trailing
// space, as 1-2-3's General format shows them.
func numRow(row int, nums ...string) string {
	line := fmt.Sprintf("%-6d", row)
	for _, n := range nums {
		line += fmt.Sprintf("%8s ", n)
	}
	return strings.TrimRight(line, " ")
}

func TestFormulaRecalc(t *testing.T) {
	s := start(t, "")
	s.keys("10", "<down>", "20", "<down>", "@SUM(A1..A2)", "<enter>")
	s.waitFor("A3: @SUM(A1..A2)")
	s.waitForLine(gridRow1+2, numRow(3, "30"))

	// Changing a precedent recalculates the total on screen.
	s.keys("<home>", "5", "<enter>")
	s.waitForLine(gridRow1+2, numRow(3, "25"))
}

func TestPointModeBuildsFormula(t *testing.T) {
	s := start(t, "")
	s.keys("4", "<right>", "6", "<right>", "+", "<left>", "<left>")
	s.waitFor("POINT")
	s.waitForLine(1, "+A1")
	s.keys("*", "<left>", "<enter>")
	s.waitFor("C1: +A1*B1")
	s.waitForLine(gridRow1, numRow(1, "4", "6", "24"))
}

func TestLabelsSpillAndAlign(t *testing.T) {
	s := start(t, "")
	s.keys("Quarterly revenue", "<down>", `"right`, "<down>", "^mid", "<enter>")
	s.waitForLine(gridRow1, "1     Quarterly revenue")
	s.waitForLine(gridRow1+1, "2         right")
	s.waitForLine(gridRow1+2, "3        mid")
}

func TestInvalidFormulaCursor(t *testing.T) {
	s := start(t, "")
	s.keys("@SUM(A1", "<enter>")
	s.waitFor("EDIT")
	s.waitFor("expected , or )")
	// The real terminal cursor sits on the edit line at the error.
	s.eventually("cursor at end of edit line", func() bool {
		x, y := s.cursor()
		return x == len("@SUM(A1") && y == 1
	})
}

func TestMenuFirstLetterSetsWidth(t *testing.T) {
	s := start(t, "")
	s.keys("/")
	s.waitFor("Worksheet  Range  File  Quit")
	s.keys("wcs", "20", "<enter>")
	s.waitFor("A1: [W20]")
}

func TestSaveQuitReopen(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	s.keys("Budget", "<down>", "1200", "<down>", "+A2*12", "<enter>")
	s.keys("/fs", "budget", "<enter>")
	s.eventually("title update", func() bool { return s.title() == "one23 - budget.o23" })
	if _, err := os.Stat(filepath.Join(dir, "budget.o23")); err != nil {
		t.Fatal(err)
	}

	s.keys("/qy")
	s.waitExit()
	// Leaving the alternate screen restores the user's shell screen.
	if scr := s.activeScreen(); scr != ghostty.ScreenPrimary {
		t.Errorf("active screen after exit = %v, want primary", scr)
	}

	r := start(t, dir, "budget.o23")
	r.waitForLine(gridRow1+2, numRow(3, "14400"))
	if !strings.Contains(r.screen(), "budget.o23") {
		t.Errorf("status line missing file name:\n%s", r.screen())
	}
}

func TestResizeShowsMoreColumns(t *testing.T) {
	s := start(t, "")
	s.waitFor(" J")
	if strings.Contains(s.line(3), " P") {
		t.Fatalf("column P visible at 100 columns: %q", s.line(3))
	}
	s.resize(160, 30)
	s.eventually("column P after resize", func() bool { return strings.Contains(s.line(3), " P") })
}

// Click handling is covered by the unit tests; this checks the program
// actually turned on mouse reporting in the terminal.
func TestMouseTrackingEnabled(t *testing.T) {
	s := start(t, "")
	s.eventually("mouse tracking enabled", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		on, _ := s.vt.MouseTracking()
		return on
	})
}
