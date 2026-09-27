package sheet

import (
	"bytes"
	"fmt"
	"testing"
)

// scaleBook is a workbook of two sheets, the first holding rows rows of
// every kind of cell a before-image keeps: numbers as typed, text,
// formatted numbers, formulas, booleans, notes and formatting alone.
func scaleBook(t testing.TB, rows int) *Sheet {
	s := New()
	cur := Format{Kind: FmtCurrency, Decimals: 2}
	for r := range rows {
		n := r + 1
		load := func(col int, in string, f Format, st Style) {
			if err := s.Load(Addr{Col: col, Row: r}, in, f, st); err != nil {
				t.Fatal(err)
			}
		}
		load(0, fmt.Sprintf("%d.50", r%1000), Format{}, Style{})
		load(1, fmt.Sprintf("item %d", r%500), Format{}, Style{Bold: r%3 == 0})
		load(2, fmt.Sprint(r*7%10000), cur, Style{})
		load(3, fmt.Sprintf("=A%d*2+C%d", n, n), Format{}, Style{})
		switch r % 4 {
		case 0:
			load(4, "TRUE", Format{}, Style{})
		case 1:
			load(4, "", Format{Kind: FmtPercent}, Style{Italic: true})
		}
	}
	for r := 0; r < rows; r += 100 {
		if err := s.SetNote(Addr{Col: 1, Row: r}, fmt.Sprintf("note %d", r)); err != nil {
			t.Fatal(err)
		}
	}
	other, err := s.Book().AddSheet("Other", 1)
	if err != nil {
		t.Fatal(err)
	}
	other.Set(Addr{}, "=SUM(Sheet1!A:A)")
	s.RecalcAll()
	s.ClearHistory()
	return s
}

// saved is the workbook as its file, and the value of Other!A1, which
// reads every number of the first sheet.
func saved(t *testing.T, s *Sheet) string {
	t.Helper()
	var b bytes.Buffer
	if err := s.Book().Write(&b); err != nil {
		t.Fatal(err)
	}
	w := s.Book()
	return b.String() + fmt.Sprint(w.Sheet(w.Len()-1).Value(Addr{}))
}

// Every whole-range operation over a large sheet undoes to the workbook
// as it was and redoes to the workbook as it made it, and its step is
// estimated at a slot or so a plain cell, not a Cell.
func TestUndoWholeRangeAtScale(t *testing.T) {
	const rows = 10000
	s := scaleBook(t, rows)
	w := s.Book()
	all := NewRect(Addr{}, Addr{Col: 4, Row: rows - 1})
	ops := []struct {
		name string
		op   func(t *testing.T)
	}{
		{"clear", func(*testing.T) { s.EraseRange(all) }},
		{"format", func(*testing.T) { s.SetFormat(all, Format{Kind: FmtPercent, Decimals: 1}) }},
		{"style", func(*testing.T) { s.SetStyle(all, func(st *Style) { st.Underline = true }) }},
		{"clear formatting", func(*testing.T) { s.ClearFormatting(all) }},
		{"fill", func(t *testing.T) {
			if err := s.FillEntry(all, all.From, "7"); err != nil {
				t.Fatal(err)
			}
		}},
		{"insert rows", func(t *testing.T) {
			if err := s.InsertRows(10, 5); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete rows", func(*testing.T) { s.DeleteRows(3, 100) }},
		{"insert columns", func(t *testing.T) {
			if err := s.InsertCols(1, 2); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete columns", func(*testing.T) { s.DeleteCols(0, 1) }},
		{"sort", func(*testing.T) { s.SortRange(all, []SortKey{{Col: 2, Desc: true}}) }},
		{"replace sheet", func(t *testing.T) {
			src := New()
			src.Set(Addr{}, "42")
			if _, err := w.ReplaceSheet(s, src, "import"); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete sheet", func(t *testing.T) {
			if err := w.DeleteSheet(s); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, o := range ops {
		before := saved(t, s)
		o.op(t)
		after := saved(t, s)
		if after == before {
			t.Fatalf("%s changed nothing", o.name)
		}
		if n := w.HistoryBytes(); n > int64(rows*5)*(slotBytes+32)+int64(rows)*(richBytes+cellBytes+formulaBytes+perFormulaByte*16) {
			t.Errorf("%s: step estimated at %d bytes", o.name, n)
		}
		if _, ok := w.Undo(); !ok || saved(t, s) != before {
			t.Fatalf("%s: undo doesn't restore the workbook", o.name)
		}
		if _, ok := w.Redo(); !ok || saved(t, s) != after {
			t.Fatalf("%s: redo doesn't make it again", o.name)
		}
		w.Undo()
		w.ClearHistory()
	}
}

// A step over a sheet of plain numbers holds about a slot a cell.
func TestUndoPlainCellsAsSlots(t *testing.T) {
	s := New()
	const n = 50000
	for r := range n {
		s.Load(Addr{Row: r}, fmt.Sprintf("%d.25", r), Format{}, Style{})
	}
	s.RecalcAll()
	s.EraseRange(NewRect(Addr{}, Addr{Row: n - 1}))
	if got := s.Book().HistoryBytes(); got > n*(slotBytes+4) {
		t.Errorf("clearing %d numbers is estimated at %d bytes", n, got)
	}
}

// A change too large to undo runs without recording, forgets the history
// and still reads as a change to the workbook.
func TestWithoutUndo(t *testing.T) {
	s := New()
	s.Set(Addr{}, "1")
	w := s.Book()
	id := w.StateID()
	w.WithoutUndo(func() { s.Set(Addr{}, "2") })
	if w.CanUndo() || w.CanRedo() || w.HistoryBytes() != 0 {
		t.Error("the history outlived a change without undo")
	}
	if inputs(s)["A1"] != "2" || w.StateID() == id || w.StateID() == 0 {
		t.Errorf("A1 %q, state %d (was %d)", inputs(s)["A1"], w.StateID(), id)
	}
	s.Set(Addr{}, "3")
	if s.Undo(); inputs(s)["A1"] != "2" {
		t.Errorf("undo after: A1 %q", inputs(s)["A1"])
	}
	if cost := s.UndoCost(NewRect(Addr{}, Addr{Col: 9, Row: 9})); cost < slotBytes || cost > 1024 {
		t.Errorf("one cell's undo cost %d", cost)
	}
}
