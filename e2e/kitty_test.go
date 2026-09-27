package e2e

import (
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// With the kitty keyboard protocol, as Ghostty has it, 012 turns on key
// events; Shift+Enter moves up after an entry and Ctrl+I is italic. A
// terminal without it sends Shift+Enter as Enter and Ctrl+I as Tab, and
// 012 does what those do.
func TestKittyKeyboard(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		s := start(t, "")
		s.eventually("key events on", func() bool {
			return s.kittyFlags()&ghostty.KittyKeyReportEvents != 0
		})
		s.keys("<down>", "<down>", "5", "<shift+enter>")
		s.waitForBar("A2", "")
		s.keys("<down>", "<ctrl+i>")
		s.waitFor("Italic on for A3")
		// Alt+letter opens its menu; Esc then a letter, however quick, is
		// Esc, then the letter typed.
		s.keys("<alt+f>")
		s.waitFor("MENU")
		s.keys("<esc>", "j")
		s.waitFor("ENTER")
		s.waitForBar("A3", "j")
	})
	t.Run("off", func(t *testing.T) {
		s := startWith(t, options{legacyKeys: true})
		if f := s.kittyFlags(); f != 0 {
			t.Fatalf("flags %v on a terminal without the protocol", f)
		}
		s.keys("<down>", "<down>", "5", "<shift+enter>")
		s.waitForBar("A4", "")
		s.keys("<up>", "<ctrl+i>")
		s.waitForBar("B3", "")
		s.keys("<alt+f>")
		s.waitFor("MENU")
	})
}

// Holding Space on a selected chart shows it across the grid until it's
// let go; without key events Space deselects the chart and starts an
// entry, as it always has.
func TestHoldSpaceOnChart(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		s := start(t, "")
		spending(s)
		insertChart(s)
		s.keys("<enter>")
		s.waitFor("hold Space to zoom")
		s.hold("<space>")
		s.eventually("chart over column A", func() bool {
			return !strings.Contains(s.line(gridRow1), "Month") && strings.Contains(s.screen(), "let go")
		})
		s.release("<space>")
		s.eventually("chart back at D1", func() bool { return strings.Contains(s.line(gridRow1), "Month") })
		s.waitFor("hold Space to zoom")
		if !strings.Contains(s.line(0), "CHART") {
			t.Errorf("released: %q", s.line(0))
		}
	})
	t.Run("off", func(t *testing.T) {
		s := startWith(t, options{legacyKeys: true})
		spending(s)
		insertChart(s)
		s.keys("<enter>")
		s.waitFor("drag the corner to resize")
		if strings.Contains(s.screen(), "hold Space") {
			t.Errorf("hint without key events:\n%s", s.screen())
		}
		s.keys(" ") // as typed: the terminal sends the space itself
		s.waitFor("ENTER")
	})
}

// Holding Space in the theme picker hides the list, showing the sheet in
// the highlighted theme, until it's let go; with a search typed, or
// without key events, Space is typed.
func TestHoldSpaceInThemePicker(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		s := start(t, "")
		openTheme(s)
		s.waitFor("hold to peek")
		s.hold("<space>")
		s.eventually("list hidden", func() bool { return !strings.Contains(s.screen(), "─ Theme ─") })
		s.release("<space>")
		s.waitFor("─ Theme ─")
		s.keys("tokyo", "<space>")
		s.waitFor("› tokyo ")
	})
	t.Run("off", func(t *testing.T) {
		s := startWith(t, options{legacyKeys: true})
		openTheme(s)
		if strings.Contains(s.screen(), "hold to peek") {
			t.Errorf("hint without key events:\n%s", s.screen())
		}
		s.keys(" x")
		s.waitFor("›  x")
	})
}
