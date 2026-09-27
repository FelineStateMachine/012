package sheet

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// roundTrip saves s's workbook and reads it back.
func roundTrip(t *testing.T, s *Sheet) *Sheet {
	t.Helper()
	var b bytes.Buffer
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&b)
	if err != nil {
		t.Fatalf("%v\n%s", err, b.String())
	}
	return got
}

func TestBorders(t *testing.T) {
	s := New()
	s.SetBorders(rng("B2:C3"), BorderOuter, LineThin)
	for _, c := range []struct {
		cell string
		want Borders
	}{
		{"B2", BordersOf(LineThin, LineNone, LineThin, LineNone)},
		{"C2", BordersOf(LineThin, LineNone, LineNone, LineThin)},
		{"B3", BordersOf(LineNone, LineThin, LineThin, LineNone)},
		{"C3", BordersOf(LineNone, LineThin, LineNone, LineThin)},
	} {
		if got := s.CellStyle(at(c.cell)).Borders; got != c.want {
			t.Errorf("outer: %s = %+v, want %+v", c.cell, got, c.want)
		}
	}
	// Neighbors see the shared edge.
	if s.EdgeAbove(at("B4")) != LineThin || s.EdgeLeft(at("D2")) != LineThin || s.EdgeAbove(at("B3")) != LineNone {
		t.Error("shared edges")
	}
	// Inner draws between the cells only; a later thick top clears the
	// neighbor's facing bottom, so the last change shows.
	s.SetBorders(rng("B2:C3"), BorderInner, LineDouble)
	if got := s.CellStyle(at("B2")).Borders; got != BordersOf(LineThin, LineDouble, LineThin, LineDouble) {
		t.Errorf("inner B2 = %+v", got)
	}
	s.SetBorders(rng("B1"), BorderBottom, LineThin)
	s.SetBorders(rng("B2"), BorderTop, LineThick)
	if got := s.CellStyle(at("B1")).Borders.Bottom(); got != LineNone {
		t.Errorf("facing bottom kept: %v", got)
	}
	if s.EdgeAbove(at("B2")) != LineThick {
		t.Error("top not thick")
	}
	// None removes every line of the range and the facing ones.
	s.SetBorders(rng("A1:D4"), BorderNone, LineThin)
	for a := range s.cells.all() {
		if b := s.CellStyle(a).Borders; !b.IsZero() {
			t.Errorf("%s still %+v", a, b)
		}
	}
	if s.cells.len() != 0 {
		t.Errorf("formatting-only cells left: %d", s.cells.len())
	}
	s.Undo()
	if s.CellStyle(at("C3")).Borders.Right() != LineThin {
		t.Error("undo none")
	}
	// Whole columns keep borders on the column.
	s.SetBorders(rng("E:F"), BorderAll, LineThin)
	if s.RowFormats()[0] != (Format{}) || s.EdgeLeft(at("F900")) != LineThin || s.EdgeAbove(at("E77")) != LineThin {
		t.Error("column borders")
	}
	if len(s.ColStyles()) != 2 {
		t.Errorf("column styles %v", s.ColStyles())
	}
	got := roundTrip(t, s)
	if got.CellStyle(at("C2")).Borders != s.CellStyle(at("C2")).Borders || got.EdgeLeft(at("F3")) != LineThin {
		t.Error("borders lost in the file")
	}
}

func TestWrapFile(t *testing.T) {
	s := New()
	s.Set(at("A1"), "a long line of text")
	s.SetStyle(rng("A1"), func(st *Style) { st.Wrap = WrapOn })
	s.SetStyle(rng("B:B"), func(st *Style) { st.Wrap = WrapClip })
	var b bytes.Buffer
	s.Write(&b)
	if !strings.Contains(b.String(), `"wrap": "wrap"`) && !strings.Contains(b.String(), `"wrap":"wrap"`) {
		t.Errorf("file: %s", b.String())
	}
	got := roundTrip(t, s)
	if got.CellStyle(at("A1")).Wrap != WrapOn || got.CellStyle(at("B7")).Wrap != WrapClip {
		t.Error("wrap lost in the file")
	}
	if _, err := Read(strings.NewReader(`{"version": 2, "cells": {"A1": {"input": "x", "wrap": "sideways"}}}`)); err == nil {
		t.Error("unknown wrapping read")
	}
}

func TestShapers(t *testing.T) {
	s := New()
	if s.Shaped() {
		t.Error("new sheet shaped")
	}
	s.Set(at("C5"), "words that wrap")
	s.SetStyle(rng("C5"), func(st *Style) { st.Wrap = WrapOn })
	if !s.Shaped() || !slices.Equal(s.WrappedIn(4), []int{2}) || s.WrappedIn(3) != nil {
		t.Errorf("wrapped in row 5: %v", s.WrappedIn(4))
	}
	s.SetBorders(rng("A7"), BorderBottom, LineThin)
	if !s.RuleAbove(7) || s.RuleAbove(6) || s.RuleAbove(4) {
		t.Error("rule above row 8 only")
	}
	// A number read from a file is indexed too.
	s.Set(at("A7"), "12.5")
	if got := roundTrip(t, s); !got.RuleAbove(7) || !slices.Equal(got.WrappedIn(4), []int{2}) {
		t.Error("the file's borders and wrapping aren't indexed")
	}
	s.ClearFormatting(rng("A1:Z99"))
	if s.Shaped() {
		t.Error("still shaped after clearing")
	}
	// A column that wraps wraps the text its cells hold.
	s.SetStyle(rng("D:D"), func(st *Style) { st.Wrap = WrapOn })
	s.Set(at("D3"), "x")
	if !slices.Equal(s.WrappedIn(2), []int{3}) || s.WrappedIn(3) != nil {
		t.Errorf("column wrap %v", s.WrappedIn(2))
	}
	// A merged cell doesn't grow its row.
	s.Merge(rng("D3:E3"), MergeAll)
	if s.WrappedIn(2) != nil {
		t.Error("merged cell wraps")
	}
}

func TestRowHeights(t *testing.T) {
	s := New()
	s.SetRowHeight(2, 3, 4)
	if h, ok := s.RowHeight(3); !ok || h != 4 {
		t.Errorf("height = %d, %v", h, ok)
	}
	s.Seal() // the UI seals between actions, so heights join no other step
	s.InsertRows(0, 2)
	if _, ok := s.RowHeight(2); ok {
		t.Error("row 3 kept its height")
	}
	if h, _ := s.RowHeight(5); h != 4 {
		t.Error("heights didn't move")
	}
	s.Undo()
	if h, _ := s.RowHeight(2); h != 4 {
		t.Error("undo insert")
	}
	s.SetRowHeight(2, 2, 0)
	if _, ok := s.RowHeight(2); ok {
		t.Error("fit to data")
	}
	s.Undo()
	s.SetRowHeight(9, 9, 99)
	if h, _ := s.RowHeight(9); h != MaxRowHeight {
		t.Errorf("capped at %d", h)
	}
	// Whole columns set the rows up to the last holding a cell.
	s.Set(at("B12"), "x")
	s.SetRowHeight(0, MaxRows-1, 2)
	if len(s.Heights()) != 12 {
		t.Errorf("whole column heights %v", s.Heights())
	}
	s.Undo()
	s.Undo()
	if s.ColsWidth(0, 3) != 4*DefaultWidth {
		t.Error("ColsWidth")
	}
	s.SetColWidth(2, 3)
	if s.ColsWidth(0, 3) != 3*DefaultWidth+3 || s.ColsWidth(3, 9) != 7*DefaultWidth {
		t.Error("ColsWidth with a width")
	}
	s.Undo()
	got := roundTrip(t, s)
	if !mapsEqual(got.Heights(), s.Heights()) {
		t.Errorf("file heights %v, want %v", got.Heights(), s.Heights())
	}
	// Whole rows carry their heights when copied and moved.
	clip := s.Copy(rowRect(2, 3))
	s.Paste(clip, rowRect(20, 20), false)
	if h, _ := s.RowHeight(21); h != 4 {
		t.Errorf("pasted rows' heights %v", s.Heights())
	}
	s.Move(rowRect(20, 21), Addr{Row: 30})
	if _, ok := s.RowHeight(20); ok {
		t.Error("moved rows left their heights")
	}
	if h, _ := s.RowHeight(31); h != 4 {
		t.Errorf("moved rows' heights %v", s.Heights())
	}
}

func mapsEqual(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestMerge(t *testing.T) {
	s := New()
	s.Set(at("B2"), "kept")
	s.Set(at("C3"), "lost")
	if a, loses := s.MergeLoses(rng("B2:C3"), MergeAll); !loses || a != at("C3") {
		t.Errorf("MergeLoses = %v, %v", a, loses)
	}
	if _, loses := s.MergeLoses(rng("B3:C3"), MergeVertically); loses {
		t.Error("vertical merges of one row lose nothing")
	}
	if err := s.Merge(rng("B2:C3"), MergeAll); err != nil {
		t.Fatal(err)
	}
	if m, ok := s.MergeAt(at("C3")); !ok || m != rng("B2:C3") {
		t.Errorf("MergeAt = %v, %v", m, ok)
	}
	if !s.Cell(at("C3")).Blank() || s.Value(at("B2")).Str != "kept" {
		t.Error("merging keeps only the top-left value")
	}
	if s.Grow(rng("A1:B2")) != rng("A1:C3") {
		t.Errorf("Grow = %v", s.Grow(rng("A1:B2")))
	}
	s.Undo()
	if s.Value(at("C3")).Str != "lost" || len(s.Merges()) != 0 {
		t.Error("undo merge")
	}
	s.Redo()
	if err := s.Merge(rng("A1"), MergeAll); err != ErrMergeOne {
		t.Errorf("one cell: %v", err)
	}
	if err := s.Merge(rng("A1:B1048576"), MergeHorizontally); err != ErrMergeMany {
		t.Errorf("a million merges: %v", err)
	}
	// Merges follow inserted and deleted lines and are saved.
	s.InsertCols(0, 1)
	if got := s.Merges(); len(got) != 1 || got[0] != rng("C2:D3") {
		t.Errorf("after insert %v", got)
	}
	got := roundTrip(t, s)
	if !slices.Equal(got.Merges(), s.Merges()) {
		t.Errorf("file merges %v", got.Merges())
	}
	s.DeleteCols(3, 1)
	if got := s.Merges(); len(got) != 1 || got[0] != rng("C2:C3") {
		t.Errorf("after deleting a column %v", got)
	}
	s.DeleteRows(2, 1)
	if len(s.Merges()) != 0 {
		t.Errorf("merge of one cell left %v", s.Merges())
	}
	s.Undo()
	s.Undo()
	// Rows and columns merge one by one.
	s.Merge(rng("F1:G3"), MergeHorizontally)
	if len(s.MergesIn(rng("F1:G3"))) != 3 {
		t.Errorf("horizontal merges %v", s.Merges())
	}
	if n := s.Unmerge(rng("G2")); n != 1 || len(s.MergesIn(rng("F1:G3"))) != 2 {
		t.Errorf("unmerge %d, %v", n, s.Merges())
	}
}

func TestMergeCopyMoveSort(t *testing.T) {
	s := New()
	s.Set(at("A1"), "b")
	s.Set(at("A2"), "a")
	s.Merge(rng("A1:B1"), MergeAll)
	// A copy carries the merges wholly inside it.
	s.Paste(s.Copy(rng("A1:B2")), rng("D5"), false)
	if m, ok := s.MergeAt(at("E5")); !ok || m != rng("D5:E5") {
		t.Errorf("pasted merge %v, %v", m, ok)
	}
	s.Move(rng("D5:E6"), at("D9"))
	if _, ok := s.MergeAt(at("D5")); ok {
		t.Error("moved merge left behind")
	}
	if m, _ := s.MergeAt(at("E9")); m != rng("D9:E9") {
		t.Errorf("moved merge %v", s.Merges())
	}
	// Sorting a range with merged cells is refused.
	s.SortRange(rng("A1:B2"), []SortKey{{Col: 0}})
	if s.Value(at("A1")).Str != "b" {
		t.Error("sorted over merged cells")
	}
	// A merge blocks a spill.
	s.Set(at("H1"), "=SEQUENCE(3)")
	s.Merge(rng("H2:I2"), MergeAll)
	if v := s.Value(at("H1")); v.Kind != Error {
		t.Errorf("spill over a merge = %v", v)
	}
	s.Unmerge(rng("H2"))
	if v := s.Value(at("H3")); v.Num != 3 {
		t.Errorf("spill after unmerging = %v", v)
	}
}
