package sheet

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
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
		v /= 100
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
		return formatGeneral(v.Num, inner), AlignRight
	case FmtText:
		return numString(v.Num), AlignLeft
	case FmtAccounting:
		if s, ok := accounting(v.Num, f.Decimals, width); ok {
			return s, AlignFill
		}
		return strings.Repeat("#", inner), AlignRight
	}
	s := FormatPattern(v.Num, f.pattern())
	if utf8.RuneCountInString(s) > inner {
		s = strings.Repeat("#", inner)
	}
	return s, AlignRight
}

// FormatText renders v under f with no width limit, e.g. for TEXT() or
// copying out of the grid.
func FormatText(v Value, f Format) string {
	if v.Kind != Number {
		return text(v)
	}
	switch f.Kind {
	case FmtAuto, FmtText:
		return numString(v.Num)
	}
	return FormatPattern(v.Num, f.pattern())
}

// accounting lays out Sheets' Accounting format in width columns: the $
// at the left, the number right-aligned with room for a closing
// parenthesis, negatives in parentheses and zero as a dash.
func accounting(v float64, dec, width int) (string, bool) {
	ip, fp := fixed(v, dec, roundHalfUp)
	var num string
	switch {
	case strings.Trim(ip+fp, "0") == "":
		num = "-" + strings.Repeat(" ", dec) + " "
	case v < 0:
		num = "(" + FormatPattern(-v, "#,##0"+decimals(dec)) + ")"
	default:
		num = FormatPattern(v, "#,##0"+decimals(dec)) + " "
	}
	gap := width - 1 - len(num)
	switch {
	case gap < 0:
		return "", false
	case gap >= 1:
		// Leave a column of padding before the $, like other cells.
		return " $" + strings.Repeat(" ", gap-1) + num, true
	}
	return "$" + strings.Repeat(" ", gap) + num, true
}

func decimals(n int) string {
	if n <= 0 {
		return ""
	}
	return "." + strings.Repeat("0", n)
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
	return formatGeneral(v, width)
}

// formatGeneral mimics General number format: as many digits as fit,
// rounding decimals first, then scientific notation, then #s.
func formatGeneral(v float64, width int) string {
	if width <= 0 {
		return ""
	}
	s := numString(v)
	if strings.ContainsRune(s, 'E') {
		s = strconv.FormatFloat(v, 'f', -1, 64)
	}
	if len(s) <= width {
		return s
	}
	// Try fewer decimals before falling back to scientific notation, but
	// never round a non-zero number down to zero.
	if dot := strings.IndexByte(s, '.'); dot >= 0 && dot <= width {
		for prec := max(width-dot-1, 0); prec >= 0; prec-- {
			f := strconv.FormatFloat(v, 'f', prec, 64)
			if len(f) > width {
				continue
			}
			if r, _ := strconv.ParseFloat(f, 64); r != 0 || v == 0 {
				if strings.Contains(f, ".") {
					f = strings.TrimRight(strings.TrimRight(f, "0"), ".")
				}
				return f
			}
			break
		}
	}
	for prec := width; prec >= 0; prec-- {
		if e := strconv.FormatFloat(v, 'E', prec, 64); len(e) <= width {
			return e
		}
	}
	return strings.Repeat("#", width)
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

// isWhole reports whether v is an integer.
func isWhole(v float64) bool { return v == math.Trunc(v) }
