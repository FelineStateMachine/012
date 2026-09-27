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
	for i, body := range bodies {
		s := w.sheets[i]
		amb, err := placeWhole(s, body.Cells, f.Version)
		if err != nil {
			return nil, false, err
		}
		ambiguous = ambiguous || amb
		if err := s.read(body); err != nil {
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
	case (werr == nil) != (gerr == nil):
		t.Fatalf("reading %q: streaming %v, whole %v", data, gerr, werr)
	case werr == nil && !ambiguous:
		if g, w := written(t, got), written(t, want); g != w {
			t.Fatalf("reading %q: streaming writes\n%s\nwhole writes\n%s", data, g, w)
		}
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
	one := []byte(written(t, s.Book()))
	if _, err := s.Book().AddSheet("Other", 1); err != nil {
		t.Fatal(err)
	}
	s.Book().Sheet(1).Set(Addr{}, "=SUM(Sheet1!B1:B5)")
	two := []byte(written(t, s.Book()))
	return [][]byte{one, two,
		[]byte(`{"version": 1, "widths": {"A": 14}, "cells": {"A1": "Rent", "B1": "$1,450", "B2": "=B1*12"}}`),
		[]byte(`{"cells": {"A1": "5", "B1": "=A1+1"}, "version": 2}`),
		[]byte(`{"Cells": {"A1": "5"}, "VERSION": 3, "cells": {"A1": "6", "A2": {"input": "7", "bold": true}}}`),
		[]byte(`{"sheets": [{"cells": {"A1": "1"}, "name": "S"}], "version": 4}`),
		[]byte(`{"version": 4, "sheets": [{"name": "A", "cells": {"A1": "1"}}, null], "sheets": [{"name": "B", "cells": {"B2": "A\n"}}]}`),
		[]byte(`{"version": 2, "cells": {"A1": "x", "a1": "y"}}`),
		[]byte(`{"version": 2, "cells": null, "sheets": 5}`),
		[]byte(`{"version": 4, "cells": "x", "sheets": []}`),
		[]byte(`{"version": 2, "cells": {"A1": "😀", "A2": "bad \xff", "ZZ9": 5}}`),
		[]byte(`{"version": 2, "cells": {"A1": "1"}} trailing`),
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
