package formula

import (
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

// TestFormulaLocaleRoundTrip types formulas in each locale, stores them
// canonically, parses them, and prints them back in the locale.
func TestFormulaLocaleRoundTrip(t *testing.T) {
	cases := []struct{ tag, typed, stored string }{
		{"de-DE", "=ROUND(1,5; 0)", "=ROUND(1.5, 0)"},
		{"de-DE", "=SUM(A1:A3;0,25)*2", "=SUM(A1:A3,0.25)*2"},
		{"de-DE", `=IF(A1>1,5;"a,b;c";'Q3; x'!B2)`, `=IF(A1>1.5,"a,b;c",'Q3; x'!B2)`},
		{"de-DE", "=SUM({1\\2,5;3\\4})", "=SUM({1,2.5;3,4})"},
		{"de-DE", "=1,5E-3+,5", "=1.5E-3+.5"},
		{"de-DE", "=SUM(Sheet2!A1..B3;Tax.Rate)", "=SUM(Sheet2!A1..B3,Tax.Rate)"},
		{"fr-FR", "=SUM(1,25; 2)", "=SUM(1.25, 2)"},
		{"pt-BR", "=IF(A1;1,5;2,5)", "=IF(A1,1.5,2.5)"},
		{"en-GB", "=ROUND(1.5, 0)", "=ROUND(1.5, 0)"},
		{"ja-JP", "={1,2;3,4}", "={1,2;3,4}"},
		{"en-US", "=ROUND(1.5, 0)", "=ROUND(1.5, 0)"},
	}
	for _, c := range cases {
		l := mustLocale(t, c.tag)
		stored := Delocalize(c.typed, l)
		if stored != c.stored {
			t.Errorf("%s: %q stored as %q, want %q", c.tag, c.typed, stored, c.stored)
			continue
		}
		if len(stored) != len(c.typed) {
			t.Errorf("%s: %q changed length", c.tag, c.typed)
		}
		if back := Localize(stored, l); back != c.typed {
			t.Errorf("%s: %q shown as %q, want %q", c.tag, stored, back, c.typed)
		}
		n, err := Parse(stored, testFuncs)
		if err != nil {
			t.Errorf("%s: %q: %v", c.tag, stored, err)
			continue
		}
		// The printer's canonical text, localized, reads back the same.
		printed := Localize(Text(n), l)
		n2, err := Parse(Delocalize(printed, l), testFuncs)
		if err != nil || Text(n2) != Text(n) {
			t.Errorf("%s: printed %q reads back as %v %v", c.tag, printed, n2, err)
		}
	}
}

// TestLocalizePartial translates formulas still being typed.
func TestLocalizePartial(t *testing.T) {
	de := mustLocale(t, "de-DE")
	for typed, want := range map[string]string{
		"=SUM(1,5;":  "=SUM(1.5,",
		`=IF(A1;"x;`: `=IF(A1,"x;`,
		"={1\\":      "={1,",
		"=ROUND(":    "=ROUND(",
	} {
		if got := Delocalize(typed, de); got != want {
			t.Errorf("%q = %q, want %q", typed, got, want)
		}
	}
}
