package fileio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"012/internal/sheet"
)

func addr(t testing.TB, s string) sheet.Addr {
	t.Helper()
	a, ok := sheet.ParseAddr(s)
	if !ok {
		t.Fatalf("bad address %q", s)
	}
	return a
}

// build makes a sheet from typed entries.
func build(t testing.TB, entries map[string]string) *sheet.Sheet {
	t.Helper()
	s := sheet.New()
	for a, in := range entries {
		if err := s.Set(addr(t, a), in); err != nil {
			t.Fatalf("%s %q: %v", a, in, err)
		}
	}
	return s
}

// shown is what a cell displays, without a width limit.
func shown(s *sheet.Sheet, a sheet.Addr) string {
	return sheet.FormatText(s.Value(a), s.DisplayFormat(a))
}

func input(s *sheet.Sheet, a sheet.Addr) string {
	if c := s.Cell(a); c != nil {
		return c.Input
	}
	return ""
}

func TestSniff(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       rune
	}{
		{"commas", "a,b,c\n1,2,3\n", ','},
		{"semicolons with decimal commas", "name;price\napple;1,5\npear;2,25\n", ';'},
		{"tabs", "a\tb\n1\t2\n", '\t'},
		{"bars", "a|b|c\n1|2|3\n", '|'},
		{"quoted delimiters", "\"a;b\",c\n\"1;2\",3\n", ','},
		{"quoted line breaks", "a,\"b\nc\"\n1,2\n", ','},
		{"one column", "a\nb\nc\n", ','},
		{"empty", "", ','},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sniff([]byte(tc.text), false); got != tc.want {
				t.Errorf("sniff = %q, want %q", got, tc.want)
			}
		})
	}
}

func readText(t *testing.T, k Kind, text string) (*sheet.Sheet, []string) {
	t.Helper()
	s, _, notes, err := readDelimited(context.Background(), strings.NewReader(text), k, func(int) {})
	if err != nil {
		t.Fatal(err)
	}
	return s, notes
}

func TestReadDelimited(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  Kind
		text  string
		cells map[string]string // cell -> input
		shown map[string]string // cell -> displayed
		note  string
	}{
		{
			name:  "entries become values with formats, as typed",
			kind:  CSV,
			text:  "Item,Due,Amount,Share\nRent,9/26/2026,\"$1,450.00\",12%\n",
			cells: map[string]string{"A1": "Item", "B2": "9/26/2026", "C2": "$1,450.00"},
			shown: map[string]string{"B2": "9/26/2026", "C2": "$1,450.00", "D2": "12%"},
		},
		{
			name:  "formulas stay text",
			kind:  CSV,
			text:  "=1+2,+A1,'quoted,@SUM(A1)\n",
			cells: map[string]string{"A1": "'=1+2", "B1": "'+A1", "C1": "''quoted"},
			shown: map[string]string{"A1": "=1+2", "B1": "+A1", "C1": "'quoted", "D1": "@SUM(A1)"},
		},
		{
			name:  "byte order mark",
			kind:  CSV,
			text:  "\xEF\xBB\xBFname,qty\nx,1\n",
			cells: map[string]string{"A1": "name", "B2": "1"},
		},
		{
			name:  "UTF-16 with BOM",
			kind:  TSV,
			text:  "\xFF\xFEa\x00\t\x00\xE9\x00\n\x001\x00",
			cells: map[string]string{"A1": "a", "B1": "é", "A2": "1"},
			note:  "read as UTF-16",
		},
		{
			name:  "Windows-1252",
			kind:  CSV,
			text:  "caf\xE9,\x80 5\n",
			cells: map[string]string{"A1": "café", "B1": "€ 5"},
			note:  "read as Windows-1252",
		},
		{
			name:  "semicolons",
			kind:  CSV,
			text:  "a;b\n1;2\n",
			cells: map[string]string{"A1": "a", "B2": "2"},
			note:  "separated by semicolons",
		},
		{
			name:  "quoted line breaks become spaces",
			kind:  CSV,
			text:  "\"two\nlines\",\"say \"\"hi\"\"\"\n",
			cells: map[string]string{"A1": "two lines", "B1": `say "hi"`},
		},
		{
			name:  "ragged rows and bare quotes",
			kind:  CSV,
			text:  "a,b,c\n1\n2,x\"y\n",
			cells: map[string]string{"C1": "c", "A2": "1", "B3": `x"y`},
		},
		{
			name:  "TSV keeps commas",
			kind:  TSV,
			text:  "1,5\tTRUE\n",
			cells: map[string]string{"A1": "1,5", "B1": "TRUE"},
			shown: map[string]string{"B1": "TRUE"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, notes := readText(t, tc.kind, tc.text)
			for a, want := range tc.cells {
				if got := input(s, addr(t, a)); got != want {
					t.Errorf("%s input = %q, want %q", a, got, want)
				}
			}
			for a, want := range tc.shown {
				if got := shown(s, addr(t, a)); got != want {
					t.Errorf("%s shows %q, want %q", a, got, want)
				}
			}
			if tc.note != "" && !strings.Contains(strings.Join(notes, "; "), tc.note) {
				t.Errorf("notes %q, want %q", notes, tc.note)
			}
		})
	}
}

func TestReadDelimitedTruncates(t *testing.T) {
	var b strings.Builder
	for range sheet.MaxRows + 5 {
		b.WriteString("x," + strings.Repeat("y,", sheet.MaxCols+2) + "\n")
	}
	s, notes := readText(t, CSV, b.String())
	if got := input(s, sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}); got != "y" {
		t.Errorf("last cell = %q", got)
	}
	joined := strings.Join(notes, "; ")
	for _, want := range []string{"only the first 8,192 rows fit; 5 rows left out", "only columns A to IV fit; 3 columns left out"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes %q, want %q", joined, want)
		}
	}
}

func TestDelimitedRoundTrip(t *testing.T) {
	src := build(t, map[string]string{
		"A1": "Item", "B1": "Due", "C1": "Amount", "D1": "Share", "E1": "Note",
		"A2": "Rent", "B2": "10/1/2026", "C2": "$1,450.00", "D2": "=C2/C4", "E2": "comma, \"quoted\"",
		"A3": "Food", "B3": "9/28/2026", "C3": "612.4", "D3": "=C3/C4", "E3": "tab\there",
		"A4": "Total", "C4": "=SUM(C2:C3)", "E4": "ok",
	})
	src.SetFormat(sheet.NewRect(addr(t, "D2"), addr(t, "D3")), sheet.Preset(sheet.FmtPercent))
	for _, k := range []Kind{CSV, TSV} {
		t.Run(k.String(), func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "out"+k.Ext())
			res, err := Export(context.Background(), name, k, Snap(src, sheet.Rect{}, "out"), ExportOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if res.Rows != 4 || len(res.Notes) != 1 || res.Notes[0] != "3 formulas saved as values" {
				t.Errorf("result %+v", res)
			}
			got, err := Import(context.Background(), name, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range src.Addrs() {
				if want, g := shown(src, a), shown(got.Sheet, a); g != want {
					t.Errorf("%s shows %q after the round trip, want %q", a, g, want)
				}
			}
			// Values come back as numbers with their formats, not text.
			if v := got.Sheet.Value(addr(t, "C4")); v.Kind != sheet.Number || v.Num != 2062.4 {
				t.Errorf("C4 = %+v", v)
			}
			if f := got.Sheet.DisplayFormat(addr(t, "D2")); f.Kind != sheet.FmtPercent {
				t.Errorf("D2 format = %+v", f)
			}
		})
	}
}

func TestExportDelimitedText(t *testing.T) {
	src := build(t, map[string]string{"A1": "a", "C1": "1/0", "B2": "=1/0", "A3": "TRUE"})
	name := filepath.Join(t.TempDir(), "x.csv")
	if _, err := Export(context.Background(), name, CSV, Snap(src, sheet.Rect{}, "x"), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(name)
	want := "a,,1/0\n,#DIV/0!,\nTRUE,,\n"
	if string(data) != want {
		t.Errorf("wrote %q, want %q", data, want)
	}
}

func FuzzReadDelimited(f *testing.F) {
	for _, s := range []string{"a,b\n1,2\n", "\"a\"\"b\",\n", "\xFF\xFEa\x00", "\xEF\xBB\xBF=1", "x;\"y\n", "\x00\x01,\"", "1|2|3"} {
		f.Add([]byte(s), true)
	}
	f.Fuzz(func(t *testing.T, data []byte, tsv bool) {
		k := CSV
		if tsv {
			k = TSV
		}
		readDelimited(context.Background(), strings.NewReader(string(data)), k, func(int) {})
	})
}
