package fileio

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

func rangeOf(t testing.TB, s string) sheet.Rect {
	t.Helper()
	r, ok := sheet.ParseRange(s)
	if !ok {
		t.Fatalf("bad range %q", s)
	}
	return r
}

// Wrapping, borders, heights and merges go out to Excel and come back.
func TestXLSXLayoutRoundTrip(t *testing.T) {
	src := build(t, map[string]string{"A1": "Title", "A2": "a long text that wraps", "B2": "2"})
	src.Merge(rangeOf(t, "A1:C1"), sheet.MergeAll)
	src.SetStyle(rangeOf(t, "A2"), func(st *sheet.Style) { st.Wrap = sheet.WrapOn })
	src.SetBorders(rangeOf(t, "A2:C3"), sheet.BorderAll, sheet.LineThin)
	src.SetBorders(rangeOf(t, "A2:C3"), sheet.BorderOuter, sheet.LineThick)
	src.SetBorders(rangeOf(t, "B2"), sheet.BorderBottom, sheet.LineDouble)
	src.SetRowHeight(4, 4, 3)

	name := filepath.Join(t.TempDir(), "layout.xlsx")
	if _, err := Export(context.Background(), name, XLSX, SnapBook(src), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	ws := x.GetSheetName(0)
	if mc, _ := x.GetMergeCells(ws); len(mc) != 1 || mc[0].GetStartAxis() != "A1" || mc[0].GetEndAxis() != "C1" {
		t.Errorf("merges in Excel: %v", mc)
	}
	if h, _ := x.GetRowHeight(ws, 5); h != 45 {
		t.Errorf("row 5 is %v points", h)
	}
	id, _ := x.GetCellStyle(ws, "C3") // a blank cell drawing lines
	st, _ := x.GetStyle(id)
	if st == nil || len(st.Border) == 0 {
		t.Errorf("C3's borders didn't go to Excel: %+v", st)
	}
	id, _ = x.GetCellStyle(ws, "A2")
	if st, _ = x.GetStyle(id); st == nil || st.Alignment == nil || !st.Alignment.WrapText {
		t.Errorf("A2 doesn't wrap in Excel: %+v", st)
	}
	x.Close()

	res, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Sheet
	if !slices.Equal(got.Merges(), src.Merges()) {
		t.Errorf("merges %v", got.Merges())
	}
	if h, _ := got.RowHeight(4); h != 3 {
		t.Errorf("row 5 height %d", h)
	}
	for _, a := range []string{"A2", "B2", "C2", "A3", "B3", "C3", "B4"} {
		if g, w := got.CellStyle(addr(t, a)), src.CellStyle(addr(t, a)); g != w {
			t.Errorf("%s style %+v, want %+v", a, g, w)
		}
	}
}

// What Excel writes: its other line styles, heights it fitted itself
// (not read) and ones set by hand, and merged cells.
func TestXLSXLayoutImport(t *testing.T) {
	p := oneSheetParts("", `<row r="1" ht="45" customHeight="1"><c r="A1" s="1"><v>1</v></c><c r="B1" s="2"/></row>`+
		`<row r="2" ht="60"><c r="A2" s="3" t="inlineStr"><is><t>fits</t></is></c></row>`+
		`<row r="3" ht="15.75" customHeight="1"><c r="A3" s="1"/><c r="D3"><v>4</v></c></row>`)
	p["xl/styles.xml"] = `<styleSheet ` + mainNS + `><fonts count="1"><font/></fonts>` +
		`<borders count="3"><border><left/><right/><top/><bottom/></border>` +
		`<border><left style="hair"/><right style="mediumDashed"/><top style="double"/><bottom style="thick"/></border>` +
		`<border><left style="dashDot"/></border></borders>` +
		`<cellXfs count="4"><xf numFmtId="0" fontId="0" borderId="0"/><xf numFmtId="0" fontId="0" borderId="1" applyBorder="1"/>` +
		`<xf numFmtId="0" fontId="0" borderId="2"/><xf numFmtId="0" fontId="0" borderId="0"><alignment wrapText="1"/></xf></cellXfs></styleSheet>`
	p["xl/worksheets/sheet1.xml"] = p["xl/worksheets/sheet1.xml"][:len(p["xl/worksheets/sheet1.xml"])-len(`</worksheet>`)] +
		`<mergeCells count="2"><mergeCell ref="B2:C3"/><mergeCell ref="$E$5:$E$9"/></mergeCells></worksheet>`
	name := writeParts(t, t.TempDir(), "excel.xlsx", p)
	res, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sheet
	want := sheet.Borders{Left: sheet.LineThin, Right: sheet.LineThick, Top: sheet.LineDouble, Bottom: sheet.LineThick}
	if got := s.CellStyle(addr(t, "A1")).Borders; got != want {
		t.Errorf("A1 borders %+v, want %+v", got, want)
	}
	if got := s.CellStyle(addr(t, "B1")).Borders.Left; got != sheet.LineThin {
		t.Errorf("a blank cell's border after the row's values: %v", got)
	}
	if s.CellStyle(addr(t, "A2")).Wrap != sheet.WrapOn {
		t.Error("wrapText not read")
	}
	if h, ok := s.RowHeight(0); !ok || h != 3 {
		t.Errorf("row 1 height %d, %v", h, ok)
	}
	for _, row := range []int{1, 2} {
		if _, ok := s.RowHeight(row); ok {
			t.Errorf("row %d has a height", row+1)
		}
	}
	if got := s.Merges(); len(got) != 2 || got[1] != rangeOf(t, "E5:E9") {
		t.Errorf("merges %v", got)
	}
}
