package chart

import (
	"math"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// newAxis fits dlo..dhi onto a value axis of length cells as newScale
// does, with the chart's axis options: a fixed minimum or maximum, and a
// log scale.
func newAxis(dlo, dhi float64, length, minK, maxN int, f sheet.Format, o sheet.ChartOptions) scale {
	if o.HasMin {
		dlo = o.Min
	}
	if o.HasMax {
		dhi = o.Max
	}
	if o.HasMin && !o.HasMax && dhi <= dlo {
		dhi = dlo + max(math.Abs(dlo), 1)
	}
	if o.HasMax && !o.HasMin && dlo >= dhi {
		dlo = dhi - max(math.Abs(dhi), 1)
	}
	if dlo > dhi {
		dlo, dhi = dhi, dlo
	}
	switch {
	case o.Log:
		return newLogScale(dlo, dhi, length, minK, f)
	case (o.HasMin || o.HasMax) && dlo < dhi:
		return newFixedScale(dlo, dhi, o.HasMin, o.HasMax, length, minK, maxN, f)
	}
	return newScale(dlo, dhi, length, minK, maxN, f)
}

// newFixedScale is newScale keeping the ends that are fixed where they
// are: the other end rounds out to a whole step, and with both fixed the
// step divides the range, a round one where it can.
func newFixedScale(dlo, dhi float64, fixLo, fixHi bool, length, minK, maxN int, f sheet.Format) scale {
	best := scale{lo: dlo, hi: dhi, step: dhi - dlo, n: 1, k: max(length, 1)}
	fallback := false
	for target := min(maxN, max(length/minK, 1)); target >= 1; target-- {
		s, ok := fixedTicks(dlo, dhi, fixLo, fixHi, target)
		if !ok {
			continue
		}
		s.k = length / s.n
		if s.k < minK && (target > 1 || s.k < 1) {
			continue
		}
		if fixLo && fixHi && niceStep(s.step) != s.step {
			if !fallback { // the finest division, if no round step fits
				best, fallback = s, true
			}
			continue
		}
		best = s
		break
	}
	best.label = tickFormat(best, f)
	return best
}

// fixedTicks divides dlo..dhi into about target steps, moving only the
// ends that aren't fixed.
func fixedTicks(dlo, dhi float64, fixLo, fixHi bool, target int) (scale, bool) {
	if fixLo && fixHi {
		return scale{lo: dlo, hi: dhi, step: (dhi - dlo) / float64(target), n: target}, true
	}
	step := niceStep((dhi - dlo) / float64(target))
	n := int(math.Ceil((dhi-dlo)/step - 1e-9))
	if n < 1 {
		return scale{}, false
	}
	if fixLo {
		return scale{lo: dlo, hi: dlo + float64(n)*step, step: step, n: n}, true
	}
	return scale{lo: dhi - float64(n)*step, hi: dhi, step: step, n: n}, true
}

// newLogScale is a log axis over positive dlo..dhi: its ends round out to
// powers of ten, and ticks are one decade apart, or more when decades
// would be closer than minK cells.
func newLogScale(dlo, dhi float64, length, minK int, f sheet.Format) scale {
	if dhi <= 0 {
		dlo, dhi = 1, 10
	}
	if dlo <= 0 {
		dlo = dhi / 10
	}
	lo, hi := math.Floor(math.Log10(dlo)+1e-9), math.Ceil(math.Log10(dhi)-1e-9)
	if hi <= lo {
		hi = lo + 1
	}
	decades := int(hi - lo)
	s := scale{lo: lo, log: true}
	for _, step := range []int{1, 2, 3, 5, 10, 20, 50, 100} {
		n := (decades + step - 1) / step
		s.step, s.n, s.k = float64(step), n, length/n
		if s.k >= minK || n == 1 {
			break
		}
	}
	s.k = max(s.k, 1)
	s.hi = lo + float64(s.n)*s.step
	s.label = func(v float64) string {
		return tickFormat(scale{lo: v, hi: v, step: v}, f)(v)
	}
	return s
}

// base is where bars start: zero, or the end of the axis nearest it.
func (s scale) base() float64 {
	if s.log {
		return s.tick(0)
	}
	return min(max(0, s.lo), s.hi)
}

// at is pos kept on the axis, for values beyond a fixed end.
func (s scale) at(v float64) float64 {
	p := s.pos(v)
	if math.IsNaN(p) {
		return 0
	}
	return min(max(p, 0), float64(s.cells()))
}

// crossesZero reports whether zero lies inside the axis, where images
// draw a line.
func (s scale) crossesZero() bool { return !s.log && s.lo < 0 && s.hi > 0 }

// positive is d with every value that isn't positive left out, as a log
// scale can't show them.
func positive(d sheet.ChartData) sheet.ChartData {
	out := d
	out.Series = make([]sheet.ChartSeries, len(d.Series))
	for j, s := range d.Series {
		vals := make([]float64, len(s.Values))
		for i, v := range s.Values {
			if v <= 0 {
				v = math.NaN()
			}
			vals[i] = v
		}
		out.Series[j] = sheet.ChartSeries{Name: s.Name, Values: vals}
	}
	return out
}
