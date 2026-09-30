package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// Golden screens for linked sources: two million trips on a tab of
// their own, sorted by fare and scrolled halfway down by the scrollbar;
// formulas over them on a sheet, one past max-cells saying why; the
// condition a source's column is filtered by; and a source whose file
// isn't there. Each on a light terminal too, the tab in the high-contrast
// theme as well.

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

// tripFormulas reads the trips from a sheet: counts, sums, a lookup,
// and a MEDIAN past max-cells (a million, in the screen's config),
// the pointer on it.
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
		{"Trip 1M", "=XLOOKUP(1000000,trips[trip],trips[fare])"},
		{"Median", "=MEDIAN(trips[fare])"},
	} {
		s.keys(row[0], "<tab>", row[1], "<enter>")
	}
	s.waitFor("285714")
	s.waitFor("#VALUE!")
	s.keys("<up>", "<right>")
	s.waitFor("MEDIAN would hold")
}

var sourceScreens = []screen{
	{name: "source-tab", files: tripsFile, setup: sortedTrips},
	{name: "source-formulas", opts: options{config: "max-cells = 1000000\n"}, files: tripsFile, setup: tripFormulas},
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
		if sc.name == "source-tab" {
			hc := sc
			hc.name += "-high-contrast"
			hc.opts.config = highContrastConfig
			screens = append(screens, hc)
		}
	}
}
