package sheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// FileExt is the extension of the native worksheet format.
const FileExt = ".012"

// fileVersion is the newest version Write produces and Read accepts.
// Version 1 files (cells as plain strings, no formatting) still load.
// Version 3 adds named ranges, frozen panes and a filter; a sheet using
// none of them is written as version 2, so earlier builds can open it.
// Version 4 holds several sheets; a workbook of one sheet whose formulas
// name no sheet is still written as version 2 or 3, its sheet's name in
// a "name" field that earlier builds ignore.
const fileVersion = 4

// The file is JSON with one entry per cell, keyed by address. A cell
// without formatting is just its input, as in version 1; a formatted cell
// is an object, so diffs stay one line per cell:
//
//	"B2": "Rent",
//	"C2": {"input": "1450", "format": "currency", "decimals": 2, "bold": true}
//
// Named ranges map each name to its range, or to "#REF!" once its cells
// were deleted: "names": {"Sales": "B2:B20"}. In version 4 the sheets are
// a list, each with its name and the fields a version 3 file has at the
// top, and ranges of names say their sheet: {"Sales": "Q3!B2:B20"}.
//
//	"sheets": [
//	  {
//	    "name": "Q3",
//	    "cells": {
//	      "A1": "Rent",
type fileFormat struct {
	Version int               `json:"version"`
	Names   map[string]string `json:"names,omitempty"`
	Active  int               `json:"active,omitempty"` // index of the sheet shown
	// Arithmetic is "decimal" for decimal arithmetic, which needs no
	// version bump: earlier builds ignore it and compute in binary, as
	// Sheets would. It is for the whole workbook, so it stays at the top.
	Arithmetic string      `json:"arithmetic,omitempty"`
	fileSheet              // versions 1 to 3: the only sheet
	Sheets     []fileSheet `json:"sheets,omitempty"` // version 4
}

// fileSheet is one sheet of a file.
type fileSheet struct {
	Name   string         `json:"name,omitempty"`
	Widths map[string]int `json:"widths,omitempty"`
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

// Write saves the workbook the sheet belongs to; see Workbook.Write.
func (s *Sheet) Write(w io.Writer) error { return s.wb.Write(w) }

// Write saves the workbook as JSON, storing each cell's input as typed
// and its formatting. Cells go one per line in row-major order so diffs
// read naturally.
func (w *Workbook) Write(out io.Writer) error {
	var b bytes.Buffer
	if w.single() {
		s := w.sheets[0]
		version := 2
		if v := s.view; len(w.names) > 0 || v.frozenRows > 0 || v.frozenCols > 0 || v.filter != nil {
			version = 3
		}
		fmt.Fprintf(&b, "{\n  \"version\": %d,\n", version)
		if s.name != "Sheet1" {
			fmt.Fprintf(&b, "  \"name\": %s,\n", jsonString(s.name))
		}
		if err := s.writeBody(&b, "  ", w.headLines()); err != nil {
			return err
		}
		b.WriteString("\n}\n")
	} else {
		fmt.Fprintf(&b, "{\n  \"version\": %d,\n", fileVersion)
		if head := w.headLines(); head != "" {
			b.WriteString("  " + head + ",\n")
		}
		if w.Active() > 0 {
			fmt.Fprintf(&b, "  \"active\": %d,\n", w.Active())
		}
		b.WriteString(`  "sheets": [`)
		for i, s := range w.sheets {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "\n    {\n      \"name\": %s,\n", jsonString(s.name))
			if err := s.writeBody(&b, "      ", ""); err != nil {
				return err
			}
			b.WriteString("\n    }")
		}
		b.WriteString("\n  ]\n}\n")
	}
	_, err := out.Write(b.Bytes())
	return err
}

// single reports whether the workbook fits the single-sheet format of
// versions 2 and 3: one sheet, and no formula naming a sheet.
func (w *Workbook) single() bool {
	return len(w.sheets) == 1 && len(w.crossUsers) == 0
}

func jsonString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// headLines are the workbook's fields before the sheets: the named ranges
// and the arithmetic setting, separated as the lines of the file.
func (w *Workbook) headLines() string {
	var lines []string
	if names := w.namesLine(); names != "" {
		lines = append(lines, names)
	}
	if w.decimal {
		lines = append(lines, `"arithmetic": "decimal"`)
	}
	return strings.Join(lines, ",\n  ")
}

// namesLine is the "names" field, or "" without names.
func (w *Workbook) namesLine() string {
	names := w.Names()
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`"names": {`)
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q: %s", n.Name, jsonString(n.Ref()))
	}
	b.WriteString("}")
	return b.String()
}

// writeBody writes a sheet's fields, each line starting with indent, the
// names line (if any) after the widths as version 3 has it, and no
// newline after the last field.
func (s *Sheet) writeBody(b *bytes.Buffer, indent, names string) error {
	if len(s.widths) > 0 {
		b.WriteString(indent + `"widths": {`)
		for i, c := range slices.Sorted(maps.Keys(s.widths)) {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "%q: %d", ColName(c), s.widths[c])
		}
		b.WriteString("},\n")
	}
	if names != "" {
		b.WriteString(indent + names + ",\n")
	}
	if err := s.writeView(b, indent); err != nil {
		return err
	}
	addrs := make([]Addr, 0, s.cells.len())
	for a := range s.cells.all() {
		addrs = append(addrs, a)
	}
	sortAddrs(addrs)
	b.WriteString(indent + `"cells": {`)
	for i, a := range addrs {
		raw, err := encodeCell(s.cells.get(a))
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(b, "\n%s  %q: %s", indent, a.String(), raw)
	}
	if len(addrs) > 0 {
		b.WriteString("\n" + indent)
	}
	b.WriteString("}")
	if len(s.charts) > 0 {
		b.WriteString(",\n" + indent + `"charts": [`)
		for i, c := range s.charts {
			raw, err := encodeChart(c)
			if err != nil {
				return err
			}
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, "\n%s  %s", indent, raw)
		}
		b.WriteString("\n" + indent + "]")
	}
	return nil
}

// Read loads a file written by Write, of this or an earlier version, and
// returns the sheet that was shown when it was saved.
func Read(r io.Reader) (*Sheet, error) {
	w, err := ReadBook(r)
	if err != nil {
		return nil, err
	}
	return w.Sheet(w.Active()), nil
}

// ReadBook loads a workbook written by Write, of this or an earlier
// version.
func ReadBook(r io.Reader) (*Workbook, error) {
	var f fileFormat
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, err
	}
	if f.Version < 1 || f.Version > fileVersion {
		return nil, fmt.Errorf("unsupported file version %d", f.Version)
	}
	bodies := f.Sheets
	if f.Version < 4 {
		bodies = []fileSheet{f.fileSheet}
		if bodies[0].Name == "" {
			bodies[0].Name = "Sheet1"
		}
	}
	if len(bodies) == 0 {
		return nil, fmt.Errorf("the file has no sheets")
	}
	w := emptyBook()
	for _, body := range bodies {
		if err := w.checkName(nil, body.Name); err != nil {
			return nil, fmt.Errorf("sheet %q: %w", body.Name, err)
		}
		w.insert(w.newSheet(body.Name), len(w.sheets))
	}
	if err := w.readNames(f.Names); err != nil {
		return nil, err
	}
	for i, body := range bodies {
		if err := w.sheets[i].read(body, f.Version); err != nil {
			if len(bodies) > 1 {
				err = fmt.Errorf("sheet %s: %w", body.Name, err)
			}
			return nil, err
		}
	}
	w.active = clampInt(f.Active, 0, len(w.sheets)-1)
	// Anything but "decimal" (say, a mode from a later build) computes in
	// binary, as the file would in a build without the setting.
	w.decimal = f.Arithmetic == "decimal"
	w.RecalcAll()
	return w, nil
}

// readNames defines the named ranges of a file. A range without a sheet
// is on the first sheet, as in single-sheet files.
func (w *Workbook) readNames(names map[string]string) error {
	for name, ref := range names {
		if err := ValidName(name); err != nil {
			return fmt.Errorf("name %q: %w", name, err)
		}
		if _, dup := w.LookupName(name); dup {
			return fmt.Errorf("name %q is defined twice", name)
		}
		n := Name{Name: name, Sheet: w.sheets[0], Lost: ref == "#REF!"}
		if !n.Lost {
			sheet, rest := SplitSheet(ref)
			if sheet != "" {
				if n.Sheet = w.Lookup(sheet); n.Sheet == nil {
					return fmt.Errorf("name %q: no sheet %q", name, sheet)
				}
			}
			r, ok := ParseRange(rest)
			if !ok {
				return fmt.Errorf("name %q: invalid range %q", name, ref)
			}
			n.Range = r
		}
		w.putName(nameKey(name), &n)
	}
	return nil
}

// read fills an empty sheet from its part of a file.
func (s *Sheet) read(f fileSheet, version int) error {
	for name, width := range f.Widths {
		c, ok := ParseCol(name)
		if !ok {
			return fmt.Errorf("invalid column %q", name)
		}
		s.setWidth(c, width)
	}
	for name, raw := range f.Cells {
		a, ok := ParseAddr(name)
		if !ok {
			return fmt.Errorf("invalid cell %q", name)
		}
		input, fm, st, err := decodeCell(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		// Version 1 had no formats, so there entries imply them, as if
		// typed again; since version 2 the stored format wins.
		c, err := newCell(input, fm, st, version < 2)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		s.place(a, c)
	}
	for i, fc := range f.Charts {
		c, err := decodeChart(fc)
		if err != nil {
			return fmt.Errorf("chart %d: %w", i+1, err)
		}
		s.charts = append(s.charts, c)
	}
	return s.readView(f.fileView)
}
