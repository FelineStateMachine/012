package e2e

import ghostty "go.mitchellh.com/libghostty"

// Golden screens for working across sheets: hiding a sheet and the list
// of hidden ones, sheet names in formula suggestions, and where File >
// Import puts a file.

// hideSummary builds the two-sheet budget and hides Summary from its
// tab's menu.
func hideSummary(s *session) {
	summary(s)
	s.click(ghostty.MouseButtonRight, tabX(s.t, s, "Summary"), int(s.rows)-1)
	s.waitFor("Hide sheet")
	s.keys("<down>", "<down>", "<down>", "<enter>")
	s.waitFor("Hid Summary")
}

// traceHidden hides Sheet1 and traces the precedents of Summary!B1,
// which reads only Sheet1.
func traceHidden(s *session) {
	summary(s)
	s.click(ghostty.MouseButtonRight, tabX(s.t, s, "Sheet1"), int(s.rows)-1)
	s.waitFor("Hide sheet")
	s.keys("<down>", "<down>", "<down>", "<enter>")
	s.waitFor("Hid Sheet1")
	s.waitForBar("B1", "=Sheet1!B7*12")
	s.keys("<alt+,>")
	s.waitFor("Reads only Sheet1, a hidden sheet; View > Hidden sheets shows it")
}

// importLocation opens the import location picker for budget.tsv on a
// spreadsheet with something in it.
func importLocation(s *session) {
	s.keys("Notes", "<enter>")
	openImportPicker(s)
	s.keys("tsv", "<enter>")
	s.waitFor("Import location")
}

var sheetScreens = []screen{
	{name: "sheets-menu-hide", setup: func(s *session) {
		summary(s)
		s.click(ghostty.MouseButtonRight, tabX(s.t, s, "Summary"), int(s.rows)-1)
		s.waitFor("Hide sheet")
		s.keys("<down>", "<down>", "<down>")
	}},
	{name: "sheets-hidden", setup: hideSummary},
	{name: "sheets-hidden-picker", setup: func(s *session) {
		hideSummary(s)
		s.keys("<alt+v>", "<down>")
		s.waitFor("List the hidden sheets")
		s.keys("<enter>")
		s.waitFor("1 of 1")
	}},
	{name: "trace-hidden", setup: traceHidden},
	{name: "autocomplete-sheet", setup: func(s *session) {
		summary(s)
		s.keys("<ctrl+pgup>", "<down>", "<down>", "<down>", "=B7/sum")
		s.waitFor("│ Summary!")
	}},
	{name: "autocomplete-sheet-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		summary(s)
		s.keys("<ctrl+pgup>", "<down>", "<down>", "<down>", "=su")
		s.waitFor("│ Summary!")
	}},
	{name: "autocomplete-sheet-point", setup: func(s *session) {
		summary(s)
		s.keys("<down>", "<down>", "<left>", "Rent share", "<tab>", "=sh", "<tab>", "<up>")
		s.waitFor("POINT")
	}},
	{name: "import-location", files: importDir, setup: importLocation},
	{name: "import-location-narrow", opts: options{cols: 60, rows: 16}, files: importDir, setup: importLocation},
	{name: "import-new-sheets", files: importDir, setup: func(s *session) {
		s.keys("Notes", "<enter>")
		openImportPicker(s)
		s.keys("q3", "<enter>")
		s.waitFor("Import location")
		s.keys("<enter>")
		s.waitFor("Imported q3.xlsx as")
	}},
}

func init() {
	light := map[string]bool{"sheets-hidden-picker": true, "autocomplete-sheet": true, "import-location": true}
	for _, sc := range sheetScreens {
		screens = append(screens, sc)
		if light[sc.name] {
			sc.name += "-light"
			sc.opts.light = true
			screens = append(screens, sc)
		}
	}
}
