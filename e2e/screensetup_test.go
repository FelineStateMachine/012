package e2e

import (
	"fmt"
	"strings"
)

// Fixtures shared by the golden screens in screenlist_test.go: sheets
// typed or pasted the way a user would, and steps several screens take.

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

// pivotEditor creates a pivot of the inventory in the editor: Category
// in Rows, Reorder in Columns, the sum of Qty and the average Price,
// leaving the average highlighted.
func pivotEditor(s *session) {
	inventory(s)
	s.keys("<ctrl+k>", "pivot table", "<enter>")
	s.waitFor("Data  Sheet1!A1:F41")
	s.keys("<space>", "categ", "<enter>")
	s.waitFor("   Category")
	s.keys("<down>", "<space>", "reord", "<enter>")
	s.waitFor("   Reorder")
	s.keys("<down>", "<space>", "qty", "<enter>")
	s.waitFor("‹ SUM ›")
	s.keys("a", "price", "<enter>", "<left>", "<left>", "<left>")
	s.waitFor("‹ AVERAGE ›")
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
