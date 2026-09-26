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

// numRow is how a grid row of numbers in default-width (10) columns looks:
// a 6-wide row header, then each number right-aligned with one column of
// padding.
func numRow(row int, nums ...string) string {
	line := fmt.Sprintf("%5d ", row)
	for _, n := range nums {
		line += fmt.Sprintf("%9s ", n)
	}
	return strings.TrimRight(line, " ")
}

func TestFormulaRecalc(t *testing.T) {
	s := start(t, "")
	s.keys("10", "<enter>", "20", "<enter>", "=SUM(A1:A2)", "<enter>")
	s.keys("<up>")
	s.waitFor("A3   =SUM(A1:A2)")
	s.waitForLine(gridRow1+2, numRow(3, "30"))

	// Changing a precedent recalculates the total on screen.
	s.keys("<ctrl+home>", "5", "<enter>")
	s.waitForLine(gridRow1+2, numRow(3, "25"))
}

func TestPointModeBuildsFormula(t *testing.T) {
	s := start(t, "")
	s.keys("4", "<tab>", "6", "<tab>", "=", "<left>", "<left>")
	s.waitFor("POINT")
	s.waitForLine(1, "=A1")
	s.keys("*", "<left>", "<enter>")
	// Enter after a run of Tabs returns to the starting column, as in Sheets.
	s.keys("<up>", "<right>", "<right>")
	s.waitFor("C1   =A1*B1")
	s.waitForLine(gridRow1, numRow(1, "4", "6", "24"))
}

func TestShiftSelectionShowsStats(t *testing.T) {
	s := start(t, "")
	s.keys("1", "<enter>", "2", "<enter>", "3", "<enter>", "<ctrl+home>")
	s.keys("<shift+down>", "<shift+down>")
	s.eventually("selection stats", func() bool {
		l := s.line(29)
		return strings.Contains(l, "A1:A3") && strings.Contains(l, "Sum 6") && strings.Contains(l, "Count 3")
	})
	s.keys("<delete>")
	s.waitForLine(gridRow1, "    1")
}

func TestTextOverflow(t *testing.T) {
	s := start(t, "")
	s.keys("Quarterly revenue", "<enter>", "$1,200", "<enter>", "TRUE", "<enter>")
	s.waitForLine(gridRow1, "    1  Quarterly revenue")
	s.waitForLine(gridRow1+1, numRow(2, "1200"))
	s.waitForLine(gridRow1+2, "    3    TRUE")
}

func TestInvalidFormulaCursor(t *testing.T) {
	s := start(t, "")
	s.keys("=SUM(A1", "<enter>")
	s.waitFor("EDIT")
	s.waitFor("Expected , or ) in SUM")
	// The real terminal cursor sits on the edit line at the error.
	s.eventually("cursor at end of edit line", func() bool {
		x, y := s.cursor()
		return x == len("=SUM(A1") && y == 1
	})
}

func TestMenuSetsColumnWidth(t *testing.T) {
	s := start(t, "")
	s.keys("<f10>")
	s.waitFor("File  Edit  Format")
	s.keys("f", "<enter>", "c", "20", "<enter>")
	s.waitFor("READY")
	s.eventually("wider column A", func() bool { return strings.HasPrefix(s.line(3), strings.Repeat(" ", 6)+strings.Repeat(" ", 9)+"A") })
}

func TestSaveQuitReopen(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	s.keys("Budget", "<enter>", "1200", "<enter>", "=A2*12", "<enter>")
	s.keys("<ctrl+s>", "budget", "<enter>")
	s.eventually("title update", func() bool { return s.title() == "one23 - budget.o23" })
	if _, err := os.Stat(filepath.Join(dir, "budget.o23")); err != nil {
		t.Fatal(err)
	}

	s.keys("<ctrl+q>")
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

func TestQuitAsksAboutUnsavedChanges(t *testing.T) {
	s := start(t, "")
	s.keys("1", "<enter>", "<ctrl+q>")
	s.waitFor("Cancel  Quit without saving")
	s.keys("<esc>", "<esc>")
	s.waitFor("READY")
}

func TestResizeShowsMoreColumns(t *testing.T) {
	s := start(t, "")
	s.waitFor(" I")
	if strings.Contains(s.line(3), " N") {
		t.Fatalf("column N visible at 100 columns: %q", s.line(3))
	}
	s.resize(160, 30)
	s.eventually("column N after resize", func() bool { return strings.Contains(s.line(3), " N") })
}

func TestCopyPasteAdjustsReferences(t *testing.T) {
	s := start(t, "")
	clip := s.watchClipboard()
	s.keys("2", "<tab>", "=A1*10", "<enter>", "3", "<enter>", "<ctrl+home>", "<right>", "<ctrl+c>")
	s.waitFor("Copied B1")
	s.eventually("system clipboard", func() bool { return clip.get() == "20" })
	s.keys("<down>", "<ctrl+v>")
	s.waitFor("Pasted 1 cell at B2")
	s.waitFor("B2   =A2*10")
	s.waitForLine(gridRow1+1, numRow(2, "3", "30"))

	// Esc clears the marker; the paste stays.
	s.keys("<esc>")
	s.eventually("marker cleared", func() bool { return !strings.Contains(s.line(2), "Copied") })

	// Cut and paste moves the cell and the formula that uses it follows.
	s.keys("<ctrl+home>", "<ctrl+x>", "<down>", "<down>", "<down>", "<ctrl+v>")
	s.waitFor("Moved A1 to A4")
	s.keys("<ctrl+home>", "<right>")
	s.waitFor("B1   =A4*10")
	s.waitForLine(gridRow1+3, numRow(4, "2"))
}

func TestUndoRedo(t *testing.T) {
	s := start(t, "")
	s.keys("10", "<enter>", "=A1+1", "<enter>", "<up>", "<up>", "<shift+down>", "<delete>")
	s.waitForLine(gridRow1, "    1")
	s.keys("<ctrl+z>")
	s.waitFor("Undid: clear A1:A2")
	s.waitForLine(gridRow1+1, numRow(2, "11"))
	s.keys("<ctrl+y>")
	s.waitFor("Redid: clear A1:A2")
	s.waitForLine(gridRow1+1, "    2")
	s.keys("<ctrl+shift+z>")
	s.waitFor("Nothing to redo")
}

func TestInsertRowRewritesFormulas(t *testing.T) {
	s := start(t, "")
	s.keys("1", "<enter>", "2", "<enter>", "=SUM(A1:A2)", "<enter>")
	s.keys("<up>", "<up>", "<shift+space>", "<ctrl+alt+=>")
	s.waitForLine(gridRow1+3, numRow(4, "3"))
	s.keys("<down>", "<down>")
	s.waitFor("A4   =SUM(A1:A3)")
	s.keys("<up>", "<up>", "5", "<enter>")
	s.waitForLine(gridRow1+3, numRow(4, "8"))
	s.keys("<ctrl+z>", "<ctrl+z>")
	s.waitFor("Undid: insert 1 row")
	s.waitForLine(gridRow1+2, numRow(3, "3"))
}

func TestPasteTSVFillsCells(t *testing.T) {
	s := start(t, "")
	s.paste("Item\tCost\nRent\t1450\nFood\t=B2/2\n")
	s.waitFor("Pasted 6 cells at A1:B3")
	s.waitForLine(gridRow1+2, "    3  Food           725")
}

func TestFillAndAbsoluteReferences(t *testing.T) {
	s := start(t, "")
	s.keys("2", "<enter>", "3", "<enter>", "4", "<enter>", "<ctrl+home>", "<right>")
	s.keys("=A1*A1", "<left>", "<left>", "<f4>")
	s.waitForLine(1, "=A1*$A$1")
	s.keys("<enter>", "<up>", "<shift+down>", "<shift+down>", "<ctrl+d>")
	s.waitForLine(gridRow1+2, numRow(3, "4", "8"))
	s.keys("<right>", "<shift+down>", "=B1+1", "<ctrl+enter>")
	s.waitForLine(gridRow1+1, numRow(2, "3", "6", "7"))
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
