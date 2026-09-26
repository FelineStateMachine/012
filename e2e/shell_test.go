package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// The app shell: menu bar, dropdowns, context menus, the command palette
// and the context-line confirmations.

func TestAltFOpensFileMenuAndSaves(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	s.keys("42", "<enter>", "<alt+f>")
	s.waitFor("│ Save as")
	s.waitFor("MENU")
	// "s" highlights Save (Save as also starts with s); Enter runs it.
	s.keys("s", "<enter>")
	s.waitFor("Save as: SHEET1.o23")
	s.keys("<enter>")
	s.eventually("saved file", func() bool {
		_, err := os.Stat(filepath.Join(dir, "SHEET1.o23"))
		return err == nil
	})
	s.waitFor("READY")
}

func TestMenuSetsColumnWidth(t *testing.T) {
	s := start(t, "")
	s.keys("<alt+o>", "c", "20", "<enter>")
	s.waitFor("READY")
	s.eventually("wider column A", func() bool { return strings.HasPrefix(s.line(3), strings.Repeat(" ", 6)+strings.Repeat(" ", 9)+"A") })
}

func TestPaletteSearchAndRun(t *testing.T) {
	s := start(t, "")
	s.keys("<ctrl+k>")
	s.waitFor("Search the menus")
	s.keys("goto")
	s.waitFor("│ › goto")
	s.keys("<enter>")
	s.waitFor("Go to: A1")
	s.keys("C5", "<enter>")
	s.waitForBar("C5", "")
}

func TestRightClickColumnMenu(t *testing.T) {
	s := start(t, "")
	// Right-click column C's header: it selects the column and opens the
	// column menu under the mouse.
	x := 6 + 2*10 + 4
	s.click(ghostty.MouseButtonRight, x, 3)
	s.waitFor("│ Resize column")
	s.waitFor("C1:C8192")
	s.keys("<esc>")
	s.waitFor("READY")

	// Clicking an item runs it.
	s.click(ghostty.MouseButtonRight, x, 3)
	s.waitFor("│ Resize column")
	for y, l := range strings.Split(s.screen(), "\n") {
		if strings.Contains(l, "│ Resize column") {
			s.click(ghostty.MouseButtonLeft, x+3, y)
		}
	}
	s.waitFor("Column width (1-240): 10")
}

func TestQuitAsksAboutUnsavedChanges(t *testing.T) {
	s := start(t, "")
	s.keys("1", "<enter>", "<ctrl+q>")
	s.waitFor("You have unsaved changes.")
	s.keys("<esc>")
	s.waitFor("READY")
	s.keys("<ctrl+q>", "d")
	s.waitExit()
}
