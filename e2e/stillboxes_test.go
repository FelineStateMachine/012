package e2e

import (
	"image"
	"image/color"
	"math"
)

// Box drawing and block elements, drawn to the cell's edges as
// terminals draw them, so a frame's lines meet across cells.

// boxArms gives each box-drawing character the weight of its four arms,
// up, right, down and left: 0 none, 1 light, 2 heavy, 3 double.
var boxArms = map[rune]string{
	'─': "0101", '━': "0202", '│': "1010", '┃': "2020",
	'╴': "0001", '╵': "1000", '╶': "0100", '╷': "0010",
	'╸': "0002", '╹': "2000", '╺': "0200", '╻': "0020",
	'╼': "0201", '╽': "1020", '╾': "0102", '╿': "2010",
	'╭': "0110", '╮': "0011", '╯': "1001", '╰': "1100",
}

// boxRuns are the characters U+250C to U+254B and U+2550 to U+256C, in
// order, as their arms.
const (
	boxRun1 = "0110 0210 0120 0220 0011 0012 0021 0022 1100 1200 2100 2200 1001 1002 2001 2002 " +
		"1110 1210 2110 1120 2120 2210 1220 2220 1011 1012 2011 1021 2021 2012 1022 2022 " +
		"0111 0112 0211 0212 0121 0122 0221 0222 1101 1102 1201 1202 2101 2102 2201 2202 " +
		"1111 1112 1211 1212 2111 1121 2121 2112 2211 1122 1221 2212 1222 2122 2221 2222"
	boxRun2 = "0303 3030 0310 0130 0330 0013 0031 0033 1300 3100 3300 1003 3001 3003 " +
		"1310 3130 3330 1013 3031 3033 0313 0131 0333 1303 3101 3303 1313 3131 3333"
)

// boxDashes are the dashed lines: vertical or not, heavy or not, and
// how many dashes a cell.
var boxDashes = map[rune]struct {
	vertical, heavy bool
	n               int
}{
	'┄': {false, false, 3}, '┅': {false, true, 3}, '┆': {true, false, 3}, '┇': {true, true, 3},
	'┈': {false, false, 4}, '┉': {false, true, 4}, '┊': {true, false, 4}, '┋': {true, true, 4},
	'╌': {false, false, 2}, '╍': {false, true, 2}, '╎': {true, false, 2}, '╏': {true, true, 2},
}

func init() {
	for i, arms := range splitFields(boxRun1) {
		boxArms[rune(0x250c+i)] = arms
	}
	for i, arms := range splitFields(boxRun2) {
		boxArms[rune(0x2550+i)] = arms
	}
}

func splitFields(s string) []string {
	var out []string
	for i := 0; i+4 <= len(s); i += 5 {
		out = append(out, s[i:i+4])
	}
	return out
}

// boxCell is one cell being drawn: its rectangle, center and the widths
// of its lines.
type boxCell struct {
	img    *image.NRGBA
	cell   image.Rectangle
	c      color.RGBA
	mx, my float32 // the center
	t, d   float32 // a light line's width; a double line's offset from center
	w      [4]int  // arms' weights
}

// boxDrawing draws U+2500 to U+257F; false for one it doesn't know.
func boxDrawing(img *image.NRGBA, r rune, cell image.Rectangle, c color.RGBA) bool {
	t := float32(lineWidth(cell.Dx()))
	b := boxCell{img: img, cell: cell, c: c, t: t, d: t * 2,
		mx: float32(cell.Min.X) + float32(cell.Dx())/2, my: float32(cell.Min.Y) + float32(cell.Dy())/2}
	if dash, ok := boxDashes[r]; ok {
		b.dashes(dash.vertical, dash.heavy, dash.n)
		return true
	}
	if r >= '╱' && r <= '╳' {
		b.diagonal(r)
		return true
	}
	arms, ok := boxArms[r]
	if !ok {
		return false
	}
	for i := range b.w {
		b.w[i] = int(arms[i] - '0')
	}
	if r >= '╭' && r <= '╰' {
		b.rounded(r)
		return true
	}
	for i := range b.w {
		b.arm(i)
	}
	return true
}

func (b *boxCell) thickness(weight int) float32 {
	if weight == 2 {
		return 2.5 * b.t
	}
	return b.t
}

// rect fills the part of arm i from s0 to its cell edge, s measured from
// the center outward, and q across it (down or right positive).
func (b *boxCell) rect(i int, s0, q0, q1 float32) {
	half := float32(b.cell.Dx()) / 2
	if i%2 == 0 {
		half = float32(b.cell.Dy()) / 2
	}
	var x0, y0, x1, y1 float32
	switch i {
	case 0:
		x0, x1, y0, y1 = b.mx+q0, b.mx+q1, b.my-half, b.my-s0
	case 1:
		x0, x1, y0, y1 = b.mx+s0, b.mx+half, b.my+q0, b.my+q1
	case 2:
		x0, x1, y0, y1 = b.mx+q0, b.mx+q1, b.my+s0, b.my+half
	default:
		x0, x1, y0, y1 = b.mx-half, b.mx-s0, b.my+q0, b.my+q1
	}
	rd := func(v float32) int { return int(math.Round(float64(v))) }
	fill(b.img, image.Rect(rd(x0), rd(y0), rd(x1), rd(y1)), b.c)
}

// arm draws arm i, joined to the lines across it.
func (b *boxCell) arm(i int) {
	k := b.w[i]
	if k == 0 {
		return
	}
	perpA, perpB, opp := b.w[(i+1)%4], b.w[(i+3)%4], b.w[(i+2)%4]
	if k != 3 {
		th := b.thickness(k)
		s0 := -th / 2
		switch perp := max(perpA, perpB); {
		case perp == 3 && opp == 0 && min(perpA, perpB) == 0:
			s0 = -b.d - b.t/2 // a corner: reach the far line
		case perp == 3 && opp == 0:
			s0 = b.d - b.t/2 // a tee: stop at the near line
		case perp == 3:
			s0 = -b.d - b.t/2
		case perp > 0:
			s0 = -b.thickness(perp) / 2
		}
		b.rect(i, s0, -th/2, th/2)
		return
	}
	for _, o := range []float32{-1, 1} {
		b.rect(i, b.doubleStart(i, o), o*b.d-b.t/2, o*b.d+b.t/2)
	}
}

// doubleStart is where the line of double arm i on side o starts: at
// the inner line of a double arm on its side, the outer line of one on
// the other side, or the line across it.
func (b *boxCell) doubleStart(i int, o float32) float32 {
	side, other := 2, 0 // for a horizontal arm, o > 0 is down
	if i%2 == 0 {
		side, other = 1, 3
	}
	if o < 0 {
		side, other = other, side
	}
	switch {
	case b.w[side] == 3:
		return b.d - b.t/2
	case b.w[other] == 3:
		return -b.d - b.t/2
	case b.w[side] > 0 || b.w[other] > 0:
		return -b.thickness(max(b.w[side], b.w[other])) / 2
	}
	return -b.t / 2
}

// rounded draws ╭ ╮ ╯ ╰: a quarter circle between two arms.
func (b *boxCell) rounded(r rune) {
	w, h := float32(b.cell.Dx()), float32(b.cell.Dy())
	cx, cy, rad := w/2, h/2, w/2
	var a0, a1, ox, oy float32
	switch r {
	case '╭':
		ox, oy, a0, a1 = rad, rad, 1.5*math.Pi, math.Pi
	case '╮':
		ox, oy, a0, a1 = -rad, rad, -math.Pi/2, 0
	case '╯':
		ox, oy, a0, a1 = -rad, -rad, math.Pi/2, 0
	default:
		ox, oy, a0, a1 = rad, -rad, math.Pi/2, math.Pi
	}
	fillShape(b.img, b.cell, b.c, func(s *shape) {
		pts := arc(cx+ox, cy+oy, rad, a0, a1, 16)
		if oy > 0 {
			pts = append(pts, pt{cx, h + 1})
		} else {
			pts = append(pts, pt{cx, -1})
		}
		s.stroke(pts, b.t)
	})
}

func (b *boxCell) diagonal(r rune) {
	w, h := float32(b.cell.Dx()), float32(b.cell.Dy())
	fillShape(b.img, b.cell, b.c, func(s *shape) {
		if r != '╲' {
			s.seg(pt{0, h}, pt{w, 0}, b.t)
		}
		if r != '╱' {
			s.seg(pt{0, 0}, pt{w, h}, b.t)
		}
	})
}

// dashes draws a dashed line of n dashes a cell.
func (b *boxCell) dashes(vertical, heavy bool, n int) {
	th := b.thickness(1)
	if heavy {
		th = b.thickness(2)
	}
	length, i := float32(b.cell.Dx()), 1
	if vertical {
		length, i = float32(b.cell.Dy()), 2
	}
	step := length / float32(n)
	for k := range n {
		s0 := -length/2 + step*float32(k) + step/4
		b.segment(i, s0, s0+step/2, th)
	}
}

// segment fills a stretch of the line along arm i's axis from s0 to s1,
// measured from the center.
func (b *boxCell) segment(i int, s0, s1, th float32) {
	rd := func(v float32) int { return int(math.Round(float64(v))) }
	if i == 2 {
		fill(b.img, image.Rect(rd(b.mx-th/2), rd(b.my+s0), rd(b.mx+th/2), rd(b.my+s1)), b.c)
		return
	}
	fill(b.img, image.Rect(rd(b.mx+s0), rd(b.my-th/2), rd(b.mx+s1), rd(b.my+th/2)), b.c)
}

// quadrants are U+2596 to U+259F as the quarters they fill: 1 upper
// left, 2 upper right, 4 lower left, 8 lower right.
var quadrants = [10]int{4, 8, 1, 1 | 4 | 8, 1 | 8, 1 | 2 | 4, 1 | 2 | 8, 2, 2 | 4, 2 | 4 | 8}

// blockElement draws U+2580 to U+259F.
func blockElement(img *image.NRGBA, r rune, cell image.Rectangle, c color.RGBA) bool {
	x0, y0, w, h := cell.Min.X, cell.Min.Y, cell.Dx(), cell.Dy()
	ex := func(n int) int { return x0 + (w*n+4)/8 } // n eighths across
	ey := func(n int) int { return y0 + (h*n+4)/8 } // n eighths down
	box := func(xa, ya, xb, yb int) { fill(img, image.Rect(xa, ya, xb, yb), c) }
	switch {
	case r == '▀':
		box(x0, y0, cell.Max.X, ey(4))
	case r >= '▁' && r <= '█':
		box(x0, ey(8-int(r-'▀')), cell.Max.X, cell.Max.Y)
	case r >= '▉' && r <= '▏':
		box(x0, y0, ex(8-int(r-'█')), cell.Max.Y)
	case r == '▐':
		box(ex(4), y0, cell.Max.X, cell.Max.Y)
	case r >= '░' && r <= '▓':
		fill(img, cell, color.NRGBA{c.R, c.G, c.B, uint8(64 * (r - '░' + 1))})
	case r == '▔':
		box(x0, y0, cell.Max.X, ey(1))
	case r == '▕':
		box(ex(7), y0, cell.Max.X, cell.Max.Y)
	default:
		q := quadrants[r-'▖']
		for bit, qr := range [4][4]int{{x0, y0, ex(4), ey(4)}, {ex(4), y0, cell.Max.X, ey(4)}, {x0, ey(4), ex(4), cell.Max.Y}, {ex(4), ey(4), cell.Max.X, cell.Max.Y}} {
			if q&(1<<bit) != 0 {
				box(qr[0], qr[1], qr[2], qr[3])
			}
		}
	}
	return true
}
