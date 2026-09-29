package e2e

import (
	"strings"
	"testing"
)

// A table made from the data around the cursor is read by column name,
// grows as rows are typed below it, renames its column in formulas when
// its header changes, and is saved with the file.
func TestTables(t *testing.T) {
	dir := t.TempDir()
	writeTable(t, dir, "stock.012", 4) // Item, Qty over A1:B5; Qty 1 2 3 0
	s := start(t, dir, "stock.012")

	s.keys("<down>", "<ctrl+alt+t>")
	s.waitFor("Name for the table of A1:B5: Table1")
	s.keys("Stock", "<enter>")
	s.waitFor("Stock is a table: read it by column in formulas, e.g. =SUM(Stock[Qty])")

	// Suggestions offer the table, then its columns.
	s.keys("<esc>", "<ctrl+home>", "<right>", "<right>", "<right>", "=SUM(st")
	s.waitFor("table, A1:B5")
	s.keys("<tab>", "[q")
	s.waitFor("Stock[Item, Qty]")
	s.keys("<tab>", ")", "<enter>")
	s.keys("<up>")
	s.waitForBar("D1", "=SUM(Stock[Qty])")
	s.waitFor("6")

	// A row typed just below joins the table.
	s.keys("<ctrl+home>", "<ctrl+down>", "<down>", "Row 5", "<tab>", "10", "<enter>")
	s.keys("<ctrl+home>", "<right>", "<right>", "<right>")
	s.waitForBar("D1", "=SUM(Stock[Qty])")
	s.eventually("the sum reads the new row", func() bool { return strings.Contains(s.line(gridRow1), "16") })

	// Renaming the header renames the column in the formula.
	s.keys("<left>", "<left>", "Count", "<enter>")
	s.keys("<up>", "<right>", "<right>")
	s.waitForBar("D1", "=SUM(Stock[Count])")
	s.keys("<left>", "<left>", "<down>")
	s.waitFor("Table Stock, column Count: Stock[Count] in formulas")

	// The table and its formula survive saving and opening again.
	s.keys("<ctrl+s>")
	s.eventually("saved", func() bool { return !strings.Contains(s.line(29), "modified") })
	s2 := start(t, dir, "stock.012")
	s2.keys("<right>", "<down>")
	s2.waitFor("Table Stock, column Count")
	s2.waitFor("16")
}
