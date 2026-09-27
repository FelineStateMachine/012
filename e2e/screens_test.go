package e2e

import (
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ghostty "go.mitchellh.com/libghostty"
)

var update = flag.Bool("update", false, "rewrite golden screens in testdata/screens")

// A screen is a named UI state captured as styled HTML. Goldens make every
// visual change show up in review; `make screens` renders them to a gallery
// with dark and light palettes for looking at.
type screen struct {
	name  string
	opts  options
	files func(t *testing.T, dir string) // fills the working directory first
	setup func(s *session)
}

// budget types a small sheet with text, numbers, formulas and an error the
// way a Sheets user would (Tab across, Enter back), then selects the total.
func budget(s *session) {
	s.keys("Household budget 2026", "<enter>", "<down>")
	s.keys("Rent", "<tab>", "1450", "<enter>")
	s.keys("Groceries", "<tab>", "612.4", "<enter>")
	s.keys("Transit", "<tab>", "96", "<enter>")
	s.keys("Savings rate", "<tab>", "=B3/0", "<enter>")
	s.keys("Total", "<tab>", "=SUM(B3:B5)", "<enter>")
	s.keys("<up>", "<right>")
	s.waitForBar("B7", "=SUM(B3:B5)")
}

// formatted types a bill schedule with dates, currency and percentages,
// then styles it with Sheets' shortcuts: bold title and totals, headers
// aligned over their numbers, a struck-out cancelled bill and an italic
// note. It ends by bolding the totals, so line 3 shows the feedback.
func formatted(s *session) {
	s.keys("Bills for October", "<enter>")
	s.keys("Item", "<tab>", "Due", "<tab>", "Amount", "<tab>", "Share", "<enter>")
	s.keys("Rent", "<tab>", "10/1/2026", "<tab>", "$1,450.00", "<tab>", "=C3/C$7", "<enter>")
	s.keys("Food", "<tab>", "9/28/2026", "<tab>", "612.4", "<tab>", "=C4/C$7", "<enter>")
	s.keys("Transit", "<tab>", "9/30/2026", "<tab>", "96", "<tab>", "=C5/C$7", "<enter>")
	s.keys("Gym", "<tab>", "10/5/2026", "<tab>", "40", "<tab>", "=C6/C$7", "<enter>")
	s.keys("Total", "<tab>", "<tab>", "=SUM(C3:C6)", "<tab>", "=SUM(D3:D6)", "<enter>")
	s.keys("<down>", "<left>", "<left>", "Gym cancelled from November", "<enter>")
	// Currency for the amounts, percent for the shares.
	s.keys("<ctrl+home>", "<down>", "<down>", "<down>", "<right>", "<right>", "<shift+down>", "<shift+down>", "<ctrl+shift+4>")
	s.keys("<right>", "<up>", "<shift+down>", "<shift+down>", "<shift+down>", "<shift+down>", "<ctrl+shift+5>")
	// Bold title, bold headers with the number headers on the right.
	s.keys("<ctrl+home>", "<ctrl+b>", "<down>", "<shift+right>", "<shift+right>", "<shift+right>", "<ctrl+b>")
	s.keys("<right>", "<shift+right>", "<shift+right>", "<ctrl+shift+r>")
	// The cancelled bill is struck out; the note is italic.
	s.keys("<ctrl+home>", "<down>", "<down>", "<down>", "<down>", "<down>", "<shift+right>", "<shift+right>", "<shift+right>", "<alt+shift+5>")
	s.keys("<down>", "<down>", "<down>", "<ctrl+i>")
	s.keys("<up>", "<up>", "<shift+right>", "<shift+right>", "<shift+right>", "<ctrl+b>")
	s.waitFor("Bold on for A7:D7")
}

// reviews types customer reviews with JEV columns, answered by a fake
// service so the screen is deterministic.
func reviews(s *session) {
	s.keys("Review", "<tab>", "Complaint?", "<tab>", "Likely", "<tab>", "Sentiment", "<tab>", "Urgency", "<enter>")
	for i, r := range []string{"Box arrived crushed", "Fast and friendly", "Wrong size, no reply"} {
		row := fmt.Sprint(i + 2)
		s.keys(r, "<tab>",
			`=JEV.TEST(A`+row+`, "Is this a complaint?")`, "<tab>",
			`=JEV.PROB(A`+row+`, "Is this a complaint?")`, "<tab>",
			`=JEV.CLASSIFY(A`+row+`, "Sentiment", "negative, positive")`, "<tab>",
			`=JEV.SCORE(A`+row+`, "Urgency", "low, mid, high")`, "<enter>")
	}
	s.keys("<up>", "<right>", "<right>", "<right>")
	s.waitFor("JEV: positive, 80% confident")
}

// inventory pastes a stock list of 40 items with a header row, the way
// data usually arrives, and returns to A1.
func inventory(s *session) {
	items := []string{"Bolts", "Nuts", "Washers", "Hinges", "Brackets", "Screws", "Anchors", "Rivets"}
	cats := []string{"Fasteners", "Fasteners", "Fasteners", "Hardware", "Hardware", "Fasteners", "Hardware", "Fasteners"}
	rows := []string{"Item\tCategory\tBin\tQty\tPrice\tReorder"}
	for i := range 40 {
		k := i % len(items)
		rows = append(rows, fmt.Sprintf("%s %d\t%s\tB%02d\t%d\t%.2f\t%s",
			items[k], i/len(items)+1, cats[k], i%12+1, (i*37)%90+5, float64((i*53)%400)/20+0.5, []string{"no", "yes"}[i%3/2]))
	}
	s.paste(strings.Join(rows, "\n"))
	s.waitFor("Pasted 246 cells")
	s.keys("<ctrl+home>")
	s.waitForName("A1")
}

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
}

// named is the budget with its expenses named and a formula using the
// name, ending on the total.
func named(s *session) {
	budget(s)
	s.keys("<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<alt+d>", "d", "Expenses", "<enter>")
	s.waitForName("Expenses")
	s.keys("<esc>", "<ctrl+home>", "<down>", "<right>", "<right>", "4", "<enter>")
	s.keys("<ctrl+home>", "<down>", "<down>", "<down>", "<down>", "<down>", "<down>", "<down>", "<right>")
	s.keys("=AVERAGE(Expenses)*C2", "<enter>")
	s.keys("<up>")
	s.waitForBar("B8", "=AVERAGE(Expenses)*C2")
}

// chartOfType inserts a chart, picks the type n steps right of Column in
// the editor and keeps it, leaving the chart deselected.
func chartOfType(s *session, n int, title string) {
	spending(s)
	insertChart(s)
	for range n {
		s.keys("<right>")
	}
	s.keys("<enter>")
	s.waitFor(title + " of A1:C5")
	s.keys("<esc>")
	s.waitFor("READY")
}

func openImportPicker(s *session) {
	s.keys("<alt+f>", "<down>", "<down>", "<enter>")
	s.waitFor("6 of 6")
}

// Key screens are also recorded on a light terminal, where the app picks
// its light theme from the reported background color.
func init() {
	for _, name := range []string{"budget", "point-range", "selection-stats", "menu", "palette-search", "context-menu-column", "functions", "quit-confirm", "help", "resizing-column", "copy-marker", "copy-marker-selected", "formats", "menu-format-number", "find", "replace", "jev", "import-picker", "import-progress", "import-xlsx", "frozen", "filter-picker", "filtered", "sort-bar", "fill-handle", "chart", "chart-editor", "chart-line", "chart-pie", "links-errors", "autocomplete", "signature", "named-ranges", "trace-precedents", "sheets", "sheets-point", "sheets-menu", "sheets-many"} {
		for _, sc := range screens {
			if sc.name == name {
				sc.name += "-light"
				sc.opts.light = true
				screens = append(screens, sc)
			}
		}
	}
}

func TestScreens(t *testing.T) {
	dir := filepath.Join("testdata", "screens")
	if *update {
		os.MkdirAll(dir, 0o755)
	}
	for _, sc := range screens {
		t.Run(sc.name, func(t *testing.T) {
			opts := sc.opts
			if opts.jev {
				srv, _ := fakeTypeSafe(t)
				opts.env = []string{"TYPESAFE_API_KEY=test-key", "TYPESAFE_BASE_URL=" + srv.URL}
			}
			if sc.files != nil {
				opts.dir = t.TempDir()
				sc.files(t, opts.dir)
			}
			s := startWith(t, opts)
			sc.setup(s)
			got := s.stableHTML()
			path := filepath.Join(dir, sc.name+".html")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden (run make screens): %v", err)
			}
			if got != string(want) {
				actual := filepath.Join(t.TempDir(), sc.name+".html")
				os.WriteFile(actual, []byte(got), 0o644)
				t.Errorf("screen %s changed; if intended, run make screens and review the gallery.\nplain text now:\n%s", sc.name, s.screen())
			}
		})
	}
	if *update {
		if err := writeGallery(dir); err != nil {
			t.Fatal(err)
		}
	}
}

// stableHTML waits until two captures in a row match, so a snapshot never
// catches a frame mid-render.
func (s *session) stableHTML() string {
	s.t.Helper()
	prev := s.html()
	for range 100 {
		time.Sleep(30 * time.Millisecond)
		cur := s.html()
		if cur == prev {
			return cur
		}
		prev = cur
	}
	s.t.Fatal("screen never settled")
	return ""
}

// writeGallery renders all goldens into gallery.html (git-ignored), once
// with each reference palette.
func writeGallery(dir string) error {
	var b strings.Builder
	b.WriteString(`<!doctype html><meta charset="utf-8"><title>012 screens</title>
<style>
body{margin:0;padding:32px;background:#0f1012;color:#ddd;font:14px system-ui,sans-serif}
h1{font-weight:600;margin:0 0 24px}
section{margin:0 0 40px}
h2{font:500 13px ui-monospace,monospace;color:#aaa;margin:0 0 8px}
.term{display:inline-block;padding:14px 16px;border-radius:10px;box-shadow:0 8px 30px #0008}
.screen{margin:0;font:13px/1.2 "JetBrains Mono","SF Mono",Menlo,monospace;color:var(--fg)}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(820px,1fr));gap:8px 24px}
.wide{grid-column:1/-1}
`)
	for _, p := range []struct {
		class  string
		ansi   [16]uint32
		bg, fg string
	}{{"dark", darkANSI, "#1d1f21", "#c5c8c6"}, {"light", lightANSI, "#fafafa", "#1d1f21"}} {
		fmt.Fprintf(&b, ".%s{background:%s;color:%s;--bg:%s;--fg:%s}.%s{", p.class, p.bg, p.fg, p.bg, p.fg, p.class)
		for i, c := range p.ansi {
			fmt.Fprintf(&b, "--vt-palette-%d:#%06x;", i, c)
		}
		b.WriteString("}\n")
	}
	b.WriteString("</style><h1>012 screens</h1><div class=grid>\n")
	for _, sc := range screens {
		body, err := os.ReadFile(filepath.Join(dir, sc.name+".html"))
		if err != nil {
			return err
		}
		wide := ""
		if sc.opts.cols > 120 {
			wide = " class=wide" // spans the whole gallery row
		}
		fmt.Fprintf(&b, "<section id=%q%s><h2>%s</h2><div>", sc.name, wide, html.EscapeString(sc.name))
		th := "dark"
		if sc.opts.light {
			th = "light"
		}
		fmt.Fprintf(&b, "<div class=\"term %s\">%s</div>", th, body)
		b.WriteString("</div></section>\n")
	}
	b.WriteString("</div>\n")
	return os.WriteFile(filepath.Join(dir, "gallery.html"), []byte(b.String()), 0o644)
}
