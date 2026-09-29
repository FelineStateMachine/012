package chart

import (
	"image"
	"math"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// barPlan lays out horizontal bar charts: category labels on the left,
// bars growing right, and the value axis along the bottom. Draw adds the
// legend.
//
//	Jan ▐██████████▌
//	Feb ▐███████▎
//	    └─────┬─────┬
//	    0   1,000 2,000
type barPlan struct {
	d       sheet.ChartData
	stacked bool
	grid    bool
	msg     string
	sc      scale
	catW    int // width of the category labels
	axisX   int
	axisRow int
	tickRow int
	plot    image.Rectangle
	shown   int
	slots   int // bars per category: the series, or one stack
	rows    int // rows per category, including a gap row
}

func newBarPlan(d sheet.ChartData, w, h int, o sheet.ChartOptions) *barPlan {
	d = prepare(d, o, true)
	p := &barPlan{d: d, stacked: o.Stack != sheet.StackNone, grid: !o.NoGrid, slots: len(d.Series)}
	lo, hi, ok := finite(d.Series)
	if p.stacked {
		lo, hi, ok = stackedRange(d, len(d.Categories))
		p.slots = 1
	}
	if !ok || len(d.Categories) == 0 {
		p.msg = "No numbers to chart"
		return p
	}
	if !o.Log {
		lo, hi = min(lo, 0), max(hi, 0)
	}
	p.tickRow = h - 1
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
		p.sc = newAxis(lo, hi, plotW, labelW+2, 6, d.Format, o)
		for j := 0; j <= p.sc.n; j++ {
			labelW = max(labelW, ansi.StringWidth(p.sc.label(p.sc.tick(j))))
		}
	}
	p.plot = image.Rect(x0, 0, x0+p.sc.cells(), p.axisRow)
	p.rows = p.slots + 1
	if len(d.Categories)*p.rows-1 > p.axisRow {
		p.rows = p.slots
	}
	p.shown = max(min(len(d.Categories), (p.axisRow+p.rows-p.slots)/p.rows), 1)
	return p
}

// barRow returns the row of series j's bar in category i; stacked, all
// series share one.
func (p *barPlan) barRow(i, j int) int {
	if p.stacked {
		j = 0
	}
	return i*p.rows + j
}

// piles returns category i's bars as spans, as columnPlan.piles does.
func (p *barPlan) piles(i int, buf []span) []span {
	if p.stacked {
		return stack(p.d, i, buf)
	}
	buf = buf[:0]
	base := p.sc.base()
	for j, s := range p.d.Series {
		if v := value(s, i); !math.IsNaN(v) {
			buf = append(buf, span{min(v, base), max(v, base), j})
		}
	}
	return buf
}

func (p *barPlan) draw(g *Grid, o Options) {
	p.drawAxis(g)
	for i := 0; i < p.shown; i++ {
		label := ansi.Truncate(p.d.Categories[i], p.catW, "…")
		g.text(p.catW-ansi.StringWidth(label), p.barRow(i, (p.slots-1)/2), label, Label)
	}
	if o.Image {
		return
	}
	if p.grid {
		for j := 1; j <= p.sc.n; j++ {
			for y := 0; y < p.axisRow; y++ {
				g.set(p.plot.Min.X+j*p.sc.k-1, y, "┊", Gridline)
			}
		}
	}
	drawPiles(p.sc, p.shown, p.stacked, p.piles, leftEighths, "▐", func(i, j, c int, cell Cell) {
		if at := g.At(p.plot.Min.X+c, p.barRow(i, j)); at != nil {
			*at = cell
		}
	})
}

// drawAxis draws the category axis on the left and the value axis along
// the bottom.
func (p *barPlan) drawAxis(g *Grid) {
	for y := 0; y < p.axisRow; y++ {
		g.set(p.axisX, y, "│", Axis)
	}
	g.set(p.axisX, p.axisRow, "└", Axis)
	for x := p.axisX + 1; x < p.plot.Max.X; x++ {
		g.set(x, p.axisRow, "─", Axis)
	}
	drawXTicks(g, p.sc, p.axisX, p.plot.Min.X, p.axisRow, p.tickRow)
}

// drawXTicks marks the ticks of a horizontal axis on row axisRow, the
// first at column axisX and the rest ending intervals from x0, and labels
// as many on row tickRow as fit without touching.
func drawXTicks(g *Grid, sc scale, axisX, x0, axisRow, tickRow int) {
	next := 0 // first free column
	for j := 0; j <= sc.n; j++ {
		x := axisX
		if j > 0 {
			x = x0 + j*sc.k - 1
			g.set(x, axisRow, "┬", Axis)
		}
		label := sc.label(sc.tick(j))
		lw := ansi.StringWidth(label)
		lx := min(max(x-(lw-1)/2, 0), g.W-lw)
		if lx >= next {
			g.text(lx, tickRow, label, Label)
			next = lx + lw + 1
		}
	}
}
