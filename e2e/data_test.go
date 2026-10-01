package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTable saves a sheet with a header row (Item, Qty) over n rows:
// Row 1..n with the row number mod 4.
func writeTable(t *testing.T, dir, name string, n int) {
	t.Helper()
	var cells []string
	cells = append(cells, `"A1": "Item"`, `"B1": "Qty"`)
	for i := 1; i <= n; i++ {
		cells = append(cells, fmt.Sprintf(`"A%d": "Row %d"`, i+1, i), fmt.Sprintf(`"B%d": "%d"`, i+1, i%4))
	}
	body := `{"version": 2, "cells": {` + strings.Join(cells, ", ") + `}}`
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFreezeFilterAndSort(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTable(t, dir, "data.012", 60)
	s := start(t, dir, "data.012")

	// View > Freeze > 1 row keeps the headers while scrolling.
	s.keys("<alt+v>", "<enter>", "1", "<enter>")
	s.waitFor("Froze 1 row")
	s.keys("<ctrl+end>")
	s.waitForName("B61")
	s.eventually("frozen header row", func() bool {
		return strings.HasPrefix(s.line(gridRow1), "    1  Item") && strings.HasPrefix(s.line(gridRow1+1), "──────────")
	})

	// Data > Create a filter, then uncheck 0 in the Qty column.
	s.keys("<ctrl+home>", "<right>", "<alt+d>", "c")
	s.waitFor("Created a filter on A1:B61")
	s.keys("<alt+down>")
	s.waitFor("Filter B  Qty")
	s.keys("<down>", "<space>", "<enter>")
	s.waitFor("Filter hides 15 rows")
	s.eventually("row numbers skip hidden rows", func() bool {
		return strings.HasPrefix(s.line(gridRow1+4), "    4  Row 3") && strings.HasPrefix(s.line(gridRow1+5), "    6  Row 5")
	})

	// Data > Sort sheet > Z to A sorts below the frozen row.
	s.keys("<alt+d>", "<enter>", "z")
	s.waitFor("Sorted A2:B61 by B Z→A")
	s.keys("<down>")
	s.waitForBar("B2", "3")

	// Everything survives saving and opening again.
	s.keys("<ctrl+s>")
	s.eventually("saved", func() bool { return !strings.Contains(s.line(29), "modified") })
	s2 := start(t, dir, "data.012")
	s2.waitFor("Filter hides 15 rows")
	s2.eventually("frozen divider", func() bool { return strings.HasPrefix(s2.line(gridRow1+1), "──────────") })
}

func TestSortBarAndFillHandle(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.keys("Month", "<tab>", "Sales", "<enter>", "Jan", "<tab>", "30", "<enter>", "Feb", "<tab>", "10", "<enter>")
	s.keys("<up>")
	s.waitForName("A3")

	// Drag the fill handle at A3's corner down two rows: Mar, Apr.
	s.drag([2]int{6 + 9, gridRow1 + 2}, [2]int{colX(0), gridRow1 + 3}, [2]int{colX(0), gridRow1 + 4})
	s.eventually("filled Mar, Apr", func() bool { return strings.HasPrefix(s.line(gridRow1+4), "    5  Apr") })
	s.keys("<ctrl+z>")
	s.eventually("undone", func() bool { return !strings.Contains(s.line(gridRow1+4), "Apr") })

	// Data > Sort range > Advanced: sort by Sales, header row found.
	s.keys("<ctrl+home>", "<alt+d>", "<down>", "<enter>", "a", "<enter>")
	s.waitFor("Sort A2:B3 by  A Month  A→Z")
	s.keys("<right>", "<enter>")
	s.waitFor("Sorted A2:B3 by B A→Z")
	s.waitForLine(gridRow1+1, "    2  Feb             10")
}
