package chart

import (
	"image"
	"math"

	"github.com/charmbracelet/x/ansi"

	"012/internal/sheet"
)

// columnPlan lays out column and line charts: a value axis on the left
// with labeled ticks, the plot, a category axis and labels below it, and a
// legend when there are several series.
//
//	1,500 ┤ █▆
//	1,000 ┤ ██ ▆
//	  500 ┤ ██ █
//	    0 ┼─────────
//	       Jan Feb
//	    ■ Rent   ■ Food
type columnPlan struct {
	d       sheet.ChartData
	line    bool
	msg     string
	sc      scale
	labelW  int // width of the tick labels
	axisX   int // column of the value axis
	axisRow int // row of the category axis
	catRow  int
	legend  int // row of the legend, -1 for none
	plot    image.Rectangle
	shown   int // categories drawn
	// Columns: each category is a group gw cells wide, starting off cells
	// into the plot; each series' bar in it is bw wide.
	gw, off, bw, inset int
}

func newColumnPlan(d sheet.ChartData, w, h int, line bool) *columnPlan {
	p := &columnPlan{d: d, line: line, legend: -1}
	lo, hi, ok := finite(d.Series)
	if !ok || len(d.Categories) == 0 {
		p.msg = "No numbers to chart"
		return p
	}
	if !line {
		lo, hi = min(lo, 0), max(hi, 0)
	}
	p.catRow = h - 1
	if len(d.Series) > 1 && h >= 7 {
		p.legend = h - 1
		p.catRow = h - 2
	}
	p.axisRow = p.catRow - 1
	if p.axisRow < 2 {
		p.msg = "Too small to chart"
		return p
	}
	p.sc = newScale(lo, hi, p.axisRow, 2, 6, d.Format)
	for j := 0; j <= p.sc.n; j++ {
		p.labelW = max(p.labelW, ansi.StringWidth(p.sc.label(p.sc.tick(j))))
	}
	p.axisX = p.labelW + 1
	p.plot = image.Rect(p.axisX+1, 0, w-1, p.axisRow)
	plotW := p.plot.Dx()
	if plotW < 2 {
		p.msg = "Too small to chart"
		return p
	}
	ns := len(d.Series)
	if line {
		p.shown = min(len(d.Categories), plotW*2)
		return p
	}
	p.shown = max(min(len(d.Categories), plotW/ns), 1)
	p.gw = plotW / p.shown
	gap := 0
	if p.gw > ns {
		gap = max(1, p.gw/4)
	}
	p.bw = max((p.gw-gap)/ns, 1)
	p.inset = (p.gw - p.bw*ns) / 2
	p.off = (plotW - p.shown*p.gw) / 2
	return p
}

// bar returns the columns of series j's bar in category i.
func (p *columnPlan) bar(i, j int) (x0, x1 int) {
	x0 = p.plot.Min.X + p.off + i*p.gw + p.inset + j*p.bw
	return x0, x0 + p.bw
}

// dotX returns the x of category i's point in braille dots (two per cell).
func (p *columnPlan) dotX(i int) int {
	dots := p.plot.Dx() * 2
	if p.shown <= 1 {
		return dots / 2
	}
	return int(math.Round(float64(i) * float64(dots-1) / float64(p.shown-1)))
}

// catCenter is the column under which category i's label is centered.
func (p *columnPlan) catCenter(i int) int {
	if p.line {
		return p.plot.Min.X + p.dotX(i)/2
	}
	return p.plot.Min.X + p.off + i*p.gw + p.gw/2
}

func (p *columnPlan) draw(g *Grid, o Options) {
	if p.msg != "" {
		g.message(p.msg)
		return
	}
	g.Plot = p.plot
	// Value axis with a label at each tick, up to the top one.
	for y := p.axisRow - p.sc.cells(); y < p.axisRow; y++ {
		g.set(p.axisX, y, "│", Axis)
	}
	for x := p.plot.Min.X; x < g.W; x++ {
		g.set(x, p.axisRow, "─", Axis)
	}
	for j := 0; j <= p.sc.n; j++ {
		y := p.axisRow - j*p.sc.k
		label := p.sc.label(p.sc.tick(j))
		g.text(p.labelW-ansi.StringWidth(label), y, label, Label)
		mark := "┤"
		if j == 0 {
			mark = "┼"
		}
		g.set(p.axisX, y, mark, Axis)
	}
	p.drawCategories(g)
	if p.legend >= 0 {
		g.legend(p.legend, seriesNames(p.d))
	}
	if o.Image {
		return
	}
	if p.line {
		p.drawLines(g)
	} else {
		p.drawBars(g)
	}
}

// drawCategories labels as many categories as fit without touching.
func (p *columnPlan) drawCategories(g *Grid) {
	room := p.gw - 1
	if p.line {
		room = p.plot.Dx()/max(p.shown-1, 1) - 1
	}
	room = max(room, 3)
	next := 0 // first free column
	for i := 0; i < p.shown; i++ {
		label := ansi.Truncate(p.d.Categories[i], room, "…")
		w := ansi.StringWidth(label)
		x := min(max(p.catCenter(i)-(w-1)/2, 0), g.W-w)
		if x < next {
			continue
		}
		g.text(x, p.catRow, label, Label)
		next = x + w + 1
	}
}

func (p *columnPlan) drawBars(g *Grid) {
	zero := p.sc.pos(0)
	for j, s := range p.d.Series {
		for i := 0; i < p.shown && i < len(s.Values); i++ {
			v := s.Values[i]
			if math.IsNaN(v) {
				continue
			}
			x0, x1 := p.bar(i, j)
			top := p.sc.pos(v)
			lo, hi := min(zero, top), max(zero, top)
			for r := int(math.Floor(lo)); float64(r) < hi; r++ {
				cover := min(hi, float64(r+1)) - max(lo, float64(r))
				glyph := ""
				if v >= 0 {
					glyph = lowerEighths[int(math.Round(cover*8))]
				} else {
					switch {
					case cover >= 0.75:
						glyph = "█"
					case cover >= 0.25:
						glyph = "▀" // below the axis, bars fill from the top
					}
				}
				if glyph == "" || glyph == " " {
					continue
				}
				for x := x0; x < x1; x++ {
					g.set(x, p.axisRow-1-r, glyph, SeriesRole(j))
				}
			}
		}
	}
}

// dotY maps a value to a braille dot row, four per cell, from the top.
func (p *columnPlan) dotY(v float64) int {
	rows := p.axisRow * 4
	full := p.sc.cells()*4 - 1
	y := rows - 1 - int(math.Round(p.sc.pos(v)/float64(p.sc.cells())*float64(full)))
	return min(max(y, 0), rows-1)
}

// drawLines draws each series as a braille line; where lines cross a
// cell, the later series' color wins.
func (p *columnPlan) drawLines(g *Grid) {
	w, h := p.plot.Dx(), p.axisRow
	bits := make([]uint8, w*h)
	owner := make([]int, w*h)
	dot := func(x, y, series int) {
		cx, cy := x/2, y/4
		if cx < 0 || cx >= w || cy < 0 || cy >= h {
			return
		}
		bits[cy*w+cx] |= brailleBit[x%2][y%4]
		owner[cy*w+cx] = series
	}
	for j, s := range p.d.Series {
		px, py, have := 0, 0, false
		for i := 0; i < p.shown && i < len(s.Values); i++ {
			v := s.Values[i]
			if math.IsNaN(v) {
				have = false
				continue
			}
			x, y := p.dotX(i), p.dotY(v)
			if have {
				bresenham(px, py, x, y, func(x, y int) { dot(x, y, j) })
			} else {
				dot(x, y, j)
			}
			px, py, have = x, y, true
		}
	}
	for cy := range h {
		for cx := range w {
			if b := bits[cy*w+cx]; b != 0 {
				g.set(p.plot.Min.X+cx, cy, string(rune(0x2800+int(b))), SeriesRole(owner[cy*w+cx]))
			}
		}
	}
}

// brailleBit is the bit of the braille dot at column x, row y of a cell.
var brailleBit = [2][4]uint8{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

func bresenham(x0, y0, x1, y1 int, plot func(x, y int)) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	e := dx + dy
	for {
		plot(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * e
		if e2 >= dy {
			e += dy
			x0 += sx
		}
		if e2 <= dx {
			e += dx
			y0 += sy
		}
	}
}

func abs(n int) int { return max(n, -n) }

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}
