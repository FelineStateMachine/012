package chart

import (
	"image"
	"math"
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// piePlan lays out a pie of the first series, clockwise from twelve
// o'clock as in Sheets, with a legend of labels and shares to its right.
// As text, the disc is half blocks: two square pixels per cell, each
// colored by its slice.
type piePlan struct {
	d      sheet.ChartData
	msg    string
	slices []slice
	disc   image.Rectangle // in cells
	legend image.Point     // top-left of the legend
	nameW  int
}

type slice struct {
	name       string
	value      float64
	from, to   float64 // fractions of the whole, clockwise
	colorIndex int
}

func newPiePlan(d sheet.ChartData, w, h int, o Options) *piePlan {
	p := &piePlan{d: d}
	if len(d.Series) == 0 {
		p.msg = "No numbers to chart"
		return p
	}
	total := 0.0
	for i, v := range d.Series[0].Values {
		if v > 0 && !math.IsInf(v, 0) && i < len(d.Categories) {
			p.slices = append(p.slices, slice{name: d.Categories[i], value: v, colorIndex: len(p.slices)})
			total += v
		}
	}
	if len(p.slices) == 0 {
		p.msg = "A pie needs positive numbers"
		return p
	}
	acc := 0.0
	for i := range p.slices {
		p.slices[i].from = acc / total
		acc += p.slices[i].value
		p.slices[i].to = acc / total
	}
	for _, s := range p.slices {
		p.nameW = max(p.nameW, ansi.StringWidth(s.name))
	}
	// "■ " name "  " share: shares are at most "100%".
	p.nameW = max(min(p.nameW, w/2-8), 3)
	legendW := 2 + p.nameW + 2 + 4
	a := o.aspect()
	rows := min(h, int(float64(w-legendW-3)/a))
	if rows < 2 {
		p.msg = "Too small to chart"
		return p
	}
	cols := int(math.Round(float64(rows) * a))
	x0 := max((w-(cols+3+legendW))/2, 0)
	y0 := (h - rows) / 2
	p.disc = image.Rect(x0, y0, x0+cols, y0+rows)
	n := min(len(p.slices), h)
	p.legend = image.Pt(p.disc.Max.X+3, max((h-n)/2, 0))
	return p
}

// sliceAt returns the slice under a point of the disc, given as offsets
// from its center scaled so the rim is at distance 1, or -1 outside.
func (p *piePlan) sliceAt(dx, dy float64) int {
	if dx*dx+dy*dy > 1 {
		return -1
	}
	f := math.Atan2(dx, -dy) / (2 * math.Pi)
	if f < 0 {
		f++
	}
	for i, s := range p.slices {
		if f < s.to {
			return i
		}
	}
	return len(p.slices) - 1
}

func (p *piePlan) draw(g *Grid, o Options) {
	p.drawLegend(g)
	if o.Image {
		return
	}
	cols, rows := float64(p.disc.Dx()), float64(p.disc.Dy())
	at := func(x, y float64) int { // x in columns, y in rows, from the box
		return p.sliceAt((x-cols/2)/(cols/2), (y-rows/2)/(rows/2))
	}
	for y := range p.disc.Dy() {
		for x := range p.disc.Dx() {
			top := at(float64(x)+0.5, float64(y)+0.25)
			bottom := at(float64(x)+0.5, float64(y)+0.75)
			c := g.At(p.disc.Min.X+x, p.disc.Min.Y+y)
			if c == nil {
				continue
			}
			switch {
			case top < 0 && bottom < 0:
				continue
			case top == bottom:
				c.Text, c.Fg = "█", SeriesRole(top)
			case bottom < 0:
				c.Text, c.Fg = "▀", SeriesRole(top)
			case top < 0:
				c.Text, c.Fg = "▄", SeriesRole(bottom)
			default:
				c.Text, c.Fg, c.Bg = "▀", SeriesRole(top), SeriesRole(bottom)
			}
		}
	}
}

func (p *piePlan) drawLegend(g *Grid) {
	x, y := p.legend.X, p.legend.Y
	for i, s := range p.slices {
		if y >= g.H {
			return
		}
		if y == g.H-1 && i < len(p.slices)-1 {
			g.text(x, y, "… "+strconv.Itoa(len(p.slices)-i)+" more", Label)
			return
		}
		g.set(x, y, "■", SeriesRole(i))
		g.text(x+2, y, ansi.Truncate(s.name, p.nameW, "…"), Label)
		share := strconv.Itoa(int(math.Round((s.to-s.from)*100))) + "%"
		g.text(x+2+p.nameW+2+4-ansi.StringWidth(share), y, share, Label)
		y++
	}
}
