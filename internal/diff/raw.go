package diff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A .012 file is JSON whose sheets hold their cells as a map by address
// and their other fields (widths, rules, regions) beside them; see
// docs/files/format.md. Diffing and merging read it as that JSON, field
// by field, so every field is compared and kept, including ones this
// package knows nothing about, and the workbook the fields make is read
// with the engine only for the values formulas compute.

// rawBook is a file's fields: the workbook's, and each sheet's.
type rawBook struct {
	top    map[string]json.RawMessage // the workbook's fields, compact, without "sheets"
	sheets []*rawSheet
}

// rawSheet is one sheet's fields, compact.
type rawSheet struct {
	name   string
	fields map[string]json.RawMessage // everything but name and cells
	cells  map[string]json.RawMessage // by address
}

// bookKeys are the workbook's fields; in a file of one sheet (versions
// 1 to 3) every other top-level field is the sheet's.
var bookKeys = map[string]bool{"version": true, "names": true, "active": true, "arithmetic": true,
	"locale": true, "macroOrigin": true, "shellHistory": true, "macros": true, "sheets": true}

// parseRaw reads a file's fields. An empty file (git's stand-in for a
// file that isn't there) is a workbook without sheets.
func parseRaw(data []byte) (*rawBook, error) {
	b := &rawBook{top: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(data)) == 0 {
		return b, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	if raw, ok := top["sheets"]; ok {
		var sheets []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &sheets); err != nil {
			return nil, fmt.Errorf("sheets: %w", err)
		}
		for _, fields := range sheets {
			s, err := parseSheet(fields)
			if err != nil {
				return nil, err
			}
			b.sheets = append(b.sheets, s)
		}
	} else {
		fields := map[string]json.RawMessage{}
		for k, v := range top {
			if !bookKeys[k] {
				fields[k] = v
			}
		}
		s, err := parseSheet(fields)
		if err != nil {
			return nil, err
		}
		if s.name == "" {
			s.name = "Sheet1"
		}
		b.sheets = []*rawSheet{s}
		if err := qualifyNames(top, s.name); err != nil {
			return nil, err
		}
	}
	for k, v := range top {
		if bookKeys[k] && k != "sheets" {
			b.top[k] = compact(v)
		}
	}
	return b, nil
}

// parseSheet splits a sheet's fields into its name, cells and the rest.
func parseSheet(fields map[string]json.RawMessage) (*rawSheet, error) {
	s := &rawSheet{fields: map[string]json.RawMessage{}, cells: map[string]json.RawMessage{}}
	if raw, ok := fields["name"]; ok {
		if err := json.Unmarshal(raw, &s.name); err != nil {
			return nil, fmt.Errorf("sheet name: %w", err)
		}
	}
	if raw, ok := fields["cells"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &s.cells); err != nil {
			return nil, fmt.Errorf("sheet %s: cells: %w", s.name, err)
		}
	}
	for k, v := range s.cells {
		s.cells[k] = compact(v)
	}
	for k, v := range fields {
		if k != "name" && k != "cells" {
			s.fields[k] = compact(v)
		}
	}
	return s, nil
}

// qualifyNames writes a one-sheet file's named ranges with their sheet,
// as files of several sheets have them, so files of both shapes compare.
func qualifyNames(top map[string]json.RawMessage, sheetName string) error {
	raw, ok := top["names"]
	if !ok {
		return nil
	}
	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return fmt.Errorf("names: %w", err)
	}
	for k, ref := range names {
		if ref != "#REF!" && !strings.Contains(ref, "!") {
			names[k] = sheet.QuoteSheet(sheetName) + "!" + ref
		}
	}
	top["names"], _ = json.Marshal(names)
	return nil
}

// compact is raw without insignificant space, so equal values compare
// equal byte for byte.
func compact(raw json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return raw
	}
	return b.Bytes()
}

// marshal writes the fields as a file of several sheets, which the
// engine reads whatever the count; the engine's own writer then gives
// the file its usual shape.
func (b *rawBook) marshal() []byte {
	var out bytes.Buffer
	out.WriteString("{\n")
	version := json.RawMessage("4")
	if n, err := strconv.Atoi(string(b.top["version"])); err == nil && n > 4 {
		version = b.top["version"]
	}
	fmt.Fprintf(&out, "%q: %s", "version", version)
	for _, k := range slices.Sorted(maps.Keys(b.top)) {
		if k != "version" {
			fmt.Fprintf(&out, ",\n%q: %s", k, b.top[k])
		}
	}
	out.WriteString(",\n\"sheets\": [")
	for i, s := range b.sheets {
		if i > 0 {
			out.WriteByte(',')
		}
		name, _ := json.Marshal(s.name)
		fmt.Fprintf(&out, "\n{\"name\": %s", name)
		for _, k := range slices.Sorted(maps.Keys(s.fields)) {
			fmt.Fprintf(&out, ",\n%q: %s", k, s.fields[k])
		}
		out.WriteString(",\n\"cells\": {")
		for j, k := range slices.Sorted(maps.Keys(s.cells)) {
			if j > 0 {
				out.WriteByte(',')
			}
			fmt.Fprintf(&out, "\n%q: %s", k, s.cells[k])
		}
		out.WriteString("}}")
	}
	out.WriteString("]\n}\n")
	return out.Bytes()
}

// normalize reads merged fields with the engine and writes them as 012
// saves a workbook, checking that they make one.
func normalize(b *rawBook) ([]byte, error) {
	w, err := sheet.ReadBook(bytes.NewReader(b.marshal()))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := w.Write(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// equal reports whether two compact values are the same; nil is a
// field or cell that isn't there.
func equal(a, b json.RawMessage) bool {
	return (a == nil) == (b == nil) && bytes.Equal(a, b)
}

// sheetIndex finds a sheet by name, ignoring case as the engine does.
func (b *rawBook) sheetIndex(name string) int {
	for i, s := range b.sheets {
		if strings.EqualFold(s.name, name) {
			return i
		}
	}
	return -1
}
