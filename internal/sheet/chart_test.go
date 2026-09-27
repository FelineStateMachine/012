package sheet

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"
)

// budgetCells is a small table: labels down A, two series with headers.
var budgetCells = map[string]string{
	"A1": "Month", "B1": "Rent", "C1": "Food",
	"A2": "Jan", "B2": "1450", "C2": "600",
	"A3": "Feb", "B3": "1450", "C3": "",
	"A4": "Mar", "B4": "1500", "C4": "=C2+12",
}

func rng(s string) Rect {
	r, ok := ParseRange(s)
	if !ok {
		panic(s)
	}
	return r
}

// sameFloats compares with NaN equal to NaN.
func sameFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && !(math.IsNaN(a[i]) && math.IsNaN(b[i])) {
			return false
		}
	}
	return true
}

func TestChartData(t *testing.T) {
	nan := math.NaN()
	s := sheetOf(t, budgetCells)
	tests := []struct {
		name   string
		chart  Chart
		cats   []string
		names  []string
		values [][]float64
	}{
		{"columns with header and labels", Chart{Data: rng("A1:C4"), Header: true, Labels: true},
			[]string{"Jan", "Feb", "Mar"}, []string{"Rent", "Food"},
			[][]float64{{1450, 1450, 1500}, {600, nan, 612}}},
		{"no header: the first row is data", Chart{Data: rng("B2:C4")},
			[]string{"1", "2", "3"}, []string{"Series 1", "Series 2"},
			[][]float64{{1450, 1450, 1500}, {600, nan, 612}}},
		{"series in rows", Chart{Data: rng("A1:C4"), ByRow: true, Header: true, Labels: true},
			[]string{"Rent", "Food"}, []string{"Jan", "Feb", "Mar"},
			[][]float64{{1450, 600}, {1450, nan}, {1500, 612}}},
		{"whole columns stop at the data", Chart{Data: rng("B1:B8192"), Header: true},
			[]string{"1", "2", "3"}, []string{"Rent"},
			[][]float64{{1450, 1450, 1500}}},
		{"text in a series is a gap", Chart{Data: rng("A2:B4")},
			[]string{"1", "2", "3"}, []string{"Series 1", "Series 2"},
			[][]float64{{nan, nan, nan}, {1450, 1450, 1500}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := s.ChartData(tt.chart)
			if !reflect.DeepEqual(d.Categories, tt.cats) {
				t.Errorf("categories %q, want %q", d.Categories, tt.cats)
			}
			if len(d.Series) != len(tt.names) {
				t.Fatalf("%d series, want %d", len(d.Series), len(tt.names))
			}
			for i, sr := range d.Series {
				if sr.Name != tt.names[i] || !sameFloats(sr.Values, tt.values[i]) {
					t.Errorf("series %d = %q %v, want %q %v", i, sr.Name, sr.Values, tt.names[i], tt.values[i])
				}
			}
		})
	}
}

func TestChartDataFormat(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "$1,200", "A2": "$900"})
	if d := s.ChartData(Chart{Data: rng("A1:A2")}); d.Format.Kind != FmtCurrency {
		t.Errorf("format %v, want currency", d.Format.Kind)
	}
}

func TestGuessChart(t *testing.T) {
	s := sheetOf(t, budgetCells)
	tests := []struct {
		r              string
		header, labels bool
		title          string
	}{
		{"A1:C4", true, true, "Rent and Food"},
		{"B1:B4", true, false, "Rent"},
		{"B2:C4", false, false, ""},
		{"A2:B4", false, true, ""},
		{"A1:D9", true, true, "Rent and Food"}, // trimmed to the data
	}
	for _, tt := range tests {
		c := s.GuessChart(rng(tt.r))
		if c.Header != tt.header || c.Labels != tt.labels || c.Title != tt.title || c.Type != ChartColumn {
			t.Errorf("%s: header %v labels %v title %q, want %v %v %q", tt.r, c.Header, c.Labels, c.Title, tt.header, tt.labels, tt.title)
		}
	}
}

func TestRegion(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"B2": "x", "C2": "1", "C3": "2", "D4": "3", // D4 touches C3 diagonally
		"F2": "far",
	})
	tests := []struct{ from, want string }{
		{"C3", "B2:D4"},
		{"B2", "B2:D4"},
		{"F2", "F2"},
		{"H9", "H9"},
	}
	for _, tt := range tests {
		if got := s.Region(at(tt.from)).String(); got != tt.want {
			t.Errorf("Region(%s) = %s, want %s", tt.from, got, tt.want)
		}
	}
}

func TestChartUndo(t *testing.T) {
	s := sheetOf(t, budgetCells)
	c := s.GuessChart(rng("A1:C4"))
	c.At, c.W, c.H = at("E2"), 40, 12
	i := s.AddChart(c)
	moved := c
	moved.At = at("F9")
	s.SetChart(i, moved, "move chart")
	s.DeleteChart(i)
	if len(s.Charts()) != 0 {
		t.Fatal("chart not deleted")
	}
	for _, want := range []struct {
		label string
		at    string
		n     int
	}{{"delete chart", "F9", 1}, {"move chart", "E2", 1}, {"insert chart", "", 0}} {
		ch, ok := s.Undo()
		if !ok || ch.Label != want.label {
			t.Fatalf("undo %q, want %q", ch.Label, want.label)
		}
		if got := s.Charts(); len(got) != want.n || want.n > 0 && got[0].At.String() != want.at {
			t.Fatalf("after undoing %s: %+v", want.label, got)
		}
	}
	s.Redo()
	if got := s.Charts(); len(got) != 1 || got[0].At != at("E2") {
		t.Fatalf("redo: %+v", got)
	}
	// Setting a chart to what it is isn't a step.
	id := s.StateID()
	s.SetChart(0, s.Charts()[0], "edit chart")
	if s.StateID() != id {
		t.Error("no-op edit made an undo step")
	}
}

func TestChartsFollowRowsAndColumns(t *testing.T) {
	s := sheetOf(t, budgetCells)
	s.AddChart(Chart{Data: rng("A1:C4"), At: at("E2"), W: 30, H: 10})
	s.InsertRows(2, 1)
	s.InsertCols(0, 2)
	c := s.Charts()[0]
	if c.Data.String() != "C1:E5" || c.At.String() != "G2" {
		t.Errorf("after inserts: data %s at %s", c.Data, c.At)
	}
	s.DeleteCols(2, 3)
	if n := len(s.Charts()); n != 0 {
		t.Errorf("chart over deleted columns kept (%d)", n)
	}
	s.Undo()
	if n := len(s.Charts()); n != 1 {
		t.Errorf("undo didn't bring the chart back (%d)", n)
	}
}

func TestChartFileRoundTrip(t *testing.T) {
	s := sheetOf(t, budgetCells)
	want := []Chart{
		{Type: ChartLine, Data: rng("A1:C4"), Header: true, Labels: true, Title: "Spend", At: at("E2"), W: 44, H: 14},
		{Type: ChartPie, Data: rng("B2:B4"), ByRow: true, At: at("A20"), W: 30, H: 12},
	}
	for _, c := range want {
		s.AddChart(c)
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"charts": [`+"\n    "+`{"type":"line","data":"A1:C4","at":"E2","width":44,"height":14,"header":true,"labels":true,"title":"Spend"}`) {
		t.Errorf("charts not one per line:\n%s", buf.String())
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Charts(), want) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got.Charts(), want)
	}
	// Older files have no charts field.
	old, err := Read(strings.NewReader(`{"version": 2, "cells": {"A1": "1"}}`))
	if err != nil || len(old.Charts()) != 0 {
		t.Errorf("old file: %v %v", err, old.Charts())
	}
	if _, err := Read(strings.NewReader(`{"version": 2, "cells": {}, "charts": [{"type": "radar", "data": "A1", "at": "A1"}]}`)); err == nil {
		t.Error("unknown chart type accepted")
	}
}
