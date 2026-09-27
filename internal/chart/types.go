package chart

import (
	"image"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A plan is a chart laid out in w by h cells: the layout Draw and Image
// share, so text and image line up.
type plan interface {
	// note is a message shown instead of the chart, such as "No numbers
	// to chart", or "" when there's a chart to draw.
	note() string
	// plotArea is the rectangle, in cells, that an image covers.
	plotArea() image.Rectangle
	// draw draws the chart as text; with o.Image, all but the plot area.
	draw(g *Grid, o Options)
	// image draws the plot area, one cell per cv.cw by cv.ch pixels.
	image(cv *canvas)
}

// A chartType lays out data as one type of chart. The legend of series
// is Draw's: it gives the layout the room the legend leaves, and names
// returns the series it lists, nil for a type that draws its own.
type chartType struct {
	layout func(d sheet.ChartData, w, h int, o Options) plan
	names  func(d sheet.ChartData) []string
}

// types are the chart types, each a layout whose plan draws it as text
// and as an image. The types themselves, their names and the order the
// chart editor offers them, are in the sheet package, since charts are
// saved with sheets; adding one is a constant there and a row here.
var types = map[sheet.ChartType]chartType{
	sheet.ChartColumn: {columnLayout(columnBars), allSeries},
	sheet.ChartBar: {func(d sheet.ChartData, w, h int, o Options) plan {
		return newBarPlan(d, w, h, o.Chart)
	}, allSeries},
	sheet.ChartLine: {columnLayout(columnLine), allSeries},
	sheet.ChartPie: {func(d sheet.ChartData, w, h int, o Options) plan {
		return newPiePlan(d, w, h, o)
	}, nil},
	sheet.ChartArea: {columnLayout(columnArea), allSeries},
	sheet.ChartScatter: {func(d sheet.ChartData, w, h int, o Options) plan {
		return newScatterPlan(d, w, h, o.Chart)
	}, func(d sheet.ChartData) []string {
		_, ys := scatterSeries(d)
		return seriesNames(ys)
	}},
}

func columnLayout(kind columnKind) func(d sheet.ChartData, w, h int, o Options) plan {
	return func(d sheet.ChartData, w, h int, o Options) plan { return newColumnPlan(d, w, h, kind, o.Chart) }
}

func allSeries(d sheet.ChartData) []string { return seriesNames(d.Series) }

// planFor lays out d as a chart of type t, with its legend; an unknown
// type is drawn as columns.
func planFor(t sheet.ChartType, d sheet.ChartData, w, h int, o Options) (plan, legendBox) {
	ct, ok := types[t]
	if !ok {
		ct = types[sheet.ChartColumn]
	}
	lg := legendBox{pos: sheet.LegendNone}
	if ct.names != nil {
		lg, w, h = placeLegend(ct.names(d), o.Chart.Legend, w, h)
	}
	return ct.layout(d, w, h, o), lg
}

func (p *columnPlan) note() string               { return p.msg }
func (p *columnPlan) plotArea() image.Rectangle  { return p.plot }
func (p *barPlan) note() string                  { return p.msg }
func (p *barPlan) plotArea() image.Rectangle     { return p.plot }
func (p *piePlan) note() string                  { return p.msg }
func (p *piePlan) plotArea() image.Rectangle     { return p.disc }
func (p *scatterPlan) note() string              { return p.msg }
func (p *scatterPlan) plotArea() image.Rectangle { return p.plot }
