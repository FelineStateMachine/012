package chart

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// TestSVGEveryType draws each type as SVG, checking it's well-formed
// XML with the chart's labels as text and shapes of its own kind.
func TestSVGEveryType(t *testing.T) {
	cases := []struct {
		typ   sheet.ChartType
		data  sheet.ChartData
		opts  sheet.ChartOptions
		shape string
	}{
		{sheet.ChartColumn, budget, sheet.ChartOptions{}, "<rect"},
		{sheet.ChartBar, single, sheet.ChartOptions{}, "<rect"},
		{sheet.ChartLine, budget, sheet.ChartOptions{}, "<polyline"},
		{sheet.ChartArea, budget, sheet.ChartOptions{}, "<polygon"},
		{sheet.ChartArea, budget, sheet.ChartOptions{Stack: sheet.StackNormal}, "<polygon"},
		{sheet.ChartPie, single, sheet.ChartOptions{}, "<path"},
		{sheet.ChartScatter, xy, sheet.ChartOptions{Trend: true}, "<circle"},
		{sheet.ChartColumn, signs, sheet.ChartOptions{Stack: sheet.StackNormal}, "<rect"},
	}
	for _, c := range cases {
		t.Run(c.typ.String(), func(t *testing.T) {
			svg := SVG(c.typ, c.data, 60, 18, Options{Chart: c.opts}, "Budget & more")
			d := xml.NewDecoder(strings.NewReader(svg))
			for {
				if _, err := d.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Fatalf("not XML: %v\n%s", err, svg)
				}
			}
			if !strings.Contains(svg, c.shape) {
				t.Errorf("no %s in\n%s", c.shape, svg)
			}
			if !strings.Contains(svg, "<title>Budget &amp; more</title>") {
				t.Errorf("no title")
			}
			if c.typ != sheet.ChartPie && !strings.Contains(svg, `class="ct-lb"`) {
				t.Errorf("no labels")
			}
		})
	}
}

// TestSVGAreaIsFewShapes joins an area's pixel columns into polygons.
func TestSVGAreaIsFewShapes(t *testing.T) {
	svg := SVG(sheet.ChartArea, single, 80, 20, Options{}, "")
	if n := strings.Count(svg, "<polygon"); n == 0 || n > 3 {
		t.Errorf("%d polygons", n)
	}
	if strings.Contains(svg, "<rect") {
		t.Errorf("area columns drawn as rects")
	}
}

// TestSVGNothingToPlot shows the text chart's message alone.
func TestSVGNothingToPlot(t *testing.T) {
	svg := SVG(sheet.ChartColumn, sheet.ChartData{}, 40, 10, Options{}, "")
	if !strings.Contains(svg, "No numbers to chart") || strings.Contains(svg, "<g ") {
		t.Errorf("got %s", svg)
	}
}
