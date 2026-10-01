package e2e

import (
	"fmt"
	"strings"
	"testing"
)

// inventoryQty sums the inventory fixture's quantities by category.
func inventoryQty() (fasteners, hardware int) {
	for i := range 40 {
		qty := (i*37)%90 + 5
		if k := i % 8; k == 3 || k == 4 || k == 6 {
			hardware += qty
		} else {
			fasteners += qty
		}
	}
	return fasteners, hardware
}

// pivotByCategory creates a pivot of the inventory with Category in Rows
// and SUM of Qty in Values, leaving the editor open on the value.
func pivotByCategory(s *session) {
	inventory(s)
	s.keys("<ctrl+k>", "pivot table", "<enter>")
	s.waitFor("PIVOT")
	s.waitFor("Data  Sheet1!A1:F41")
	s.keys("<space>", "categ", "<enter>")
	s.waitFor("   Category")
	s.keys("<down>", "<down>", "<space>", "qty", "<enter>")
	s.waitFor("‹ SUM ›")
}

func TestPivotTable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := start(t, dir)
	pivotByCategory(s)
	s.keys("<enter>")
	s.waitFor("READY")
	fasteners, hardware := inventoryQty()
	s.eventually("pivot results", func() bool {
		return strings.HasPrefix(s.line(gridRow1), "    1  Category") &&
			strings.Contains(s.line(gridRow1+1), fmt.Sprint(fasteners)) &&
			strings.Contains(s.line(gridRow1+3), fmt.Sprint(fasteners+hardware))
	})
	if !strings.Contains(s.line(int(s.rows)-1), "Pivot Table 1") {
		t.Errorf("no tab for the pivot: %q", s.line(int(s.rows)-1))
	}

	// Its results can't be edited.
	s.keys("<down>", "<right>", "7")
	s.waitFor("Pivot table results can't be edited")

	// It follows its source: Bolts 1's quantity goes up by 1000.
	s.keys("<ctrl+pgup>", "<f5>", "D2", "<enter>", "1005", "<enter>", "<ctrl+pgdown>")
	s.eventually("recomputed", func() bool {
		return strings.Contains(s.line(gridRow1+1), fmt.Sprint(fasteners+1000))
	})

	// Saved as its definition, and computed again on opening.
	s.keys("<ctrl+s>", "stock", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - stock.012" })
	s.keys("<ctrl+q>")
	s.waitExit()
	r := start(t, dir, "stock.012")
	r.keys("<ctrl+pgdown>")
	r.eventually("pivot after reopening", func() bool {
		return strings.Contains(r.line(gridRow1+1), fmt.Sprint(fasteners+1000)) && strings.Contains(r.screen(), "Grand Total")
	})

	// Esc in the editor undoes its changes; undo takes the edit back.
	r.keys("<ctrl+k>", "edit pivot", "<enter>")
	r.waitFor("PIVOT")
	r.keys("<down>", "<delete>")
	r.eventually("category removed", func() bool { return !strings.Contains(r.line(gridRow1), "Category") })
	r.keys("<esc>")
	r.waitFor("READY")
	r.eventually("category back", func() bool { return strings.HasPrefix(r.line(gridRow1), "    1  Category") })
}

func TestFrequencyTable(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	inventory(s)
	s.keys("<right>", "<down>", "<alt+shift+f>")
	s.waitFor("Counted Category: 2 distinct values")
	fasteners := 0
	for i := range 40 {
		if k := i % 8; k != 3 && k != 4 && k != 6 {
			fasteners++
		}
	}
	s.eventually("counts", func() bool {
		return strings.HasPrefix(s.line(gridRow1), "    1  Category") &&
			strings.Contains(s.line(gridRow1+1), "Fasteners") && strings.Contains(s.line(gridRow1+1), fmt.Sprint(fasteners)) &&
			strings.Contains(s.line(gridRow1+3), "100.00%")
	})
	s.keys("<ctrl+z>")
	s.eventually("undone", func() bool { return !strings.Contains(s.line(int(s.rows)-1), "Frequency of") })
}

func TestPivotRenameAndColumnSubtotals(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	pivotByCategory(s)
	// R renames the value on the context line.
	s.keys("r")
	s.waitFor("Value name: SUM of Qty")
	s.keys("Units", "<enter>")
	s.waitFor("PIVOT")
	s.eventually("renamed header", func() bool { return strings.Contains(s.line(gridRow1), "Units") })
	// Category moves from Rows to Columns, after Reorder: a subtotal
	// column follows each Reorder value.
	s.keys("<up>", "<up>", "<up>", "<delete>")
	s.eventually("category removed", func() bool { return !strings.Contains(s.line(gridRow1), "Category") })
	s.keys("<space>", "reord", "<enter>")
	s.waitFor("   Reorder")
	s.keys("a", "categ", "<enter>")
	s.waitFor("   Category")
	s.keys("<enter>")
	s.waitFor("READY")
	s.eventually("subtotal columns", func() bool {
		return strings.Contains(s.line(gridRow1), "no Total") && strings.Contains(s.line(gridRow1), "yes Total")
	})
}
