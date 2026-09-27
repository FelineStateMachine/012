package e2e

import (
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// The trip plan's layout draws as a table, the notes wrapped over their
// rows, and comes back from its file as it was.
func TestLayoutDrawsAndSurvivesSave(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	tripPlan(s)
	checkTrip := func(s *session) {
		s.t.Helper()
		screen := s.screen()
		for _, want := range []string{"Trip to Lisbon", "┏━━━━━━━━━┯", "╠═════════╪", "│Tram 28  ┃", "┃Total    │", "┗━━━━━━━━━┷"} {
			if !strings.Contains(screen, want) {
				t.Errorf("no %q in\n%s", want, screen)
			}
		}
	}
	checkTrip(s)
	s.keys("<ctrl+s>", "trip", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - trip.012" })
	s.keys("<ctrl+q>")
	s.waitExit()

	r := start(t, dir, "trip.012")
	r.waitFor("Trip to Lisbon")
	checkTrip(r)
	// The merged title is one cell: clicking its middle selects A1, and
	// Right leaves it at D1.
	r.click(ghostty.MouseButtonLeft, 6+10+4, gridRow1)
	r.waitForBar("A1", "Trip to Lisbon")
	r.keys("<right>")
	r.waitForName("D1")
}

// Dragging a row number's corner makes the row taller; double-clicking
// it fits the row again.
func TestDragRowHeight(t *testing.T) {
	s := start(t, "")
	s.keys("Tall", "<enter>")
	s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonUnknown, 5, gridRow1, 0)
	s.waitFor("▄")
	s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, 5, gridRow1, 0)
	s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonLeft, 5, gridRow1+2, 0)
	s.waitFor("Row 1 height 3 lines")
	s.mouse(ghostty.MouseActionRelease, ghostty.MouseButtonLeft, 5, gridRow1+2, 0)
	// The value sits on the row's last line, beside its number.
	s.waitForLine(gridRow1+2, "    1  Tall")
	s.waitForLine(gridRow1+3, "    2")
	s.click(ghostty.MouseButtonLeft, 5, gridRow1+2)
	s.click(ghostty.MouseButtonLeft, 5, gridRow1+2)
	s.waitForLine(gridRow1, "    1  Tall")
}
