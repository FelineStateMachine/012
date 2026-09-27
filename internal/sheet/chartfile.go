package sheet

import (
	"encoding/json"
	"fmt"
)

// fileChart is a chart, one per line after the cells:
//
//	{"type": "column", "data": "A1:C7", "at": "E2", "width": 44, "height": 14, "header": true, "labels": true}
//
// Options at their defaults are left out, so they need no version bump:
// older versions of 012 draw such a chart with the defaults, and older
// versions still ignore the field and drop the charts. A type they don't
// know makes them refuse the file.
type fileChart struct {
	Type   string `json:"type"`
	Data   string `json:"data"`
	At     string `json:"at"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	ByRow  bool   `json:"byRow,omitempty"`
	Header bool   `json:"header,omitempty"`
	Labels bool   `json:"labels,omitempty"`
	Title  string `json:"title,omitempty"`
	// Options: "stacked" or "percent"; a trend line; the ends of the value
	// axis; a log scale; gridlines, only when off; "right" or "none".
	Stack     string   `json:"stack,omitempty"`
	Trend     bool     `json:"trend,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Log       bool     `json:"log,omitempty"`
	Gridlines *bool    `json:"gridlines,omitempty"`
	Legend    string   `json:"legend,omitempty"`
}

func encodeChart(c Chart) ([]byte, error) {
	fc := fileChart{
		Type: c.Type.String(), Data: c.Data.String(), At: c.At.String(), Width: c.W, Height: c.H,
		ByRow: c.ByRow, Header: c.Header, Labels: c.Labels, Title: c.Title,
		Stack: c.Stack.String(), Trend: c.Trend, Log: c.Log,
		Min: axisEnd(c.Min, c.HasMin), Max: axisEnd(c.Max, c.HasMax),
	}
	if c.NoGrid {
		fc.Gridlines = new(bool)
	}
	if c.Legend != LegendBottom {
		fc.Legend = c.Legend.String()
	}
	return json.Marshal(fc)
}

func decodeChart(fc fileChart) (Chart, error) {
	t, ok := ParseChartType(fc.Type)
	if !ok {
		return Chart{}, fmt.Errorf("unknown chart type %q", fc.Type)
	}
	data, ok := ParseRange(fc.Data)
	if !ok {
		return Chart{}, fmt.Errorf("invalid chart range %q", fc.Data)
	}
	at, ok := ParseAddr(fc.At)
	if !ok {
		return Chart{}, fmt.Errorf("invalid chart position %q", fc.At)
	}
	c := Chart{Type: t, Data: data, At: at, W: fc.Width, H: fc.Height,
		ByRow: fc.ByRow, Header: fc.Header, Labels: fc.Labels, Title: fc.Title}
	// Options a later build writes and this one doesn't know are left at
	// their defaults, as an older build would.
	c.Stack, _ = ParseChartStack(fc.Stack)
	c.Legend, _ = ParseChartLegend(fc.Legend)
	c.Trend, c.Log = fc.Trend, fc.Log
	c.NoGrid = fc.Gridlines != nil && !*fc.Gridlines
	if fc.Min != nil {
		c.Min, c.HasMin = *fc.Min, true
	}
	if fc.Max != nil {
		c.Max, c.HasMax = *fc.Max, true
	}
	return c.clamped(), nil
}
