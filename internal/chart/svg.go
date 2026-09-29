package chart

import (
	"fmt"
	"html"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// SVG charts are the text chart and the image in one vector drawing,
// for the HTML export: the axes, labels and legend Draw lays out, as
// text on a grid of SVGCellW by SVGCellH pixels, and the plot area as
// the shapes Image paints, as SVG shapes rather than pixels. Colors are
// classes the page styles, so a chart follows the page's light or dark
// palette:
//
//	ct-ax, ct-lb, ct-mu  text of the axes, the labels and messages
//	ct-s0 .. ct-s5       text in a series' color
//	cf-0 .. cf-5         shapes filled in a series' color
//	cs-0 .. cs-5         lines stroked in a series' color
//	cs-gl                gridlines
//	cs-box               axes, in their text class's color
//	cs-gap               the gaps between a pie's slices
const (
	SVGCellW = 9
	SVGCellH = 21
)

// vecGrid and vecSeries stand for the colors in the palette a vector
// canvas paints with, so each shape finds its class again: the red
// channel is the series and the blue one says it's a gridline.
var vecGrid = color.RGBA{B: 1, A: 255}

func vecSeries(i int) color.RGBA { return color.RGBA{R: uint8(i), A: 255} }

// SVG draws d as a chart of type t in w by h cells, as an svg element
// of w*SVGCellW by h*SVGCellH pixels whose title is title.
func SVG(t sheet.ChartType, d sheet.ChartData, w, h int, o Options, title string) string {
	w, h = max(w, 0), max(h, 0)
	o.Image, o.CellW, o.CellH = true, SVGCellW, SVGCellH
	g := Draw(t, d, w, h, o)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" class="chart" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`,
		w*SVGCellW, h*SVGCellH, w*SVGCellW, h*SVGCellH)
	if title != "" {
		b.WriteString("<title>" + html.EscapeString(title) + "</title>")
	}
	if !g.Plot.Empty() {
		b.WriteString(plotSVG(t, d, w, h, o, g.Plot))
	}
	gridText(&b, g)
	b.WriteString("</svg>")
	return b.String()
}

// plotSVG is the plot area's shapes, placed over its cells.
func plotSVG(t sheet.ChartType, d sheet.ChartData, w, h int, o Options, plot image.Rectangle) string {
	p, _ := planFor(t, d, w, h, o)
	pal := Palette{Grid: vecGrid}
	for i := range pal.Series {
		pal.Series[i] = vecSeries(i)
	}
	v := &vector{}
	cv := &canvas{
		RGBA: &image.RGBA{Rect: image.Rect(0, 0, plot.Dx()*SVGCellW, plot.Dy()*SVGCellH)},
		cw:   SVGCellW, ch: SVGCellH, pal: pal, vec: v,
	}
	p.image(cv)
	v.flush()
	return fmt.Sprintf(`<g transform="translate(%d %d)">%s</g>`, plot.Min.X*SVGCellW, plot.Min.Y*SVGCellH, v.b.String())
}

// gridText writes the text chart's cells, a run of one role at a time.
func gridText(b *strings.Builder, g *Grid) {
	for y := range g.H {
		x := 0
		for x < g.W {
			c := g.At(x, y)
			if c.Text == " " || c.Text == "" || c.Fg == None {
				x++
				continue
			}
			if arms, ok := boxArms[c.Text]; ok {
				boxLines(b, x, y, arms, textClass(c.Fg))
				x++
				continue
			}
			start, role := x, c.Fg
			var run strings.Builder
			for x < g.W && g.At(x, y).Fg == role && !isBox(g.At(x, y).Text) {
				run.WriteString(g.At(x, y).Text)
				x++
			}
			fmt.Fprintf(b, `<text x="%d" y="%.1f" class="%s" xml:space="preserve">%s</text>`, start*SVGCellW,
				float64(y)*SVGCellH+SVGCellH*0.72, textClass(role), html.EscapeString(strings.TrimRight(run.String(), " ")))
		}
	}
}

// boxArms are the box-drawing characters the axes use, as the arms
// each draws from the middle of its cell: up, down, left, right. They
// are drawn as lines, which meet whatever font the page has.
var boxArms = map[string][4]bool{
	"│": {true, true, false, false}, "─": {false, false, true, true},
	"└": {true, false, false, true}, "┘": {true, false, true, false},
	"┌": {false, true, false, true}, "┐": {false, true, true, false},
	"├": {true, true, false, true}, "┤": {true, true, true, false},
	"┬": {false, true, true, true}, "┴": {true, false, true, true},
	"┼": {true, true, true, true},
}

func isBox(s string) bool { _, ok := boxArms[s]; return ok }

// boxLines draws a box-drawing character's arms in cell x, y.
func boxLines(b *strings.Builder, x, y int, arms [4]bool, class string) {
	cx, cy := float64(x)*SVGCellW+SVGCellW/2, float64(y)*SVGCellH+SVGCellH/2
	ends := [4][2]float64{{cx, float64(y) * SVGCellH}, {cx, float64(y+1) * SVGCellH},
		{float64(x) * SVGCellW, cy}, {float64(x+1) * SVGCellW, cy}}
	for i, on := range arms {
		if on {
			fmt.Fprintf(b, `<line x1="%s" y1="%s" x2="%s" y2="%s" class="%s cs-box"/>`, num(cx), num(cy), num(ends[i][0]), num(ends[i][1]), class)
		}
	}
}

func textClass(r Role) string {
	switch {
	case r == Axis, r == Gridline:
		return "ct-ax"
	case r == Label:
		return "ct-lb"
	case r >= Series:
		return fmt.Sprintf("ct-s%d", int(r-Series)%Colors)
	}
	return "ct-mu"
}

// vector collects a canvas's shapes as SVG. Area charts paint a pixel
// column at a time; those columns are joined into one polygon per color
// for as long as they run on without a gap.
type vector struct {
	b      strings.Builder
	strips map[color.RGBA]*strip
	order  []color.RGBA
}

// strip is a run of one-pixel columns of one color: their tops and
// bottoms, left to right.
type strip struct {
	x0, x1     float64
	top, below [][2]float64
}

func paint(c color.RGBA) (series int, grid bool, opacity string) {
	if c.A < 255 {
		opacity = fmt.Sprintf(` fill-opacity="%.2f"`, float64(c.A)/255)
	}
	return int(c.R) % Colors, c.B == 1, opacity
}

func (v *vector) rect(x0, y0, x1, y1 float64, c color.RGBA) {
	if x1 <= x0 || y1 <= y0 {
		return
	}
	if x1-x0 <= 1.001 {
		v.column(x0, y0, x1, y1, c)
		return
	}
	v.flush()
	s, _, op := paint(c)
	fmt.Fprintf(&v.b, `<rect x="%s" y="%s" width="%s" height="%s" class="cf-%d"%s/>`, num(x0), num(y0), num(x1-x0), num(y1-y0), s, op)
}

// column adds a pixel column to its color's strip, starting a new strip
// when it doesn't continue the last one.
func (v *vector) column(x0, y0, x1, y1 float64, c color.RGBA) {
	if v.strips == nil {
		v.strips = map[color.RGBA]*strip{}
	}
	st := v.strips[c]
	if st != nil && math.Abs(st.x1-x0) > 0.001 {
		v.emitStrip(c, st)
		st = nil
	}
	if st == nil {
		st = &strip{x0: x0}
		v.strips[c] = st
		v.order = append(v.order, c)
	}
	st.x1 = x1
	st.top = append(st.top, [2]float64{x0, y0}, [2]float64{x1, y0})
	st.below = append(st.below, [2]float64{x0, y1}, [2]float64{x1, y1})
}

// flush writes the strips still open, in the order they started, so
// they're under whatever is drawn next.
func (v *vector) flush() {
	for _, c := range v.order {
		if st := v.strips[c]; st != nil {
			v.emitStrip(c, st)
		}
	}
	v.order = v.order[:0]
}

func (v *vector) emitStrip(c color.RGBA, st *strip) {
	delete(v.strips, c)
	pts := simplify(st.top)
	below := simplify(st.below)
	for i := len(below) - 1; i >= 0; i-- {
		pts = append(pts, below[i])
	}
	s, _, op := paint(c)
	fmt.Fprintf(&v.b, `<polygon points="%s" class="cf-%d"%s/>`, points(pts), s, op)
}

func (v *vector) line(x0, y0, x1, y1 float64, c color.RGBA) {
	v.flush()
	class := "cs-gl"
	if s, grid, _ := paint(c); !grid {
		class = fmt.Sprintf("cs-%d", s)
	}
	fmt.Fprintf(&v.b, `<line x1="%s" y1="%s" x2="%s" y2="%s" class="%s"/>`, num(x0), num(y0), num(x1), num(y1), class)
}

func (v *vector) polyline(pts [][2]float64, width float64, c color.RGBA) {
	v.flush()
	s, _, _ := paint(c)
	if len(pts) == 1 {
		fmt.Fprintf(&v.b, `<circle cx="%s" cy="%s" r="%s" class="cf-%d"/>`, num(pts[0][0]), num(pts[0][1]), num(width/2), s)
		return
	}
	fmt.Fprintf(&v.b, `<polyline points="%s" fill="none" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round" class="cs-%d"/>`,
		points(pts), num(width), s)
}

func (v *vector) disc(cx, cy, r float64, c color.RGBA) {
	v.flush()
	s, _, _ := paint(c)
	fmt.Fprintf(&v.b, `<circle cx="%s" cy="%s" r="%s" class="cf-%d"/>`, num(cx), num(cy), num(r), s)
}

// vector draws the pie as one path a slice, with the page's background
// stroked between them as the image leaves a gap.
func (p *piePlan) vector(cv *canvas) {
	v := cv.vec
	b := cv.Rect
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	r := min(cx, cy) - 1
	if len(p.slices) == 1 {
		fmt.Fprintf(&v.b, `<circle cx="%s" cy="%s" r="%s" class="cf-0"/>`, num(cx), num(cy), num(r))
		return
	}
	at := func(f float64) string {
		a := f * 2 * math.Pi
		return num(cx+r*math.Sin(a)) + " " + num(cy-r*math.Cos(a))
	}
	for i, s := range p.slices {
		if s.to <= s.from {
			continue
		}
		large := 0
		if s.to-s.from > 0.5 {
			large = 1
		}
		fmt.Fprintf(&v.b, `<path d="M%s %s L%s A%s %s 0 %d 1 %s Z" class="cf-%d cs-gap"/>`,
			num(cx), num(cy), at(s.from), num(r), num(r), large, at(s.to), i%Colors)
	}
}

// simplify drops the points of a path that lie on the line between
// their neighbors.
func simplify(pts [][2]float64) [][2]float64 {
	if len(pts) < 3 {
		return pts
	}
	out := [][2]float64{pts[0]}
	for i := 1; i < len(pts)-1; i++ {
		a, b, c := out[len(out)-1], pts[i], pts[i+1]
		if math.Abs((b[0]-a[0])*(c[1]-a[1])-(b[1]-a[1])*(c[0]-a[0])) > 0.25 {
			out = append(out, b)
		}
	}
	return append(out, pts[len(pts)-1])
}

func points(pts [][2]float64) string {
	var b strings.Builder
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(num(p[0]) + "," + num(p[1]))
	}
	return b.String()
}

// num writes a coordinate to a tenth of a pixel, without trailing
// zeros.
func num(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	s = strings.TrimSuffix(s, ".0")
	if s == "-0" {
		return "0"
	}
	return s
}
