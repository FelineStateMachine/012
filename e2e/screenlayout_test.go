package e2e

import (
	"strings"

	ghostty "go.mitchellh.com/libghostty"
)

// Golden screens for the grid's layout: a trip plan with its title merged
// across the table, borders around and inside it (a double line under
// the headers, a thick outline), notes that wrap and grow their rows, and
// a row made taller by hand; the question before a merge clears values,
// the Borders menu, and a row's height being dragged.

// palette runs a command by searching the palette for it.
func palette(s *session, search string) {
	s.keys("<ctrl+k>", search, "<enter>")
}

// tripPlan types the plan and lays it out through the Format menu's
// commands, ending on the notes column.
func tripPlan(s *session) {
	s.keys("Trip to Lisbon", "<enter>")
	s.keys("Day", "<tab>", "Cost", "<tab>", "Notes", "<enter>")
	s.keys("Fri", "<tab>", "120", "<tab>", "Train, book the early fare", "<enter>")
	s.keys("Sat", "<tab>", "45", "<tab>", "Tram 28 and the castle, before the crowds", "<enter>")
	s.keys("Sun", "<tab>", "60", "<tab>", "Fly home", "<enter>")
	s.keys("Total", "<tab>", "=SUM(B3:B5)", "<enter>")
	// The title across the table.
	s.keys("<ctrl+home>", "<shift+right>", "<shift+right>")
	palette(s, "merge all")
	s.waitFor("Merged A1:C1")
	// Lines inside, a double one under the headers, a thick outline.
	s.keys("<down>", "<shift+down>", "<shift+down>", "<shift+down>", "<shift+down>", "<shift+right>", "<shift+right>")
	palette(s, "all borders")
	s.waitFor("All borders for A2:C6")
	palette(s, "thick lines")
	s.keys("<alt+shift+7>")
	s.waitFor("Outer borders for A2:C6")
	s.keys("<esc>", "<shift+right>", "<shift+right>")
	palette(s, "double lines")
	s.keys("<alt+shift+3>")
	s.waitFor("Bottom border for A2:C2")
	// The notes wrap, and the total's row is two lines tall.
	s.keys("<esc>", "<down>", "<right>", "<right>", "<shift+down>", "<shift+down>")
	palette(s, "wrap")
	s.waitFor("Wrapping: wrap for C3:C5")
	s.keys("<esc>", "<down>", "<down>", "<down>")
	palette(s, "row height")
	s.keys("2", "<enter>")
	s.keys("<up>", "<up>")
	s.waitForBar("C4", "Tram 28 and the castle, before the crowds")
}

// coloredPlan is the trip plan with its outline drawn again in blue and
// the total's values in the middle of its tall row.
func coloredPlan(s *session) {
	tripPlan(s)
	s.keys("<down>", "<down>", "<left>", "<left>")
	palette(s, "row height")
	s.keys("<right>", "<enter>")
	s.keys("<shift+right>", "<shift+right>")
	palette(s, "align middle")
	s.waitFor("Aligned to the middle")
	s.keys("<esc>", "<ctrl+home>", "<down>")
	for range 4 {
		s.keys("<shift+down>")
	}
	s.keys("<shift+right>", "<shift+right>")
	palette(s, "border color")
	s.waitFor("Automatic")
	s.keys("blue", "<enter>")
	s.waitFor("Borders draw in blue now")
	palette(s, "thick lines")
	s.keys("<alt+shift+7>")
	s.waitFor("Outer borders for A2:C6")
	s.keys("<esc>", "<down>")
	s.waitForBar("A3", "Fri")
}

var layoutScreens = []screen{
	{name: "layout", setup: tripPlan},
	{name: "layout-colors", setup: coloredPlan},
	{name: "layout-colors-high-contrast", opts: options{config: highContrastConfig}, setup: coloredPlan},
	{name: "border-color-picker", setup: func(s *session) {
		budget(s)
		palette(s, "border color")
		s.waitFor("Automatic")
		s.keys("<down>", "<down>")
	}},
	{name: "layout-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		tripPlan(s)
		s.keys("<ctrl+home>") // the pointer on the merged title
		s.waitForBar("A1", "Trip to Lisbon")
	}},
	{name: "layout-high-contrast", opts: options{config: highContrastConfig}, setup: tripPlan},
	{name: "layout-merge-confirm", setup: func(s *session) {
		budget(s)
		s.keys("<up>", "<up>", "<up>", "<left>", "<shift+right>", "<shift+down>")
		palette(s, "merge all")
		s.waitFor("keeps only the top-left value")
	}},
	{name: "menu-format-borders", setup: func(s *session) {
		budget(s)
		s.keys("<alt+o>", "b", "b", "<right>")
		s.waitFor("│ Double lines")
	}},
	{name: "resizing-row", setup: func(s *session) {
		budget(s)
		s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, 5, gridRow1+2, 0)
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonLeft, 5, gridRow1+4, 0)
		s.waitFor("Row 3 height 3 lines")
	}},
}

func init() {
	for _, sc := range layoutScreens {
		screens = append(screens, sc)
		if sc.name != "layout-narrow" && !strings.HasSuffix(sc.name, "-high-contrast") {
			sc.name += "-light"
			sc.opts.light = true
			screens = append(screens, sc)
		}
	}
}
