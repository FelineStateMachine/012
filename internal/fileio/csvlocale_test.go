package fileio

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/sheet"
)

func mustLocale(t *testing.T, tag string) *locale.Locale {
	t.Helper()
	l, ok := locale.Lookup(tag)
	if !ok {
		t.Fatalf("no locale %s", tag)
	}
	return l
}

// TestReadDelimitedInLocales reads CSV files in a locale: its own
// separators and date order, or the other decimal separator when the
// file writes numbers that way.
func TestReadDelimitedInLocales(t *testing.T) {
	date := numfmt.DateSerial(2026, 9, 26)
	cases := []struct {
		name, tag, text string
		values          map[string]float64
		texts           map[string]string
		note            string
	}{
		{"en-US reads decimal commas", "en-US", "name;price\napple;1,5\npear;2,25\n",
			map[string]float64{"B2": 1.5, "B3": 2.25}, nil, "numbers with decimal commas"},
		{"de-DE reads its own", "de-DE", "a;b;c\n1.234,5;26.09.2026;12,50 €\n",
			map[string]float64{"A2": 1234.5, "B2": date, "C2": 12.5}, nil, ""},
		{"de-DE ties go to semicolons", "de-DE", "1,5;2,5\n3,5;4,5\n",
			map[string]float64{"A1": 1.5, "B2": 4.5}, nil, ""},
		{"de-DE reads decimal points", "de-DE", "a,b\n1.25,2.75\n3.5,4.125\n",
			map[string]float64{"A2": 1.25, "B2": 2.75, "B3": 4.125}, nil, "numbers with decimal points"},
		{"en-GB reads its date order", "en-GB", "d\n26/09/2026\n",
			map[string]float64{"A2": date}, nil, ""},
		{"de-DE keeps its formulas text", "de-DE", "=SUM(1;2)\t-A1*0,5\n",
			nil, map[string]string{"A1": "=SUM(1;2)", "B1": "-A1*0,5"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k := CSV
			if strings.Contains(c.text, "\t") {
				k = TSV
			}
			s, _, notes, err := readDelimited(context.Background(), strings.NewReader(c.text), k, 0, mustLocale(t, c.tag), func(int) {})
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]sheet.Value{}
			for a, n := range c.values {
				want[a] = sheet.Value{Kind: sheet.Number, Num: n}
			}
			for a, s := range c.texts {
				want[a] = sheet.Value{Kind: sheet.Text, Str: s}
			}
			for a, w := range want {
				if v := s.Value(addr(t, a)); v != w {
					t.Errorf("%s = %v, want %v", a, v, w)
				}
			}
			if got := numbersNote(notes); got != c.note {
				t.Errorf("notes %q, want %q", notes, c.note)
			}
		})
	}
}

// numbersNote is the note saying how numbers were read, or "".
func numbersNote(notes []string) string {
	for _, n := range notes {
		if strings.HasPrefix(n, "numbers") {
			return n
		}
	}
	return ""
}

// TestExportDelimitedInLocale writes CSV as a locale shows it, with ;
// between fields where the decimal separator is a comma.
func TestExportDelimitedInLocale(t *testing.T) {
	src := build(t, map[string]string{"A1": "1234.5", "B1": "9/26/2026", "C1": "Tee, schwarz", "A2": "=A1/4"})
	src.SetFormat(sheet.NewRect(addr(t, "A1"), addr(t, "A1")), sheet.Preset(sheet.FmtCurrency))
	src.Book().SetLocale("de-DE")
	name := filepath.Join(t.TempDir(), "x.csv")
	res, err := Export(context.Background(), name, CSV, Snap(src, sheet.Rect{}, "x"), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(name)
	if want := "1.234,50 €;26.9.2026;Tee, schwarz\n308,63\u00a0€;;\n"; string(data) != want {
		t.Errorf("wrote %q, want %q", data, want)
	}
	if !slices.Contains(res.Notes, "separated by semicolons") {
		t.Errorf("notes %q", res.Notes)
	}
	// Read back in de-DE, the values are the same.
	back, _, _, err := readDelimited(context.Background(), strings.NewReader(string(data)), CSV, 0, mustLocale(t, "de-DE"), func(int) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"A1", "B1"} {
		if got, want := back.Value(addr(t, a)), src.Value(addr(t, a)); got != want {
			t.Errorf("%s read back as %v, want %v", a, got, want)
		}
	}
}
