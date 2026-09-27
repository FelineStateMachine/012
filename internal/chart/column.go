package chart

import (
	"image"
	"math"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// columnPlan lays out column, line and area charts: a value axis on the
// left with labeled ticks, the plot, and a category axis and labels
// below it. Draw adds the legend.
//
//	1,500 ┤ █▆
//	1,000 ┤ ██ ▆
//	  500 ┤ ██ █
//	    0 ┼─────────
//	       Jan Feb
type columnPlan struct {
	d       sheet.ChartData
	kind    columnKind
	stacked bool
	grid    bool
	msg     string
	sc      scale
	labelW  int // width of the tick labels
	axisX   int // column of the value axis
	axisRow int // row of the category axis
	catRow  int
	plot    image.Rectangle
	shown   int // categories drawn
	// Columns: each category is a group gw cells wide, starting off cells
	// into the plot; each series' bar in it (the stack, when stacked) is
	// bw wide.
	gw, off, bw, inset int
}

// columnKind is what a columnPlan draws at each category.
type columnKind uint8

const (
	columnBars columnKind = iota
	columnLine
	columnArea
)

func newColumnPlan(d sheet.ChartData, w, h int, kind columnKind, o sheet.ChartOptions) *columnPlan {
	stacked := o.Stack != sheet.StackNone && kind != columnLine
	d = prepare(d, o, kind != columnLine)
	p := &columnPlan{d: d, kind: kind, stacked: stacked, grid: !o.NoGrid}
	lo, hi, ok := finite(d.Series)
	if stacked {
		lo, hi, ok = stackedRange(d, len(d.Categories))
	}
	if !ok || len(d.Categories) == 0 {
		p.msg = "No numbers to chart"
		return p
	}
	if kind != columnLine && !o.Log {
		lo, hi = min(lo, 0), max(hi, 0)
	}
	p.catRow = h - 1
	p.axisRow = p.catRow - 1
	if p.axisRow < 2 {
		p.msg = "Too small to chart"
		return p
	}
	p.sc = newAxis(lo, hi, p.axisRow, 2, 6, d.Format, o)
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
	if kind != columnBars {
		p.shown = min(len(d.Categories), plotW*2)
		return p
	}
	slots := len(d.Series)
	if stacked {
		slots = 1
	}
	p.shown = max(min(len(d.Categories), plotW/slots), 1)
	p.gw = plotW / p.shown
	gap := 0
	if p.gw > slots {
		gap = max(1, p.gw/4)
	}
	p.bw = max((p.gw-gap)/slots, 1)
	p.inset = (p.gw - p.bw*slots) / 2
	p.off = (plotW - p.shown*p.gw) / 2
	return p
}

// bar returns the columns of series j's bar in category i; stacked, all
// series share the first.
func (p *columnPlan) bar(i, j int) (x0, x1 int) {
	if p.stacked {
		j = 0
	}
	x0 = p.plot.Min.X + p.off + i*p.gw + p.inset + j*p.bw
	return x0, x0 + p.bw
}

// piles returns category i's bars as piles of spans in values, each with
// the series whose slot it takes: one pile when stacked, one per series
// otherwise.
func (p *columnPlan) piles(i int, buf []span) []span {
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
	if p.kind != columnBars {
		return p.plot.Min.X + p.dotX(i)/2
	}
	return p.plot.Min.X + p.off + i*p.gw + p.gw/2
}

func (p *columnPlan) draw(g *Grid, o Options) {
	drawValueAxis(g, p.sc, p.labelW, p.axisX, p.axisRow, p.plot.Min.X, p.plot.Max.X+1)
	p.drawCategories(g)
	if o.Image {
		return
	}
	if p.grid {
		hGridlines(g, p.sc, p.axisRow, p.plot)
	}
	switch p.kind {
	case columnLine:
		p.drawLines(g)
	case columnArea:
		p.drawArea(g)
	default:
		p.drawBars(g)
	}
}

// drawValueAxis draws a vertical value axis at column axisX with a label
// at each tick, and the category axis along row axisRow from x0 to x1.
func drawValueAxis(g *Grid, sc scale, labelW, axisX, axisRow, x0, x1 int) {
	for y := axisRow - sc.cells(); y < axisRow; y++ {
		g.set(axisX, y, "│", Axis)
	}
	for x := x0; x < min(x1, g.W); x++ {
		g.set(x, axisRow, "─", Axis)
	}
	for j := 0; j <= sc.n; j++ {
		y := axisRow - j*sc.k
		label := sc.label(sc.tick(j))
		g.text(labelW-ansi.StringWidth(label), y, label, Label)
		mark := "┤"
		if j == 0 {
			mark = "┼"
		}
		g.set(axisX, y, mark, Axis)
	}
}

// hGridlines draws a dashed line across the plot at each tick above the
// axis.
func hGridlines(g *Grid, sc scale, axisRow int, plot image.Rectangle) {
	for j := 1; j <= sc.n; j++ {
		y := axisRow - j*sc.k
		for x := plot.Min.X; x < plot.Max.X; x++ {
			g.set(x, y, "┈", Gridline)
		}
	}
}

// drawCategories labels as many categories as fit without touching.
func (p *columnPlan) drawCategories(g *Grid) {
	room := p.gw - 1
	if p.kind != columnBars {
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
	drawPiles(p.sc, p.shown, p.stacked, p.piles, lowerEighths, "▀", func(i, j, r int, cell Cell) {
		x0, x1 := p.bar(i, j)
		for x := x0; x < x1; x++ {
			if at := g.At(x, p.axisRow-1-r); at != nil {
				*at = cell
			}
		}
	})
}

// dotY maps a value to a braille dot row, four per cell, from the top.
func (p *columnPlan) dotY(v float64) int {
	rows := p.axisRow * 4
	full := p.sc.cells()*4 - 1
	y := rows - 1 - int(math.Round(p.sc.at(v)/float64(p.sc.cells())*float64(full)))
	return min(max(y, 0), rows-1)
}

// drawLines draws each series as a braille line; where lines cross a
// cell, the later series' color wins.
func (p *columnPlan) drawLines(g *Grid) {
	b := newBraille(p.plot.Dx(), p.axisRow)
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
				b.line(px, py, x, y, j)
			} else {
				b.dot(x, y, j)
			}
			px, py, have = x, y, true
		}
	}
	b.draw(g, p.plot.Min.X, 0)
}

// braille is a canvas of braille dots, two by four per cell, each cell
// colored by the series that drew in it last.
type braille struct {
	w, h  int
	bits  []uint8
	owner []int
}

func newBraille(w, h int) *braille {
	return &braille{w: w, h: h, bits: make([]uint8, w*h), owner: make([]int, w*h)}
}

func (b *braille) dot(x, y, series int) {
	cx, cy := x/2, y/4
	if x < 0 || y < 0 || cx >= b.w || cy >= b.h {
		return
	}
	b.bits[cy*b.w+cx] |= brailleBit[x%2][y%4]
	b.owner[cy*b.w+cx] = series
}

func (b *braille) line(x0, y0, x1, y1, series int) {
	bresenham(x0, y0, x1, y1, func(x, y int) { b.dot(x, y, series) })
}

// draw puts the dots on g with the top-left cell at x, y.
func (b *braille) draw(g *Grid, x, y int) {
	for cy := range b.h {
		for cx := range b.w {
			if bits := b.bits[cy*b.w+cx]; bits != 0 {
				g.set(x+cx, y+cy, string(rune(0x2800+int(bits))), SeriesRole(b.owner[cy*b.w+cx]))
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
