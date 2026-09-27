package chart

import (
	"image"
	"math"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// barPlan lays out horizontal bar charts: category labels on the left,
// bars growing right, and the value axis along the bottom.
//
//	Jan │██████████▌
//	Feb │███████▎
//	    └─────┬─────┬
//	    0   1,000 2,000
type barPlan struct {
	d       sheet.ChartData
	msg     string
	sc      scale
	catW    int // width of the category labels
	axisX   int
	axisRow int
	tickRow int
	legend  int
	plot    image.Rectangle
	shown   int
	rows    int // rows per category, including a gap row
}

func newBarPlan(d sheet.ChartData, w, h int) *barPlan {
	p := &barPlan{d: d, legend: -1}
	lo, hi, ok := finite(d.Series)
	if !ok || len(d.Categories) == 0 {
		p.msg = "No numbers to chart"
		return p
	}
	lo, hi = min(lo, 0), max(hi, 0)
	p.tickRow = h - 1
	ns := len(d.Series)
	if ns > 1 && h >= 7 {
		p.legend = h - 1
		p.tickRow = h - 2
	}
	p.axisRow = p.tickRow - 1
	if p.axisRow < 1 {
		p.msg = "Too small to chart"
		return p
	}
	for _, c := range d.Categories {
		p.catW = max(p.catW, ansi.StringWidth(c))
	}
	p.catW = max(min(p.catW, w/3), 1)
	p.axisX = p.catW + 1
	x0 := p.axisX + 1
	// Tick labels are centered on their ticks, so the last one needs half
	// its width to the right of the plot.
	labelW := 1
	for range 2 {
		plotW := w - x0 - (labelW+1)/2
		if plotW < 2 {
			p.msg = "Too small to chart"
			return p
		}
		p.sc = newScale(lo, hi, plotW, labelW+2, 6, d.Format)
		for j := 0; j <= p.sc.n; j++ {
			labelW = max(labelW, ansi.StringWidth(p.sc.label(p.sc.tick(j))))
		}
	}
	p.plot = image.Rect(x0, 0, x0+p.sc.cells(), p.axisRow)
	p.rows = ns + 1
	if len(d.Categories)*p.rows-1 > p.axisRow {
		p.rows = ns
	}
	p.shown = max(min(len(d.Categories), (p.axisRow+p.rows-ns)/p.rows), 1)
	return p
}

// barRow returns the row of series j's bar in category i.
func (p *barPlan) barRow(i, j int) int { return i*p.rows + j }

func (p *barPlan) draw(g *Grid, o Options) {
	p.drawAxis(g)
	ns := len(p.d.Series)
	for i := 0; i < p.shown; i++ {
		label := ansi.Truncate(p.d.Categories[i], p.catW, "…")
		g.text(p.catW-ansi.StringWidth(label), p.barRow(i, (ns-1)/2), label, Label)
	}
	if p.legend >= 0 {
		g.legend(p.legend, seriesNames(p.d))
	}
	if o.Image {
		return
	}
	zero := p.sc.pos(0)
	for j, s := range p.d.Series {
		for i := 0; i < p.shown && i < len(s.Values); i++ {
			v := s.Values[i]
			if math.IsNaN(v) {
				continue
			}
			// Left of the axis, bars fill from the right.
			barCells(zero, p.sc.pos(v), v >= 0, leftEighths, "▐", func(c int, glyph string) {
				g.set(p.plot.Min.X+c, p.barRow(i, j), glyph, SeriesRole(j))
			})
		}
	}
}

// drawAxis draws the category axis on the left and the value axis along
// the bottom, labeling as many ticks as fit without touching.
func (p *barPlan) drawAxis(g *Grid) {
	for y := 0; y < p.axisRow; y++ {
		g.set(p.axisX, y, "│", Axis)
	}
	g.set(p.axisX, p.axisRow, "└", Axis)
	for x := p.axisX + 1; x < p.plot.Max.X; x++ {
		g.set(x, p.axisRow, "─", Axis)
	}
	next := 0 // first free column
	for j := 0; j <= p.sc.n; j++ {
		x := p.axisX
		if j > 0 {
			x = p.plot.Min.X + j*p.sc.k - 1
			g.set(x, p.axisRow, "┬", Axis)
		}
		label := p.sc.label(p.sc.tick(j))
		lw := ansi.StringWidth(label)
		lx := min(max(x-(lw-1)/2, 0), g.W-lw)
		if lx >= next {
			g.text(lx, p.tickRow, label, Label)
			next = lx + lw + 1
		}
	}
}

// barCells walks the cells a bar covers from zero to end, distances
// along its axis in cells, calling fill with each cell and its glyph.
// A positive bar grows away from the axis in eighths; a negative one
// has no eighths growing the other way, so its partly covered cells are
// a full block or the half block half.
func barCells(zero, end float64, positive bool, eighths []string, half string, fill func(c int, glyph string)) {
	lo, hi := min(zero, end), max(zero, end)
	for c := int(math.Floor(lo)); float64(c) < hi; c++ {
		cover := min(hi, float64(c+1)) - max(lo, float64(c))
		glyph := ""
		switch {
		case positive:
			glyph = eighths[int(math.Round(cover*8))]
		case cover >= 0.75:
			glyph = "█"
		case cover >= 0.25:
			glyph = half
		}
		if glyph != "" && glyph != " " {
			fill(c, glyph)
		}
	}
}
