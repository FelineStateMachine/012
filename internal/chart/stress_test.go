//go:build stress

// Stress benchmarks of chart drawing: every chart type as text and as an
// image, at a typical size and a large one, over a few categories and
// over a full column of them (only what fits is drawn). Only built with
// -tags stress (see `make stress` and docs/contributing/limits.md).
package chart

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// stressData is n categories of three series of wavy values.
func stressData(n int) sheet.ChartData {
	d := sheet.ChartData{Categories: make([]string, n)}
	for i := range n {
		d.Categories[i] = "Cat " + strconv.Itoa(i+1)
	}
	for j := range 3 {
		s := sheet.ChartSeries{Name: "Series " + strconv.Itoa(j+1), Values: make([]float64, n)}
		for i := range n {
			s.Values[i] = 1000*math.Sin(float64(i+j)/7) + 200*float64(j)
		}
		d.Series = append(d.Series, s)
	}
	return d
}

var stressTypes = []struct {
	name string
	t    sheet.ChartType
}{
	{"column", sheet.ChartColumn},
	{"bar", sheet.ChartBar},
	{"line", sheet.ChartLine},
	{"pie", sheet.ChartPie},
	{"area", sheet.ChartArea},
	{"scatter", sheet.ChartScatter},
}

var stressSizes = []struct{ w, h int }{{24, 10}, {120, 40}}

// BenchmarkDraw is a chart drawn as text, as every frame showing it does.
func BenchmarkDraw(b *testing.B) {
	for _, n := range []int{12, 8192} {
		d := stressData(n)
		for _, ct := range stressTypes {
			for _, sz := range stressSizes {
				b.Run(fmt.Sprintf("%s/%dcats/%dx%d", ct.name, n, sz.w, sz.h), func(b *testing.B) {
					for b.Loop() {
						Draw(ct.t, d, sz.w, sz.h, Options{})
					}
				})
			}
		}
	}
}

// BenchmarkImage is a chart's plot drawn as a kitty image.
func BenchmarkImage(b *testing.B) {
	pal := Palette{Grid: color.RGBA{128, 128, 128, 80}}
	d := stressData(12)
	for _, ct := range stressTypes {
		for _, sz := range stressSizes {
			b.Run(fmt.Sprintf("%s/%dx%d", ct.name, sz.w, sz.h), func(b *testing.B) {
				for b.Loop() {
					Image(ct.t, d, sz.w, sz.h, Options{CellW: 10, CellH: 20}, pal)
				}
			})
		}
	}
}

// BenchmarkSixel is a chart's image encoded as sixel, in 256 colors, as
// it is whenever its data, size or colors change.
func BenchmarkSixel(b *testing.B) {
	pal := Palette{Series: [Colors]color.RGBA{{205, 49, 49, 255}, {13, 188, 121, 255}, {36, 114, 200, 255}}, Grid: color.RGBA{128, 128, 128, 80}}
	d := stressData(12)
	for _, ct := range stressTypes {
		for _, sz := range stressSizes {
			img := Image(ct.t, d, sz.w, sz.h, Options{CellW: 10, CellH: 20}, pal)
			b.Run(fmt.Sprintf("%s/%dx%d", ct.name, sz.w, sz.h), func(b *testing.B) {
				n := 0
				for b.Loop() {
					n = len(Sixel(img, color.RGBA{0, 0, 0, 255}, 256))
				}
				b.ReportMetric(float64(n), "bytes")
			})
		}
	}
}
