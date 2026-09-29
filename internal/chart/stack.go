package chart

import (
	"math"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A span is a piece of a bar, or of an area at one point, from lo to hi
// along the value axis, in values, colored as series j.
type span struct {
	lo, hi float64
	j      int
}

// prepare readies d for a chart's options: values a log scale can't show
// left out, and with 100% stacking each value as its share of its
// category.
func prepare(d sheet.ChartData, o sheet.ChartOptions, stackable bool) sheet.ChartData {
	if o.Log {
		d = positive(d)
	}
	if stackable && o.Stack == sheet.StackPercent {
		d = shares(d)
	}
	return d
}

// shares is d with each value divided by the sum of its category's sizes,
// formatted as percentages.
func shares(d sheet.ChartData) sheet.ChartData {
	out := sheet.ChartData{Categories: d.Categories, Format: sheet.Format{Kind: sheet.FmtPercent}}
	totals := make([]float64, len(d.Categories))
	for _, s := range d.Series {
		for i, v := range s.Values {
			if i < len(totals) && !math.IsNaN(v) && !math.IsInf(v, 0) {
				totals[i] += math.Abs(v)
			}
		}
	}
	for _, s := range d.Series {
		vals := make([]float64, len(s.Values))
		for i, v := range s.Values {
			vals[i] = math.NaN()
			if i < len(totals) && totals[i] > 0 && !math.IsInf(v, 0) {
				vals[i] = v / totals[i]
			}
		}
		out.Series = append(out.Series, sheet.ChartSeries{Name: s.Name, Values: vals})
	}
	return out
}

// value returns series s's value i, NaN when missing or infinite.
func value(s sheet.ChartSeries, i int) float64 {
	if i >= len(s.Values) || math.IsInf(s.Values[i], 0) {
		return math.NaN()
	}
	return s.Values[i]
}

// stackedRange is the range of the stacks' ends over the first n
// categories: positive values pile up from zero and negative ones down.
func stackedRange(d sheet.ChartData, n int) (lo, hi float64, ok bool) {
	for i := range n {
		up, down := 0.0, 0.0
		for _, s := range d.Series {
			if v := value(s, i); v > 0 {
				up += v
				ok = true
			} else if v <= 0 {
				down += v
				ok = true
			}
		}
		lo, hi = min(lo, down), max(hi, up)
	}
	return lo, hi, ok
}

// stack returns category i's spans, one per series with a value, piled
// up from zero (positive values) and down from it (negative ones).
func stack(d sheet.ChartData, i int, out []span) []span {
	return pileUp(len(d.Series), func(j int) float64 { return value(d.Series[j], i) }, out)
}

// pileUp stacks n values, v(j) for series j, into out.
func pileUp(n int, v func(j int) float64, out []span) []span {
	out = out[:0]
	up, down := 0.0, 0.0
	for j := range n {
		switch v := v(j); {
		case v > 0:
			out = append(out, span{up, up + v, j})
			up += v
		case v < 0:
			out = append(out, span{down + v, down, j})
			down += v
		}
	}
	return out
}

// pileCell is cell c, counted in cells from the start of the axis, of a
// pile of spans given in cells along it, drawn with eighths growing from
// the start of the cell (lowerEighths or leftEighths) and half, the
// glyph for its far half. The span covering the near edge of the cell
// is the foreground, reaching as far as it goes; the one covering the far
// edge is the background. A cell only its far part of which is covered
// is a full block or the half block, as eighths can't grow the other
// way. ok is false for an empty cell.
func pileCell(spans []span, c int, eighths []string, half string) (Cell, bool) {
	const edge = 1.0 / 16
	near, far := -1, -1
	for k, s := range spans {
		if s.lo <= float64(c)+edge && s.hi > float64(c)+edge {
			near = k
		}
		if s.lo < float64(c+1)-edge && s.hi >= float64(c+1)-edge {
			far = k
		}
	}
	switch {
	case near >= 0 && near == far:
		return Cell{Text: "█", Fg: SeriesRole(spans[near].j)}, true
	case near >= 0:
		n := int(math.Round((spans[near].hi - float64(c)) * 8))
		if n >= 8 || far < 0 && n >= 1 {
			bg := None
			if far >= 0 {
				bg = SeriesRole(spans[far].j)
			}
			return Cell{Text: eighths[min(n, 8)], Fg: SeriesRole(spans[near].j), Bg: bg}, true
		}
		if n >= 1 {
			return Cell{Text: eighths[n], Fg: SeriesRole(spans[near].j), Bg: SeriesRole(spans[far].j)}, true
		}
	}
	if far < 0 {
		return Cell{}, false
	}
	switch cover := float64(c+1) - spans[far].lo; {
	case cover >= 0.75:
		return Cell{Text: "█", Fg: SeriesRole(spans[far].j)}, true
	case cover >= 0.25:
		return Cell{Text: half, Fg: SeriesRole(spans[far].j)}, true
	}
	return Cell{}, false
}

// drawPiles draws the bars of the first shown categories cell by cell:
// piles gives category i's spans in values, one pile of them all when
// stacked, and put draws cell c along the axis of series j's bar (the
// stack's, when stacked) in category i.
func drawPiles(sc scale, shown int, stacked bool, piles func(int, []span) []span, eighths []string, half string, put func(i, j, c int, cell Cell)) {
	var vals, cells []span
	for i := 0; i < shown; i++ {
		vals = piles(i, vals)
		for k := range vals {
			pile := vals[k : k+1]
			if stacked {
				pile = vals
			}
			cells = inCells(sc, pile, cells)
			from, to := pileExtent(cells)
			for c := from; c <= to; c++ {
				if cell, ok := pileCell(cells, c, eighths, half); ok {
					put(i, vals[k].j, c, cell)
				}
			}
			if stacked {
				break
			}
		}
	}
}

// inCells maps spans in values to spans in cells along sc, into out.
// Text ticks and the axis are lines through the middle of their cells,
// so values are placed half a cell back from where sc puts them: a
// bar starts at the axis line, in the middle of cell -1 (the axis row
// or column), and a value on a tick ends on the tick's line.
func inCells(sc scale, spans []span, out []span) []span {
	out = out[:0]
	for _, s := range spans {
		out = append(out, span{sc.at(s.lo) - 0.5, sc.at(s.hi) - 0.5, s.j})
	}
	return out
}

// pileExtent is the first and last cell a pile in cells touches.
func pileExtent(spans []span) (from, to int) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, s := range spans {
		lo, hi = min(lo, s.lo), max(hi, s.hi)
	}
	if lo > hi {
		return 0, -1
	}
	return int(math.Floor(lo)), int(math.Ceil(hi)) - 1
}
