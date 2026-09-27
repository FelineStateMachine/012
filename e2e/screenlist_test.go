package e2e

import ghostty "go.mitchellh.com/libghostty"

// The golden screens: every UI state recorded, how to reach it, and which
// are recorded on a light terminal too. The fixtures they share are in
// screensetup_test.go; TestScreens in screens_test.go records them.

var screens = []screen{
	{name: "ready-empty", setup: func(s *session) {}},
	{name: "budget", setup: budget},
	{name: "entry-formula", setup: func(s *session) {
		budget(s)
		s.keys("<down>", "=B7*12")
		s.waitFor("ENTER")
	}},
	{name: "point-range", setup: func(s *session) {
		budget(s)
		s.keys("<right>", "=AVERAGE(", "<left>", "<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>")
		s.waitFor("=AVERAGE(B3:B5")
	}},
	{name: "selection-stats", setup: func(s *session) {
		budget(s)
		s.keys("<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>")
		s.waitFor("Sum 2158.4")
	}},
	{name: "column-select", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+space>")
		s.waitFor("B1:B8192")
	}},
	{name: "edit-error", setup: func(s *session) {
		s.keys("=SUM(A1", "<enter>")
		s.waitFor("Expected , or ) in SUM")
	}},
	{name: "menu", setup: func(s *session) {
		budget(s)
		s.keys("<alt+f>", "<down>", "<down>", "<down>")
		s.waitFor("Save the sheet")
	}},
	{name: "palette", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>")
		s.waitFor("Search the menus")
	}},
	{name: "palette-search", setup: func(s *session) {
		budget(s)
		s.keys("<alt+/>", "col")
		s.waitFor("│ › col")
	}},
	{name: "palette-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>", "sa")
		s.waitFor("│ › sa")
	}},
	{name: "context-menu", setup: func(s *session) {
		budget(s)
		s.click(ghostty.MouseButtonRight, 6+10+4, 4+3)
		s.waitFor("│ Clear")
	}},
	{name: "context-menu-column", setup: func(s *session) {
		budget(s)
		s.click(ghostty.MouseButtonRight, 6+10+4, 3)
		s.waitFor("│ Resize column")
	}},
	{name: "menu-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<alt+h>")
		s.waitFor("│ About 012")
	}},
	{name: "functions", setup: func(s *session) {
		budget(s)
		s.keys("<alt+h>", "f", "if")
		s.waitFor("│ › if")
	}},
	{name: "about", setup: func(s *session) {
		s.keys("<alt+h>", "a")
		s.waitFor("Google Sheets keys")
	}},
	{name: "help-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		s.keys("<f1>")
		s.waitFor("Keyboard shortcuts")
	}},
	{name: "palette-wide", opts: options{cols: 200, rows: 30}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>", "sel")
		s.waitFor("│ › sel")
	}},
	{name: "help-wide", opts: options{cols: 200, rows: 45}, setup: func(s *session) {
		s.keys("<f1>")
		s.waitFor("Keyboard shortcuts")
	}},
	{name: "quit-confirm", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+q>")
		s.waitFor("unsaved changes")
	}},
	{name: "prompt-width", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+home>", "<alt+o>", "c", "<enter>", "<right>", "<right>", "<right>")
		s.waitFor("Column width (1-240): 13")
	}},
	{name: "error-goto", setup: func(s *session) {
		s.keys("<f5>", "nope", "<enter>")
		s.waitFor("Not a cell, range or named range")
	}},
	{name: "help", setup: func(s *session) {
		s.keys("<f1>")
		s.waitFor("HELP")
	}},
	{name: "narrow", opts: options{cols: 60, rows: 16}, setup: budget},
	{name: "hover-resize-handle", setup: func(s *session) {
		budget(s)
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonUnknown, 6+10-1, 3, 0)
		s.waitFor("▐")
	}},
	{name: "resizing-column", setup: func(s *session) {
		budget(s)
		s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, 6+10-1, 3, 0)
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonLeft, 6+10+5, 3, 0)
		s.waitFor("Column A width 16")
	}},
	{name: "copy-marker", setup: func(s *session) {
		budget(s)
		s.keys("<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<ctrl+c>", "<right>", "<up>")
		s.waitFor("Copied B3:B5")
	}},
	{name: "copy-marker-selected", setup: func(s *session) {
		budget(s)
		s.keys("<left>", "<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<shift+right>", "<ctrl+x>")
		s.waitFor("Cut A3:B5")
	}},
	{name: "undo-note", setup: func(s *session) {
		budget(s)
		s.keys("<up>", "<shift+up>", "<delete>", "<ctrl+z>")
		s.waitFor("Undid: clear B5:B6")
	}},
	{name: "narrow-copy", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+c>", "<down>")
		s.waitFor("Copied B7")
	}},
	{name: "formats", setup: formatted},
	{name: "jev", opts: options{jev: true}, setup: reviews},
	{name: "find", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+f>", "r")
		s.waitFor("1 of 4")
	}},
	{name: "replace", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+home>", "<shift+down>", "<shift+down>", "<shift+down>", "<shift+down>", "<shift+down>", "<shift+down>")
		s.keys("<ctrl+h>", "Rent", "<tab>", "Lease", "<alt+c>")
		s.waitFor("in A1:A7")
	}},
	{name: "find-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+h>", "tr", "<tab>", "x")
		s.waitFor("Replace")
	}},
	{name: "menu-format-number", setup: func(s *session) {
		formatted(s)
		s.keys("<alt+o>", "<right>")
		s.waitFor("Currency rounded")
	}},
	{name: "formats-narrow", opts: options{cols: 60, rows: 16}, setup: formatted},
	{name: "autocomplete", setup: func(s *session) {
		budget(s)
		s.keys("<down>", "=B7/su")
		s.waitFor("│ SUM ")
	}},
	{name: "autocomplete-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<down>", "=MAX(B7,co")
		s.waitFor("│ COUNT ")
	}},
	{name: "signature", setup: func(s *session) {
		budget(s)
		s.keys("<down>", `=SUMIF(A3:A5, "R*", `)
		s.waitFor("criterion, [sum_range])")
	}},
	{name: "named-ranges", setup: func(s *session) {
		named(s)
		s.keys("<alt+d>", "n")
		s.waitFor("│ › Type a name")
	}},
	{name: "trace-precedents", setup: func(s *session) {
		named(s)
		s.keys("<alt+,>")
		s.waitFor("precedents of B8")
	}},
	{name: "chart", setup: func(s *session) {
		spending(s)
		insertChart(s)
		s.keys("<enter>", "<esc>")
		s.waitFor("READY")
	}},
	{name: "chart-editor", setup: func(s *session) {
		spending(s)
		insertChart(s)
	}},
	{name: "chart-selected", setup: func(s *session) {
		spending(s)
		insertChart(s)
		s.keys("<enter>")
		s.waitFor("Column chart of A1:C5")
	}},
	{name: "chart-bar", setup: func(s *session) { chartOfType(s, 1, "Bar chart") }},
	{name: "chart-line", setup: func(s *session) { chartOfType(s, 2, "Line chart") }},
	{name: "chart-pie", setup: func(s *session) { chartOfType(s, 3, "Pie chart") }},
	{name: "chart-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		spending(s)
		insertChart(s)
	}},
	{name: "chart-wide", opts: options{cols: 200, rows: 30}, setup: func(s *session) {
		spending(s)
		insertChart(s)
		s.keys("<enter>", "<esc>")
		s.waitFor("READY")
	}},
	{name: "links-errors", setup: func(s *session) {
		s.keys("Docs", "<tab>", "https://example.com/docs", "<enter>")
		s.keys("Help", "<tab>", `=HYPERLINK("example.org/help", "Help center")`, "<enter>")
		s.keys("Ratio", "<tab>", "=B5/0", "<enter>")
		s.keys("Total", "<tab>", "=B3*2", "<enter>")
		s.keys("<up>", "<right>")
		s.waitFor("From B3: division by zero in B5/0")
	}},
	{name: "decimal", setup: func(s *session) {
		money(s)
		s.keys("<ctrl+k>", "decimal", "<enter>")
		s.waitFor("Decimal arithmetic on")
	}},
	{name: "menu-settings", setup: func(s *session) {
		money(s)
		s.keys("<ctrl+k>", "decimal", "<enter>")
		s.waitFor("Decimal arithmetic on")
		s.keys("<alt+f>", "<up>", "<up>", "<right>")
		s.waitFor("Compute money exactly")
	}},
	{name: "menu-freeze", setup: func(s *session) {
		inventory(s)
		s.keys("<alt+v>", "<right>", "<down>")
		s.waitFor("Keep the first row on screen")
	}},
	{name: "frozen", setup: func(s *session) {
		inventory(s)
		s.keys("<alt+v>", "<enter>", "1", "<enter>")
		s.waitFor("Froze 1 row")
		s.keys("<alt+v>", "<enter>", "1", "1", "<enter>")
		s.waitFor("Froze 1 column")
		s.keys("<ctrl+end>", "<up>", "<up>", "<shift+up>", "<shift+up>")
		s.waitForName("F37:F39")
	}},
	{name: "frozen-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		inventory(s)
		s.keys("<right>", "<down>", "<alt+v>", "<enter>", "u", "<enter>")
		s.waitFor("Froze 2 rows")
		s.keys("<alt+v>", "<enter>", "u", "u", "<enter>")
		s.waitFor("Froze 2 columns")
		s.keys("<pgdown>", "<right>", "<right>", "<right>")
		s.waitForName("E10")
	}},
	{name: "filter-picker", setup: func(s *session) {
		inventory(s)
		s.keys("<right>", "<alt+d>", "c")
		s.waitFor("Created a filter")
		s.keys("<alt+down>", "<down>", "<down>", "<space>", "<down>")
		s.waitFor("[ ] Hardware")
	}},
	{name: "filter-condition", setup: func(s *session) {
		inventory(s)
		s.keys("<alt+d>", "c")
		s.waitFor("Created a filter")
		s.keys("<right>", "<right>", "<right>", "<alt+down>", "<tab>")
		for range 8 {
			s.keys("<down>")
		}
		s.keys("50")
		s.waitFor("‹ Greater than ›  50")
	}},
	{name: "filtered", setup: func(s *session) {
		inventory(s)
		s.keys("<alt+d>", "c")
		s.waitFor("Created a filter")
		s.keys("<right>", "<alt+down>", "<down>", "<down>", "<space>", "<enter>")
		s.keys("<right>", "<right>", "<alt+down>", "<tab>")
		for range 8 {
			s.keys("<down>")
		}
		s.keys("50", "<enter>")
		s.waitFor("Filtered column D")
		s.keys("<down>", "<down>")
	}},
	{name: "sort-bar", setup: func(s *session) {
		inventory(s)
		s.keys("<right>", "<alt+d>", "<down>", "<enter>", "a", "<enter>")
		s.waitFor("Sort A2:F41 by")
		s.keys("<alt+a>", "<right>", "<right>", "<right>", "<space>")
		s.waitFor("D Qty  Z→A")
	}},
	{name: "fill-handle", setup: func(s *session) {
		s.keys("Week", "<tab>", "Day", "<tab>", "Batch", "<enter>")
		s.keys("1", "<tab>", "Mon", "<tab>", "Item 1", "<enter>")
		s.keys("2", "<tab>", "Tue", "<tab>", "Item 2", "<enter>")
		s.keys("<up>", "<up>", "<shift+down>", "<shift+right>", "<shift+right>")
		s.waitForName("A2:C3")
		s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, 6+29, gridRow1+2, 0)
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonLeft, 6+25, gridRow1+6, 0)
		s.waitFor("Fill A2:C7")
	}},
	{name: "import-picker", files: importDir, setup: openImportPicker},
	{name: "import-picker-narrow", opts: options{cols: 60, rows: 16}, files: importDir, setup: openImportPicker},
	{name: "import-progress", setup: slowCSV},
	{name: "import-xlsx", files: importDir, setup: func(s *session) {
		openImportPicker(s)
		s.keys("q3", "<enter>")
		s.waitFor("Imported q3.xlsx")
	}},
	{name: "save-imported", files: importDir, setup: func(s *session) {
		openImportPicker(s)
		s.keys("q3", "<enter>")
		s.waitFor("Imported q3.xlsx")
		s.keys("<ctrl+s>")
		s.waitFor("q3.xlsx was imported.")
	}},
	{name: "menu-download", setup: func(s *session) {
		budget(s)
		s.keys("<alt+f>", "<down>", "<down>", "<down>", "<down>", "<down>", "<right>")
		s.waitFor("SQLite database (.sqlite)")
	}},
	{name: "sheets", setup: summary},
	{name: "sheets-wide", opts: options{cols: 200, rows: 30}, setup: summary},
	{name: "sheets-point", setup: func(s *session) {
		summary(s)
		s.keys("<down>", "<down>", "<left>", "Rent share", "<tab>", "=", "<ctrl+pgup>", "<up>", "<up>")
		s.waitForBar("Summary!B3", "=Sheet1!B3")
	}},
	{name: "sheets-many", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		for range 11 {
			s.keys("<shift+f11>")
		}
		s.waitFor("Added Sheet12")
		for range 5 {
			s.keys("<ctrl+pgup>")
		}
		s.waitFor("‹")
	}},
	{name: "sheets-rename", setup: func(s *session) {
		summary(s)
		x := tabX(s.t, s, "Summary")
		s.click(ghostty.MouseButtonLeft, x, int(s.rows)-1)
		s.click(ghostty.MouseButtonLeft, x, int(s.rows)-1)
		s.waitFor("Rename sheet: Summary")
		s.keys("Q3 totals")
	}},
	{name: "sheets-menu", setup: func(s *session) {
		summary(s)
		s.click(ghostty.MouseButtonRight, tabX(s.t, s, "Summary"), int(s.rows)-1)
		s.waitFor("Move right")
	}},
	{name: "sheets-picker", setup: func(s *session) {
		summary(s)
		s.keys("<alt+shift+k>")
		s.waitFor("Go to sheet")
	}},
	{name: "find-all-sheets", setup: func(s *session) {
		summary(s)
		s.keys("<ctrl+f>", "total", "<alt+s>")
		s.waitFor("in all sheets")
		s.waitFor("2 of 2 on Summary")
		s.keys("<enter>")
		s.waitFor("1 of 2 on Sheet1")
	}},
	{name: "pivot-editor", setup: pivotEditor},
	{name: "pivot-editor-narrow", opts: options{cols: 60, rows: 16}, setup: pivotEditor},
	{name: "pivot", setup: func(s *session) {
		pivotEditor(s)
		s.keys("<enter>", "<down>", "<down>", "<right>", "<right>", "5")
		s.waitFor("Pivot table results can't be edited")
	}},
	{name: "pivot-filter", setup: func(s *session) {
		pivotEditor(s)
		s.keys("<down>", "<space>", "item", "<enter>", "<space>", "bolts")
		s.waitFor("Filter Item")
	}},
	{name: "frequency", setup: func(s *session) {
		inventory(s)
		s.keys("<right>", "<right>", "<down>", "<alt+shift+f>")
		s.waitFor("Counted Bin: 12 distinct values")
	}},
}

// Key screens are also recorded on a light terminal, where the app picks
// its light theme from the reported background color.
func init() {
	for _, name := range []string{"budget", "point-range", "selection-stats", "menu", "palette-search", "context-menu-column", "functions", "quit-confirm", "help", "resizing-column", "copy-marker", "copy-marker-selected", "formats", "menu-format-number", "find", "replace", "jev", "import-picker", "import-progress", "import-xlsx", "frozen", "filter-picker", "filtered", "sort-bar", "fill-handle", "chart", "chart-editor", "chart-line", "chart-pie", "links-errors", "autocomplete", "signature", "named-ranges", "trace-precedents", "sheets", "sheets-point", "sheets-menu", "sheets-many", "pivot-editor", "pivot", "frequency"} {
		for _, sc := range screens {
			if sc.name == name {
				sc.name += "-light"
				sc.opts.light = true
				screens = append(screens, sc)
			}
		}
	}
}
