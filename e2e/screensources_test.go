package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// Golden screens for linked sources: two million trips on a tab of
// their own, sorted by fare and scrolled halfway down by the scrollbar;
// formulas over them on a sheet in their columns' formats, and one past
// max-cells saying why at 80 columns, the rest on F1; the condition a
// source's column is filtered by; and a source whose file isn't there.
// Each on a light terminal too, the tab and the explanation in the
// high-contrast theme as well.

func tripsFile(t *testing.T, dir string) { writeTrips(t, dir, tripRows) }

// sortedTrips links trips.parquet, sorts it by fare Z to A and drags
// the scrollbar halfway down.
func sortedTrips(s *session) {
	s.linkSource("trips.parquet")
	s.waitFor("2,000,000 rows, 5 columns")
	s.keys("<right>", "<right>")
	s.keys("<ctrl+k>", "Sort sheet Z to A", "<enter>")
	s.waitFor("sorted by fare Z to A")
	s.waitFor("92.99")
	bar := int(s.cols) - 1
	top, bottom := 7, int(s.rows)-2
	s.click(ghostty.MouseButtonLeft, bar, (top+bottom)/2)
	s.waitFor("┃")
	s.eventually("the rows halfway down", func() bool {
		return strings.Contains(s.line(1), " 48") && !strings.Contains(s.screen(), " … ")
	})
}

// tripFormulas reads the trips from a sheet, max-cells a million in the
// screen's config: counts, sums, a lookup, the last day, a MEDIAN and a
// percentile, each in its column's format, and a STDEV past max-cells;
// column B widened for the fares, the pointer on the median.
func tripFormulas(s *session) {
	s.linkSource("trips.parquet")
	s.keys("<ctrl+pgup>")
	s.waitFor("Sheet1")
	for _, row := range [][2]string{
		{"Trips", "=ROWS(trips)"},
		{"Fares", "=SUM(trips[fare])"},
		{"By card", `=SUMIFS(trips[fare],trips[payment],"card")`},
		{"Vouchers", `=COUNTIF(trips[payment],"voucher")`},
		{"Longest", "=MAX(trips[miles])"},
		{"Last day", "=MAX(trips[day])"},
		{"Trip 1M", "=XLOOKUP(1000000,trips[trip],trips[fare])"},
		{"Median", "=MEDIAN(trips[fare])"},
		{"90th pct", "=PERCENTILE(trips[fare],0.9)"},
		{"Spread", "=STDEV(trips[fare])"},
	} {
		s.keys(row[0], "<tab>", row[1], "<enter>")
	}
	s.keys("<ctrl+home>", "<right>", "<ctrl+k>", "Column width", "<enter>")
	s.waitFor("Column width (1-240)")
	s.keys("14", "<enter>")
	s.waitFor("285,714")
	s.waitFor("95,990,000.00")
	s.eventually("every answer", func() bool { return !strings.Contains(s.screen(), "Loading") })
	for range 7 {
		s.keys("<down>")
	}
	s.waitFor("=MEDIAN(trips[fare])")
}

// tripWhy is tripFormulas with the pointer on the STDEV past max-cells
// and F1 pressed: its whole explanation in a box beside it.
func tripWhy(s *session) {
	tripCut(s)
	s.keys("<f1>")
	s.waitFor("#VALUE! in B10")
}

// tripCut is tripFormulas with the pointer on the STDEV past max-cells:
// the context line says the first of why, with F1 for the rest.
func tripCut(s *session) {
	tripFormulas(s)
	s.keys("<down>", "<down>")
	s.waitFor("Raise max-cells to 2,000,000")
	s.waitFor("F1")
}

var sourceScreens = []screen{
	{name: "source-tab", files: tripsFile, setup: sortedTrips},
	{name: "source-formulas", opts: options{config: "max-cells = 1000000\n"}, files: tripsFile, setup: tripFormulas},
	{name: "source-formulas-cut", opts: options{cols: 80, rows: 24, config: "max-cells = 1000000\n"}, files: tripsFile, setup: tripCut},
	{name: "source-formulas-why", opts: options{config: "max-cells = 1000000\n"}, files: tripsFile, setup: tripWhy},
	{name: "source-filter", files: tripsFile, setup: func(s *session) {
		s.linkSource("trips.parquet")
		s.waitFor("card")
		s.keys("<right>")
		s.keys("<ctrl+k>", "Filter by column", "<enter>")
		s.waitFor("Filter B  payment")
		for range 7 {
			s.keys("<down>")
		}
		s.keys("card")
		s.waitFor("Text is exactly")
		s.waitFor("card ")
	}},
	{name: "source-missing", files: func(t *testing.T, dir string) {
		writeText(t, filepath.Join(dir, "rides.012"),
			`{"version": 7, "sheets": [{"name": "Sheet1", "cells": {"A1": "=SUM(trips[fare])"}}, `+
				`{"name": "trips", "cells": {}, "regions": [{"name": "trips", "at": "A1", "path": "trips.parquet", "paged": true}]}]}`)
	}, args: []string{"rides.012"}, setup: func(s *session) {
		s.waitFor("#REF!")
		s.keys("<ctrl+pgdown>")
		s.waitFor("! trips.parquet: the file isn't there")
	}},
}

func init() {
	for _, sc := range sourceScreens {
		light := sc
		light.name += "-light"
		light.opts.light = true
		screens = append(screens, sc, light)
		if sc.name == "source-tab" || sc.name == "source-formulas-why" {
			hc := sc
			hc.name += "-high-contrast"
			hc.opts.config = sc.opts.config + highContrastConfig
			screens = append(screens, hc)
		}
	}
}
