package value

import (
	"strconv"
	"strings"
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
	return ParseDateTime(s)
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
	dot, exp, odd := scanNumber(s)
	if err != nil || odd || strings.EqualFold(s, "inf") {
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
	if dot {
		dec = 2
	}
	var f Format
	switch {
	case cur:
		f = Format{Kind: FmtCurrency, Decimals: dec}
	case pct:
		f = Format{Kind: FmtPercent, Decimals: dec}
	case exp:
		f = Preset(FmtScientific)
	case grouped:
		f = Format{Kind: FmtNumber, Decimals: dec}
	}
	return v, f, true
}

// scanNumber reports whether s has a decimal point, an exponent, and
// what ParseFloat accepts but an entry isn't: hexadecimal, underscores.
// One pass, as every number loaded goes through it.
func scanNumber(s string) (dot, exp, odd bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.':
			dot = true
		case 'e', 'E':
			exp = true
		case 'x', 'X', 'p', 'P', '_':
			odd = true
		}
	}
	return dot, exp, odd
}

// validGrouping checks thousands separators: groups of three digits
// before the decimal point, and none after it (1.234,5 is text).
func validGrouping(s string) bool {
	intPart, frac, _ := strings.Cut(s, ".")
	if strings.Contains(frac, ",") {
		return false
	}
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

func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
