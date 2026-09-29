package sheet

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/value"
)

// liveRow makes a row of entries, as a CSV line's fields.
func liveRow(fields ...string) LiveRow {
	row := make(LiveRow, len(fields))
	for i, f := range fields {
		v, fm := EntryValue(f)
		row[i] = LiveCell{V: v, F: fm}
	}
	return row
}

// linkedSheet is a sheet with a region at A1 showing a header and n rows
// of numbers 1..n in column B.
func linkedSheet(t *testing.T, window int) (*Sheet, string) {
	t.Helper()
	s := New()
	id, err := s.AddLinked(Addr{}, LinkSource{Path: "log.csv", Window: window})
	if err != nil {
		t.Fatal(err)
	}
	return s, id
}

func apply(t *testing.T, s *Sheet, op LiveOp) LinkedRegion {
	t.Helper()
	if err := s.Book().ApplyLive(op); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Book().LinkedRegion(op.Region)
	return r
}

func TestLiveResetAndAppend(t *testing.T) {
	s, id := linkedSheet(t, 0)
	s.Set(Addr{Col: 4}, "=SUM(B:B)")
	s.Set(Addr{Col: 5}, "=COUNTA(A2:A100)")
	r := apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("name", "n"), Rows: []LiveRow{liveRow("a", "1"), liveRow("b", "2")}})
	if r.Rows != 2 || r.Area != NewRect(Addr{}, Addr{Col: 1, Row: 2}) || r.Stale {
		t.Fatalf("after reset: %+v", r)
	}
	if got := s.Value(Addr{Col: 4}).Num; got != 3 {
		t.Fatalf("SUM = %v, want 3", got)
	}
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	r = apply(t, s, LiveOp{Region: id, At: at, Rows: []LiveRow{liveRow("c", "3", "extra")}})
	if r.Rows != 3 || r.Area.To != (Addr{Col: 2, Row: 3}) || !r.Updated.Equal(at) {
		t.Fatalf("after append: %+v", r)
	}
	if got := s.Value(Addr{Col: 4}).Num; got != 6 {
		t.Fatalf("SUM = %v, want 6", got)
	}
	if got := s.Value(Addr{Col: 5}).Num; got != 3 {
		t.Fatalf("COUNTA = %v, want 3", got)
	}
	if !s.Cell(Addr{Row: 3}).Spilled() || s.Value(Addr{Col: 2, Row: 3}).Str != "extra" {
		t.Fatalf("row 4: %+v", s.Cell(Addr{Row: 3}))
	}
	if s.CanUndo() && s.Book().UndoLabel() != "edit F1" {
		t.Fatalf("rows arriving made an undo step: %q", s.Book().UndoLabel())
	}
	// A reset with fewer rows clears the rest.
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("name", "n"), Rows: []LiveRow{liveRow("z", "10")}})
	if s.Filled(Addr{Row: 2}) || s.Filled(Addr{Col: 2, Row: 3}) {
		t.Fatal("a reset left old rows")
	}
	if got := s.Value(Addr{Col: 4}).Num; got != 10 {
		t.Fatalf("SUM = %v, want 10", got)
	}
}

func TestLiveWindow(t *testing.T) {
	s, id := linkedSheet(t, 3)
	s.Set(Addr{Col: 4}, "=SUM(B2:B10)")
	var rows []LiveRow
	for _, n := range []string{"1", "2", "3", "4", "5"} {
		rows = append(rows, liveRow("r"+n, n))
	}
	r := apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("name", "n"), Rows: rows[:2]})
	if r.Rows != 2 || r.Dropped != 0 {
		t.Fatalf("%+v", r)
	}
	r = apply(t, s, LiveOp{Region: id, Rows: rows[2:]})
	if r.Rows != 3 || r.Dropped != 2 {
		t.Fatalf("after the window filled: %+v", r)
	}
	var got []string
	for row := 1; row <= 3; row++ {
		got = append(got, s.Value(Addr{Row: row}).Str)
	}
	if strings.Join(got, ",") != "r3,r4,r5" || s.Value(Addr{Row: 0}).Str != "name" {
		t.Fatalf("rows %v, header %v", got, s.Value(Addr{}))
	}
	if s.Filled(Addr{Row: 4}) {
		t.Fatal("the window left a row below")
	}
	if v := s.Value(Addr{Col: 4}).Num; v != 12 {
		t.Fatalf("SUM = %v, want 12", v)
	}
	// One more row drops one more.
	r = apply(t, s, LiveOp{Region: id, Rows: []LiveRow{liveRow("r6", "6")}})
	if r.Dropped != 3 || s.Value(Addr{Row: 1}).Str != "r4" || s.Value(Addr{Row: 3}).Str != "r6" {
		t.Fatalf("%+v: %v..%v", r, s.Value(Addr{Row: 1}), s.Value(Addr{Row: 3}))
	}
}

func TestLiveMaxCells(t *testing.T) {
	SetMaxCells(8) // a header and three rows of two
	defer SetMaxCells(0)
	s, id := linkedSheet(t, 0)
	var rows []LiveRow
	for range 5 {
		rows = append(rows, liveRow("x", "1"))
	}
	r := apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a", "b"), Rows: rows})
	if r.Rows != 3 || !strings.Contains(r.Note, "only the first 3 rows") {
		t.Fatalf("%+v", r)
	}
}

func TestLiveRefusesEditsAndBlocks(t *testing.T) {
	s, id := linkedSheet(t, 0)
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a"), Rows: []LiveRow{liveRow("1")}})
	if err := s.Set(Addr{Row: 1}, "5"); !errors.Is(err, ErrLinkedEdit) {
		t.Fatalf("Set in a region: %v", err)
	}
	if _, r, ok := s.InRegion(NewRect(Addr{Row: 1}, Addr{Col: 3, Row: 5})); !ok || r.Name != id {
		t.Fatal("InRegion missed the linked file")
	}
	// Formatting a linked cell keeps its value.
	s.SetStyle(NewRect(Addr{Row: 1}, Addr{Row: 1}), func(st *Style) { st.Bold = true })
	if c := s.Cell(Addr{Row: 1}); c.Value.Num != 1 || !c.Style.Bold || !c.Spilled() {
		t.Fatalf("formatted linked cell: %+v", c)
	}
	// Rows that would overwrite a typed cell aren't shown, and the first
	// cell says why; clearing the cell has the file read again.
	s.Set(Addr{Row: 3}, "mine")
	r := apply(t, s, LiveOp{Region: id, Rows: []LiveRow{liveRow("2"), liveRow("3")}})
	if !strings.Contains(r.Err, "overwrite data in A4") || r.Rows != 0 || s.Value(Addr{Row: 3}).Str != "mine" ||
		s.Value(Addr{}) != value.ErrRef || s.Filled(Addr{Row: 1}) {
		t.Fatalf("%+v", r)
	}
	s.Set(Addr{Row: 3}, "")
	if r, _ := s.Book().LinkedRegion(id); !r.Stale {
		t.Fatalf("clearing the cell in the way: %+v", r)
	}
}

func TestLiveErrorShowsInAnchor(t *testing.T) {
	s, id := linkedSheet(t, 0)
	r := apply(t, s, LiveOp{Region: id, Err: "no such file"})
	if r.Err != "no such file" || s.Value(Addr{}) != value.ErrRef {
		t.Fatalf("%+v, anchor %v", r, s.Value(Addr{}))
	}
	r = apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a"), Rows: []LiveRow{liveRow("1")}})
	if r.Err != "" || s.Value(Addr{}).Str != "a" {
		t.Fatalf("%+v, anchor %v", r, s.Value(Addr{}))
	}
	// An error once rows show keeps them.
	r = apply(t, s, LiveOp{Region: id, Err: "gone"})
	if r.Err != "gone" || s.Value(Addr{Row: 1}).Num != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestUnlinkAndUndo(t *testing.T) {
	s, id := linkedSheet(t, 0)
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a"), Rows: []LiveRow{liveRow("1")}})
	if err := s.Book().Unlink(id); err != nil {
		t.Fatal(err)
	}
	if s.HasLinked() || s.Cell(Addr{Row: 1}).Spilled() || s.Value(Addr{Row: 1}).Num != 1 {
		t.Fatalf("unlinked: %+v", s.Cell(Addr{Row: 1}))
	}
	if err := s.Set(Addr{Row: 1}, "7"); err != nil {
		t.Fatal(err)
	}
	s.Undo()
	s.Undo() // the unlink
	r, ok := s.Book().LinkedRegion(id)
	if !ok || !r.Stale || s.Filled(Addr{Row: 1}) {
		t.Fatalf("undo of unlink: %+v %v, A2 %v", r, ok, s.Value(Addr{Row: 1}))
	}
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a"), Rows: []LiveRow{liveRow("2")}})
	s.Undo() // making the region
	if s.HasLinked() || s.Filled(Addr{}) || s.Filled(Addr{Row: 1}) {
		t.Fatal("undoing the region left its cells")
	}
	if err := s.Book().ApplyLive(LiveOp{Region: id}); !errors.Is(err, ErrNoRegion) {
		t.Fatalf("op for a region undone: %v", err)
	}
	s.Redo()
	if r, ok := s.Book().LinkedRegion(id); !ok || !r.Stale {
		t.Fatalf("redo: %+v", r)
	}
}

func TestLinkedShiftsWithRows(t *testing.T) {
	s := New()
	s.Set(Addr{Row: 0}, "above")
	id, err := s.AddLinked(Addr{Row: 2}, LinkSource{Path: "log.csv"})
	if err != nil {
		t.Fatal(err)
	}
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a"), Rows: []LiveRow{liveRow("1")}})
	s.Set(Addr{Col: 3, Row: 1}, "x")
	if err := s.InsertRows(1, 2); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Book().LinkedRegion(id)
	if r.Anchor != (Addr{Row: 4}) || !r.Stale || s.Filled(Addr{Row: 2}) || s.Filled(Addr{Row: 4}) {
		t.Fatalf("%+v", r)
	}
	if s.Value(Addr{Col: 3, Row: 3}).Str != "x" {
		t.Fatal("a cell moving past the region was lost")
	}
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a"), Rows: []LiveRow{liveRow("1")}})
	if s.Value(Addr{Row: 5}).Num != 1 {
		t.Fatal("the region wasn't written at its new place")
	}
	// Rows inserted below it leave it be.
	s.InsertRows(20, 1)
	if r, _ := s.Book().LinkedRegion(id); r.Stale {
		t.Fatal("rows below the region made it stale")
	}
	s.DeleteRows(4, 1) // its anchor's row
	if s.HasLinked() {
		t.Fatal("deleting the anchor's row kept the region")
	}
}

func TestLinkedFileRoundTrip(t *testing.T) {
	s, id := linkedSheet(t, 500)
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("a"), Rows: []LiveRow{liveRow("1")}})
	var b bytes.Buffer
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	if !strings.Contains(text, `"regions": [`) || !strings.Contains(text, `{"name":"log","at":"A1","path":"log.csv","window":500}`) ||
		strings.Contains(text, `"A2"`) || strings.Contains(text, `"notebook"`) {
		t.Fatalf("written:\n%s", text)
	}
	got, err := Read(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	rs := got.LinkedRegions()
	if len(rs) != 1 || rs[0].Name != "log" || rs[0].Source != (LinkSource{Path: "log.csv", Window: 500}) || !rs[0].Stale {
		t.Fatalf("read: %+v", rs)
	}
	for _, bad := range []string{`[{"name": "a", "at": "ZZZZ9", "path": "a"}]`, `[{"name": "a", "at": "A1", "path": "a", "window": -1}]`,
		`[{"name": "a", "at": "A1", "path": "a"}, {"name": "a", "at": "A3", "path": "b"}]`, `[{"name": "a", "at": "A1", "path": "a", "command": "ls"}]`} {
		if _, err := Read(strings.NewReader(`{"version": 2, "cells": {}, "regions": ` + bad + `}`)); err == nil {
			t.Errorf("read links %s", bad)
		}
	}
}

func TestEntryValue(t *testing.T) {
	for in, want := range map[string]Value{"12": value.Num(12), "$5": value.Num(5), "true": value.Boolean(true),
		"'12": value.Str("12"), "=A1": value.Str("=A1"), "a\x1bb": value.Str("ab"), "": {}} {
		if got, _ := EntryValue(in); got != want {
			t.Errorf("EntryValue(%q) = %v, want %v", in, got, want)
		}
	}
	if _, f := EntryValue("9/26/2026"); f.IsZero() {
		t.Error("a date implied no format")
	}
}
