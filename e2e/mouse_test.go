package e2e

import (
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// leftClick presses and releases the left button at a screen cell, with
// modifiers.
func (s *session) leftClick(col, row int, mods ghostty.Mods) {
	s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, col, row, mods)
	s.mouse(ghostty.MouseActionRelease, ghostty.MouseButtonLeft, col, row, mods)
}

// drag presses at one cell, moves through the path and releases at the end.
func (s *session) drag(path ...[2]int) {
	first, last := path[0], path[len(path)-1]
	s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, first[0], first[1], 0)
	for _, p := range path[1:] {
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonLeft, p[0], p[1], 0)
	}
	s.mouse(ghostty.MouseActionRelease, ghostty.MouseButtonLeft, last[0], last[1], 0)
}

// wheel scrolls at a cell; up is button 4, down is button 5.
func (s *session) wheel(col, row, clicks int) {
	button := ghostty.MouseButtonFive
	if clicks < 0 {
		button, clicks = ghostty.MouseButtonFour, -clicks
	}
	for range clicks {
		s.mouse(ghostty.MouseActionPress, button, col, row, 0)
	}
}

// Screen x of a cell in column c with default widths and no scrolling.
func colX(c int) int { return 6 + c*10 + 2 }

func TestMouseClickDragAndShiftClick(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.leftClick(colX(1), gridRow1+2, 0)
	s.waitForName("B3")
	s.drag([2]int{colX(0), gridRow1}, [2]int{colX(1), gridRow1 + 1}, [2]int{colX(2), gridRow1 + 2})
	s.eventually("drag selection", func() bool { return strings.Contains(s.line(29), "A1:C3") })
	s.leftClick(colX(3), gridRow1+4, ghostty.ModShift)
	s.eventually("shift+click extends", func() bool { return strings.Contains(s.line(29), "A1:D5") })

	// The app asked the terminal to pass Shift+click through.
	s.mu.Lock()
	raw := string(s.raw)
	s.mu.Unlock()
	if !strings.Contains(raw, "\x1b[>1s") {
		t.Error("XTSHIFTESCAPE not requested")
	}
}

func TestMouseClickWhileTypingAndFormulaReference(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.keys("40", "<enter>", "2", "<enter>", "=")
	s.leftClick(colX(0), gridRow1, 0)
	s.waitForEntry("=A1")
	s.keys("+")
	s.leftClick(colX(0), gridRow1+1, 0)
	s.waitForEntry("=A1+A2")
	s.keys("<enter>")
	s.waitForLine(gridRow1+2, numRow(3, "42"))

	// Clicking another cell while typing plain text accepts it there.
	s.keys("note")
	s.leftClick(colX(2), gridRow1, 0)
	s.waitForName("C1")
	s.waitForLine(gridRow1+3, "    4  note")
}

func TestMouseResizeAndWheel(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	border := 6 + 10 - 1
	s.drag([2]int{border, 3}, [2]int{border + 4, 3}, [2]int{border + 8, 3})
	s.eventually("column A wider", func() bool {
		return strings.HasPrefix(s.line(3), strings.Repeat(" ", 6)+strings.Repeat(" ", 8)+"A")
	})
	s.wheel(colX(2), gridRow1+5, 4)
	s.waitForLine(gridRow1, "   13")
	// The active cell didn't move; arrowing to A2 scrolls just enough to
	// show it again.
	s.keys("<down>")
	s.waitForLine(gridRow1, "    2")
}

func TestMouseDoubleClickEdits(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.keys("hello", "<enter>")
	s.leftClick(colX(0), gridRow1, 0)
	s.leftClick(colX(0), gridRow1, 0)
	s.waitFor("EDIT")
	s.waitForEntry("hello")
}
