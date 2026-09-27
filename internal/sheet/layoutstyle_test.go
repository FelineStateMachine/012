package sheet

import (
	"strings"
	"testing"
	"unsafe"
)

// A Cell stays in its allocation size class with a style of twelve
// bytes, which fits in what the flags after it would pad.
func TestCellSize(t *testing.T) {
	if n := unsafe.Sizeof(Cell{}); n > 256 {
		t.Errorf("Cell is %d bytes", n)
	}
	if n := unsafe.Sizeof(Style{}); n > 12 {
		t.Errorf("Style is %d bytes", n)
	}
}

// Vertical alignment and border colors are saved, undone and cleared
// with the rest of a cell's style.
func TestVAlignAndBorderColors(t *testing.T) {
	s := New()
	s.Set(at("B2"), "x")
	s.SetStyle(rng("B2"), func(st *Style) { st.VAlign = VAlignTop })
	s.SetBorderStroke(rng("B2:C3"), BorderOuter, Stroke{Line: LineThick, Color: ColorRed})
	s.SetBorderStroke(rng("B2:C3"), BorderInner, Stroke{Line: LineThin})
	b := s.CellStyle(at("B2")).Borders
	if b.Stroke(EdgeTop) != (Stroke{LineThick, ColorRed}) || b.Stroke(EdgeBottom) != (Stroke{Line: LineThin}) {
		t.Errorf("B2 borders %x", uint32(b))
	}
	if st := s.StrokeAbove(at("B4")); st != (Stroke{LineThick, ColorRed}) {
		t.Errorf("below the outline %+v", st)
	}
	got := roundTrip(t, s)
	if st := got.CellStyle(at("B2")); st.VAlign != VAlignTop || st.Borders != b {
		t.Errorf("read back %+v", st)
	}
	var buf strings.Builder
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"valign":"top"`) || !strings.Contains(buf.String(), `"topColor":"red"`) {
		t.Errorf("file:\n%s", buf.String())
	}
	s.Book().Undo()
	s.Book().Undo()
	if s.CellStyle(at("B2")).Borders != 0 {
		t.Error("undo kept the borders")
	}
	// A line removed takes its color with it.
	if b := Borders(0).WithStroke(EdgeLeft, Stroke{LineThin, ColorBlue}).With(EdgeLeft, LineNone); b != 0 {
		t.Errorf("no line keeps color %x", uint32(b))
	}
	for v, want := range map[VAlign][3]int{VAlignAuto: {3, 3, 1}, VAlignTop: {0, 0, 0}, VAlignMiddle: {1, 0, 1}, VAlignBottom: {3, 3, 3}} {
		if got := [3]int{v.Offset(1, 4, false), v.Offset(3, 4, false) * 3, v.Offset(1, 4, true)}; got != want {
			t.Errorf("%v offsets %v, want %v", v, got, want)
		}
	}
	for _, name := range []string{"", "top", "middle", "bottom"} {
		if v, ok := ParseVAlign(name); !ok || v.String() != name {
			t.Errorf("valign %q", name)
		}
	}
}

// The outline of whole columns or rows draws the sheet's first and last
// edges, as lines where it covers the whole sheet.
func TestSheetEdgeBorders(t *testing.T) {
	s := New()
	cols := Rect{From: Addr{Col: 1}, To: Addr{Col: 2, Row: MaxRows - 1}}
	s.SetBorders(cols, BorderOuter, LineThick)
	if s.EdgeAbove(Addr{Col: 1}) != LineThick || s.EdgeAbove(Addr{Col: 2}) != LineThick || s.EdgeAbove(Addr{Col: 3}) != LineNone {
		t.Error("no top edge over whole columns")
	}
	if s.CellStyle(Addr{Col: 2, Row: MaxRows - 1}).Borders.Bottom() != LineThick || s.EdgeAbove(Addr{Col: 1, Row: 5}) != LineNone {
		t.Error("bottom edge of whole columns")
	}
	if s.EdgeLeft(Addr{Col: 1, Row: 7}) != LineThick || s.EdgeLeft(Addr{Col: 3, Row: 7}) != LineThick {
		t.Error("sides of whole columns")
	}
	rows := Rect{From: Addr{Row: 4}, To: Addr{Col: MaxCols - 1, Row: 5}}
	s.SetBorders(rows, BorderLeft, LineDouble)
	if s.EdgeLeft(Addr{Row: 4}) != LineDouble || s.EdgeLeft(Addr{Row: 6}) != LineNone {
		t.Error("left edge of whole rows")
	}
	all := Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}}
	s.SetBorders(all, BorderOuter, LineThin)
	if s.EdgeAbove(Addr{Col: 900}) != LineThin || s.EdgeLeft(Addr{Row: 900}) != LineThin {
		t.Error("the whole sheet's outline")
	}
	if n := s.Len(); n > 16 {
		t.Errorf("%d cells for the outlines", n)
	}
}
