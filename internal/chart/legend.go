package chart

import (
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// legendBox is where a chart's legend of series goes: a row under the
// plot, a column right of it, or nowhere. Draw lays the chart out in the
// room the legend leaves.
type legendBox struct {
	names []string
	pos   sheet.ChartLegend
	row   int // the bottom legend's row
	x, w  int // the right legend's first column and width
}

// Legend sizes.
const (
	legendNameW  = 16 // longest name in a right legend
	minRightPlot = 16 // narrowest chart a right legend leaves
)

// placeLegend makes room for a legend of names in a w by h chart and
// returns the room left for the chart. A legend needs two series or more,
// and space: a bottom one seven rows, a right one a chart still
// minRightPlot wide beside it.
func placeLegend(names []string, pos sheet.ChartLegend, w, h int) (legendBox, int, int) {
	lg := legendBox{pos: sheet.LegendNone}
	if len(names) < 2 {
		return lg, w, h
	}
	switch pos {
	case sheet.LegendBottom:
		if h >= 7 {
			return legendBox{names: names, pos: pos, row: h - 1}, w, h - 1
		}
	case sheet.LegendRight:
		nameW := 1
		for _, n := range names {
			nameW = max(nameW, ansi.StringWidth(n))
		}
		lw := 2 + min(nameW, legendNameW)
		if w-lw-1 >= minRightPlot {
			return legendBox{names: names, pos: pos, x: w - lw, w: lw}, w - lw - 1, h
		}
	}
	return lg, w, h
}

func (lg legendBox) draw(g *Grid) {
	switch lg.pos {
	case sheet.LegendBottom:
		g.legend(lg.row, lg.names)
	case sheet.LegendRight:
		g.legendColumn(lg.x, lg.w, lg.names)
	}
}

// legend draws "■ Rent   ■ Food" centered on row y, shortening names to
// fit.
func (g *Grid) legend(y int, names []string) {
	const gap = "   "
	for limit := 24; limit >= 1; limit-- {
		items := make([]string, len(names))
		width := 0
		for i, n := range names {
			items[i] = "■ " + ansi.Truncate(n, limit, "…")
			width += ansi.StringWidth(items[i])
		}
		width += len(gap) * (len(items) - 1)
		if width > g.W && limit > 1 {
			continue
		}
		x := max((g.W-width)/2, 0)
		for i, it := range items {
			if x+ansi.StringWidth(it) > g.W {
				g.text(max(x, 0), y, "…", Label)
				return
			}
			g.set(x, y, "■", SeriesRole(i))
			g.text(x+2, y, it[len("■ "):], Label)
			x += ansi.StringWidth(it) + len(gap)
		}
		return
	}
}

// legendColumn draws the legend one series a row from column x, w wide,
// centered down the chart, ending in "… 3 more" when they don't all fit.
func (g *Grid) legendColumn(x, w int, names []string) {
	n := min(len(names), g.H)
	y := (g.H - n) / 2
	for i, name := range names {
		if y == g.H-1 && i < len(names)-1 {
			g.text(x, y, ansi.Truncate("… "+strconv.Itoa(len(names)-i)+" more", w, "…"), Label)
			return
		}
		g.set(x, y, "■", SeriesRole(i))
		g.text(x+2, y, ansi.Truncate(name, w-2, "…"), Label)
		y++
	}
}

func seriesNames(series []sheet.ChartSeries) []string {
	names := make([]string, len(series))
	for i, s := range series {
		names[i] = s.Name
	}
	return names
}
