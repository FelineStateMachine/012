package headless

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// TestDescribeFixture describes the workbook of every feature the
// release fixtures keep.
func TestDescribeFixture(t *testing.T) {
	f, err := Open("../sheet/testdata/fixtures/new/workbook.012", false)
	if err != nil {
		t.Fatal(err)
	}
	d := Describe(f.Book)
	var text bytes.Buffer
	if err := WriteDescription(&text, d); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Sales (shown)  A1:J13, 13 rows x 10 columns",
		"  header row 1: Date, Region, Units, Price, Total",
		"  chart 1  Units by date  column of A1:C9 at K2",
		"Lists (hidden)",
		"  table Targets  D1:E3  Region, Target",
		"  pivot table of Sales!A1:E9",
		"  output files  A1:B3",
		"  linked app  F2  following logs/app.csv",
		"Notebook (notebook)  5 cells",
		"  4 code  $files | nope  (failed: Command `nope` not found)",
		"  Units  Sales!C2:C9",
	} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text lacks %q:\n%s", want, text.String())
		}
	}
	var js bytes.Buffer
	if err := Encode(&js, "json", d); err != nil {
		t.Fatal(err)
	}
	var back Description
	if err := json.Unmarshal(js.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Sheets) != len(d.Sheets) || back.Sheets[0].Charts[2].Type != "pie" || back.Sheets[6].Notebook[1].Name != "files" {
		t.Errorf("JSON round trip lost something: %+v", back.Sheets[0])
	}
	var nu bytes.Buffer
	if err := Encode(&nu, "nuon", d); err != nil || !strings.HasPrefix(nu.String(), "{sheets: [") {
		t.Errorf("NUON: %v %.40s", err, nu.String())
	}
}

// TestGuessHeader takes a first row of text over typed rows, or a bold
// one, as a header, and not a table of text.
func TestGuessHeader(t *testing.T) {
	for _, c := range []struct {
		rows [][]string
		bold bool
		want int
	}{
		{[][]string{{"Item", "Price"}, {"Apple", "1.5"}}, false, 1},
		{[][]string{{"Item", "Kind"}, {"Apple", "fruit"}}, false, 0},
		{[][]string{{"Item", "Kind"}, {"Apple", "fruit"}}, true, 1},
		{[][]string{{"2024", "2025"}, {"1", "2"}}, false, 0},
		{[][]string{{"Only", "row"}}, true, 0},
	} {
		s := sheet.New()
		for r, row := range c.rows {
			for col, in := range row {
				s.Set(sheet.Addr{Col: col, Row: r}, in)
			}
		}
		if c.bold {
			s.SetStyle(sheet.Rect{To: sheet.Addr{Col: 1}}, func(st *sheet.Style) { st.Bold = true })
		}
		used, _ := s.UsedRange()
		if got, _ := guessHeader(s, used); got != c.want {
			t.Errorf("%v bold %v: header %d, want %d", c.rows, c.bold, got, c.want)
		}
	}
}
