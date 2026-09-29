package diff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

func addr(s string) sheet.Addr {
	a, _ := sheet.ParseAddr(s)
	return a
}

// file writes a workbook of sheets, each a map of cells to inputs, after
// edit has its way with it.
func file(t *testing.T, sheets []string, cells []map[string]string, edit func(w *sheet.Workbook)) []byte {
	t.Helper()
	w := sheet.NewBook()
	for i, name := range sheets {
		s := w.Sheet(0)
		if i == 0 {
			if err := w.RenameSheet(s, name); err != nil {
				t.Fatal(err)
			}
		} else {
			var err error
			if s, err = w.AddSheet(name, i); err != nil {
				t.Fatal(err)
			}
		}
		for a, in := range cells[i] {
			if err := s.Set(addr(a), in); err != nil {
				t.Fatal(err)
			}
		}
	}
	if edit != nil {
		edit(w)
	}
	var b bytes.Buffer
	if err := w.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func read(t *testing.T, data []byte) *Book {
	t.Helper()
	b, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// lines is the text output of comparing a with b.
func lines(t *testing.T, a, b []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := WriteChanges(&out, Compare(read(t, a), read(t, b)), "text", false); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestCompareCells(t *testing.T) {
	a := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "Item", "B1": "5", "B2": "=B1*2", "C1": "gone", "D1": "=NOW()", "D2": "=D1+1"}}, nil)
	b := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "Item", "B1": "6", "B2": "=B1*2", "C2": "new", "D1": "=NOW()", "D2": "=D1+1"}}, func(w *sheet.Workbook) {
		s := w.Sheet(0)
		s.SetStyle(sheet.NewRect(addr("A1"), addr("A1")), func(st *sheet.Style) { st.Bold = true })
		s.SetNote(addr("A1"), "the thing")
		s.SetColWidth(2, 14)
	})
	want := `Sheet1!A1  format  + bold
Sheet1!A1  note  + the thing
Sheet1!B1  input  5 → 6
Sheet1!C1  input  - gone
Sheet1!B2  value  10 → 12
Sheet1!C2  input  + new
Sheet1 widths  C  + 14
`
	if got := lines(t, a, b); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := lines(t, a, a); got != "" {
		t.Errorf("a workbook against itself:\n%s", got)
	}
}

func TestCompareSheets(t *testing.T) {
	cells := map[string]string{"A1": "a", "A2": "b", "A3": "c", "A4": "d"}
	edited := map[string]string{"A1": "a", "A2": "b", "A3": "c", "A4": "D"}
	a := file(t, []string{"One", "Two", "Three", "Gone"}, []map[string]string{{"A1": "1"}, cells, {"A1": "3"}, {"A1": "x"}}, nil)
	b := file(t, []string{"Three", "One", "Renamed", "Added"}, []map[string]string{{"A1": "3"}, {"A1": "1"}, edited, {"A1": "y"}}, nil)
	got := lines(t, a, b)
	for _, want := range []string{
		"sheet Gone  removed\n",
		"sheet Added  added\n",
		"sheet Renamed  renamed from Two\n",
		"sheet Three  moved from 3 to 1\n",
		"Renamed!A4  input  d → D\n",
		"Gone!A1  input  - x\n",
		"Added!A1  input  + y\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sheet One  moved") {
		t.Errorf("more moves than needed:\n%s", got)
	}
}

func TestCompareShapes(t *testing.T) {
	// One sheet written as a version 2 file and as a version 4 file is
	// the same workbook.
	v2 := []byte(`{"version": 2, "names": {"Sales": "A1:A2"}, "cells": {"A1": "1", "A2": "=A1+1"}}`)
	v4 := []byte(`{"version": 4, "names": {"Sales": "Sheet1!A1:A2"}, "sheets": [{"name": "Sheet1", "cells": {"A1": "1", "A2": "=A1 + 1"}}]}`)
	got := lines(t, v2, v4)
	if got != "Sheet1!A2  input  =A1+1 → =A1 + 1\n" {
		t.Errorf("got\n%s", got)
	}
	empty := lines(t, nil, v2)
	if !strings.HasPrefix(empty, "sheet Sheet1  added\n") || !strings.Contains(empty, "name Sales  added  + Sheet1!A1:A2") {
		t.Errorf("from nothing:\n%s", empty)
	}
}

func TestCompareRegionsAndBook(t *testing.T) {
	a := []byte(`{"version": 4, "locale": "en-US", "macros": [{"name": "m1", "api": 1, "source": "x"}], "sheets": [{"name": "S", "regions": [{"name": "app", "at": "A1", "path": "app.csv"}, {"name": "old", "at": "D1", "path": "old.csv"}], "cells": {}}]}`)
	b := []byte(`{"version": 4, "locale": "de-DE", "active": 0, "sheets": [{"name": "S", "regions": [{"name": "app", "at": "A1", "path": "app.log"}, {"name": "sales", "at": "H1", "output": true}], "cells": {}}]}`)
	want := `S region app  path  app.csv → app.log
S region old  removed  - old.csv
S region sales  added  + a notebook cell's output
macro m1  removed  - x
workbook locale  en-US → de-DE
`
	if got := lines(t, a, b); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCompareTables(t *testing.T) {
	cells := []map[string]string{{"A1": "Item", "B1": "Cost", "A2": "Rent", "B2": "5"}}
	a := file(t, []string{"S"}, cells, func(w *sheet.Workbook) {
		w.Sheet(0).CreateTable("Costs", sheet.NewRect(addr("A1"), addr("B2")))
		w.Sheet(0).CreateTable("Old", sheet.NewRect(addr("D1"), addr("D2")))
	})
	b := file(t, []string{"S"}, cells, func(w *sheet.Workbook) {
		w.Sheet(0).CreateTable("Costs", sheet.NewRect(addr("A1"), addr("B3")))
	})
	want := `S!D1  input  - Column1
S table Costs  range  A1:B2 → A1:B3
S table Old  removed  - D1:D2
`
	if got := lines(t, a, b); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCompareNotebooks(t *testing.T) {
	nb := func(cells string) []byte {
		return []byte(`{"version": 4, "sheets": [{"name": "Sheet1", "cells": {}}, {"name": "Notebook", "tab": "notebook", "notebookCells": [` + cells + `], "cells": {}}]}`)
	}
	a := nb(`{"kind": "note", "source": "# Sales"}, {"source": "sales = open s.csv", "output": "[[n]; [1]]"}, {"source": "$sales | length", "output": "1"}, {"source": "gone"}`)
	b := nb(`{"source": "new = ls"}, {"kind": "note", "source": "# Sales"}, {"source": "sales = open s.csv", "output": "[[n]; [2]]"}, {"source": "$sales | length", "error": "boom"}`)
	want := `Notebook cell 1  added  + new = ls
Notebook cell 3  output  [[n]; [1]] → [[n]; [2]]
Notebook cell 4  error  + boom
Notebook cell 4  output  - 1
Notebook cell 4  removed  - gone
`
	if got := lines(t, a, b); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	var out bytes.Buffer
	Dump(&out, read(t, b))
	if !strings.Contains(out.String(), "Notebook cell 3  sales = open s.csv  = [[n]; [2]]\nNotebook cell 4  $sales | length  error: boom\n") {
		t.Errorf("dump:\n%s", out.String())
	}
}

func TestWriteChanges(t *testing.T) {
	a := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "=1+1", "B1": "x"}}, nil)
	b := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "=1+2"}}, nil)
	changes := Compare(read(t, a), read(t, b))
	var out bytes.Buffer
	WriteChanges(&out, changes, "nuon", false)
	v, err := nuon.Parse(out.Bytes())
	if err != nil || v.Kind != nuon.List || len(v.List) != 3 {
		t.Fatalf("NUON %v: %s", err, out.String())
	}
	if got := v.List[1].String(); got != `{kind: cell, sheet: Sheet1, item: A1, field: value, old: 2, new: 3}` {
		t.Errorf("a value's record: %s", got)
	}
	out.Reset()
	WriteChanges(&out, changes, "json", false)
	if !strings.Contains(out.String(), `{"kind":"cell","sheet":"Sheet1","item":"B1","field":"input","old":"x","new":null}`) {
		t.Errorf("JSON:\n%s", out.String())
	}
	out.Reset()
	WriteChanges(&out, changes[:1], "text", true)
	if out.String() != "\x1b[1mSheet1!A1\x1b[0m  \x1b[36minput\x1b[0m  \x1b[31m=1+1\x1b[0m → \x1b[32m=1+2\x1b[0m\n" {
		t.Errorf("colored: %q", out.String())
	}
	if err := WriteChanges(&out, changes, "xml", false); err == nil {
		t.Error("xml")
	}
}

func TestDump(t *testing.T) {
	data := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "Rent", "B1": "1450", "B2": "=B1*12", "C1": "=RAND()"}}, func(w *sheet.Workbook) {
		s := w.Sheet(0)
		s.SetNote(addr("A1"), "monthly")
		s.SetColWidth(0, 12)
	})
	var out bytes.Buffer
	if err := Dump(&out, read(t, data)); err != nil {
		t.Fatal(err)
	}
	want := `sheet Sheet1
Sheet1 widths  {"A":12}
Sheet1!A1  Rent  note: monthly
Sheet1!B1  1450
Sheet1!C1  =RAND()
Sheet1!B2  =B1*12  = 17400
`
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}
