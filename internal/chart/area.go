package chart

import (
	"math"
	"slices"
)

// Area charts are line charts filled down to the axis. Each column of
// the plot (each pixel column of the image) samples the series between
// the two categories around it, so drawing costs the plot's width times
// the series, whatever the data.

// pointX is category i's point in cells from the plot's left edge: the
// center of its braille dot, so fills meet the lines.
func (p *columnPlan) pointX(i int) float64 { return (float64(p.dotX(i)) + 0.5) / 2 }

// areaCursor finds the categories around points along the plot, left to
// right.
type areaCursor struct {
	p *columnPlan
	k int
}

// at returns category i and how far x (cells into the plot) lies from its
// point toward the next one's, or false outside the first and last points.
func (c *areaCursor) at(x float64) (int, float64, bool) {
	p := c.p
	if p.shown == 1 {
		return 0, 0, math.Abs(x-p.pointX(0)) <= 0.5
	}
	if x < p.pointX(0) || x > p.pointX(p.shown-1) {
		return 0, 0, false
	}
	for c.k < p.shown-2 && p.pointX(c.k+1) < x {
		c.k++
	}
	x0, x1 := p.pointX(c.k), p.pointX(c.k+1)
	if x1 <= x0 {
		return c.k, 0, true
	}
	return c.k, (x - x0) / (x1 - x0), true
}

// valueAt is series j between categories i and i+1, t of the way, or NaN
// where either is missing.
func (p *columnPlan) valueAt(j, i int, t float64) float64 {
	s := p.d.Series[j]
	a := value(s, i)
	if t == 0 || i+1 >= p.shown {
		return a
	}
	b := value(s, i+1)
	return a + (b-a)*t
}

// areaSpans returns the area's spans at one point: stacked, the series'
// piles; otherwise each series from the axis to its value, the smaller
// in front, so each shows the part no smaller one covers.
func (p *columnPlan) areaSpans(i int, t float64, vals []float64, out []span) []span {
	vals = vals[:0]
	for j := range p.d.Series {
		vals = append(vals, p.valueAt(j, i, t))
	}
	if p.stacked {
		return pileUp(len(vals), func(j int) float64 { return vals[j] }, out)
	}
	return overlap(vals, p.sc.base(), out)
}

// overlap turns values drawn from base, each in front of the larger ones,
// into the visible spans: from base up through the positive values in
// order, and down through the negative ones.
func overlap(vals []float64, base float64, out []span) []span {
	out = out[:0]
	order := make([]int, 0, len(vals))
	for j, v := range vals {
		if !math.IsNaN(v) {
			order = append(order, j)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmpDistance(vals[a]-base, vals[b]-base) })
	up, down := base, base
	for _, j := range order {
		switch v := vals[j]; {
		case v > up:
			out = append(out, span{up, v, j})
			up = v
		case v < down:
			out = append(out, span{v, down, j})
			down = v
		}
	}
	return out
}

// cmpDistance orders offsets by their distance from zero.
func cmpDistance(a, b float64) int {
	switch a, b = math.Abs(a), math.Abs(b); {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// drawArea fills each column of the plot under the series with eighths.
func (p *columnPlan) drawArea(g *Grid) {
	cur := areaCursor{p: p}
	vals := make([]float64, 0, len(p.d.Series))
	var spans, cells []span
	for x := range p.plot.Dx() {
		i, t, ok := cur.at(float64(x) + 0.5)
		if !ok {
			continue
		}
		spans = p.areaSpans(i, t, vals, spans)
		cells = inCells(p.sc, spans, cells)
		from, to := pileExtent(cells)
		for r := from; r <= to; r++ {
			if c, ok := pileCell(cells, r, lowerEighths, "▀"); ok {
				if at := g.At(p.plot.Min.X+x, p.axisRow-1-r); at != nil {
					*at = c
				}
			}
		}
	}
}

// areaImage fills the area a pixel column at a time: stacked piles
// opaque, otherwise each series translucent over the ones before, then
// strokes the lines on top.
func (p *columnPlan) areaImage(cv *canvas, y func(float64) float64) {
	cur := areaCursor{p: p}
	vals := make([]float64, 0, len(p.d.Series))
	var spans []span
	for px := range cv.Rect.Dx() {
		i, t, ok := cur.at((float64(px) + 0.5) / cv.cw)
		if !ok {
			continue
		}
		if p.stacked {
			spans = p.areaSpans(i, t, vals, spans)
			for _, s := range spans {
				cv.rect(float64(px), y(s.hi), float64(px+1), y(s.lo), cv.pal.Series[s.j%Colors])
			}
			continue
		}
		base := y(p.sc.base())
		for j := range p.d.Series {
			if v := p.valueAt(j, i, t); !math.IsNaN(v) {
				c := cv.pal.Series[j%Colors]
				c.A = 150
				cv.rect(float64(px), min(y(v), base), float64(px+1), max(y(v), base), c)
			}
		}
	}
	if !p.stacked {
		p.lineImage(cv, y)
	}
}
