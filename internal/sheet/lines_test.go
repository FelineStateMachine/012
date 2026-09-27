package sheet

import (
	"bytes"
	"testing"
)

func col(c int) Rect { return colRect(c, c) }

// Formatting whole columns and rows keeps the format on the line: no
// cells are made, every cell of the line shows it, and the latest
// formatting wins where lines cross or a cell differs.
func TestLineFormats(t *testing.T) {
	s := New()
	s.Set(at("B2"), "5")
	s.SetFormat(at1("B2"), Preset(FmtPercent))
	s.SetFormat(col(1), Preset(FmtCurrency))
	if n := s.cells.len(); n != 1 {
		t.Errorf("%d cells stored, want only B2", n)
	}
	for _, a := range []string{"B2", "B1", "B1048576"} {
		if f := s.DisplayFormat(at(a)); f.Kind != FmtCurrency {
			t.Errorf("%s shows %v, want currency", a, f.Kind)
		}
	}
	if c := s.Cell(at("B2")); !c.Format.IsZero() {
		t.Errorf("B2 keeps its own %v under the column's format", c.Format.Kind)
	}

	// Bold the column, then take the bold off one cell of it.
	s.SetStyle(col(0), func(st *Style) { st.Bold = true })
	s.SetStyle(at1("A5"), func(st *Style) { st.Bold = false })
	if s.CellStyle(at("A5")).Bold || !s.CellStyle(at("A6")).Bold {
		t.Errorf("A5 bold %v, A6 bold %v", s.CellStyle(at("A5")).Bold, s.CellStyle(at("A6")).Bold)
	}
	// Styling a row changes what its cells show, column formats included.
	s.SetStyle(rowRect(2, 2), func(st *Style) { st.Italic = true })
	if st := s.CellStyle(at("A3")); !st.Italic || !st.Bold {
		t.Errorf("A3 = %+v, want the column's bold and the row's italic", st)
	}
	s.SetStyle(col(0), func(st *Style) { st.Underline = true })
	if st := s.CellStyle(at("A3")); !st.Italic || !st.Underline {
		t.Errorf("A3 after underlining column A = %+v", st)
	}
	if st := s.CellStyle(at("C3")); !st.Italic || st.Underline {
		t.Errorf("C3 = %+v, want the row's italic only", st)
	}

	// Typing into a plain text column keeps the entry as text.
	s.SetFormat(col(3), Format{Kind: FmtText})
	s.Set(at("D7"), "0012")
	if v := s.Value(at("D7")); v.Kind != Text || v.Str != "0012" {
		t.Errorf("D7 = %+v, want the text 0012", v)
	}

	// Saved and loaded, the formats read the same.
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := Read(&buf)
	if err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	for _, a := range []string{"A3", "A5", "A6", "C3", "B9", "D7", "Z1"} {
		if got, want := back.effective(at(a)), s.effective(at(a)); got != want {
			t.Errorf("%s after loading = %+v, want %+v", a, got, want)
		}
		if got, want := back.DisplayFormat(at(a)), s.DisplayFormat(at(a)); got != want {
			t.Errorf("%s format after loading = %+v, want %+v", a, got, want)
		}
	}
}

func at1(a string) Rect { return Rect{From: at(a), To: at(a)} }

// Line formats undo, redo, move with inserted columns and clear.
func TestLineFormatsUndoAndInsert(t *testing.T) {
	s := New()
	s.Set(at("C4"), "7")
	s.SetFormat(col(2), Preset(FmtCurrency))
	s.Undo()
	if f := s.DisplayFormat(at("C9")); !f.IsZero() {
		t.Errorf("after undo C9 shows %v", f.Kind)
	}
	s.Redo()
	if f := s.DisplayFormat(at("C9")); f.Kind != FmtCurrency {
		t.Errorf("after redo C9 shows %v", f.Kind)
	}
	if err := s.InsertCols(0, 2); err != nil {
		t.Fatal(err)
	}
	if f := s.DisplayFormat(at("E9")); f.Kind != FmtCurrency {
		t.Errorf("after inserting two columns E9 shows %v", f.Kind)
	}
	if f := s.DisplayFormat(at("C9")); !f.IsZero() {
		t.Errorf("after inserting two columns C9 shows %v", f.Kind)
	}
	s.ClearFormatting(col(4))
	if len(s.lines.cols) != 0 || s.DisplayFormat(at("E4")).Kind == FmtCurrency {
		t.Errorf("clearing column E left %v", s.lines.cols)
	}
}

// Formatting the whole sheet sets the sheet's own format, once, and
// changes the columns and rows that have one so they show it too.
func TestWholeSheetFormat(t *testing.T) {
	s := New()
	s.Set(at("XFD1048576"), "1")
	s.SetStyle(col(3), func(st *Style) { st.Italic = true })
	all := Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}}
	s.SetStyle(all, func(st *Style) { st.Bold = true })
	if len(s.lines.cols) != 1 || s.cells.len() != 1 || !s.lines.sheet.Style.Bold {
		t.Errorf("%d column formats, %d cells, sheet %+v", len(s.lines.cols), s.cells.len(), s.lines.sheet)
	}
	if !s.CellStyle(at("XFD1048576")).Bold || !s.CellStyle(at("Q77")).Bold {
		t.Error("not bold everywhere")
	}
	if st := s.CellStyle(at("D9")); !st.Bold || !st.Italic {
		t.Errorf("D9 = %+v, want column D's italic and the sheet's bold", st)
	}
	var buf bytes.Buffer
	s.Write(&buf)
	if !bytes.Contains(buf.Bytes(), []byte(`"A:XFD": {"bold":true}`)) {
		t.Errorf("the sheet's format isn't written:\n%.300s", buf.String())
	}
	back, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !back.CellStyle(at("Q77")).Bold || !back.CellStyle(at("D9")).Italic || len(back.lines.cols) != 1 {
		t.Error("the sheet's format didn't load")
	}
	s.Undo()
	if s.CellStyle(at("Q77")).Bold || s.CellStyle(at("D9")).Bold {
		t.Error("undo left bold")
	}
	s.Redo()
	s.ClearFormatting(all)
	if len(s.lines.cols) != 0 || !s.lines.sheet.IsZero() {
		t.Errorf("%d column formats after clearing", len(s.lines.cols))
	}
}
