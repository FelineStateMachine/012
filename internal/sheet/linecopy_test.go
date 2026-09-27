package sheet

import "testing"

func bold(st *Style) { st.Bold = true }

// Copying whole columns and pasting them at the top of a column carries
// the column formats themselves: no cells are made for them, and the
// paste undoes as one step.
func TestCopyWholeColumnsCarriesFormats(t *testing.T) {
	s := New()
	s.Set(at("B1"), "5")
	s.Set(at("B2"), "6")
	s.SetFormat(col(1), Preset(FmtCurrency))
	s.SetStyle(at1("B2"), bold)
	s.SetFormat(col(3), Preset(FmtPercent)) // D, to be replaced
	s.SetStyle(rowRect(4, 4), func(st *Style) { st.Italic = true })
	cells := s.cells.len()

	dst, err := s.Paste(s.Copy(col(1)), at1("D1"), false)
	if err != nil || dst != col(3) {
		t.Fatalf("pasted at %v, %v; want D:D", dst, err)
	}
	if f := s.lines.cols[3].Format; f.Kind != FmtCurrency {
		t.Errorf("column D's format = %v, want currency", f.Kind)
	}
	for _, a := range []string{"D1", "D2", "D900000"} {
		if f := s.DisplayFormat(at(a)); f.Kind != FmtCurrency {
			t.Errorf("%s shows %v, want currency", a, f.Kind)
		}
	}
	if !s.CellStyle(at("D2")).Bold || s.CellStyle(at("D3")).Bold {
		t.Error("B2's bold didn't travel to D2 alone")
	}
	if !s.CellStyle(at("D5")).Italic {
		t.Error("row 5's italic is lost where it crosses column D")
	}
	if n := s.cells.len(); n != cells+2 {
		t.Errorf("%d cells after the paste, want %d: only B1 and B2's copies", n, cells+2)
	}
	s.Undo()
	if f := s.DisplayFormat(at("D9")); f.Kind != FmtPercent {
		t.Errorf("after undo D9 shows %v, want percent again", f.Kind)
	}
	if s.cells.get(at("D1")) != nil {
		t.Error("undo left D1")
	}

	// Pasted anywhere else, the copy is a block of cells showing what
	// the column showed.
	if _, err := s.Paste(s.Copy(col(1)), at1("F5"), false); err != nil {
		t.Fatal(err)
	}
	if s.DisplayFormat(at("F6")).Kind != FmtCurrency || !s.CellStyle(at("F6")).Bold || s.lines.cols[5] != (lineFmt{}) {
		t.Errorf("F6 shows %v bold %v; column F %+v", s.DisplayFormat(at("F6")).Kind, s.CellStyle(at("F6")).Bold, s.lines.cols[5])
	}
	if f := s.DisplayFormat(at("F7")); !f.IsZero() {
		t.Errorf("F7 shows %v, past the copied cells", f.Kind)
	}
}

// Whole rows carry their row formats, and filling whole rows down
// copies the row's format to the rows filled.
func TestCopyWholeRowsCarriesFormats(t *testing.T) {
	s := New()
	s.Set(at("A2"), "1")
	s.SetStyle(rowRect(1, 1), bold)
	if _, err := s.Paste(s.Copy(rowRect(1, 1)), at1("A7"), false); err != nil {
		t.Fatal(err)
	}
	if !s.lines.rows[6].Style.Bold || s.Value(at("A7")).Num != 1 {
		t.Errorf("row 7 = %+v, A7 = %v", s.lines.rows[6], s.Value(at("A7")))
	}
	if _, err := s.FillDown(rowRect(6, 9)); err != nil {
		t.Fatal(err)
	}
	if !s.lines.rows[9].Style.Bold || !s.CellStyle(at("ZZ10")).Bold {
		t.Errorf("fill down didn't carry row 7's bold to row 10: %+v", s.lines.rows[9])
	}
}

// A block pasted into cells shows the formats its source cells showed,
// from their own, their row's, column's or the sheet's, and replaces
// the formats of the lines it lands on.
func TestPasteBlockAppliesEffectiveFormats(t *testing.T) {
	s := New()
	s.Set(at("A1"), "1")
	s.SetFormat(col(0), Preset(FmtCurrency))
	s.SetStyle(rowRect(1, 1), bold)
	s.SetFormat(col(4), Preset(FmtPercent)) // E
	s.Set(at("G1"), "2")                    // plain

	if _, err := s.Paste(s.Copy(Rect{From: at("A1"), To: at("A3")}), at1("C1"), false); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"C1", "C2", "C3"} {
		if f := s.DisplayFormat(at(a)); f.Kind != FmtCurrency {
			t.Errorf("%s shows %v, want currency", a, f.Kind)
		}
	}
	if !s.CellStyle(at("C2")).Bold {
		t.Error("C2 lost row 2's bold")
	}
	if f := s.DisplayFormat(at("C4")); !f.IsZero() {
		t.Errorf("C4 shows %v, past the paste", f.Kind)
	}

	// Plain cells pasted into a percent column show plain, as their
	// source did.
	if _, err := s.Paste(s.Copy(Rect{From: at("G1"), To: at("G2")}), at1("E1"), false); err != nil {
		t.Fatal(err)
	}
	if f := s.DisplayFormat(at("E1")); !f.IsZero() {
		t.Errorf("E1 shows %v, want the plain format it was copied with", f.Kind)
	}
	if f := s.DisplayFormat(at("E5")); f.Kind != FmtPercent {
		t.Errorf("E5 shows %v, want its column's percent", f.Kind)
	}

	// Pasting values only keeps the destination's formats.
	if _, err := s.Paste(s.Copy(at1("G1")), at1("E7"), true); err != nil {
		t.Fatal(err)
	}
	if f := s.DisplayFormat(at("E7")); f.Kind != FmtPercent {
		t.Errorf("E7 shows %v after pasting values, want percent", f.Kind)
	}
}

// Cutting whole columns moves their formats, leaving the source plain;
// a cut block takes the formats its cells showed.
func TestMoveCarriesFormats(t *testing.T) {
	w := NewBook()
	s := w.Sheets()[0]
	s.Set(at("B1"), "5")
	s.SetFormat(col(1), Preset(FmtCurrency))
	c := s.Copy(col(1))
	if r := c.MoveRange(at("F1")); r != col(1) {
		t.Fatalf("a cut of B:B pasted at F1 moves %v", r)
	}
	if r := c.MoveRange(at("F3")); r != at1("B1") {
		t.Fatalf("a cut of B:B pasted at F3 moves %v", r)
	}
	if _, err := s.Move(col(1), at("F1")); err != nil {
		t.Fatal(err)
	}
	if s.DisplayFormat(at("F9")).Kind != FmtCurrency || !s.DisplayFormat(at("B9")).IsZero() {
		t.Errorf("after moving B:B to F:F, F9 shows %v and B9 %v", s.DisplayFormat(at("F9")).Kind, s.DisplayFormat(at("B9")).Kind)
	}
	s.Undo()
	if s.DisplayFormat(at("B9")).Kind != FmtCurrency || !s.DisplayFormat(at("F9")).IsZero() {
		t.Error("undo didn't bring column B's format back")
	}

	// To another sheet.
	o, _ := w.AddSheet("Other", 1)
	if _, err := s.MoveTo(o, col(1), at("C1")); err != nil {
		t.Fatal(err)
	}
	if o.DisplayFormat(at("C1")).Kind != FmtCurrency || o.Value(at("C1")).Num != 5 || !s.DisplayFormat(at("B1")).IsZero() {
		t.Errorf("Other!C1 shows %v %v; B1 %v", o.DisplayFormat(at("C1")).Kind, o.Value(at("C1")), s.DisplayFormat(at("B1")).Kind)
	}

	// A block keeps its formats; the source is left plain, as Sheets
	// clears cut cells, though its column stays currency.
	o.Set(at("C2"), "7")
	if _, err := o.MoveTo(s, Rect{From: at("C1"), To: at("C2")}, at("E4")); err != nil {
		t.Fatal(err)
	}
	if s.DisplayFormat(at("E5")).Kind != FmtCurrency || !o.DisplayFormat(at("C1")).IsZero() || o.DisplayFormat(at("C3")).Kind != FmtCurrency {
		t.Errorf("E5 shows %v, Other!C1 %v, Other!C3 %v", s.DisplayFormat(at("E5")).Kind, o.DisplayFormat(at("C1")).Kind, o.DisplayFormat(at("C3")).Kind)
	}
}

// Cutting a block on its sheet leaves the cells it left plain, over
// formatted columns and rows, except where it lands; undo restores them.
func TestMoveBlockLeavesSourcePlain(t *testing.T) {
	s := New()
	s.SetFormat(col(1), Preset(FmtCurrency))
	s.SetStyle(rowRect(1, 1), bold) // row 2
	s.Set(at("B1"), "5")
	s.Set(at("B2"), "6")
	s.Set(at("C3"), "7")
	if _, err := s.Move(Rect{From: at("B1"), To: at("C3")}, at("C2")); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"B1", "B2", "B3", "C1"} {
		if !s.DisplayFormat(at(a)).IsZero() || s.CellStyle(at(a)).Bold {
			t.Errorf("%s shows %v, bold %v, want plain", a, s.DisplayFormat(at(a)).Kind, s.CellStyle(at(a)).Bold)
		}
	}
	if s.DisplayFormat(at("C2")).Kind != FmtCurrency || s.DisplayFormat(at("B4")).Kind != FmtCurrency || !s.CellStyle(at("A2")).Bold {
		t.Errorf("C2 %v (moved from B1), B4 %v, A2 bold %v", s.DisplayFormat(at("C2")).Kind, s.DisplayFormat(at("B4")).Kind, s.CellStyle(at("A2")).Bold)
	}
	s.Undo()
	if s.DisplayFormat(at("B3")).Kind != FmtCurrency || !s.CellStyle(at("B2")).Bold || s.cells.get(at("B3")) != nil {
		t.Errorf("undo: B3 %v, B2 bold %v", s.DisplayFormat(at("B3")).Kind, s.CellStyle(at("B2")).Bold)
	}
}

// Pasting a column format re-infers formulas reading the column's blanks.
func TestPasteColumnReinfers(t *testing.T) {
	s := New()
	s.SetFormat(col(0), Preset(FmtCurrency))
	s.Set(at("C1"), "=D5*2")
	if _, err := s.Paste(s.Copy(col(0)), at1("D1"), false); err != nil {
		t.Fatal(err)
	}
	if f := s.DisplayFormat(at("C1")); f.Kind != FmtCurrency {
		t.Errorf("C1 shows %v, want currency from column D", f.Kind)
	}
}
