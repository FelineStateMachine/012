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
// Version 5 is version 4 with pivot tables; a workbook without one is
// still written as version 4.
const fileVersion = 5

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
	Arithmetic string `json:"arithmetic,omitempty"`
	// Macros and where they were made need no version bump either:
	// earlier builds ignore them, and the sheets read the same.
	MacroOrigin string      `json:"macroOrigin,omitempty"`
	Macros      []fileMacro `json:"macros,omitempty"`
	fileSheet               // versions 1 to 3: the only sheet
	Sheets      []fileSheet `json:"sheets,omitempty"` // version 4
}

// fileSheet is one sheet of a file.
type fileSheet struct {
	Name   string                     `json:"name,omitempty"`
	Hidden bool                       `json:"hidden,omitempty"` // version 4, ignored by older builds
	Widths map[string]int             `json:"widths,omitempty"`
	Lines  map[string]json.RawMessage `json:"lines,omitempty"` // column and row formats, see linefile.go
	fileView
	Cells  map[string]json.RawMessage `json:"cells"`
	Charts []fileChart                `json:"charts,omitempty"`
	Pivot  *filePivot                 `json:"pivot,omitempty"` // version 5
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
		version := 4
		if w.hasPivots() {
			version = fileVersion
		}
		fmt.Fprintf(&b, "{\n  \"version\": %d,\n", version)
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
			if s.tabHidden {
				b.WriteString("      \"hidden\": true,\n")
			}
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
	return len(w.sheets) == 1 && len(w.crossUsers) == 0 && !w.hasPivots()
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
	if len(w.macros) > 0 {
		if w.macroOrigin != "" {
			lines = append(lines, `"macroOrigin": `+jsonString(w.macroOrigin))
		}
		lines = append(lines, w.macrosLines())
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
	if err := s.writeLines(b, indent); err != nil {
		return err
	}
	if names != "" {
		b.WriteString(indent + names + ",\n")
	}
	if err := s.writeView(b, indent); err != nil {
		return err
	}
	addrs := make([]Addr, 0, s.cells.len())
	for a, c := range s.cells.all() {
		if c.saved() != nil {
			addrs = append(addrs, a)
		}
	}
	sortAddrs(addrs)
	b.WriteString(indent + `"cells": {`)
	for i, a := range addrs {
		raw, err := encodeCell(s.cells.get(a).saved())
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
	return s.writeObjects(b, indent)
}

// writeObjects writes the fields after the cells: the charts, one per
// line, and the pivot table's definition.
func (s *Sheet) writeObjects(b *bytes.Buffer, indent string) error {
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
	if p := s.pivot.def; p != nil {
		raw, err := encodePivot(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, ",\n%s\"pivot\": %s", indent, raw)
	}
	return nil
}

// Read loads a file written by Write, of this or an earlier version, and
// returns the sheet that was shown when it was saved.
func Read(r io.Reader) (*Sheet, error) { return ReadTraced(r, nil) }

// ReadTraced is Read giving the workbook trace (see SetTrace) before it
// recalculates, so that recalculation nests in the caller's span.
func ReadTraced(r io.Reader, trace any) (*Sheet, error) {
	w, err := readBook(r, trace)
	if err != nil {
		return nil, err
	}
	return w.Sheet(w.Active()), nil
}

// ReadBook loads a workbook written by Write, of this or an earlier
// version.
func ReadBook(r io.Reader) (*Workbook, error) { return readBook(r, nil) }

func readBook(r io.Reader, trace any) (*Workbook, error) {
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
	w.trace = trace
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
	w.settleHidden()
	// Anything but "decimal" (say, a mode from a later build) computes in
	// binary, as the file would in a build without the setting.
	w.decimal = f.Arithmetic == "decimal"
	if err := w.readMacros(f.Macros); err != nil {
		return nil, err
	}
	w.macroOrigin = f.MacroOrigin
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
	s.tabHidden = f.Hidden
	for name, width := range f.Widths {
		c, ok := ParseCol(name)
		if !ok {
			return fmt.Errorf("invalid column %q", name)
		}
		s.setWidth(c, width)
	}
	if err := s.readLines(f.Lines); err != nil {
		return err
	}
	for name, raw := range f.Cells {
		a, ok := ParseAddr(name)
		if !ok {
			return fmt.Errorf("invalid cell %q", name)
		}
		input, fm, st, note, err := decodeNoted(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		// Version 1 had no formats, so there entries imply them, as if
		// typed again; since version 2 the stored format wins.
		c, err := newCell(input, fm, st, version < 2)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if c = c.withNote(CleanNote(note)); c != nil {
			s.place(a, c)
		}
	}
	for i, fc := range f.Charts {
		c, err := decodeChart(fc)
		if err != nil {
			return fmt.Errorf("chart %d: %w", i+1, err)
		}
		s.charts = append(s.charts, c)
	}
	if f.Pivot != nil {
		p, err := decodePivot(*f.Pivot)
		if err != nil {
			return err
		}
		s.pivot = pivotState{def: p, stale: true}
	}
	return s.readView(f.fileView)
}
