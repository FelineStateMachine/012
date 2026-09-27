// Package chart draws a sheet's charts: as text for any terminal (block
// elements for bars, braille for lines, half blocks for pies), and the plot
// area as an image for terminals with kitty graphics. Both share one
// layout, so axis labels, category labels and the legend stay terminal
// text around the image, in the terminal's own font.
package chart

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Role is what a cell of a drawn chart shows, so the UI can style it with
// its theme.
type Role uint8

const (
	None     Role = iota // background
	Axis                 // axis lines and tick marks
	Label                // tick values, category labels and legend text
	Muted                // messages such as "No numbers to chart"
	Gridline             // gridlines
	Series               // Series+i is the color of series (or slice) i
)

// Colors is how many series colors there are; later series reuse them.
const Colors = 6

// SeriesRole is the role of series i.
func SeriesRole(i int) Role { return Series + Role(i%Colors) }

// Cell is one terminal cell of a drawn chart.
type Cell struct {
	Text string // one grapheme; "" continues a wide one to the left
	Fg   Role
	Bg   Role // only the pie's half blocks set a background
}

// Grid is a drawn chart, W by H cells.
type Grid struct {
	W, H  int
	Cells []Cell
	// Plot is the area an image covers, in cells. It is empty when the
	// chart has nothing to plot.
	Plot image.Rectangle
}

// Options control drawing.
type Options struct {
	// Image leaves the plot area blank, for an image drawn by Image.
	Image bool
	// CellW and CellH are the size of a terminal cell in pixels; they set
	// a pie's proportions. Zero means the usual 1:2.
	CellW, CellH int
	// Chart is the chart's own settings: stacking, the value axis, the
	// gridlines and the legend.
	Chart sheet.ChartOptions
}

func (o Options) aspect() float64 {
	if o.CellW <= 0 || o.CellH <= 0 {
		return 2
	}
	return float64(o.CellH) / float64(o.CellW)
}

func newGrid(w, h int) *Grid {
	g := &Grid{W: w, H: h, Cells: make([]Cell, w*h)}
	for i := range g.Cells {
		g.Cells[i].Text = " "
	}
	return g
}

// At returns the cell at x, y, or nil outside the grid.
func (g *Grid) At(x, y int) *Cell {
	if x < 0 || y < 0 || x >= g.W || y >= g.H {
		return nil
	}
	return &g.Cells[y*g.W+x]
}

func (g *Grid) set(x, y int, s string, fg Role) {
	if c := g.At(x, y); c != nil {
		c.Text, c.Fg, c.Bg = s, fg, None
	}
}

// text writes s from x, clipped to the grid.
func (g *Grid) text(x, y int, s string, fg Role) {
	for _, r := range s {
		w := ansi.StringWidth(string(r))
		if w == 0 {
			continue
		}
		if x+w > g.W {
			return
		}
		g.set(x, y, string(r), fg)
		for k := 1; k < w; k++ {
			g.set(x+k, y, "", fg)
		}
		x += w
	}
}

// String returns the grid as plain text, for tests.
func (g *Grid) String() string {
	var b strings.Builder
	for y := range g.H {
		var line strings.Builder
		for x := range g.W {
			line.WriteString(g.At(x, y).Text)
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		if y < g.H-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Draw draws d as a chart of type t in w by h cells.
func Draw(t sheet.ChartType, d sheet.ChartData, w, h int, o Options) *Grid {
	g := newGrid(max(w, 0), max(h, 0))
	if w < 8 || h < 3 {
		return g
	}
	p, lg := planFor(t, d, w, h, o)
	if msg := p.note(); msg != "" {
		g.message(msg)
		return g
	}
	g.Plot = p.plotArea()
	p.draw(g, o)
	lg.draw(g)
	return g
}

// message centers a note in the grid, e.g. when there's nothing to plot.
func (g *Grid) message(s string) {
	s = ansi.Truncate(s, g.W-2, "…")
	g.text((g.W-ansi.StringWidth(s))/2, (g.H-1)/2, s, Muted)
}

// finite returns the range of the finite values in d's series, and false
// if there are none.
func finite(series []sheet.ChartSeries) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, s := range series {
		for _, v := range s.Values {
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				lo, hi, ok = min(lo, v), max(hi, v), true
			}
		}
	}
	return lo, hi, ok
}

// scale is a value axis: lo..hi divided into n intervals of step, each k
// cells long, so every tick lands on a cell boundary. On a log scale lo,
// hi and step are powers of ten (their logarithms), see axis.go.
type scale struct {
	lo, hi, step float64
	n, k         int
	log          bool
	label        func(float64) string
}

// cells is the axis length in cells.
func (s scale) cells() int { return s.n * s.k }

// pos maps a value to a distance along the axis in cells.
func (s scale) pos(v float64) float64 {
	if s.log {
		if v <= 0 {
			return math.Inf(-1)
		}
		v = math.Log10(v)
	}
	return (v - s.lo) / (s.hi - s.lo) * float64(s.cells())
}

// tick returns the value of tick j.
func (s scale) tick(j int) float64 {
	if s.log {
		return math.Pow(10, s.lo+float64(j)*s.step)
	}
	return s.lo + float64(j)*s.step
}

// newScale fits dlo..dhi onto length cells, with ticks at least minK
// cells apart and at most maxN intervals.
func newScale(dlo, dhi float64, length, minK, maxN int, f sheet.Format) scale {
	if dlo == dhi {
		if dlo == 0 {
			dhi = 1
		} else if dlo > 0 {
			dlo = 0
		} else {
			dhi = 0
		}
	}
	best := scale{lo: dlo, hi: dhi, step: dhi - dlo, n: 1, k: max(length, 1)}
	for target := min(maxN, max(length/minK, 1)); target >= 1; target-- {
		step := niceStep((dhi - dlo) / float64(target))
		lo := math.Floor(dlo/step+1e-9) * step
		hi := math.Ceil(dhi/step-1e-9) * step
		n := int(math.Round((hi - lo) / step))
		if n < 1 {
			continue
		}
		if k := length / n; k >= minK || target == 1 && k >= 1 {
			best = scale{lo: lo, hi: hi, step: step, n: n, k: k}
			break
		}
	}
	best.label = tickFormat(best, f)
	return best
}

// niceStep rounds x up to 1, 2 or 5 times a power of ten.
func niceStep(x float64) float64 {
	if x <= 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(x)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if x <= m*p*(1+1e-9) {
			return m * p
		}
	}
	return 10 * p
}

// tickFormat labels ticks compactly in the data's format: "$1,500",
// "25%", "12K".
func tickFormat(s scale, f sheet.Format) func(float64) string {
	mult, prefix, suffix := 1.0, "", ""
	switch f.Kind {
	case sheet.FmtPercent:
		mult, suffix = 100, "%"
	case sheet.FmtCurrency, sheet.FmtAccounting:
		prefix = "$"
	}
	step, top := s.step*mult, math.Max(math.Abs(s.lo), math.Abs(s.hi))*mult
	div, unit := 1.0, ""
	switch {
	case top >= 1e9 && step >= 1e8:
		div, unit = 1e9, "B"
	case top >= 1e6 && step >= 1e5:
		div, unit = 1e6, "M"
	case top >= 1e4 && step >= 1e3:
		div, unit = 1e3, "K"
	}
	decimals := 0
	if st := step / div; st < 1 {
		decimals = int(math.Ceil(-math.Log10(st) - 1e-9))
		if r := st * math.Pow(10, float64(decimals)); math.Abs(r-math.Round(r)) > 1e-6 {
			decimals++ // 0.25
		}
	}
	return func(v float64) string {
		v = v * mult / div
		sign := ""
		if v < 0 && math.Abs(v) > 1e-12 {
			sign, v = "-", -v
		}
		return sign + prefix + groupDigits(strconv.FormatFloat(v, 'f', decimals, 64)) + unit + suffix
	}
}

// groupDigits adds thousands separators to a formatted number.
func groupDigits(s string) string {
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteString("." + frac)
	}
	return b.String()
}

// eighths are the block elements filling 0/8 to 8/8 of a cell from the
// bottom, and from the left.
var (
	lowerEighths = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	leftEighths  = []string{" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}
)
