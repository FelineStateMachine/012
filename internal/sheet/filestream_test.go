package sheet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
)

// readWhole reads a file the way encoding/json reads it whole, into
// fileFormat, as the reference the streaming reader is held to. It
// reports ambiguous for a file whose result depends on map order: a
// sheet naming one cell by two keys ("A1" and "a1").
func readWhole(data []byte) (w *Workbook, ambiguous bool, err error) {
	var f fileFormat
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		return nil, false, err
	}
	if f.Version < 1 || f.Version > fileVersion {
		return nil, false, fmt.Errorf("unsupported file version %d", f.Version)
	}
	bodies := f.Sheets
	if f.Version < 4 {
		bodies = []fileSheet{f.fileSheet}
		if bodies[0].Name == "" {
			bodies[0].Name = "Sheet1"
		}
	}
	if len(bodies) == 0 {
		return nil, false, errors.New("the file has no sheets")
	}
	w = emptyBook()
	for _, body := range bodies {
		if err := w.checkName(nil, body.Name); err != nil {
			return nil, false, err
		}
		w.insert(w.newSheet(body.Name), len(w.sheets))
	}
	if err := w.readNames(f.Names); err != nil {
		return nil, false, err
	}
	var old []oldRegion
	for i, body := range bodies {
		s := w.sheets[i]
		amb, err := placeWhole(s, body.Cells, f.Version)
		if err != nil {
			return nil, false, err
		}
		ambiguous = ambiguous || amb
		if err := s.read(body, &old); err != nil {
			return nil, false, err
		}
	}
	w.active = clampInt(f.Active, 0, len(w.sheets)-1)
	w.settleHidden()
	w.decimal = f.Arithmetic == "decimal"
	if l, ok := locale.Lookup(f.Locale); ok {
		w.locale = l
	}
	if err := w.readMacros(f.Macros); err != nil {
		return nil, false, err
	}
	w.macroOrigin = f.MacroOrigin
	w.convertOld(old)
	w.nb.changed = 0
	w.RecalcAll()
	return w, ambiguous, nil
}

// placeWhole places a sheet's cells decoded whole, and reports whether
// two keys name one cell.
func placeWhole(s *Sheet, cells map[string]json.RawMessage, version int) (ambiguous bool, err error) {
	seen := map[Addr]bool{}
	for name, raw := range cells {
		a, ok := ParseAddr(name)
		if !ok {
			return false, fmt.Errorf("invalid cell %q", name)
		}
		ambiguous = ambiguous || seen[a]
		seen[a] = true
		input, fm, st, note, err := decodeNoted(raw)
		if err != nil {
			return false, err
		}
		c, err := newCell(input, fm, st, version < 2)
		if err != nil {
			return false, err
		}
		if c = c.withNote(CleanNote(note)); c != nil {
			s.place(a, c)
		}
	}
	return ambiguous, nil
}

// written is the file w writes.
func written(t testing.TB, w *Workbook) string {
	var b bytes.Buffer
	if err := w.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// sameAsWhole checks that the streaming reader reads data as readWhole
// does: both fail, or both make workbooks that write the same file.
func sameAsWhole(t *testing.T, data []byte) {
	want, ambiguous, werr := readWhole(data)
	got, gerr := ReadBook(bytes.NewReader(data))
	switch {
	case errors.Is(gerr, errTwice):
		// Refused, where decoding the whole file took the last (and for
		// sheets, merged the two lists).
	case gerr != nil && werr == nil && duplicateKeys(data):
		// Streaming reads every entry of a cell given twice, and refuses
		// an invalid one that decoding the whole file overwrote.
	case (werr == nil) != (gerr == nil):
		t.Fatalf("reading %q: streaming %v, whole %v", data, gerr, werr)
	case werr == nil && !ambiguous:
		if g, w := written(t, got), written(t, want); g != w {
			t.Fatalf("reading %q: streaming writes\n%s\nwhole writes\n%s", data, g, w)
		}
	}
}

// duplicateKeys reports whether an object in data gives a key twice.
func duplicateKeys(data []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	type level struct {
		keys  map[string]bool
		isKey bool // the next token in this object is a key
	}
	var stack []*level
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		top := (*level)(nil)
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		switch tok {
		case json.Delim('{'), json.Delim('['):
			if top != nil && top.keys != nil {
				top.isKey = true // the object or array is the value
			}
			l := &level{}
			if tok == json.Delim('{') {
				l.keys, l.isKey = map[string]bool{}, true
			}
			stack = append(stack, l)
			continue
		case json.Delim('}'), json.Delim(']'):
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return false
			}
			continue
		}
		if top == nil || top.keys == nil {
			continue
		}
		if top.isKey {
			k := tok.(string)
			if top.keys[k] {
				return true
			}
			top.keys[k] = true
		}
		top.isKey = !top.isKey
	}
}

// streamSeeds are files to read: what Write makes of sheets holding the
// parser's fuzz seeds and every kind of field, and files written by hand
// with keys out of order, in other cases, escaped and given twice.
func streamSeeds(t testing.TB) [][]byte {
	s := New()
	inputs := []string{"=A1*2", "=SUM(A1:B3)", "=IF(AND(A1>2,B1),1,NA())", `="a"&"b"`, "=1.5E-3^2", "$1,200", "12%", "@SUM(A1..B3)", "=$A$1+A$2*$B3", "=SUM($A1:B$3)", "=#REF!+1", "=A$$1",
		"Rent", "TRUE", "1.50", "'007", "<b>&amp;</b>", "tab\tquote\"back\\slash", "café  ", strings.Repeat("long ", 20000)}
	for i, in := range inputs {
		s.Set(Addr{Col: i % 4, Row: i / 4}, in)
	}
	s.SetFormat(NewRect(Addr{Row: 10}, Addr{Col: 2, Row: 12}), Format{Kind: FmtCurrency, Decimals: 2})
	s.SetStyle(NewRect(Addr{Row: 11}, Addr{Col: 3, Row: 11}), func(st *Style) { st.Bold = true })
	s.SetNote(Addr{Col: 5, Row: 1}, "a note")
	s.SetColWidth(1, 20)
	s.SetStyle(NewRect(Addr{Row: 4}, Addr{Row: 4}), func(st *Style) { st.Wrap = WrapOn })
	s.SetStyle(NewRect(Addr{Col: 4}, Addr{Col: 4, Row: MaxRows - 1}), func(st *Style) { st.Wrap = WrapClip })
	s.SetBorders(NewRect(Addr{Row: 1}, Addr{Col: 2, Row: 3}), BorderAll, LineThin)
	s.SetBorders(NewRect(Addr{Row: 1}, Addr{Col: 2, Row: 3}), BorderOuter, LineDouble)
	s.SetBorderStroke(NewRect(Addr{Col: 3, Row: 1}, Addr{Col: 3, Row: 2}), BorderRight, Stroke{Line: LineThick, Color: ColorMagenta})
	s.SetStyle(NewRect(Addr{Row: 6}, Addr{Col: 1, Row: 6}), func(st *Style) { st.VAlign = VAlignMiddle })
	s.SetRowHeight(6, 7, 3)
	s.LoadMerge(NewRect(Addr{Col: 6}, Addr{Col: 8, Row: 1}))
	one := []byte(written(t, s.Book()))
	if _, err := s.Book().AddSheet("Other", 1); err != nil {
		t.Fatal(err)
	}
	s.Book().Sheet(1).Set(Addr{}, "=SUM(Sheet1!B1:B5)")
	two := []byte(written(t, s.Book()))
	// Many cells, so keys and values straddle the reader's buffer.
	big := New()
	for r := range 20000 {
		big.Load(Addr{Row: r}, fmt.Sprintf("%d.%02d", r, r%100), Format{}, Style{})
		big.Load(Addr{Col: 1, Row: r}, fmt.Sprintf("text %d", r%300), Format{}, Style{Bold: r%7 == 0})
	}
	return [][]byte{one, two, []byte(written(t, big.Book())),
		[]byte(`{"version": 1, "widths": {"A": 14}, "cells": {"A1": "Rent", "B1": "$1,450", "B2": "=B1*12"}}`),
		[]byte(`{"cells": {"A1": "5", "B1": "=A1+1"}, "version": 2}`),
		[]byte(`{"Cells": {"A1": "5"}, "VERSION": 3, "cells": {"A1": "6", "A2": {"input": "7", "bold": true}}}`),
		[]byte(`{"sheets": [{"cells": {"A1": "1"}, "name": "S"}], "version": 4}`),
		[]byte(`{"version": 4, "sheets": [{"name": "A", "cells": {"A1": "1"}}, null], "sheets": [{"name": "B", "cells": {"B2": "A\n"}}]}`),
		[]byte(`{"version": 2, "cells": {"A1": "x", "a1": "y"}}`),
		[]byte(`{"version": 6, "sheets": [{"name": "S", "tables": [{"name":"Sales","range":"A1:B3","columns":["Item","Amount"],"banded":true}], "cells": {"A1": "Item", "B1": "Amount", "B2": "5", "C1": "=SUM(Sales[Amount])+Sales[[#Headers],[Item]]"}}]}`),
		[]byte(`{"version": 2, "cells": {"A1": "=(", "A1": "1", "B1": "2", "B1": ""}}`),
		[]byte(`{"version": 2, "cells": null, "sheets": 5}`),
		[]byte(`{"version": 4, "cells": "x", "sheets": []}`),
		[]byte(`{"version": 2, "cells": {"A1": "😀", "A2": "bad \xff", "ZZ9": 5}}`),
		[]byte(`{"version": 2, "cells": {"A1": "1"}} trailing`),
		[]byte(`{"version": 2, "heights": {"2": 3, "9": 99}, "merges": ["A1:B1", "B1:C2", "D4"], "cells": {"A1": {"input": "5", "wrap": "wrap", "borders": {"top": "thin", "right": "double"}}}}`),
		[]byte(`{"version": 2, "cells": {"A1": {"input": "x", "wrap": "sideways"}, "B1": {"borders": {"left": "dotted"}}}}`),
		[]byte(`{"version": 2, "cells": {"A1": {"input": "5", "valign": "top", "borders": {"top": "thick", "topColor": "red", "left": "thin", "leftColor": "blue"}}, "B1": {"borders": {"rightColor": "green"}}}}`),
		[]byte(`{"version": 2, "cells": {"A1": {"input": "x", "valign": "sideways"}, "B1": {"borders": {"left": "thin", "leftColor": "mauve"}}}}`),
		[]byte(`{"version": 2, "cells": {"A1": "5"}, "conditionalFormats": [
			{"ranges":"A1:A9","dataBar":{"color":"blue","min":{"type":"min"},"max":{"type":"percentile","value":"90"},"barOnly":true}},
			{"ranges":"B1:B9","iconSet":{"icons":"circles","points":[{"type":"percent","value":"20"},{"type":"percent","value":"40"},{"type":"num","value":"6"},{"type":"percent","value":"80"}],"reverse":true}},
			{"ranges":"C1:C9","condition":"top_percent","values":["10"],"fill":"green"},
			{"ranges":"D1:D9","condition":"date_is","values":["next month"],"bold":true}],
			"validations": [{"ranges":"E1:E9","criteria":"checkbox","items":["Yes","No"]}, {"ranges":"F1:F9","criteria":"list","items":["a"],"display":"chip"}]}`),
		[]byte(`{"version": 2, "conditionalFormats": [{"ranges":"A1","dataBar":{"color":"none","min":{"type":"min"},"max":{"type":"max"}}}]}`),
		[]byte(`{"version": 2, "conditionalFormats": [{"ranges":"A1","iconSet":{"icons":"symbols","points":[{"type":"percent","value":"50"}]}}]}`),
		[]byte(`{"version": 2, "validations": [{"ranges":"A1","criteria":"list","items":["a"],"display":"bubbles"}]}`),
		[]byte(`{"version": 2, "heights": {"0": 2}, "merges": ["A1:"], "cells": {}}`),
		[]byte(`{"version": 2, "macroOrigin": "m", "cells": {"C1": "x"}, "regions": [{"name": "app", "at": "A1", "path": "logs/app.csv", "window": 3}, {"name": "t", "at": "E5", "path": "/tmp/t.db", "format": "SQLite", "query": "select 1"}]}`),
		[]byte(`{"version": 4, "sheets": [{"name": "Log", "cells": {}, "regions": [{"name": "a", "at": "B2", "path": "a.nuon"}]}, {"name": "S", "cells": {"A1": "=SUM(Log!B:B)+SUM(nu.a)"}}]}`),
		[]byte(`{"version": 2, "cells": {}, "regions": [{"name": "a", "at": "A1", "path": "a", "command": "ls"}, {"name": "b", "at": "A3", "path": "b", "window": -2}]}`),
		[]byte(`{"version": 2, "macroOrigin": "m1", "shellHistory": ["ls", "$r1 | first"], "notebook": true, "cells": {"A3": {"bold": true}}, "regions": [
			{"name": "r1", "command": "ls", "at": "A1", "rows": 3, "cols": 2},
			{"name": "big", "command": "$r1 | where size > 1kb", "at": "A6", "rows": 2, "cols": 2, "reads": ["r1"], "input": "Sheet1!D1:E4", "sort": [{"column": 2, "desc": true}]}]}`),
		[]byte(`{"version": 2, "regions": [{"name": "r1", "command": "ls", "at": "A1"}, {"name": "R1", "command": "x", "at": "A5"}]}`),
		[]byte(`{"version": 2, "regions": [{"name": "in", "command": "ls", "at": "A1"}, {"name": "r2", "at": "ZZZZ1", "rows": -1}]}`),
		[]byte(`{"version": 2, "regions": [{"name": "a", "command": "$b", "at": "A1", "reads": ["b"]}, {"name": "b", "command": "$a", "at": "A3", "reads": ["a"], "sort": [{"column": 0}]}]}`),
		[]byte(`{"version": 4, "sheets": [{"name": "S", "cells": {}, "regions": [{"name": "files", "at": "B2", "output": true}]},
			{"name": "Notebook", "tab": "notebook", "reactive": true, "cells": {}, "notebookCells": [
			{"kind": "note", "source": "# Files"}, {"source": "files = ls", "output": "[[name, size]; [a, 2kb]]"},
			{"source": "$files | nope", "error": "Command not found", "detail": "help: ls", "note": "x"}, {"source": "ls **/*", "unsaved": true}]}]}`),
		[]byte(`{"version": 4, "sheets": [{"name": "N", "tab": "notebook", "cells": {}, "notebookCells": [{"kind": "table", "source": "x"}]}]}`),
		[]byte(`{"version": 4, "sheets": [{"name": "N", "cells": {}, "notebookCells": [{"source": "x"}]}]}`),
		[]byte(`{"version": 2, "cells": {}, "regions": [{"name": "o", "at": "A1", "output": true, "command": "ls"}, {"name": "p", "at": "A1", "output": true, "path": "p"}]}`),
		[]byte(`{"version": 4, "sheets": [{"name": "Notebook", "tab": "notebook", "cells": {}}, {"name": "Shell 1", "notebook": true, "cells": {}, "regions": [{"name": "r1", "command": "ls", "at": "A1"}]}]}`),
		[]byte(`null`), []byte(`[]`), []byte(`{"version": 2, "cells": {"A1": "1",}}`), []byte(`{"version": 2, "cells": {"A1"`),
	}
}

// The streaming reader reads every seed as encoding/json reading the
// whole file did.
func TestStreamReadsAsWhole(t *testing.T) {
	for _, data := range streamSeeds(t) {
		sameAsWhole(t, data)
	}
}

func FuzzRead(f *testing.F) {
	for _, data := range streamSeeds(f) {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		sameAsWhole(t, data)
	})
}

// Cells are written as encodeCell writes each, in row-major order, one
// per line.
func TestWriteCellsAsEncoded(t *testing.T) {
	data := streamSeeds(t)[0]
	s, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var addrs []Addr
	for a := range s.cells.keys() {
		addrs = append(addrs, a)
	}
	sortAddrs(addrs)
	var want strings.Builder
	want.WriteString(`"cells": {`)
	for i, a := range addrs {
		raw, err := encodeCell(s.cells.get(a).saved())
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			want.WriteByte(',')
		}
		fmt.Fprintf(&want, "\n    %q: %s", a.String(), raw)
	}
	want.WriteString("\n  }")
	if got := written(t, s.Book()); !strings.Contains(got, want.String()) || string(data) != got {
		t.Errorf("cells written as\n%s\nwant\n%s", got, want.String())
	}
	for _, a := range []Addr{{}, {Col: 25, Row: 9}, {Col: 26}, {Col: 701, Row: 99}, {Col: 702}, {Col: MaxCols - 1, Row: MaxRows - 1}} {
		if got := string(appendAddr(nil, a)); got != a.String() {
			t.Errorf("appendAddr %q, want %q", got, a.String())
		}
	}
}
