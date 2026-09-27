package sheet

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/numfmt"
)

// ParseNumber recognizes numbers the way Google Sheets does on entry:
// optional sign, optional leading currency symbol, thousands separators,
// decimals, exponent, and a trailing percent sign ("12%" is 0.12).
func ParseNumber(s string) (float64, bool) {
	v, _, ok := parseNumberFormat(s)
	return v, ok
}

// ParseValue recognizes everything Sheets turns into a number on entry:
// numbers, currency, percentages, dates and times. It also returns the
// format Sheets applies, e.g. Currency for "$1,200" or Date for
// "9/26/2026"; plain numbers get Automatic.
func ParseValue(s string) (float64, Format, bool) {
	if v, f, ok := parseNumberFormat(s); ok {
		return v, f, true
	}
	return parseDateTime(s)
}

func parseNumberFormat(s string) (float64, Format, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, Format{}, false
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}
	cur := strings.HasPrefix(s, "$")
	s = strings.TrimPrefix(s, "$")
	pct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	if s == "" || !isDigit(s[0]) && s[0] != '.' {
		return 0, Format{}, false
	}
	grouped := strings.Contains(s, ",")
	if grouped {
		if !validGrouping(s) {
			return 0, Format{}, false
		}
		s = strings.ReplaceAll(s, ",", "")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || strings.ContainsAny(s, "xXpP_") || strings.EqualFold(s, "inf") {
		return 0, Format{}, false
	}
	if pct {
		// Shift the decimal point rather than divide, so "0.7%" is the
		// double nearest 0.007 and not 0.006999999999999999.
		if p, err := strconv.ParseFloat(s+"e-2", 64); err == nil {
			v = p
		} else {
			v /= 100
		}
	}
	if neg {
		v = -v
	}
	// The format Sheets infers: two decimals if any were typed.
	dec := 0
	if strings.Contains(s, ".") {
		dec = 2
	}
	var f Format
	switch {
	case cur:
		f = Format{Kind: FmtCurrency, Decimals: dec}
	case pct:
		f = Format{Kind: FmtPercent, Decimals: dec}
	case strings.ContainsAny(s, "eE"):
		f = Preset(FmtScientific)
	case grouped:
		f = Format{Kind: FmtNumber, Decimals: dec}
	}
	return v, f, true
}

// validGrouping checks thousands separators: groups of three digits
// before the decimal point.
func validGrouping(s string) bool {
	intPart, _, _ := strings.Cut(s, ".")
	groups := strings.Split(intPart, ",")
	if len(groups[0]) == 0 || len(groups[0]) > 3 {
		return false
	}
	for _, g := range groups[1:] {
		if len(g) != 3 {
			return false
		}
	}
	return true
}

// Display renders v under format f for a cell width columns wide, as the
// grid shows it: the text without padding, and where it goes. Numbers go
// right, text left, booleans and errors center. A formatted number that
// doesn't fit in width-1 columns becomes a run of #, as in Sheets;
// Automatic first drops decimals and falls back to scientific notation.
// Accounting returns AlignFill: exactly width columns, $ at the left.
func Display(v Value, f Format, width int) (string, Align) {
	inner := max(width-1, 0)
	switch v.Kind {
	case Empty:
		return "", AlignLeft
	case Text:
		return v.Str, AlignLeft
	case Bool, Error:
		return v.String(), AlignCenter
	}
	switch f.Kind {
	case FmtAuto:
		return numfmt.GeneralFit(v.Num, inner), AlignRight
	case FmtText:
		return numfmt.General(v.Num), AlignLeft
	case FmtAccounting:
		if s, ok := numfmt.Accounting(v.Num, f.Decimals, width); ok {
			return s, AlignFill
		}
		return strings.Repeat("#", inner), AlignRight
	}
	s := numfmt.Format(v.Num, f.pattern())
	if utf8.RuneCountInString(s) > inner {
		s = strings.Repeat("#", inner)
	}
	return s, AlignRight
}

// FormatPattern renders v with a number format pattern, as TEXT() does.
func FormatPattern(v float64, pat string) string { return numfmt.Format(v, pat) }

// FormatText renders v under f with no width limit, e.g. for TEXT() or
// copying out of the grid.
func FormatText(v Value, f Format) string {
	if v.Kind != Number {
		return text(v)
	}
	switch f.Kind {
	case FmtAuto, FmtText:
		return numfmt.General(v.Num)
	}
	return numfmt.Format(v.Num, f.pattern())
}

// FormatValue renders v in width columns with one column of padding, the
// way the grid shows it in Automatic format: numbers right-aligned,
// booleans and errors centered. Text is not handled here because it can
// overflow into neighboring cells.
func FormatValue(v Value, width int) string {
	switch v.Kind {
	case Number:
		s, _ := Display(v, Format{}, width)
		return padLeft(s, width-1) + " "
	case Bool, Error:
		return centerPad(v.String(), width)
	}
	return ""
}

// FormatNumber formats a number in the General format within width
// columns, for use outside the grid (e.g. the status line).
func FormatNumber(v float64, width int) string {
	return numfmt.GeneralFit(v, width)
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func centerPad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	pad := width - len(s)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}
