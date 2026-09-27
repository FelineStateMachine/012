package sheet

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
)

func mustLocale(t *testing.T, tag string) *locale.Locale {
	t.Helper()
	l, ok := locale.Lookup(tag)
	if !ok {
		t.Fatalf("no locale %s", tag)
	}
	return l
}

// TestEntriesInLocales types entries in a locale, stores them, and checks
// the value, the format and what editing the cell shows again.
func TestEntriesInLocales(t *testing.T) {
	cases := []struct {
		tag, typed, stored, shown string
		kind                      FormatKind
	}{
		{"de-DE", "1.234,5", "1,234.5", "1.234,5", FmtNumber},
		{"de-DE", "12,50 €", "$12.50", "12,50 €", FmtCurrency},
		{"de-DE", "26.09.2026", "09/26/2026", "26.09.2026", FmtDate},
		{"de-DE", "=SUM(1;2)", "=SUM(1,2)", "=SUM(1;2)", FmtAuto},
		{"de-DE", "=ROUND(A1*1,19; 2)", "=ROUND(A1*1.19, 2)", "=ROUND(A1*1,19; 2)", FmtAuto},
		{"de-DE", "-A1*0,5", "-A1*0.5", "-A1*0,5", FmtAuto},
		{"de-DE", "Hallo, Welt", "Hallo, Welt", "Hallo, Welt", FmtAuto},
		{"fr-FR", "1.5", "'1.5", "1.5", FmtAuto},
		{"de-DE", "1,234.5", "'1,234.5", "1,234.5", FmtAuto},
		{"de-DE", "'1,5", "'1,5", "'1,5", FmtAuto},
		{"de-DE", "TRUE", "TRUE", "TRUE", FmtAuto},
		{"en-GB", "26/09/2026", "09/26/2026", "26/09/2026", FmtDate},
		{"en-GB", "9/26/2026", "'9/26/2026", "9/26/2026", FmtAuto},
		{"en-US", "1,234.5", "1,234.5", "1,234.5", FmtNumber},
	}
	for _, c := range cases {
		l := mustLocale(t, c.tag)
		stored := CanonicalEntry(c.typed, l)
		if stored != c.stored {
			t.Errorf("%s %q stored as %q, want %q", c.tag, c.typed, stored, c.stored)
			continue
		}
		if shown := LocalEntry(stored, l); shown != c.shown {
			t.Errorf("%s %q shown as %q, want %q", c.tag, stored, shown, c.shown)
		}
		s := New()
		if err := s.Set(at("A1"), stored); err != nil {
			if !strings.Contains(c.typed, "SUM") {
				t.Errorf("%s %q: %v", c.tag, c.typed, err)
			}
			continue
		}
		if k := s.Cell(at("A1")).Format.Kind; k != c.kind {
			t.Errorf("%s %q: format %v, want %v", c.tag, c.typed, k, c.kind)
		}
	}
}

// TestSetLocale switches a workbook's locale: values stay, the display
// follows, undo puts it back, and the file keeps it.
func TestSetLocale(t *testing.T) {
	s := New()
	for a, in := range map[string]string{"A1": "1234.5", "A2": "$12.50", "A3": "9/26/2026", "A4": "=A1/4"} {
		if err := s.Set(at(a), in); err != nil {
			t.Fatal(err)
		}
	}
	s.SetFormat(NewRect(at("A1"), at("A1")), Preset(FmtNumber))
	shown := func(a string) string {
		text, _ := DisplayIn(s.Cell(at(a)).Value, s.DisplayFormat(at(a)), 20, s.Locale())
		return text
	}
	before := s.Cell(at("A4")).Value
	if !s.Book().SetLocale("de-DE") {
		t.Fatal("de-DE unknown")
	}
	if got := s.Cell(at("A4")).Value; got != before {
		t.Errorf("A4 changed to %v", got)
	}
	for a, want := range map[string]string{"A1": "1.234,50", "A2": "12,50 €", "A3": "26.9.2026", "A4": "308,63"} {
		if got := shown(a); got != want {
			t.Errorf("de-DE %s = %q, want %q", a, got, want)
		}
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"locale": "de-DE"`) || !strings.Contains(buf.String(), `"A4": "=A1/4"`) {
		t.Errorf("file:\n%s", buf.String())
	}
	back, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if back.Book().LocaleTag() != "de-DE" || back.Cell(at("A4")).Value != before {
		t.Errorf("read back %q, %v", back.Book().LocaleTag(), back.Cell(at("A4")).Value)
	}
	if _, ok := s.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if s.Book().LocaleTag() != "" || shown("A1") != "1,234.50" {
		t.Errorf("after undo: %q, %q", s.Book().LocaleTag(), shown("A1"))
	}
	if s.Book().SetLocale("xx-YY") {
		t.Error("xx-YY accepted")
	}
}

// TestDefaultLocale follows the config's locale unless the file names
// its own.
func TestDefaultLocale(t *testing.T) {
	defer SetDefaultLocale(nil)
	SetDefaultLocale(mustLocale(t, "fr-FR"))
	w := NewBook()
	if w.Locale().Tag != "fr-FR" {
		t.Errorf("new workbook in %s", w.Locale().Tag)
	}
	w.SetLocale("ja-JP")
	if w.Locale().Tag != "ja-JP" {
		t.Errorf("workbook in %s", w.Locale().Tag)
	}
	SetDefaultLocale(nil)
	if NewBook().Locale().Tag != "en-US" {
		t.Error("the default isn't en-US")
	}
}
