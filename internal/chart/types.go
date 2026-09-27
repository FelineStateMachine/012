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

// A layout lays out data as one type of chart.
type layout func(d sheet.ChartData, w, h int, o Options) plan

// types are the chart types, each a layout whose plan draws it as text
// and as an image. The types themselves, their names and the order the
// chart editor offers them, are in the sheet package, since charts are
// saved with sheets; adding one is a constant there and a row here.
var types = map[sheet.ChartType]layout{
	sheet.ChartColumn: func(d sheet.ChartData, w, h int, _ Options) plan { return newColumnPlan(d, w, h, false) },
	sheet.ChartBar:    func(d sheet.ChartData, w, h int, _ Options) plan { return newBarPlan(d, w, h) },
	sheet.ChartLine:   func(d sheet.ChartData, w, h int, _ Options) plan { return newColumnPlan(d, w, h, true) },
	sheet.ChartPie:    func(d sheet.ChartData, w, h int, o Options) plan { return newPiePlan(d, w, h, o) },
}

// planFor lays out d as a chart of type t; an unknown type is drawn as
// columns.
func planFor(t sheet.ChartType, d sheet.ChartData, w, h int, o Options) plan {
	lay, ok := types[t]
	if !ok {
		lay = types[sheet.ChartColumn]
	}
	return lay(d, w, h, o)
}

func (p *columnPlan) note() string              { return p.msg }
func (p *columnPlan) plotArea() image.Rectangle { return p.plot }
func (p *barPlan) note() string                 { return p.msg }
func (p *barPlan) plotArea() image.Rectangle    { return p.plot }
func (p *piePlan) note() string                 { return p.msg }
func (p *piePlan) plotArea() image.Rectangle    { return p.disc }
