package fileio

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// TestXLSXLocaleFormats checks a workbook in de-DE is downloaded with
// its Currency and Date formats in de-DE's codes, which read back as
// those formats in de-DE, and keep their euro and day-first order as
// patterns in a workbook in en-US.
func TestXLSXLocaleFormats(t *testing.T) {
	src := build(t, map[string]string{"A1": "$1,234.50", "B1": "9/26/2026", "C1": "14:30:00", "D1": "$5"})
	if !src.Book().SetLocale("de-DE") {
		t.Fatal("no de-DE")
	}
	r := func(s string) sheet.Rect { return sheet.NewRect(addr(t, s), addr(t, s)) }
	src.SetFormat(r("C1"), sheet.Format{Kind: sheet.FmtTime})
	de, _ := locale.Lookup("de-DE")
	for f, want := range map[sheet.Format]string{
		{Kind: sheet.FmtCurrency, Decimals: 2}: `#,##0.00\ [$€-407]`,
		{Kind: sheet.FmtDate}:                  "[$-407]d.m.yyyy",
		{Kind: sheet.FmtTime}:                  "[$-407]hh:mm:ss",
	} {
		if got := excelCodeIn(f, de); got != want {
			t.Errorf("%+v in de-DE = %q, want %q", f, got, want)
		}
	}
	name := filepath.Join(t.TempDir(), "de.xlsx")
	if _, err := Export(context.Background(), name, XLSX, Snap(src, sheet.Rect{}, "De"), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := Import(context.Background(), name, Options{Locale: de})
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range []string{"A1", "B1", "C1", "D1"} {
		a := addr(t, cell)
		if g, w := got.Sheet.Cell(a).Format, src.Cell(a).Format; g != w {
			t.Errorf("de-DE %s format %+v, want %+v", cell, g, w)
		}
	}
	got, err = Import(context.Background(), name, Options{Locale: locale.Canonical})
	if err != nil {
		t.Fatal(err)
	}
	for cell, want := range map[string]string{"A1": "1,234.50 €", "B1": "26.9.2026"} {
		a := addr(t, cell)
		c := got.Sheet.Cell(a)
		if g := sheet.FormatText(c.Value, c.Format); g != want {
			t.Errorf("en-US %s shows %q, want %q", cell, g, want)
		}
	}
}
