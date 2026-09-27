package e2e

import (
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

func TestNotes(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	s.keys("Rent", "<tab>", "1450", "<enter>", "<up>", "<right>")
	s.waitForName("B1")

	// Insert > Note (Shift+F2): typed on the context line, Alt+Enter for
	// a new line.
	s.keys("<shift+f2>")
	s.waitFor("Note on B1:")
	s.keys("Due on the 1st", "<alt+enter>", "autopay", "<enter>")
	s.waitFor("Note  Due on the 1st ↵ autopay")
	s.eventually("the note's mark", func() bool { return strings.Contains(s.line(gridRow1), "1450▝") })

	// Hovering the cell from elsewhere shows the note in a box.
	s.keys("<left>")
	s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonUnknown, colX(1), gridRow1, 0)
	s.waitFor("┌─ Note ─")
	s.waitFor("│ autopay")

	// It's saved with the sheet.
	s.keys("<ctrl+s>", "notes", "<enter>")
	s.eventually("saved", func() bool { return strings.Contains(s.line(29), "notes.012") && !strings.Contains(s.line(29), "modified") })
	s2 := start(t, dir, "notes.012")
	s2.keys("<right>")
	s2.waitFor("Note  Due on the 1st ↵ autopay")
}

func TestProtectedRange(t *testing.T) {
	s := start(t, "")
	s.keys("Price", "<enter>", "10", "<enter>", "<up>", "<shift+up>")
	s.waitForName("A1:A2")

	// Data > Protect sheets and ranges: protect the selection.
	s.keys("<alt+d>")
	s.waitFor("Protect sheets and ranges")
	s.keys("<esc>", "<ctrl+k>", "protect range", "<enter>")
	s.waitFor("Describe A1:A2 (optional):")
	s.keys("Prices", "<enter>")
	s.waitFor("A1:A2 is protected: edits to it ask first")

	// Typing into it asks first; Esc backs out, Enter goes on.
	s.keys("<esc>", "9")
	s.waitFor("A1:A2 is protected.")
	s.waitFor("Enter  Edit anyway")
	s.keys("<esc>")
	s.waitForBar("A2", "10")
	s.keys("9")
	s.waitFor("A1:A2 is protected.")
	s.keys("<enter>", "<enter>")
	s.waitForBar("A3", "")
	s.keys("<up>")
	s.waitForBar("A2", "9")

	// Undo brings back the value, and outside the range nothing asks.
	s.keys("<ctrl+z>")
	s.waitForBar("A2", "10")
	s.keys("<right>", "5", "<enter>")
	s.waitForBar("B3", "")
}
