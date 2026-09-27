package chart

import (
	"image"
	"image/color"
	"math"

	"012/internal/sheet"
)

// Palette holds the colors an image draws with: the terminal's colors for
// the series roles, so the image matches the text legend, and a faint
// color for gridlines.
type Palette struct {
	Series [Colors]color.RGBA
	Grid   color.RGBA
}

// Image draws the plot area of the chart Draw lays out with the same
// arguments, cw by ch pixels per cell, on a transparent background. It
// returns nil when there's nothing to plot.
func Image(t sheet.ChartType, d sheet.ChartData, w, h int, o Options, pal Palette) *image.RGBA {
	cw, ch := o.CellW, o.CellH
	if cw <= 0 || ch <= 0 {
		cw, ch = 10, 20
		o.CellW, o.CellH = cw, ch
	}
	if w < 8 || h < 3 {
		return nil
	}
	var plot image.Rectangle
	var draw func(c *canvas)
	switch t {
	case sheet.ChartBar:
		p := newBarPlan(d, w, h)
		plot, draw = p.plot, p.image
		if p.msg != "" {
			return nil
		}
	case sheet.ChartPie:
		p := newPiePlan(d, w, h, o)
		plot, draw = p.disc, p.image
		if p.msg != "" {
			return nil
		}
	default:
		p := newColumnPlan(d, w, h, t == sheet.ChartLine)
		plot, draw = p.plot, p.image
		if p.msg != "" {
			return nil
		}
	}
	if plot.Empty() {
		return nil
	}
	c := &canvas{
		RGBA: image.NewRGBA(image.Rect(0, 0, plot.Dx()*cw, plot.Dy()*ch)),
		cw:   float64(cw), ch: float64(ch), pal: pal,
	}
	draw(c)
	return c.RGBA
}

// canvas is an image with blending and the size of a cell.
type canvas struct {
	*image.RGBA
	cw, ch float64
	pal    Palette
}

// blend paints c over the pixel at x, y with coverage a.
func (cv *canvas) blend(x, y int, c color.RGBA, a float64) {
	if !(image.Point{x, y}.In(cv.Rect)) || a <= 0 {
		return
	}
	a = min(a, 1) * float64(c.A) / 255
	i := cv.PixOffset(x, y)
	p := cv.Pix[i : i+4 : i+4]
	// Premultiplied source over.
	for k, v := range []uint8{c.R, c.G, c.B} {
		p[k] = uint8(math.Round(float64(v)*a + float64(p[k])*(1-a)))
	}
	p[3] = uint8(math.Round(255*a + float64(p[3])*(1-a)))
}

// rect fills x0..x1, y0..y1 in pixels, antialiasing fractional edges.
func (cv *canvas) rect(x0, y0, x1, y1 float64, c color.RGBA) {
	for y := int(math.Floor(y0)); float64(y) < y1; y++ {
		cy := min(y1, float64(y+1)) - max(y0, float64(y))
		for x := int(math.Floor(x0)); float64(x) < x1; x++ {
			cx := min(x1, float64(x+1)) - max(x0, float64(x))
			cv.blend(x, y, c, cx*cy)
		}
	}
}

// hline draws a one pixel horizontal line at y.
func (cv *canvas) hline(y int, c color.RGBA) {
	for x := cv.Rect.Min.X; x < cv.Rect.Max.X; x++ {
		cv.blend(x, y, c, 1)
	}
}

// vline draws a one pixel vertical line at x.
func (cv *canvas) vline(x int, c color.RGBA) {
	for y := cv.Rect.Min.Y; y < cv.Rect.Max.Y; y++ {
		cv.blend(x, y, c, 1)
	}
}

// polyline strokes the points with a round-capped line width px wide.
// Coverage is kept per pixel, so joints aren't painted twice.
func (cv *canvas) polyline(pts [][2]float64, width float64, c color.RGBA) {
	b := cv.Rect
	cover := make([]float64, b.Dx()*b.Dy())
	r := width / 2
	seg := func(ax, ay, bx, by float64) {
		x0, x1 := int(math.Floor(min(ax, bx)-r-1)), int(math.Ceil(max(ax, bx)+r+1))
		y0, y1 := int(math.Floor(min(ay, by)-r-1)), int(math.Ceil(max(ay, by)+r+1))
		for y := max(y0, b.Min.Y); y < min(y1, b.Max.Y); y++ {
			for x := max(x0, b.Min.X); x < min(x1, b.Max.X); x++ {
				d := segDist(float64(x)+0.5, float64(y)+0.5, ax, ay, bx, by)
				if a := r + 0.5 - d; a > 0 {
					i := (y-b.Min.Y)*b.Dx() + (x - b.Min.X)
					cover[i] = max(cover[i], min(a, 1))
				}
			}
		}
	}
	for i := range pts {
		if i == 0 {
			seg(pts[0][0], pts[0][1], pts[0][0], pts[0][1])
			continue
		}
		seg(pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1])
	}
	for i, a := range cover {
		if a > 0 {
			cv.blend(b.Min.X+i%b.Dx(), b.Min.Y+i/b.Dx(), c, a)
		}
	}
}

// segDist is the distance from p to the segment a-b.
func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = min(max(((px-ax)*dx+(py-ay)*dy)/l, 0), 1)
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// image draws columns or lines. Values map exactly, while gridlines sit
// on the cell boundaries the text ticks label.
func (p *columnPlan) image(cv *canvas) {
	bottom := float64(p.plot.Dy()) * cv.ch
	y := func(v float64) float64 { return bottom - p.sc.pos(v)*cv.ch }
	for j := 1; j <= p.sc.n; j++ {
		cv.hline(int(math.Round(y(p.sc.tick(j)))), cv.pal.Grid)
	}
	if p.sc.lo < 0 && !p.line {
		cv.hline(int(math.Round(y(0))), cv.pal.Grid)
	}
	if p.line {
		width := max(cv.ch/8, 2)
		for j, s := range p.d.Series {
			var run [][2]float64
			flush := func() {
				if len(run) > 0 {
					cv.polyline(run, width, cv.pal.Series[j%Colors])
				}
				run = run[:0]
			}
			for i := 0; i < p.shown && i < len(s.Values); i++ {
				if math.IsNaN(s.Values[i]) {
					flush()
					continue
				}
				x := (float64(p.dotX(i)) + 0.5) * cv.cw / 2
				run = append(run, [2]float64{x, y(s.Values[i])})
			}
			flush()
		}
		return
	}
	inset := min(cv.cw/6, 2)
	for j, s := range p.d.Series {
		for i := 0; i < p.shown && i < len(s.Values); i++ {
			v := s.Values[i]
			if math.IsNaN(v) {
				continue
			}
			x0, x1 := p.bar(i, j)
			px0 := float64(x0-p.plot.Min.X)*cv.cw + inset
			px1 := float64(x1-p.plot.Min.X)*cv.cw - inset
			cv.rect(px0, min(y(v), y(0)), px1, max(y(v), y(0)), cv.pal.Series[j%Colors])
		}
	}
}

// image draws horizontal bars.
func (p *barPlan) image(cv *canvas) {
	x := func(v float64) float64 { return p.sc.pos(v) * cv.cw }
	for j := 1; j <= p.sc.n; j++ {
		cv.vline(int(math.Round(x(p.sc.tick(j))))-1, cv.pal.Grid)
	}
	if p.sc.lo < 0 {
		cv.vline(int(math.Round(x(0))), cv.pal.Grid)
	}
	inset := min(cv.ch/8, 2)
	for j, s := range p.d.Series {
		for i := 0; i < p.shown && i < len(s.Values); i++ {
			v := s.Values[i]
			if math.IsNaN(v) {
				continue
			}
			row := float64(p.barRow(i, j))
			cv.rect(min(x(v), x(0)), row*cv.ch+inset, max(x(v), x(0)), (row+1)*cv.ch-inset, cv.pal.Series[j%Colors])
		}
	}
}

// image draws the pie, supersampled, with a thin gap between slices.
func (p *piePlan) image(cv *canvas) {
	b := cv.Rect
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	r := min(cx, cy) - 1
	const n = 4 // samples per pixel along each axis
	gap := 1.0  // pixels between slices
	counts := make([]float64, len(p.slices))
	for y := range b.Dy() {
		for x := range b.Dx() {
			clear(counts)
			for sy := range n {
				for sx := range n {
					dx := (float64(x) + (float64(sx)+0.5)/n - cx) / r
					dy := (float64(y) + (float64(sy)+0.5)/n - cy) / r
					i := p.sliceAt(dx, dy)
					if i < 0 || len(p.slices) > 1 && p.nearEdge(i, dx, dy, gap/r) {
						continue
					}
					counts[i]++
				}
			}
			for i, k := range counts {
				if k > 0 {
					cv.blend(x, y, cv.pal.Series[i%Colors], k/(n*n))
				}
			}
		}
	}
}

// nearEdge reports whether a point of slice i lies within half of gap
// (as a fraction of the radius) of the slice's straight edges.
func (p *piePlan) nearEdge(i int, dx, dy, gap float64) bool {
	for _, f := range []float64{p.slices[i].from, p.slices[i].to} {
		a := f * 2 * math.Pi
		// The edge runs from the center toward (sin a, -cos a).
		ex, ey := math.Sin(a), -math.Cos(a)
		along := dx*ex + dy*ey
		if along > 0 && math.Abs(dx*ey-dy*ex) < gap/2 {
			return true
		}
	}
	return false
}
