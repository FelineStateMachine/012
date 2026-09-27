package chart

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// scatterPlan lays out a scatter chart as Sheets does: the first series
// is the X values and every other one a series of Y values plotted
// against them, each point a dot, with an optional linear trend line per
// series. The value axis on the left is Y, with the chart's axis options;
// X runs along the bottom.
//
//	60 ┤    ⠛  ⠛⠛
//	40 ┤ ⠛⠛  ⠛
//	20 ┤⠛
//	 0 ┼──┬──┬──┬
//	   0  2  4  6
type scatterPlan struct {
	xs      []float64
	ys      []sheet.ChartSeries
	trend   bool
	grid    bool
	msg     string
	xsc     scale
	ysc     scale
	labelW  int
	axisX   int
	axisRow int
	tickRow int
	plot    image.Rectangle
}

// scatterSeries splits d into X values and Y series: the first series is
// X when there are two or more, and otherwise the categories are, read as
// numbers or else counted from 1.
func scatterSeries(d sheet.ChartData) ([]float64, []sheet.ChartSeries) {
	if len(d.Series) >= 2 {
		return d.Series[0].Values, d.Series[1:]
	}
	xs := make([]float64, len(d.Categories))
	for i, c := range d.Categories {
		x, err := strconv.ParseFloat(c, 64)
		if err != nil {
			x = float64(i + 1)
		}
		xs[i] = x
	}
	return xs, d.Series
}

func newScatterPlan(d sheet.ChartData, w, h int, o sheet.ChartOptions) *scatterPlan {
	d = prepare(d, o, false)
	xs, ys := scatterSeries(d)
	p := &scatterPlan{xs: xs, ys: ys, trend: o.Trend, grid: !o.NoGrid}
	xlo, xhi, okX := finite([]sheet.ChartSeries{{Values: xs}})
	ylo, yhi, okY := finite(ys)
	if !okX || !okY {
		p.msg = "No numbers to chart"
		return p
	}
	p.tickRow = h - 1
	p.axisRow = h - 2
	if p.axisRow < 2 {
		p.msg = "Too small to chart"
		return p
	}
	p.ysc = newAxis(ylo, yhi, p.axisRow, 2, 6, d.Format, o)
	for j := 0; j <= p.ysc.n; j++ {
		p.labelW = max(p.labelW, ansi.StringWidth(p.ysc.label(p.ysc.tick(j))))
	}
	p.axisX = p.labelW + 1
	x0 := p.axisX + 1
	labelW := 1
	for range 2 {
		plotW := w - x0 - (labelW+1)/2
		if plotW < 2 {
			p.msg = "Too small to chart"
			return p
		}
		p.xsc = newScale(xlo, xhi, plotW, labelW+2, 6, d.Format)
		for j := 0; j <= p.xsc.n; j++ {
			labelW = max(labelW, ansi.StringWidth(p.xsc.label(p.xsc.tick(j))))
		}
	}
	p.plot = image.Rect(x0, 0, x0+p.xsc.cells(), p.axisRow)
	return p
}

// points calls fn with each point of every series, in cells from the
// plot's bottom left.
func (p *scatterPlan) points(fn func(j int, x, y float64)) {
	for j, s := range p.ys {
		for i, x := range p.xs {
			y := value(s, i)
			if math.IsNaN(y) || math.IsNaN(x) || math.IsInf(x, 0) {
				continue
			}
			fn(j, p.xsc.at(x), p.ysc.at(y))
		}
	}
}

// fit returns the least squares line y = a + b*x through series j's
// points, and false with fewer than two distinct X values.
func (p *scatterPlan) fit(j int) (a, b float64, ok bool) {
	var n, sx, sy, sxx, sxy float64
	for i, x := range p.xs {
		y := value(p.ys[j], i)
		if math.IsNaN(y) || math.IsNaN(x) || math.IsInf(x, 0) {
			continue
		}
		n++
		sx, sy, sxx, sxy = sx+x, sy+y, sxx+x*x, sxy+x*y
	}
	d := n*sxx - sx*sx
	if n < 2 || math.Abs(d) < 1e-12*max(1, sxx*n) {
		return 0, 0, false
	}
	b = (n*sxy - sx*sy) / d
	return (sy - b*sx) / n, b, true
}

// trendPoints calls fn with points along series j's trend line, in cells
// from the plot's bottom left, steps per cell across the plot; ok is
// false where the line leaves the value axis.
func (p *scatterPlan) trendPoints(j int, steps float64, fn func(x, y float64, ok bool)) {
	a, b, ok := p.fit(j)
	if !ok {
		return
	}
	cells := float64(p.xsc.cells())
	n := int(cells * steps)
	for k := 0; k <= n; k++ {
		x := cells * float64(k) / float64(max(n, 1))
		v := a + b*(p.xsc.lo+x/cells*(p.xsc.hi-p.xsc.lo))
		y := p.ysc.pos(v)
		fn(x, y, !math.IsNaN(y) && y >= 0 && y <= float64(p.ysc.cells()))
	}
}

func (p *scatterPlan) draw(g *Grid, o Options) {
	drawValueAxis(g, p.ysc, p.labelW, p.axisX, p.axisRow, p.plot.Min.X, p.plot.Max.X)
	drawXTicks(g, p.xsc, p.axisX, p.plot.Min.X, p.axisRow, p.tickRow)
	if o.Image {
		return
	}
	if p.grid {
		hGridlines(g, p.ysc, p.axisRow, p.plot)
	}
	w, h := p.plot.Dx(), p.axisRow
	b := newBraille(w, h)
	// Dots: two per cell across, four down, from the top left.
	dot := func(x, y float64) (int, int) {
		dx := int(math.Round(x / float64(max(p.xsc.cells(), 1)) * float64(w*2-1)))
		dy := h*4 - 1 - int(math.Round(y/float64(max(p.ysc.cells(), 1))*float64(p.ysc.cells()*4-1)))
		return min(max(dx, 0), w*2-1), min(max(dy, 0), h*4-1)
	}
	if p.trend {
		for j := range p.ys {
			px, py, have := 0, 0, false
			p.trendPoints(j, 2, func(x, y float64, ok bool) {
				if !ok {
					have = false
					return
				}
				dx, dy := dot(x, y)
				if have {
					b.line(px, py, dx, dy, j)
				}
				px, py, have = dx, dy, true
			})
		}
	}
	p.points(func(j int, x, y float64) {
		dx, dy := dot(x, y)
		// A point is two dots by two, kept inside the plot.
		dx, dy = min(dx, w*2-2), max(dy-1, 0)
		for k := range 4 {
			b.dot(dx+k%2, dy+k/2, j)
		}
	})
	b.draw(g, p.plot.Min.X, 0)
}

// image draws the points as discs and the trend lines through them.
func (p *scatterPlan) image(cv *canvas) {
	bottom := float64(p.plot.Dy()) * cv.ch
	px := func(x float64) float64 { return x * cv.cw }
	py := func(y float64) float64 { return bottom - y*cv.ch }
	if p.grid {
		for j := 1; j <= p.ysc.n; j++ {
			cv.hline(int(math.Round(py(float64(j*p.ysc.k)))), cv.pal.Grid)
		}
		for j := 1; j <= p.xsc.n; j++ {
			cv.vline(int(math.Round(px(float64(j*p.xsc.k))))-1, cv.pal.Grid)
		}
	}
	if p.trend {
		width := max(cv.ch/12, 1.5)
		for j := range p.ys {
			var run [][2]float64
			flush := func() {
				if len(run) > 1 {
					cv.polyline(run, width, cv.pal.Series[j%Colors])
				}
				run = run[:0]
			}
			p.trendPoints(j, 1, func(x, y float64, ok bool) {
				if !ok {
					flush()
					return
				}
				run = append(run, [2]float64{px(x), py(y)})
			})
			flush()
		}
	}
	r := max(min(cv.cw, cv.ch)*0.3, 2.5)
	p.points(func(j int, x, y float64) {
		cv.disc(px(x), py(y), r, cv.pal.Series[j%Colors])
	})
}

// disc fills a circle of radius r centered at cx, cy, antialiased.
func (cv *canvas) disc(cx, cy, r float64, c color.RGBA) {
	for y := int(math.Floor(cy - r - 1)); y <= int(math.Ceil(cy+r+1)); y++ {
		for x := int(math.Floor(cx - r - 1)); x <= int(math.Ceil(cx+r+1)); x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			cv.blend(x, y, c, r+0.5-d)
		}
	}
}
