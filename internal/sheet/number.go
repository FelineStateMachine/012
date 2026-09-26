package sheet

import (
	"strconv"
	"strings"
)

// ParseNumber recognizes numbers the way Google Sheets does on entry:
// optional sign, optional leading currency symbol, thousands separators,
// decimals, exponent, and a trailing percent sign ("12%" is 0.12).
func ParseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}
	s = strings.TrimPrefix(s, "$")
	pct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	if s == "" || !isDigit(s[0]) && s[0] != '.' {
		return 0, false
	}
	if strings.Contains(s, ",") {
		if !validGrouping(s) {
			return 0, false
		}
		s = strings.ReplaceAll(s, ",", "")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if pct {
		v /= 100
	}
	if neg {
		v = -v
	}
	return v, true
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

// FormatValue renders v in width columns with one column of padding, the
// way the grid shows it: numbers right-aligned, booleans and errors
// centered. Text is not handled here because it can overflow into
// neighboring cells.
func FormatValue(v Value, width int) string {
	inner := width - 1
	switch v.Kind {
	case Number:
		return padLeft(formatGeneral(v.Num, inner), inner) + " "
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
	s := strconv.FormatFloat(v, 'f', -1, 64)
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
