package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// vimOn turns on File > Settings > Vim keys through the menus.
func vimOn(s *session) {
	s.t.Helper()
	s.keys("<alt+f>", "<up>", "<up>", "<right>", "<down>")
	s.waitFor("Move with hjkl")
	s.keys("<enter>")
	s.waitFor("NORMAL")
}

// Vim keys through a real terminal: letters move and act, counts size
// operators, : goes to cells and writes the file, and :q quits.
func TestVimKeys(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	vimOn(s)
	s.keys("i", "Rent", "<tab>", "i", "1450", "<enter>")
	s.keys("i", "Food", "<tab>", "a", "600", "<enter>")
	s.keys("=", "SUM(B1:B2)", "<enter>")
	s.waitForLine(gridRow1+2, numRow(3, "2050"))

	// Shift+letters, $ and counts arrive as typed. Pasted rows go in
	// below, widening the SUM; deleting rows narrows it again.
	s.keys("gg", "$")
	s.waitForBar("B1", "1450")
	s.keys("0", "yy", "2p")
	s.waitForBar("A2", "Rent")
	s.keys("G")
	s.waitForBar("A5", "=SUM(B1:B4)")
	s.waitForLine(gridRow1+4, numRow(5, "4950"))
	s.keys("2k", "2dd")
	s.waitForBar("A3", "=SUM(B1:B2)")
	s.waitForLine(gridRow1+2, numRow(3, "2900"))

	s.keys("V", "j")
	s.waitFor("VISUAL")
	s.keys("<esc>")
	s.waitFor("NORMAL")

	s.keys(":", "fill")
	s.waitFor("COMMAND")
	s.waitFor("edit.fill_down")
	s.keys("<esc>", ":B2", "<enter>")
	s.waitForBar("B2", "1450")
	s.keys("x")
	s.waitForBar("B2", "")
	s.keys("u")
	s.waitForBar("B2", "1450")

	s.keys(":w vim", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - vim.012" })
	if _, err := os.Stat(filepath.Join(dir, "vim.012")); err != nil {
		t.Fatal(err)
	}
	s.keys(":q", "<enter>")
	s.waitExit()
}
