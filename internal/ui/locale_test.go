package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// File > Settings > Locale picks German (Germany): the sheet shows
// decimal commas without changing a value, entries and formulas are
// typed the German way and stored the en-US way, the formula bar and
// editing show them German again, and undo goes back to en-US.
func TestLocaleSetting(t *testing.T) {
	m := newModel()
	press(t, m, "1234.5", "<enter>", "=A1/4", "<enter>")
	press(t, m, "<alt+f>", "<up>", "<up>", "<right>", "<down>", "<enter>")
	if !strings.Contains(screen(m), "German (Germany)") {
		t.Fatalf("no locale picker:\n%s", screen(m))
	}
	press(t, m, "Germany", "<enter>")
	if tag := m.book().LocaleTag(); tag != "de-DE" || !strings.Contains(line(m, contextLine), "1.234,56") {
		t.Fatalf("locale %q, context %q", tag, line(m, contextLine))
	}
	if v := m.sheet.Value(addr("A2")).Num; v != 308.625 || !strings.Contains(screen(m), "308,625") {
		t.Errorf("A2 = %v:\n%s", v, screen(m))
	}
	press(t, m, "1,5", "<enter>")
	if input(m, "A3") != "1.5" || m.sheet.Value(addr("A3")).Num != 1.5 {
		t.Errorf("A3 %q = %v", input(m, "A3"), m.sheet.Value(addr("A3")))
	}
	press(t, m, "=ROUND(A1;")
	if ctx := line(m, contextLine); !strings.HasPrefix(ctx, "ROUND(value; [places])") {
		t.Errorf("signature %q", ctx)
	}
	press(t, m, "<backspace>", "*1,5; 1)", "<enter>")
	if input(m, "A4") != "=ROUND(A1*1.5, 1)" || m.sheet.Value(addr("A4")).Num != 1851.8 {
		t.Errorf("A4 %q = %v", input(m, "A4"), m.sheet.Value(addr("A4")))
	}
	m.cur = addr("A4")
	if bar := line(m, formulaLine); !strings.Contains(bar, "=ROUND(A1*1,5; 1)") {
		t.Errorf("formula bar %q", bar)
	}
	press(t, m, "<f2>")
	if m.line.Text() != "=ROUND(A1*1,5; 1)" {
		t.Errorf("editing %q", m.line.Text())
	}
	press(t, m, "<esc>", "<ctrl+z>", "<ctrl+z>", "<ctrl+z>")
	if tag := m.book().LocaleTag(); tag != "" || !strings.Contains(screen(m), "308.625") {
		t.Errorf("after undo: locale %q\n%s", tag, screen(m))
	}
}

// Pasted text is read in the workbook's locale, and copied values are
// written in it.
func TestLocalePasteAndCopy(t *testing.T) {
	m := newModel()
	m.book().SetLocale("de-DE")
	m.handlePaste("1,5\t26.09.2026\n2.000\tText")
	if v := m.sheet.Value(addr("A1")).Num; v != 1.5 {
		t.Errorf("A1 = %v", v)
	}
	if f := m.sheet.Cell(addr("B1")).Format; f.Kind != sheet.FmtDate {
		t.Errorf("B1 format %v", f)
	}
	if v := m.sheet.Value(addr("A2")).Num; v != 2000 {
		t.Errorf("A2 = %v", v)
	}
	clip := m.sheet.Copy(sheet.NewRect(addr("A1"), addr("A2")))
	if got := formatTSV(clip.TextIn(m.locale())); got != "1,5\n2000" {
		t.Errorf("copied %q", got)
	}
}

// In German, a formula that doesn't parse says which separator was
// expected in German's syntax, and dates show German month names.
func TestLocaleErrorsAndNames(t *testing.T) {
	m := newModel()
	m.book().SetLocale("de-DE")
	press(t, m, "=SUM(1;2 3)", "<enter>")
	if ctx := line(m, contextLine); !strings.Contains(ctx, "Expected ; or ) in SUM") {
		t.Errorf("context %q", ctx)
	}
	press(t, m, "<esc>", "26.10.2026", "<enter>")
	m.sheet.SetFormat(sheet.NewRect(addr("A1"), addr("A1")), sheet.Format{Kind: sheet.FmtCustom, Pattern: "dddd d. mmmm"})
	m.sheet.SetColWidth(0, 22)
	if !strings.Contains(screen(m), "Montag 26. Oktober") {
		t.Errorf("no German names:\n%s", screen(m))
	}
}
