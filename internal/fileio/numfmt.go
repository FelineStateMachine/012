package fileio

import (
	"strings"
	"sync"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Number formats in files are Excel format codes. 012's formats map to
// the codes Sheets writes for them, and codes map back to the format
// that writes them, so formats survive a round trip; any other code is
// kept as a custom pattern, which 012 renders as Sheets would.

// excelCode is the Excel format code for f, or "" for General.
func excelCode(f sheet.Format) string {
	dec := ""
	if f.Decimals > 0 {
		dec = "." + strings.Repeat("0", min(f.Decimals, sheet.MaxDecimals))
	}
	switch f.Kind {
	case sheet.FmtText:
		return "@"
	case sheet.FmtNumber:
		return "#,##0" + dec
	case sheet.FmtPercent:
		return "0" + dec + "%"
	case sheet.FmtScientific:
		return "0" + dec + "E+00"
	case sheet.FmtAccounting:
		return `_("$"* #,##0` + dec + `_);_("$"* \(#,##0` + dec + `\);_("$"* "-"??_);_(@_)`
	case sheet.FmtFinancial:
		return "#,##0" + dec + ";(#,##0" + dec + ")"
	case sheet.FmtCurrency:
		return `"$"#,##0` + dec
	case sheet.FmtDate:
		return orDefault(f.Pattern, "m/d/yyyy")
	case sheet.FmtTime:
		return orDefault(f.Pattern, "h:mm:ss AM/PM")
	case sheet.FmtDateTime:
		return orDefault(f.Pattern, "m/d/yyyy h:mm:ss")
	case sheet.FmtDuration:
		return orDefault(f.Pattern, "[h]:mm:ss")
	case sheet.FmtCustom:
		return f.Pattern
	}
	return ""
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// builtinCodes are Excel's built-in number formats by ID, for files that
// refer to them without spelling out the code.
var builtinCodes = map[int]string{
	1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00",
	5: `"$"#,##0_);\("$"#,##0\)`, 6: `"$"#,##0_);[Red]\("$"#,##0\)`,
	7: `"$"#,##0.00_);\("$"#,##0.00\)`, 8: `"$"#,##0.00_);[Red]\("$"#,##0.00\)`,
	9: "0%", 10: "0.00%", 11: "0.00E+00", 12: "# ?/?", 13: "# ??/??",
	14: "m/d/yyyy", 15: "d-mmm-yy", 16: "d-mmm", 17: "mmm-yy",
	18: "h:mm AM/PM", 19: "h:mm:ss AM/PM", 20: "h:mm", 21: "h:mm:ss", 22: "m/d/yyyy h:mm",
	37: "#,##0 ;(#,##0)", 38: "#,##0 ;[Red](#,##0)", 39: "#,##0.00;(#,##0.00)", 40: "#,##0.00;[Red](#,##0.00)",
	41: `_(* #,##0_);_(* \(#,##0\);_(* "-"_);_(@_)`, 42: `_("$"* #,##0_);_("$"* \(#,##0\);_("$"* "-"_);_(@_)`,
	43: `_(* #,##0.00_);_(* \(#,##0.00\);_(* "-"??_);_(@_)`, 44: `_("$"* #,##0.00_);_("$"* \(#,##0.00\);_("$"* "-"??_);_(@_)`,
	45: "mm:ss", 46: "[h]:mm:ss", 47: "mm:ss.0", 48: "##0.0E+0", 49: "@",
}

// builtinFormats are built-in IDs whose meaning is one of 012's formats
// although the code differs from the one 012 writes.
var builtinFormats = map[int]sheet.Format{
	5: {Kind: sheet.FmtCurrency}, 6: {Kind: sheet.FmtCurrency},
	7: {Kind: sheet.FmtCurrency, Decimals: 2}, 8: {Kind: sheet.FmtCurrency, Decimals: 2},
	37: {Kind: sheet.FmtFinancial}, 38: {Kind: sheet.FmtFinancial},
	39: {Kind: sheet.FmtFinancial, Decimals: 2}, 40: {Kind: sheet.FmtFinancial, Decimals: 2},
	42: {Kind: sheet.FmtAccounting},
}

var (
	knownOnce  sync.Once
	knownCodes map[string]sheet.Format // normalized code -> format
)

// normCode folds the spellings of a code that mean the same thing: case,
// a locale tag, and \$ for "$".
func normCode(code string) string {
	code = strings.ReplaceAll(code, "[$-409]", "")
	code = strings.ReplaceAll(code, "[$$-409]", `"$"`)
	code = strings.ReplaceAll(code, `\$`, `"$"`)
	return strings.ToLower(strings.TrimSpace(code))
}

// formatOf maps an Excel number format, by built-in ID or code, to a 012
// format.
func formatOf(id int, code string) sheet.Format {
	if f, ok := builtinFormats[id]; ok && code == "" {
		return f
	}
	if code == "" {
		code = builtinCodes[id]
	}
	switch normCode(code) {
	case "", "general":
		return sheet.Format{}
	}
	knownOnce.Do(func() {
		knownCodes = map[string]sheet.Format{}
		for _, k := range []sheet.FormatKind{sheet.FmtAccounting, sheet.FmtFinancial, sheet.FmtScientific,
			sheet.FmtPercent, sheet.FmtCurrency, sheet.FmtNumber} {
			for d := sheet.MaxDecimals; d >= 0; d-- {
				f := sheet.Format{Kind: k, Decimals: d}
				knownCodes[normCode(excelCode(f))] = f
			}
		}
		for _, k := range []sheet.FormatKind{sheet.FmtText, sheet.FmtDate, sheet.FmtTime, sheet.FmtDateTime, sheet.FmtDuration} {
			knownCodes[normCode(excelCode(sheet.Format{Kind: k}))] = sheet.Format{Kind: k}
		}
		knownCodes["m/d/yyyy h:mm"] = sheet.Format{Kind: sheet.FmtDateTime, Pattern: "m/d/yyyy h:mm"}
	})
	if f, ok := knownCodes[normCode(code)]; ok {
		return f
	}
	if f, ok := builtinFormats[id]; ok {
		return f
	}
	switch kind := dateKind(code); kind {
	case sheet.FmtDate, sheet.FmtTime, sheet.FmtDateTime, sheet.FmtDuration:
		return sheet.Format{Kind: kind, Pattern: code}
	}
	return sheet.Format{Kind: sheet.FmtCustom, Pattern: code}
}

// dateKind says whether a code shows a date, a time, both, or a
// duration, looking at its letters outside quotes and brackets; it
// returns FmtCustom for anything else.
func dateKind(code string) sheet.FormatKind {
	date, clock, elapsed := false, false, false
	inQuote, inBracket := false, false
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
		case c == '\\':
			i++
		case c == '[':
			inBracket = true
			if i+1 < len(code) && strings.ContainsRune("hHmMsS", rune(code[i+1])) {
				elapsed = true
			}
		case c == ']':
			inBracket = false
		case inBracket:
		case c == 'd' || c == 'D' || c == 'y' || c == 'Y':
			date = true
		case c == 'h' || c == 'H' || c == 's' || c == 'S':
			clock = true
		case c == '0' || c == '#' || c == '?':
			if !clock && !date {
				return sheet.FmtCustom
			}
		}
	}
	switch {
	case elapsed:
		return sheet.FmtDuration
	case date && clock:
		return sheet.FmtDateTime
	case date:
		return sheet.FmtDate
	case clock:
		return sheet.FmtTime
	}
	return sheet.FmtCustom
}

// isDateFormat reports whether f shows a calendar date, whose serial
// numbers differ between Excel and 012 before March 1900.
func isDateFormat(f sheet.Format) bool {
	switch f.Kind {
	case sheet.FmtDate, sheet.FmtDateTime:
		return true
	case sheet.FmtCustom:
		k := dateKind(f.Pattern)
		return k == sheet.FmtDate || k == sheet.FmtDateTime
	}
	return false
}

// Excel and 1-2-3 count 1900 as a leap year, so their serials before
// March 1, 1900 are one less than 012's (and Sheets').
const leapBug = 61

func fromExcelSerial(v float64, f sheet.Format) float64 {
	if isDateFormat(f) && v >= 1 && v < leapBug-1 {
		return v + 1
	}
	return v
}

func toExcelSerial(v float64, f sheet.Format) float64 {
	if isDateFormat(f) && v >= 1 && v < leapBug {
		return v - 1
	}
	return v
}
