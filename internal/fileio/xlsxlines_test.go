package fileio

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Column and row formats go to Excel as column and row styles and come
// back as line formats, without a cell per formatted blank.
func TestXLSXLineFormats(t *testing.T) {
	src := build(t, map[string]string{"A1": "Item", "B1": "Price", "B2": "3.5", "A3": "total"})
	src.SetFormat(sheet.Rect{From: sheet.Addr{Col: 1}, To: sheet.Addr{Col: 2, Row: sheet.MaxRows - 1}}, sheet.Preset(sheet.FmtCurrency))
	src.SetStyle(sheet.Rect{From: sheet.Addr{Row: 2}, To: sheet.Addr{Col: sheet.MaxCols - 1, Row: 2}}, func(st *sheet.Style) { st.Bold = true })
	src.SetStyle(sheet.Rect{From: sheet.Addr{Row: 99}, To: sheet.Addr{Col: sheet.MaxCols - 1, Row: 99}}, func(st *sheet.Style) { st.Italic = true })

	name := filepath.Join(t.TempDir(), "lines.xlsx")
	if _, err := Export(context.Background(), name, XLSX, Snap(src, sheet.Rect{}, "Lines"), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := x.GetColStyle("Lines", "C"); id == 0 {
		t.Error("column C has no style in Excel")
	}
	x.Close()

	res, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := res.Sheet
	for _, a := range []string{"B9", "C1048576"} {
		if f := got.DisplayFormat(addr(t, a)); f.Kind != sheet.FmtCurrency {
			t.Errorf("%s shows %v, want currency", a, f.Kind)
		}
	}
	if !got.CellStyle(addr(t, "D3")).Bold || !got.CellStyle(addr(t, "Q100")).Italic || got.CellStyle(addr(t, "D4")).Bold {
		t.Error("row styles didn't come back")
	}
	if n := len(got.ColFormats()); n != 2 {
		t.Errorf("%d column formats, want 2", n)
	}

	// The whole sheet's format goes out on every column and comes back
	// as the sheet's.
	src.SetStyle(sheet.Rect{To: sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}}, func(st *sheet.Style) { st.Underline = true })
	if _, err := Export(context.Background(), name, XLSX, Snap(src, sheet.Rect{}, "Lines"), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	if res, err = Import(context.Background(), name, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, st := res.Sheet.SheetFormat(); !st.Underline || !res.Sheet.CellStyle(addr(t, "ZZ9")).Underline {
		t.Errorf("the sheet's style came back as %+v", st)
	}
	if f := res.Sheet.DisplayFormat(addr(t, "C9")); f.Kind != sheet.FmtCurrency || !res.Sheet.CellStyle(addr(t, "C9")).Underline {
		t.Errorf("column C came back as %v", f.Kind)
	}
}
