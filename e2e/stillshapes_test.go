package e2e

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// shape builds a filled outline for the rasterizer. Every polygon is
// wound the same way, so overlapping parts add up rather than cancel,
// except holes, wound the other way.
type shape struct {
	z *vector.Rasterizer
}

type pt = [2]float32

// poly adds a closed polygon; hole winds it the other way.
func (s *shape) poly(pts []pt, hole bool) {
	var area float32
	for i, p := range pts {
		q := pts[(i+1)%len(pts)]
		area += p[0]*q[1] - q[0]*p[1]
	}
	if (area < 0) != hole {
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	s.z.MoveTo(pts[0][0], pts[0][1])
	for _, p := range pts[1:] {
		s.z.LineTo(p[0], p[1])
	}
	s.z.ClosePath()
}

// arc is n+1 points along a circle's arc from a0 to a1 (radians, y down).
func arc(cx, cy, r, a0, a1 float32, n int) []pt {
	pts := make([]pt, 0, n+1)
	for i := 0; i <= n; i++ {
		a := float64(a0 + (a1-a0)*float32(i)/float32(n))
		pts = append(pts, pt{cx + r*float32(math.Cos(a)), cy + r*float32(math.Sin(a))})
	}
	return pts
}

func (s *shape) disk(cx, cy, r float32) {
	pts := arc(cx, cy, r, 0, 2*math.Pi, 32)
	s.poly(pts[:32], false)
}

// ring is a circle's outline t wide, centered on radius r.
func (s *shape) ring(cx, cy, r, t float32) {
	outer := arc(cx, cy, r+t/2, 0, 2*math.Pi, 32)
	inner := arc(cx, cy, r-t/2, 0, 2*math.Pi, 32)
	s.poly(outer[:32], false)
	s.poly(inner[:32], true)
}

// pie is the slice of a disk from a0 to a1.
func (s *shape) pie(cx, cy, r, a0, a1 float32) {
	s.poly(append([]pt{{cx, cy}}, arc(cx, cy, r, a0, a1, 24)...), false)
}

// seg is a straight line t wide from a to b.
func (s *shape) seg(a, b pt, t float32) {
	dx, dy := b[0]-a[0], b[1]-a[1]
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l == 0 {
		return
	}
	nx, ny := -dy/l*t/2, dx/l*t/2
	s.poly([]pt{{a[0] + nx, a[1] + ny}, {b[0] + nx, b[1] + ny}, {b[0] - nx, b[1] - ny}, {a[0] - nx, a[1] - ny}}, false)
}

// stroke is a line through pts, t wide, with round joins and ends.
func (s *shape) stroke(pts []pt, t float32) {
	for i := range pts {
		if i > 0 {
			s.seg(pts[i-1], pts[i], t)
		}
		s.disk(pts[i][0], pts[i][1], t/2)
	}
}

// roundRect is a rectangle with corners of radius r.
func (s *shape) roundRect(x0, y0, x1, y1, r float32) {
	var pts []pt
	pts = append(pts, arc(x1-r, y0+r, r, -math.Pi/2, 0, 8)...)
	pts = append(pts, arc(x1-r, y1-r, r, 0, math.Pi/2, 8)...)
	pts = append(pts, arc(x0+r, y1-r, r, math.Pi/2, math.Pi, 8)...)
	pts = append(pts, arc(x0+r, y0+r, r, math.Pi, 3*math.Pi/2, 8)...)
	s.poly(pts, false)
}

// symbol draws in a square the cell's width, centered on the cell:
// (u, v) from 0 to 1 across it.
type symbol struct {
	*shape
	x0, y0, side float32
}

func (y symbol) p(u, v float32) pt { return pt{y.x0 + u*y.side, y.y0 + v*y.side} }

func (y symbol) tri(a, b, c pt) {
	y.poly([]pt{y.p(a[0], a[1]), y.p(b[0], b[1]), y.p(c[0], c[1])}, false)
}

func (y symbol) line(t float32, uv ...float32) {
	var pts []pt
	for i := 0; i+1 < len(uv); i += 2 {
		pts = append(pts, y.p(uv[i], uv[i+1]))
	}
	y.stroke(pts, t*y.side)
}

func (y symbol) box(u0, v0, u1, v1, t float32) {
	y.line(t, u0, v0, u1, v0, u1, v1, u0, v1, u0, v0)
}

func (y symbol) rect(u0, v0, u1, v1 float32) {
	y.poly([]pt{y.p(u0, v0), y.p(u1, v0), y.p(u1, v1), y.p(u0, v1)}, false)
}

// symbols are the shapes 012 draws its chrome with that fonts draw at
// odd sizes or not at all: triangles, circles, checks and the notebook
// toolbar's icons.
var symbols = map[rune]func(y symbol){
	'▶': func(y symbol) { y.tri(pt{.2, .12}, pt{.92, .5}, pt{.2, .88}) },
	'▸': func(y symbol) { y.tri(pt{.3, .28}, pt{.76, .5}, pt{.3, .72}) },
	'◀': func(y symbol) { y.tri(pt{.8, .12}, pt{.08, .5}, pt{.8, .88}) },
	'◂': func(y symbol) { y.tri(pt{.7, .28}, pt{.24, .5}, pt{.7, .72}) },
	'▼': func(y symbol) { y.tri(pt{.08, .22}, pt{.92, .22}, pt{.5, .86}) },
	'▾': func(y symbol) { y.tri(pt{.22, .34}, pt{.78, .34}, pt{.5, .7}) },
	'▲': func(y symbol) { y.tri(pt{.08, .8}, pt{.92, .8}, pt{.5, .16}) },
	'▴': func(y symbol) { y.tri(pt{.22, .68}, pt{.78, .68}, pt{.5, .32}) },
	'■': func(y symbol) { y.rect(.14, .14, .86, .86) },
	'●': func(y symbol) { y.disk(y.p(.5, .5)[0], y.p(.5, .5)[1], .4*y.side) },
	'○': func(y symbol) { circle(y, 0, 0) },
	'◔': func(y symbol) { circle(y, -math.Pi/2, 0) },
	'◑': func(y symbol) { circle(y, -math.Pi/2, math.Pi/2) },
	'◕': func(y symbol) { circle(y, -math.Pi/2, math.Pi) },
	'✓': func(y symbol) { y.line(.12, .14, .54, .4, .8, .88, .2) },
	'✗': func(y symbol) { y.line(.12, .2, .2, .8, .8); y.line(.12, .8, .2, .2, .8) },
	'❯': func(y symbol) { y.line(.16, .3, .16, .72, .5, .3, .84) },
	'⧉': func(y symbol) { y.box(.08, .08, .64, .64, .08); y.box(.36, .36, .92, .92, .08) },
	'⎘': func(y symbol) { y.box(.16, .16, .84, .94, .08); y.rect(.34, .04, .66, .26) },
	'↻': reload,
	'↵': func(y symbol) {
		y.line(.09, .84, .12, .84, .62, .3, .62)
		y.tri(pt{.06, .62}, pt{.36, .38}, pt{.36, .86})
	},
	'✂': func(y symbol) {
		y.ring(y.p(.22, .26)[0], y.p(.22, .26)[1], .13*y.side, .08*y.side)
		y.ring(y.p(.22, .74)[0], y.p(.22, .74)[1], .13*y.side, .08*y.side)
		y.line(.09, .32, .34, .94, .7)
		y.line(.09, .32, .66, .94, .3)
	},
}

// circle is an outline with the slice from a0 to a1 filled.
func circle(y symbol, a0, a1 float32) {
	c := y.p(.5, .5)
	y.ring(c[0], c[1], .36*y.side, .09*y.side)
	if a1 > a0 {
		y.pie(c[0], c[1], .38*y.side, a0, a1)
	}
}

// reload is ↻: most of a circle, clockwise, with the arrowhead at its
// end.
func reload(y symbol) {
	c := y.p(.5, .52)
	r := .32 * y.side
	a0, a1 := float32(-math.Pi/5), float32(1.4*math.Pi)
	y.stroke(arc(c[0], c[1], r, a0, a1, 24), .09*y.side)
	end := pt{c[0] + r*float32(math.Cos(float64(a1))), c[1] + r*float32(math.Sin(float64(a1)))}
	dir := pt{-float32(math.Sin(float64(a1))), float32(math.Cos(float64(a1)))}
	n := pt{-dir[1], dir[0]}
	h := .22 * y.side
	y.poly([]pt{
		{end[0] + dir[0]*h, end[1] + dir[1]*h},
		{end[0] + n[0]*h*.8, end[1] + n[1]*h*.8},
		{end[0] - n[0]*h*.8, end[1] - n[1]*h*.8},
	}, false)
}

// drawSpecial draws r as terminals draw it rather than from the font:
// box drawing, blocks and braille fill the cell and join their
// neighbors, and symbols are drawn at one size. False when r is text.
func drawSpecial(img *image.NRGBA, r rune, cell image.Rectangle, c color.RGBA, fs *stillFaces) bool {
	switch {
	case r >= 0x2500 && r <= 0x257f:
		return boxDrawing(img, r, cell, c)
	case r >= 0x2580 && r <= 0x259f:
		return blockElement(img, r, cell, c)
	case r >= 0x2800 && r <= 0x28ff:
		braille(img, r, cell, c)
		return true
	}
	draw, ok := symbols[r]
	if !ok {
		return false
	}
	// A symbol is as big as a capital letter, centered on one, and may
	// reach a little into the cells beside it, as fonts draw them.
	side := float32(stillFontSize) * .9
	area := cell.Inset(-cell.Dx() / 4)
	mid := float32(fs.ascent+cell.Dx()/4) - float32(stillFontSize)*.33
	fillShape(img, area, c, func(s *shape) {
		draw(symbol{shape: s, x0: (float32(area.Dx()) - side) / 2, y0: mid - side/2, side: side})
	})
	return true
}

// braille draws the dots of a braille pattern, two columns of four.
func braille(img *image.NRGBA, r rune, cell image.Rectangle, c color.RGBA) {
	bits := int(r - 0x2800)
	// dots 1-3 and 7 down the left, 4-6 and 8 down the right
	pos := [8][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {0, 3}, {1, 3}}
	w, h := float32(cell.Dx()), float32(cell.Dy())
	fillShape(img, cell, c, func(s *shape) {
		for i, p := range pos {
			if bits&(1<<i) != 0 {
				s.disk(w*(.27+.46*float32(p[0])), h*(.14+.24*float32(p[1])), w*.14)
			}
		}
	})
}
