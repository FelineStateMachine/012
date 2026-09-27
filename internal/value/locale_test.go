package value_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
)

func loc(t *testing.T, tag string) *locale.Locale {
	t.Helper()
	l, ok := locale.Lookup(tag)
	if !ok {
		t.Fatalf("no locale %s", tag)
	}
	return l
}

func TestParseValueInLocales(t *testing.T) {
	date := numfmt.DateSerial(2026, 9, 26)
	cases := []struct {
		tag, typed string
		ok         bool
		v          float64
		kind       value.FormatKind
		canonical  string
	}{
		{"en-US", "1,234.5", true, 1234.5, value.FmtNumber, "1,234.5"},
		{"en-US", "1,5", false, 0, 0, ""},
		{"de-DE", "1.234,5", true, 1234.5, value.FmtNumber, "1,234.5"},
		{"de-DE", "1,5", true, 1.5, value.FmtAuto, "1.5"},
		{"de-DE", "-0,25", true, -0.25, value.FmtAuto, "-0.25"},
		{"de-DE", "12,5 %", true, 0.125, value.FmtPercent, "12.5%"},
		{"de-DE", "1.234,50 €", true, 1234.5, value.FmtCurrency, "$1,234.50"},
		{"de-DE", "-5 €", true, -5, value.FmtCurrency, "-$5"},
		{"de-DE", "1,5E3", true, 1500, value.FmtScientific, "1.5E3"},
		{"de-DE", "1,234.5", false, 0, 0, ""},
		{"de-DE", "$5", false, 0, 0, ""},
		{"de-DE", "26.09.2026", true, date, value.FmtDate, "09/26/2026"},
		{"de-DE", "26.9.2026.", true, date, value.FmtDate, "9/26/2026"},
		{"de-DE", "26/09/2026", true, date, value.FmtDate, "09/26/2026"},
		{"de-DE", "9/26/2026", false, 0, 0, ""},
		{"de-DE", "2026-09-26", true, date, value.FmtDate, "2026-09-26"},
		{"de-DE", "Sep 26, 2026", true, date, value.FmtDate, "Sep 26, 2026"},
		{"de-DE", "26.09.2026 14:30", true, date + numfmt.TimeSerial(14, 30, 0), value.FmtDateTime, "09/26/2026 14:30"},
		{"de-DE", "14:30", true, numfmt.TimeSerial(14, 30, 0), value.FmtTime, "14:30"},
		{"en-GB", "26/09/2026", true, date, value.FmtDate, "09/26/2026"},
		{"en-GB", "9/26/2026", false, 0, 0, ""},
		{"en-GB", "£1,200", true, 1200, value.FmtCurrency, "$1,200"},
		{"en-GB", "$1,200", false, 0, 0, ""},
		{"en-GB", "1.5", true, 1.5, value.FmtAuto, "1.5"},
		{"fr-FR", "1 234,5", true, 1234.5, value.FmtNumber, "1,234.5"},
		{"fr-FR", "1 234,5", true, 1234.5, value.FmtNumber, "1,234.5"},
		{"fr-FR", "12 %", true, 0.12, value.FmtPercent, "12%"},
		{"fr-FR", "3,50 €", true, 3.5, value.FmtCurrency, "$3.50"},
		{"fr-FR", "1.5", false, 0, 0, ""},
		{"es-ES", "1.000,25", true, 1000.25, value.FmtNumber, "1,000.25"},
		{"it-IT", "26/9", true, numfmt.DateSerial(value.Now().Year(), 9, 26), value.FmtDate, "9/26"},
		{"pt-BR", "R$ 1.234,56", true, 1234.56, value.FmtCurrency, "$1,234.56"},
		{"nl-NL", "26-9-2026", true, date, value.FmtDate, "9/26/2026"},
		{"nl-NL", "€ 5", true, 5, value.FmtCurrency, "$5"},
		{"sv-SE", "2026-09-26", true, date, value.FmtDate, "09/26/2026"},
		{"sv-SE", "100 kr", true, 100, value.FmtCurrency, "$100"},
		{"pl-PL", "1 000,5 zł", true, 1000.5, value.FmtCurrency, "$1,000.5"},
		{"de-CH", "1'234.5", true, 1234.5, value.FmtNumber, "1,234.5"},
		{"de-CH", "CHF 5", true, 5, value.FmtCurrency, "$5"},
		{"ja-JP", "2026/9/26", true, date, value.FmtDate, "9/26/2026"},
		{"ja-JP", "¥1,000", true, 1000, value.FmtCurrency, "$1,000"},
		{"zh-CN", "26/9/26", true, date, value.FmtDate, "9/26/26"},
		{"de-CH", "26.9", true, 26.9, value.FmtAuto, "26.9"},
	}
	for _, c := range cases {
		l := loc(t, c.tag)
		v, f, ok := value.ParseValueIn(c.typed, l)
		if ok != c.ok {
			t.Errorf("%s %q: parsed %v, want %v", c.tag, c.typed, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if v != c.v || f.Kind != c.kind {
			t.Errorf("%s %q = %v %v, want %v %v", c.tag, c.typed, v, f.Kind, c.v, c.kind)
		}
		got, _ := value.Canonicalize(c.typed, l)
		if got != c.canonical {
			t.Errorf("%s %q: canonical %q, want %q", c.tag, c.typed, got, c.canonical)
		}
	}
}

// TestLocalizeRoundTrips checks that every entry as stored, written in
// each locale's form, is typed back to the same value and format.
func TestLocalizeRoundTrips(t *testing.T) {
	stored := []string{"1234.5", "1,234.5", "$1,234.50", "-$5", "-12.5%", "1.5E3", "0.001",
		"9/26/2026", "09/26/2026", "9/26", "2026-09-26", "Sep 26, 2026", "26 Sep 2026",
		"14:30", "2:30 PM", "25:30", "9/26/2026 14:30:05", "14:30:15.5"}
	for _, l := range locale.All() {
		for _, s := range stored {
			want, wf, ok := value.ParseValue(s)
			if !ok {
				t.Fatalf("%q isn't a value", s)
			}
			shown := value.Localize(s, l)
			got, gf, ok := value.ParseValueIn(shown, l)
			if !ok || got != want || gf.Kind != wf.Kind {
				t.Errorf("%s: %q shown as %q reads %v %v %v, want %v %v", l.Tag, s, shown, got, gf.Kind, ok, want, wf.Kind)
			}
		}
	}
}

func TestLocalizeForms(t *testing.T) {
	cases := []struct{ tag, stored, shown string }{
		{"en-US", "$1,234.50", "$1,234.50"},
		{"de-DE", "$1,234.50", "1.234,50 €"},
		{"de-DE", "-12.5%", "-12,5%"},
		{"de-DE", "9/26/2026", "26.9.2026"},
		{"de-DE", "9/26/2026 14:30:15.5", "26.9.2026 14:30:15,5"},
		{"de-DE", "2026-09-26", "2026-09-26"},
		{"de-DE", "hello", "hello"},
		{"en-GB", "$5", "£5"},
		{"fr-FR", "1,234.5", "1 234,5"},
		{"pt-BR", "$5", "R$ 5"},
		{"ja-JP", "9/26/2026", "2026/9/26"},
		{"nl-NL", "9/26", "26-9"},
	}
	for _, c := range cases {
		if got := value.Localize(c.stored, loc(t, c.tag)); got != c.shown {
			t.Errorf("%s %q shown as %q, want %q", c.tag, c.stored, got, c.shown)
		}
	}
}

func TestDisplayInLocales(t *testing.T) {
	v := 1234.5
	date := numfmt.DateSerial(2026, 9, 26) + numfmt.TimeSerial(14, 5, 0)
	cases := []struct {
		tag  string
		f    value.Format
		n    float64
		want string
	}{
		{"en-US", value.Preset(value.FmtNumber), v, "1,234.50"},
		{"en-US", value.Preset(value.FmtCurrency), v, "$1,234.50"},
		{"en-US", value.Preset(value.FmtDate), date, "9/26/2026"},
		{"en-US", value.Preset(value.FmtTime), date, "2:05:00 PM"},
		{"de-DE", value.Preset(value.FmtNumber), v, "1.234,50"},
		{"de-DE", value.Preset(value.FmtPercent), 0.125, "12,50%"},
		{"de-DE", value.Preset(value.FmtScientific), v, "1,23E+03"},
		{"de-DE", value.Preset(value.FmtCurrency), -v, "-1.234,50 €"},
		{"de-DE", value.Preset(value.FmtDate), date, "26.9.2026"},
		{"de-DE", value.Preset(value.FmtTime), date, "14:05:00"},
		{"de-DE", value.Preset(value.FmtDateTime), date, "26.9.2026 14:05:00"},
		{"de-DE", value.Format{Kind: value.FmtDate, Pattern: "yyyy-mm-dd"}, date, "2026-09-26"},
		{"de-DE", value.Format{Kind: value.FmtCustom, Pattern: `"$"#,##0.0`}, v, "$1.234,5"},
		{"fr-FR", value.Preset(value.FmtNumber), v, "1 234,50"},
		{"en-GB", value.Preset(value.FmtCurrency), v, "£1,234.50"},
		{"en-GB", value.Preset(value.FmtDate), date, "26/9/2026"},
		{"pt-BR", value.Preset(value.FmtCurrency), v, "R$ 1.234,50"},
		{"sv-SE", value.Preset(value.FmtDate), date, "2026-09-26"},
		{"ja-JP", value.Preset(value.FmtDate), date, "2026/9/26"},
		{"de-CH", value.Preset(value.FmtNumber), v, "1’234.50"},
	}
	for _, c := range cases {
		l := loc(t, c.tag)
		if got := numfmt.FormatIn(c.n, c.f.CodeIn(l), l); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.tag, c.f, got, c.want)
		}
	}
}

func TestAccountingInLocales(t *testing.T) {
	cases := []struct {
		tag  string
		v    float64
		want string
	}{
		{"en-US", 1234.5, " $  1,234.50 "},
		{"en-US", -1234.5, " $ (1,234.50)"},
		{"de-DE", 1234.5, "  1.234,50 € "},
		{"de-DE", 0, "       -   € "},
		{"pt-BR", 5, " R$     5,00 "},
	}
	for _, c := range cases {
		got, ok := numfmt.AccountingIn(c.v, 2, 13, loc(t, c.tag))
		if !ok || got != c.want {
			t.Errorf("%s %v: %q %v, want %q", c.tag, c.v, got, ok, c.want)
		}
	}
}

// TestLocaleDocTable checks the table of locales in docs/sheets/locale.md
// shows each as it renders.
func TestLocaleDocTable(t *testing.T) {
	doc, err := os.ReadFile("../../docs/sheets/locale.md")
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	date := numfmt.DateSerial(2026, 9, 26)
	for _, l := range locale.All() {
		fmt.Fprintf(&want, "| `%s` | %s | %s | %s | %s |\n", l.Tag, l.Name,
			numfmt.FormatIn(1234.56, "#,##0.00", l),
			numfmt.FormatIn(date, value.Preset(value.FmtDate).CodeIn(l), l),
			numfmt.FormatIn(1234.56, value.Preset(value.FmtCurrency).CodeIn(l), l))
	}
	if !strings.Contains(string(doc), want.String()) {
		t.Errorf("docs/sheets/locale.md's table isn't:\n%s", want.String())
	}
}
