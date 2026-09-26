package oracle

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"one23/internal/sheet"
)

// patterns are number formats: Sheets' Format > Number presets and
// common custom ones.
var patterns = []string{
	"#,##0.00", "#,##0", "0.00%", "0%", "0.00E+00", `"$"#,##0.00`, `"$"#,##0`,
	"#,##0.00;(#,##0.00)", "0.0#", "000", "#,##0,", "0.00 \"kg\"",
	"m/d/yyyy", "yyyy-mm-dd", "mmm d, yyyy", "mmmm d, yyyy", "dddd", "d mmm yyyy",
	"h:mm:ss am/pm", "h:mm AM/PM", "hh:mm:ss", "m/d/yyyy h:mm:ss", "[h]:mm:ss", "h:mm",
}

var values = []float64{0, 1, -1, 0.5, 1234.5678, -1234.5678, 0.125, 1e6, 46291, 46291.6041666667, 1.0423611111}

// skippedFormats are pattern and value pairs where one23 deliberately
// differs, or excelize departs from Excel.
var skippedFormats = map[string]string{
	"#,##0, -1":             "one23 drops the sign of a number that rounds to zero; excelize shows -0",
	"0.0# 46291.6041666667": "excelize keeps a trailing zero under #",
	"[h]:mm:ss 1e+06":       "excelize overflows",
}

// skipReason explains whole classes of skipped comparisons.
func skipReason(pat string, v float64) string {
	switch {
	case isDateOrTime(pat) && v < 0:
		return "dates before 1899-12-30 are negative serials in Sheets; Excel has none"
	case isDate(pat) && v < 61:
		return "Excel counts a fictitious 1900-02-29, so its dates before March 1900 are a day off; Sheets and one23 don't"
	}
	return skippedFormats[fmt.Sprintf("%s %v", pat, v)]
}

func TestFormatsAgainstExcelize(t *testing.T) {
	x := excelize.NewFile()
	matched, skipped := 0, 0
	for _, pat := range patterns {
		p := pat
		style, err := x.NewStyle(&excelize.Style{CustomNumFmt: &p})
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range values {
			if why := skipReason(pat, v); why != "" {
				t.Logf("skip %q of %v: %s", pat, v, why)
				skipped++
				continue
			}
			x.SetCellValue("Sheet1", "A1", v)
			x.SetCellStyle("Sheet1", "A1", "A1", style)
			theirs, err := x.GetCellValue("Sheet1", "A1")
			if err != nil {
				t.Fatal(err)
			}
			ours := sheet.FormatPattern(v, pat)
			// one23 shows AM and PM in capitals, as Sheets' Time format does;
			// excelize copies the case of the pattern's am/pm.
			if ours != theirs && !(strings.Contains(pat, "am/pm") && strings.EqualFold(ours, theirs)) {
				t.Errorf("%q of %v: one23 %q, excelize %q", pat, v, ours, theirs)
				continue
			}
			matched++
		}
	}
	t.Logf("%d formatted values match excelize, %d skipped", matched, skipped)
}

func isDateOrTime(pat string) bool { return strings.ContainsAny(pat, "ydhsm") }

func isDate(pat string) bool { return strings.ContainsAny(pat, "yd") }
