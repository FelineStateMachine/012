package sheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
)

// FileExt is the extension of the native worksheet format.
const FileExt = ".012"

// fileVersion is the newest version Write produces and Read accepts.
// Version 1 files (cells as plain strings, no formatting) still load.
// Version 3 adds named ranges, frozen panes and a filter; a sheet using
// none of them is written as version 2, so earlier builds can open it.
const fileVersion = 3

// The file is JSON with one entry per cell, keyed by address. A cell
// without formatting is just its input, as in version 1; a formatted cell
// is an object, so diffs stay one line per cell:
//
//	"B2": "Rent",
//	"C2": {"input": "1450", "format": "currency", "decimals": 2, "bold": true}
//
// Named ranges map each name to its range, or to "#REF!" once its cells
// were deleted: "names": {"Sales": "B2:B20"}.
type fileFormat struct {
	Version int               `json:"version"`
	Widths  map[string]int    `json:"widths,omitempty"`
	Names   map[string]string `json:"names,omitempty"`
	fileView
	Cells  map[string]json.RawMessage `json:"cells"`
	Charts []fileChart                `json:"charts,omitempty"`
}

// fileChart is a chart, one per line after the cells:
//
//	{"type": "column", "data": "A1:C7", "at": "E2", "width": 44, "height": 14, "header": true, "labels": true}
//
// Older versions of 012 ignore the field and drop the charts.
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
}

func encodeChart(c Chart) ([]byte, error) {
	return json.Marshal(fileChart{
		Type: c.Type.String(), Data: c.Data.String(), At: c.At.String(), Width: c.W, Height: c.H,
		ByRow: c.ByRow, Header: c.Header, Labels: c.Labels, Title: c.Title,
	})
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
	return c.clamped(), nil
}

type fileCell struct {
	Input         string `json:"input,omitempty"`
	Format        string `json:"format,omitempty"`
	Decimals      *int   `json:"decimals,omitempty"`
	Pattern       string `json:"pattern,omitempty"`
	Bold          bool   `json:"bold,omitempty"`
	Italic        bool   `json:"italic,omitempty"`
	Underline     bool   `json:"underline,omitempty"`
	Strikethrough bool   `json:"strikethrough,omitempty"`
	Align         string `json:"align,omitempty"`
}

func encodeCell(c *Cell) (json.RawMessage, error) {
	if c.Format.IsZero() && c.Style.IsZero() {
		return json.Marshal(c.Input)
	}
	fc := fileCell{
		Input:         c.Input,
		Pattern:       c.Format.Pattern,
		Bold:          c.Style.Bold,
		Italic:        c.Style.Italic,
		Underline:     c.Style.Underline,
		Strikethrough: c.Style.Strikethrough,
		Align:         c.Style.Align.String(),
	}
	if !c.Format.IsZero() {
		fc.Format = c.Format.Kind.String()
	}
	if c.Format.Kind.hasDecimals() {
		d := c.Format.Decimals
		fc.Decimals = &d
	}
	return json.Marshal(fc)
}

func decodeCell(raw json.RawMessage) (string, Format, Style, error) {
	var input string
	if err := json.Unmarshal(raw, &input); err == nil {
		return input, Format{}, Style{}, nil
	}
	var fc fileCell
	if err := json.Unmarshal(raw, &fc); err != nil {
		return "", Format{}, Style{}, err
	}
	var f Format
	if fc.Format != "" {
		k, ok := ParseFormatKind(fc.Format)
		if !ok {
			return "", Format{}, Style{}, fmt.Errorf("unknown format %q", fc.Format)
		}
		f = Format{Kind: k, Pattern: fc.Pattern}
		if fc.Decimals != nil {
			f.Decimals = clampInt(*fc.Decimals, 0, MaxDecimals)
		}
	}
	al, ok := ParseAlign(fc.Align)
	if !ok {
		return "", Format{}, Style{}, fmt.Errorf("unknown alignment %q", fc.Align)
	}
	st := Style{Bold: fc.Bold, Italic: fc.Italic, Underline: fc.Underline, Strikethrough: fc.Strikethrough, Align: al}
	return fc.Input, f, st, nil
}

// Write saves the worksheet as JSON, storing each cell's input as typed
// and its formatting. Cells go one per line in row-major order so diffs
// read naturally.
func (s *Sheet) Write(w io.Writer) error {
	var b bytes.Buffer
	version := 2
	if v := s.view; len(s.names) > 0 || v.frozenRows > 0 || v.frozenCols > 0 || v.filter != nil {
		version = 3
	}
	fmt.Fprintf(&b, "{\n  \"version\": %d,\n", version)
	if len(s.widths) > 0 {
		b.WriteString(`  "widths": {`)
		for i, c := range slices.Sorted(maps.Keys(s.widths)) {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q: %d", ColName(c), s.widths[c])
		}
		b.WriteString("},\n")
	}
	if names := s.Names(); len(names) > 0 {
		b.WriteString(`  "names": {`)
		for i, n := range names {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q: %q", n.Name, n.Ref())
		}
		b.WriteString("},\n")
	}
	if err := s.writeView(&b); err != nil {
		return err
	}
	addrs := make([]Addr, 0, len(s.cells))
	for a := range s.cells {
		addrs = append(addrs, a)
	}
	sortAddrs(addrs)
	b.WriteString(`  "cells": {`)
	for i, a := range addrs {
		raw, err := encodeCell(s.cells[a])
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "\n    %q: %s", a.String(), raw)
	}
	if len(addrs) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteString("}")
	if len(s.charts) > 0 {
		b.WriteString(",\n  \"charts\": [")
		for i, c := range s.charts {
			raw, err := encodeChart(c)
			if err != nil {
				return err
			}
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "\n    %s", raw)
		}
		b.WriteString("\n  ]")
	}
	b.WriteString("\n}\n")
	_, err := w.Write(b.Bytes())
	return err
}

// Read loads a worksheet written by Write, of this or an earlier version.
func Read(r io.Reader) (*Sheet, error) {
	var f fileFormat
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, err
	}
	if f.Version < 1 || f.Version > fileVersion {
		return nil, fmt.Errorf("unsupported file version %d", f.Version)
	}
	s := New()
	for name, width := range f.Widths {
		c, ok := ParseCol(name)
		if !ok {
			return nil, fmt.Errorf("invalid column %q", name)
		}
		s.SetColWidth(c, width)
	}
	for name, ref := range f.Names {
		if err := ValidName(name); err != nil {
			return nil, fmt.Errorf("name %q: %w", name, err)
		}
		if _, dup := s.LookupName(name); dup {
			return nil, fmt.Errorf("name %q is defined twice", name)
		}
		n := Name{Name: name, Lost: ref == "#REF!"}
		if !n.Lost {
			r, ok := ParseRange(ref)
			if !ok {
				return nil, fmt.Errorf("name %q: invalid range %q", name, ref)
			}
			n.Range = r
		}
		s.putName(nameKey(name), &n)
	}
	for name, raw := range f.Cells {
		a, ok := ParseAddr(name)
		if !ok {
			return nil, fmt.Errorf("invalid cell %q", name)
		}
		input, fm, st, err := decodeCell(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		// Version 1 had no formats, so there entries imply them, as if
		// typed again; in version 2 the stored format wins.
		c, err := newCell(input, fm, st, f.Version < 2)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		s.place(a, c)
	}
	for i, fc := range f.Charts {
		c, err := decodeChart(fc)
		if err != nil {
			return nil, fmt.Errorf("chart %d: %w", i+1, err)
		}
		s.charts = append(s.charts, c)
	}
	if err := s.readView(f.fileView); err != nil {
		return nil, err
	}
	s.RecalcAll()
	return s, nil
}
