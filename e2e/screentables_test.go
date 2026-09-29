package e2e

// Golden screens for tables: a banded table with its header styled and
// a formula reading it by column, the name asked for on converting, the
// column suggestions inside a table's brackets, and the list of tables;
// on a light terminal and in the high-contrast theme too.

// sales types six rows of sales under a header, makes them the table
// Sales with banded rows, and sums its Amount column beside it, the
// cursor left on a cell of the table.
func sales(s *session) {
	s.keys("Region", "<tab>", "Month", "<tab>", "Units", "<tab>", "Amount", "<enter>")
	for _, row := range [][]string{
		{"North", "Jan", "12", "480"}, {"South", "Jan", "7", "280"}, {"East", "Feb", "20", "800"},
		{"North", "Feb", "9", "360"}, {"West", "Mar", "15", "600"}, {"South", "Mar", "4", "160"},
	} {
		s.keys(row[0], "<tab>", row[1], "<tab>", row[2], "<tab>", row[3], "<enter>")
	}
	s.keys("<ctrl+home>", "<ctrl+alt+t>")
	s.waitFor("Name for the table of A1:D7")
	s.keys("Sales", "<enter>")
	s.waitFor("Sales is a table")
	s.keys("<esc>", "<ctrl+home>", "<right>", "<right>", "<right>", "<right>", "<right>")
	s.keys("Total", "<enter>", "=SUM(Sales[Amount])", "<enter>")
	s.keys("Per unit", "<enter>", "=F2/SUM(Sales[Units])", "<enter>")
	s.keys("<ctrl+home>", "<down>", "<down>", "<right>", "<right>", "<right>")
	s.waitFor("Table Sales, column Amount")
	s.keys("<ctrl+k>", "banded rows", "<enter>")
	s.waitFor("Table Sales, column Amount")
}

var tableScreens = []screen{
	{name: "table", setup: sales},
	{name: "table-name", setup: func(s *session) {
		s.keys("Item", "<tab>", "Cost", "<enter>", "Rent", "<tab>", "1450", "<enter>", "<up>", "<ctrl+alt+t>")
		s.waitFor("Name for the table of A1:B2: Table1")
	}},
	{name: "table-suggest", setup: func(s *session) {
		sales(s)
		s.keys("<ctrl+home>", "<down>", "<down>", "<down>", "<down>", "<down>", "<down>", "<down>", "<down>", "<right>", "<right>", "<right>", "<right>", "<right>")
		s.keys("=AVERAGE(Sales[u")
		s.waitFor("Sales[Region, Month, Units, Amount]")
	}},
	{name: "tables-picker", setup: func(s *session) {
		sales(s)
		s.keys("<alt+d>")
		s.waitFor("Named ranges")
		s.keys("<esc>")
		s.keys("<ctrl+k>", "tables")
		s.waitFor("│ › tables")
		s.keys("<enter>")
		s.waitFor("F2  rename")
	}},
	{name: "table-narrow", opts: options{cols: 60, rows: 16}, setup: sales},
}

func init() {
	light := map[string]bool{"table": true, "table-suggest": true, "tables-picker": true}
	for _, sc := range tableScreens {
		screens = append(screens, sc)
		if light[sc.name] {
			l := sc
			l.name += "-light"
			l.opts.light = true
			screens = append(screens, l)
		}
		if sc.name == "table" {
			hc := sc
			hc.name += "-high-contrast"
			hc.opts.config = highContrastConfig
			screens = append(screens, hc)
		}
	}
}
