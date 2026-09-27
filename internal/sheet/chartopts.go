package sheet

import (
	"math"
	"slices"
	"strings"
)

// ChartOptions are a chart's settings beyond its type and data, as in
// the Customize tab of Sheets' chart editor. The zero value is Sheets'
// default: not stacked, no trend line, an automatic linear value axis
// with gridlines, and the legend at the bottom. Every field is a plain
// value, so charts stay comparable with ==.
type ChartOptions struct {
	// Stack stacks the series of column, bar and area charts.
	Stack ChartStack
	// Trend draws a linear trend line through each series of a scatter
	// chart.
	Trend bool
	// Min and Max fix the ends of the value axis when HasMin and HasMax
	// are set; otherwise the axis fits the data.
	Min, Max       float64
	HasMin, HasMax bool
	// Log puts the value axis on a logarithmic scale, leaving out values
	// that aren't positive.
	Log bool
	// NoGrid hides the gridlines.
	NoGrid bool
	// Legend is where the legend goes.
	Legend ChartLegend
}

// ChartStack is how the series of a chart pile up.
type ChartStack int

const (
	StackNone    ChartStack = iota // side by side
	StackNormal                    // on top of each other
	StackPercent                   // on top of each other, as shares of 100%
)

var chartStackNames = [...]string{StackNone: "", StackNormal: "stacked", StackPercent: "percent"}

// String is the stacking as files store it, "" for none.
func (s ChartStack) String() string {
	if s < 0 || int(s) >= len(chartStackNames) {
		return ""
	}
	return chartStackNames[s]
}

// Title is the stacking as the chart editor shows it.
func (s ChartStack) Title() string {
	switch s {
	case StackNormal:
		return "Stacked"
	case StackPercent:
		return "100% stacked"
	}
	return "Not stacked"
}

// ParseChartStack parses a stacking as String writes it.
func ParseChartStack(s string) (ChartStack, bool) {
	i := slices.Index(chartStackNames[:], strings.ToLower(s))
	return ChartStack(max(i, 0)), i >= 0
}

// Stackable reports whether charts of type t can stack their series.
func (t ChartType) Stackable() bool {
	return t == ChartColumn || t == ChartBar || t == ChartArea
}

// ChartLegend is where a chart's legend goes.
type ChartLegend int

const (
	LegendBottom ChartLegend = iota // under the plot, centered
	LegendRight                     // a column right of the plot
	LegendNone                      // no legend
)

var chartLegendNames = [...]string{LegendBottom: "bottom", LegendRight: "right", LegendNone: "none"}

// String is the position as files store it.
func (l ChartLegend) String() string {
	if l < 0 || int(l) >= len(chartLegendNames) {
		return chartLegendNames[0]
	}
	return chartLegendNames[l]
}

// ParseChartLegend parses a position as String writes it.
func ParseChartLegend(s string) (ChartLegend, bool) {
	i := slices.Index(chartLegendNames[:], strings.ToLower(s))
	return ChartLegend(max(i, 0)), i >= 0
}

// axisEnd is an optional end of the value axis as files store it: nil
// for automatic.
func axisEnd(v float64, ok bool) *float64 {
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}
