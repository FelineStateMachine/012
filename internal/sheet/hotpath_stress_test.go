//go:build stress

// Stress benchmarks of the hot paths under every edit and every frame:
// parsing formulas, recognizing typed values, and rendering numbers
// through their formats.
package sheet_test

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

var parseInputs = []string{
	"=A1+B2*3",
	"=SUM($A$1:A100)/COUNT(A1:A100)",
	`=IF(AND(B2>0,C2<>""),VLOOKUP(B2,Data!A1:D500,3,FALSE),"none")`,
	`=TEXT(DATE(2026,9,26),"yyyy-mm-dd")&" "&'Q3 plan'!B7`,
	"=-2^2+1.5E-3*(A1..B3)%",
}

// BenchmarkParse lexes and parses a mix of formulas.
func BenchmarkParse(b *testing.B) {
	for b.Loop() {
		for _, src := range parseInputs {
			if _, err := sheet.Parse(src); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkParseValue recognizes typed numbers, currency, dates and times.
func BenchmarkParseValue(b *testing.B) {
	inputs := []string{"1234.5", "$1,200.00", "12%", "9/26/2026", "2026-09-26", "Sep 26, 2026", "2:30 PM", "9/26/2026 14:30:05", "hello"}
	for b.Loop() {
		for _, s := range inputs {
			sheet.ParseValue(s)
		}
	}
}

// BenchmarkFormatPattern renders numbers and dates through patterns, as
// every visible formatted cell does each frame.
func BenchmarkFormatPattern(b *testing.B) {
	cases := []struct {
		v   float64
		pat string
	}{
		{1234567.891, "#,##0.00"},
		{-0.1234, "0.0%"},
		{12345.678, "0.00E+00"},
		{-1234.5, "$#,##0;($#,##0)"},
		{46291.604, "m/d/yyyy h:mm:ss am/pm"},
		{1.5, "[h]:mm:ss.00"},
		{46291, "dddd, mmmm d, yyyy"},
	}
	for b.Loop() {
		for _, c := range cases {
			sheet.FormatPattern(c.v, c.pat)
		}
	}
}

// BenchmarkDisplay renders values the way the grid does, per visible cell.
func BenchmarkDisplay(b *testing.B) {
	num := sheet.Value{Kind: sheet.Number, Num: 1234.5678}
	formats := []sheet.Format{{}, sheet.Preset(sheet.FmtNumber), sheet.Preset(sheet.FmtCurrency),
		sheet.Preset(sheet.FmtAccounting), sheet.Preset(sheet.FmtDate), sheet.Preset(sheet.FmtPercent)}
	for b.Loop() {
		for _, f := range formats {
			sheet.Display(num, f, 12)
		}
	}
}
