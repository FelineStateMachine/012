package chart

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"flag"
	"image"
	"image/color"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

var update = flag.Bool("update", false, "rewrite text chart goldens in testdata")

var nan = math.NaN()

var budget = sheet.ChartData{
	Categories: []string{"Jan", "Feb", "Mar", "Apr"},
	Series: []sheet.ChartSeries{
		{Name: "Rent", Values: []float64{1450, 1450, 1500, 1500}},
		{Name: "Food", Values: []float64{600, nan, 612, 700}},
	},
	Format: sheet.Format{Kind: sheet.FmtCurrency},
}

var single = sheet.ChartData{
	Categories: []string{"North", "South", "East", "West"},
	Series:     []sheet.ChartSeries{{Name: "Sales", Values: []float64{120, 80, 45, 30}}},
}

var mixed = sheet.ChartData{
	Categories: []string{"Q1", "Q2", "Q3", "Q4"},
	Series:     []sheet.ChartSeries{{Name: "Profit", Values: []float64{0.12, -0.05, 0.08, 0.2}}},
	Format:     sheet.Format{Kind: sheet.FmtPercent},
}

// signs has positive and negative values in the same category.
var signs = sheet.ChartData{
	Categories: []string{"Q1", "Q2", "Q3"},
	Series: []sheet.ChartSeries{
		{Name: "Sales", Values: []float64{30, 40, 20}},
		{Name: "Costs", Values: []float64{-20, -10, -30}},
		{Name: "Other", Values: []float64{10, -5, 15}},
	},
}

// xy is a scatter: X in the first series, two series of Y against it.
var xy = sheet.ChartData{
	Categories: []string{"1", "2", "3", "4", "5", "6"},
	Series: []sheet.ChartSeries{
		{Name: "Hours", Values: []float64{1, 2, 3, 5, 6, 8}},
		{Name: "Score", Values: []float64{52, 55, 61, 70, 71, 80}},
		{Name: "Retake", Values: []float64{40, nan, 50, 58, 66, 69}},
	},
}

// growth spans several powers of ten, for log scales.
var growth = sheet.ChartData{
	Categories: []string{"Y1", "Y2", "Y3", "Y4", "Y5"},
	Series:     []sheet.ChartSeries{{Name: "Users", Values: []float64{3, 40, 700, 9000, 0}}},
}

// TestDrawGoldens renders every chart type as text; the goldens in
// testdata are the review surface for how charts look.
func TestDrawGoldens(t *testing.T) {
	cases := []struct {
		name string
		t    sheet.ChartType
		d    sheet.ChartData
		w, h int
		o    sheet.ChartOptions
	}{
		{"column", sheet.ChartColumn, budget, 44, 12, sheet.ChartOptions{}},
		{"column-single", sheet.ChartColumn, single, 40, 10, sheet.ChartOptions{}},
		{"column-negative", sheet.ChartColumn, mixed, 36, 10, sheet.ChartOptions{}},
		{"column-narrow", sheet.ChartColumn, budget, 22, 8, sheet.ChartOptions{}},
		{"bar", sheet.ChartBar, budget, 44, 12, sheet.ChartOptions{}},
		{"bar-single", sheet.ChartBar, single, 40, 8, sheet.ChartOptions{}},
		{"line", sheet.ChartLine, budget, 44, 12, sheet.ChartOptions{}},
		{"line-single", sheet.ChartLine, single, 40, 10, sheet.ChartOptions{}},
		{"pie", sheet.ChartPie, single, 40, 10, sheet.ChartOptions{}},
		{"empty", sheet.ChartColumn, sheet.ChartData{Categories: []string{"a"}, Series: []sheet.ChartSeries{{Values: []float64{nan}}}}, 30, 8, sheet.ChartOptions{}},
		{"pie-negative", sheet.ChartPie, sheet.ChartData{Categories: []string{"a"}, Series: []sheet.ChartSeries{{Values: []float64{-1}}}}, 30, 8, sheet.ChartOptions{}},
		{"pie-no-legend", sheet.ChartPie, single, 40, 10, sheet.ChartOptions{Legend: sheet.LegendNone}},
		{"area", sheet.ChartArea, budget, 44, 12, sheet.ChartOptions{}},
		{"area-stacked", sheet.ChartArea, budget, 44, 12, sheet.ChartOptions{Stack: sheet.StackNormal}},
		{"area-negative", sheet.ChartArea, mixed, 36, 10, sheet.ChartOptions{}},
		{"column-stacked", sheet.ChartColumn, budget, 44, 12, sheet.ChartOptions{Stack: sheet.StackNormal}},
		{"column-percent", sheet.ChartColumn, budget, 44, 12, sheet.ChartOptions{Stack: sheet.StackPercent}},
		{"column-stacked-negative", sheet.ChartColumn, signs, 40, 12, sheet.ChartOptions{Stack: sheet.StackNormal}},
		{"bar-stacked", sheet.ChartBar, budget, 44, 12, sheet.ChartOptions{Stack: sheet.StackNormal}},
		{"bar-percent", sheet.ChartBar, budget, 44, 12, sheet.ChartOptions{Stack: sheet.StackPercent}},
		{"scatter", sheet.ChartScatter, xy, 44, 12, sheet.ChartOptions{}},
		{"scatter-trend", sheet.ChartScatter, xy, 44, 12, sheet.ChartOptions{Trend: true}},
		{"scatter-single", sheet.ChartScatter, single, 40, 10, sheet.ChartOptions{}},
		{"legend-right", sheet.ChartColumn, budget, 44, 12, sheet.ChartOptions{Legend: sheet.LegendRight}},
		{"legend-none", sheet.ChartLine, budget, 44, 12, sheet.ChartOptions{Legend: sheet.LegendNone}},
		{"no-gridlines", sheet.ChartColumn, single, 40, 10, sheet.ChartOptions{NoGrid: true}},
		{"axis-min-max", sheet.ChartColumn, budget, 44, 12, sheet.ChartOptions{Min: 500, HasMin: true, Max: 1600, HasMax: true}},
		{"axis-min", sheet.ChartLine, budget, 44, 12, sheet.ChartOptions{Min: 400, HasMin: true}},
		{"axis-log", sheet.ChartColumn, growth, 40, 12, sheet.ChartOptions{Log: true}},
		{"bar-log", sheet.ChartBar, growth, 40, 10, sheet.ChartOptions{Log: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Draw(tc.t, tc.d, tc.w, tc.h, Options{Chart: tc.o}).String() + "\n"
			path := filepath.Join("testdata", tc.name+".txt")
			if *update {
				os.MkdirAll("testdata", 0o755)
				os.WriteFile(path, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden (go test ./internal/chart -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("chart changed:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestScale(t *testing.T) {
	tests := []struct {
		lo, hi         float64
		length         int
		f              sheet.Format
		wantLo, wantHi float64
		labels         []string
	}{
		{0, 1500, 9, sheet.Format{Kind: sheet.FmtCurrency}, 0, 1500, []string{"$0", "$500", "$1,000", "$1,500"}},
		{0, 1, 10, sheet.Format{}, 0, 1, []string{"0.0", "0.2", "0.4", "0.6", "0.8", "1.0"}},
		{-0.05, 0.2, 7, sheet.Format{Kind: sheet.FmtPercent}, -0.1, 0.2, []string{"-10%", "0%", "10%", "20%"}},
		{0, 48000, 8, sheet.Format{}, 0, 60000, []string{"0K", "20K", "40K", "60K"}},
		{0, 0.75, 6, sheet.Format{}, 0, 0.75, []string{"0.00", "0.25", "0.50", "0.75"}},
		{5, 5, 4, sheet.Format{}, 0, 5, nil},
	}
	for _, tt := range tests {
		s := newScale(tt.lo, tt.hi, tt.length, 2, 6, tt.f)
		if s.lo != tt.wantLo || s.hi != tt.wantHi {
			t.Errorf("%v..%v: scale %v..%v, want %v..%v", tt.lo, tt.hi, s.lo, s.hi, tt.wantLo, tt.wantHi)
		}
		if s.cells() > tt.length || s.k < 2 {
			t.Errorf("%v..%v: %d intervals of %d cells in %d", tt.lo, tt.hi, s.n, s.k, tt.length)
		}
		if tt.labels == nil {
			continue
		}
		var got []string
		for j := 0; j <= s.n; j++ {
			got = append(got, s.label(s.tick(j)))
		}
		if strings.Join(got, " ") != strings.Join(tt.labels, " ") {
			t.Errorf("%v..%v: labels %q, want %q", tt.lo, tt.hi, got, tt.labels)
		}
	}
}

func TestLayout(t *testing.T) {
	// Columns: every bar lies inside the plot and bars don't overlap.
	p := newColumnPlan(budget, 44, 12, columnBars, sheet.ChartOptions{})
	prev := 0
	for i := range 4 {
		for j := range 2 {
			x0, x1 := p.bar(i, j)
			if x0 < p.plot.Min.X || x1 > p.plot.Max.X || x0 < prev {
				t.Errorf("bar %d,%d at %d..%d overlaps or leaves %v", i, j, x0, x1, p.plot)
			}
			prev = x1
		}
	}
	// Ticks land on cell boundaries, the top one inside the plot.
	if top := p.sc.cells(); top > p.plot.Dy() || p.sc.pos(p.sc.hi) != float64(top) {
		t.Errorf("value axis %d cells in a %d row plot", top, p.plot.Dy())
	}
	// Too many categories: only what fits is drawn.
	many := sheet.ChartData{Series: []sheet.ChartSeries{{Name: "x"}}}
	for i := range 100 {
		many.Categories = append(many.Categories, "c")
		many.Series[0].Values = append(many.Series[0].Values, float64(i))
	}
	if p := newColumnPlan(many, 30, 10, columnBars, sheet.ChartOptions{}); p.shown != p.plot.Dx() {
		t.Errorf("shown %d categories in %d columns", p.shown, p.plot.Dx())
	}
	// The plot rectangle is what Image fills.
	for _, ty := range sheet.ChartTypes {
		o := Options{CellW: 8, CellH: 16}
		g := Draw(ty, budget, 44, 12, o)
		img := Image(ty, budget, 44, 12, o, testPalette)
		if img == nil || img.Bounds().Dx() != g.Plot.Dx()*8 || img.Bounds().Dy() != g.Plot.Dy()*16 {
			t.Errorf("%v: image for plot %v", ty, g.Plot)
		}
	}
}

// TestTypes checks every chart type the editor offers has a layout.
func TestTypes(t *testing.T) {
	for _, ty := range sheet.ChartTypes {
		if types[ty].layout == nil {
			t.Errorf("%v has no layout", ty)
		}
	}
	if len(types) != len(sheet.ChartTypes) {
		t.Errorf("%d layouts for %d chart types", len(types), len(sheet.ChartTypes))
	}
}

func TestImageLeavesPlotToImage(t *testing.T) {
	g := Draw(sheet.ChartColumn, budget, 44, 12, Options{Image: true})
	for y := g.Plot.Min.Y; y < g.Plot.Max.Y; y++ {
		for x := g.Plot.Min.X; x < g.Plot.Max.X; x++ {
			if c := g.At(x, y); c.Text != " " {
				t.Fatalf("plot cell %d,%d = %q with an image", x, y, c.Text)
			}
		}
	}
	if !strings.Contains(g.String(), "$1,500 ┤") || !strings.Contains(g.String(), "■ Rent") {
		t.Errorf("labels missing:\n%s", g)
	}
}

var testPalette = Palette{
	Series: [Colors]color.RGBA{{255, 0, 0, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}, {9, 9, 9, 255}, {8, 8, 8, 255}, {7, 7, 7, 255}},
	Grid:   color.RGBA{128, 128, 128, 80},
}

func TestImagePixels(t *testing.T) {
	o := Options{CellW: 8, CellH: 16}
	p := newColumnPlan(single, 40, 10, columnBars, sheet.ChartOptions{})
	img := Image(sheet.ChartColumn, single, 40, 10, o, testPalette)
	x0, x1 := p.bar(0, 0)
	mid := ((x0+x1)/2 - p.plot.Min.X) * 8
	bottom := img.Bounds().Dy() - 2
	if c := img.RGBAAt(mid, bottom); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("bar pixel %v", c)
	}
	if c := img.RGBAAt(mid, 1); c.A != 0 {
		t.Errorf("above the bar %v, want transparent", c)
	}
	// Pie: inside the first slice (top right) is its color.
	pie := Image(sheet.ChartPie, single, 40, 10, o, testPalette)
	b := pie.Bounds()
	if c := pie.RGBAAt(b.Dx()/2+b.Dx()/8, b.Dy()/2-b.Dy()/4); c != testPalette.Series[0] {
		t.Errorf("pie slice pixel %v", c)
	}
	if c := pie.RGBAAt(0, 0); c.A != 0 {
		t.Errorf("pie corner %v, want transparent", c)
	}
}

func TestTransmit(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Pix = []uint8{255, 0, 0, 255, 0, 0, 64, 128} // opaque red, half blue (premultiplied)
	got := Transmit(7, img, 3, 2, nil)
	re := regexp.MustCompile("^\x1b_Ga=T,q=2,f=32,o=z,s=2,v=1,i=7,U=1,c=3,r=2,m=0;([A-Za-z0-9+/=]+)\x1b\\\\$")
	m := re.FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("transmit = %q", got)
	}
	z, _ := base64.StdEncoding.DecodeString(m[1])
	zr, err := zlib.NewReader(bytes.NewReader(z))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(zr)
	if want := []byte{255, 0, 0, 255, 0, 0, 127, 128}; !bytes.Equal(raw, want) {
		t.Errorf("pixels %v, want straight alpha %v", raw, want)
	}

	// Big images go in chunks of at most 4096 bytes, each wrapped.
	noise := image.NewRGBA(image.Rect(0, 0, 200, 200))
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range noise.Pix {
		noise.Pix[i] = uint8(rng.IntN(256))
	}
	wrapped := Transmit(9, noise, 20, 10, func(s string) string { return "<" + s + ">" })
	chunks := strings.Split(strings.TrimSuffix(strings.TrimPrefix(wrapped, "<"), ">"), "><")
	if len(chunks) < 2 {
		t.Fatalf("%d chunks", len(chunks))
	}
	for i, c := range chunks {
		head, body, _ := strings.Cut(strings.TrimPrefix(c, "\x1b_G"), ";")
		last := i == len(chunks)-1
		switch {
		case i == 0 && !strings.HasPrefix(head, "a=T,q=2,") || i > 0 && !strings.HasPrefix(head, "q=2,"):
			t.Errorf("chunk %d options %q", i, head)
		case last != strings.HasSuffix(head, "m=0"):
			t.Errorf("chunk %d of %d: %q", i, len(chunks), head)
		case len(strings.TrimSuffix(body, "\x1b\\")) > 4096:
			t.Errorf("chunk %d is %d bytes", i, len(body))
		}
	}
}

func TestPlaceholderAndDelete(t *testing.T) {
	if got := Placeholder(0, 2); got != "\U0010EEEE̅̎" {
		t.Errorf("placeholder %q", got)
	}
	if got := Delete(7, nil); got != "\x1b_Ga=d,d=I,i=7,q=2\x1b\\" {
		t.Errorf("delete %q", got)
	}
	if got := Query(); got != "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\" {
		t.Errorf("query %q", got)
	}
}
