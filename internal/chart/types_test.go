package chart

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// imageCases are the images TestImageHashes pins: every type, and the
// options that change how the plot is drawn.
var imageCases = []struct {
	name string
	t    sheet.ChartType
	d    sheet.ChartData
	o    sheet.ChartOptions
}{
	{"column", sheet.ChartColumn, budget, sheet.ChartOptions{}},
	{"column-negative", sheet.ChartColumn, mixed, sheet.ChartOptions{}},
	{"bar", sheet.ChartBar, budget, sheet.ChartOptions{}},
	{"line", sheet.ChartLine, budget, sheet.ChartOptions{}},
	{"pie", sheet.ChartPie, single, sheet.ChartOptions{}},
	{"area", sheet.ChartArea, budget, sheet.ChartOptions{}},
	{"area-stacked", sheet.ChartArea, budget, sheet.ChartOptions{Stack: sheet.StackNormal}},
	{"column-stacked", sheet.ChartColumn, signs, sheet.ChartOptions{Stack: sheet.StackNormal}},
	{"column-percent", sheet.ChartColumn, budget, sheet.ChartOptions{Stack: sheet.StackPercent}},
	{"bar-stacked", sheet.ChartBar, signs, sheet.ChartOptions{Stack: sheet.StackNormal}},
	{"bar-percent", sheet.ChartBar, budget, sheet.ChartOptions{Stack: sheet.StackPercent}},
	{"scatter", sheet.ChartScatter, xy, sheet.ChartOptions{}},
	{"scatter-trend", sheet.ChartScatter, xy, sheet.ChartOptions{Trend: true}},
	{"no-gridlines", sheet.ChartColumn, budget, sheet.ChartOptions{NoGrid: true}},
	{"axis-min-max", sheet.ChartLine, budget, sheet.ChartOptions{Min: 500, HasMin: true, Max: 1600, HasMax: true}},
	{"axis-log", sheet.ChartColumn, growth, sheet.ChartOptions{Log: true}},
	{"legend-right", sheet.ChartColumn, budget, sheet.ChartOptions{Legend: sheet.LegendRight}},
}

// TestImageHashes pins the image of each case by a hash of its pixels,
// kept in testdata/images.txt (go test ./internal/chart -update rewrites
// it). A changed hash means the image changed: look at it before
// updating.
func TestImageHashes(t *testing.T) {
	var got strings.Builder
	for _, tc := range imageCases {
		img := Image(tc.t, tc.d, 40, 12, Options{CellW: 8, CellH: 16, Chart: tc.o}, testPalette)
		if img == nil {
			t.Errorf("%s: no image", tc.name)
			continue
		}
		sum := sha256.Sum256(img.Pix)
		fmt.Fprintf(&got, "%s %dx%d %x\n", tc.name, img.Bounds().Dx(), img.Bounds().Dy(), sum[:8])
	}
	path := filepath.Join("testdata", "images.txt")
	if *update {
		os.WriteFile(path, []byte(got.String()), 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing hashes (go test ./internal/chart -update): %v", err)
	}
	if got.String() != string(want) {
		t.Errorf("images changed:\n%s\nwant:\n%s", got.String(), want)
	}
}

// TestNewTypesPixels checks a few pixels of the stacked, scatter and
// area images directly.
func TestNewTypesPixels(t *testing.T) {
	o := Options{CellW: 8, CellH: 16}
	// A stacked column: Rent at the bottom, Food above it.
	o.Chart = sheet.ChartOptions{Stack: sheet.StackNormal}
	p := newColumnPlan(budget, 44, 11, columnBars, o.Chart)
	img := Image(sheet.ChartColumn, budget, 44, 12, o, testPalette)
	x0, x1 := p.bar(0, 0)
	mid := ((x0+x1)/2 - p.plot.Min.X) * 8
	bottom := img.Bounds().Dy()
	rentTop := int(float64(bottom) - p.sc.pos(1450)*16)
	if c := img.RGBAAt(mid, bottom-2); c != testPalette.Series[0] {
		t.Errorf("stack bottom %v, want Rent's color", c)
	}
	if c := img.RGBAAt(mid, rentTop-4); c != testPalette.Series[1] {
		t.Errorf("stack top %v, want Food's color", c)
	}
	if x0b, _ := p.bar(0, 1); x0b != x0 {
		t.Errorf("stacked series at %d and %d, want one column", x0, x0b)
	}
	// A scatter point is its series' color at its center.
	o.Chart = sheet.ChartOptions{}
	sp := newScatterPlan(xy, 40, 11, o.Chart)
	simg := Image(sheet.ChartScatter, xy, 40, 12, o, testPalette)
	cx := int(sp.xsc.at(1) * 8)
	cy := int(float64(simg.Bounds().Dy()) - sp.ysc.at(52)*16)
	if c := simg.RGBAAt(cx, cy); c != testPalette.Series[0] {
		t.Errorf("scatter point %v at %d,%d", c, cx, cy)
	}
	// The area under a line is filled, above it is clear.
	aimg := Image(sheet.ChartArea, single, 40, 10, o, testPalette)
	b := aimg.Bounds()
	if c := aimg.RGBAAt(2, b.Dy()-3); c.A == 0 {
		t.Errorf("under the area %v, want filled", c)
	}
	if c := aimg.RGBAAt(b.Dx()-2, 2); c.A != 0 {
		t.Errorf("above the area %v, want clear", c)
	}
}

func TestLogAndFixedScales(t *testing.T) {
	s := newAxis(3, 9000, 10, 2, 6, sheet.Format{}, sheet.ChartOptions{Log: true})
	if s.tick(0) != 1 || s.tick(s.n) != 10000 || s.n != 4 {
		t.Errorf("log scale %v..%v in %d", s.tick(0), s.tick(s.n), s.n)
	}
	if got := s.label(1000); got != "1,000" {
		t.Errorf("log label %q", got)
	}
	if p := s.pos(100); p != float64(2*s.k) {
		t.Errorf("100 at %v cells", p)
	}
	f := newAxis(0, 1500, 9, 2, 6, sheet.Format{}, sheet.ChartOptions{Min: 200, HasMin: true})
	if f.lo != 200 || f.hi < 1500 {
		t.Errorf("fixed minimum: %v..%v", f.lo, f.hi)
	}
	f = newAxis(0, 1500, 8, 2, 6, sheet.Format{}, sheet.ChartOptions{Min: 0, HasMin: true, Max: 2000, HasMax: true})
	if f.lo != 0 || f.hi != 2000 || niceStep(f.step) != f.step {
		t.Errorf("fixed ends: %v..%v step %v", f.lo, f.hi, f.step)
	}
	// A value past a fixed end stops at the end of the axis.
	if at := f.at(5000); at != float64(f.cells()) {
		t.Errorf("clipped at %v", at)
	}
}

// TestLegendPlacement checks the legend takes its room from the chart.
func TestLegendPlacement(t *testing.T) {
	names := []string{"Rent", "Food"}
	if lg, w, h := placeLegend(names, sheet.LegendRight, 44, 12); lg.pos != sheet.LegendRight || w != 44-6-1 || h != 12 || lg.x != 38 {
		t.Errorf("right: %+v in %dx%d", lg, w, h)
	}
	if lg, w, h := placeLegend(names, sheet.LegendBottom, 44, 12); lg.row != 11 || w != 44 || h != 11 {
		t.Errorf("bottom: %+v in %dx%d", lg, w, h)
	}
	if lg, w, h := placeLegend(names, sheet.LegendNone, 44, 12); lg.pos != sheet.LegendNone || w != 44 || h != 12 {
		t.Errorf("none: %+v in %dx%d", lg, w, h)
	}
	// One series, or too little room: no legend.
	if lg, _, _ := placeLegend(names[:1], sheet.LegendBottom, 44, 12); lg.pos != sheet.LegendNone {
		t.Errorf("one series: %+v", lg)
	}
	if lg, _, _ := placeLegend(names, sheet.LegendRight, 20, 12); lg.pos != sheet.LegendNone {
		t.Errorf("narrow: %+v", lg)
	}
}
