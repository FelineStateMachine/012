package numfmt

import (
	"math"
	"strconv"
	"strings"
)

// Rounding is how Fixed and Round treat the digits they drop.
type Rounding int

const (
	HalfUp Rounding = iota // ROUND and number formats
	Up                     // away from zero: ROUNDUP
	Down                   // toward zero: ROUNDDOWN
)

// Fixed renders |x| rounded to n decimal places (n may be negative) as
// its integer digits (no leading zeros, "" for zero) and exactly n
// decimal digits. It works on the 15 significant digits spreadsheets
// show, so 1.005 rounds to 1.01 as users expect rather than to the 1.00
// its binary value would give.
func Fixed(x float64, n int, mode Rounding) (whole, frac string) {
	x = math.Abs(x)
	nFrac := max(n, 0)
	if x == 0 || math.IsInf(x, 0) || math.IsNaN(x) {
		return "", strings.Repeat("0", nFrac)
	}
	s := strconv.FormatFloat(x, 'e', 14, 64)
	mant, e, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(e)
	d := []byte(strings.Replace(mant, ".", "", 1))
	point := exp + 1 // digits before the decimal point
	if point < 0 {
		d = append([]byte(strings.Repeat("0", -point)), d...)
		point = 0
	}
	keep := point + n
	if keep < 0 {
		// Far smaller than the rounding unit.
		if mode == Up {
			return "1" + strings.Repeat("0", -n), strings.Repeat("0", nFrac)
		}
		return "", strings.Repeat("0", nFrac)
	}
	if keep < len(d) {
		if roundsUp(d[keep:], mode) {
			d, point = increment(d[:keep], point)
		} else {
			d = d[:keep]
		}
	}
	for len(d) < point+nFrac {
		d = append(d, '0')
	}
	whole = strings.TrimLeft(string(d[:point]), "0")
	frac = string(d[point : point+nFrac])
	return whole, frac
}

// roundsUp reports whether dropping the digits dropped rounds the kept
// ones up.
func roundsUp(dropped []byte, mode Rounding) bool {
	switch mode {
	case HalfUp:
		return dropped[0] >= '5'
	case Up:
		return strings.Trim(string(dropped), "0") != ""
	}
	return false
}

// increment adds one to the last of the digits d, carrying; a carry out
// of the first digit adds a digit before the point.
func increment(d []byte, point int) ([]byte, int) {
	for i := len(d) - 1; i >= 0; i-- {
		if d[i] != '9' {
			d[i]++
			return d, point
		}
		d[i] = '0'
	}
	return append([]byte{'1'}, d...), point + 1
}

// Round rounds x to places decimal places (negative places round to
// tens, hundreds...) with mode, on x's 15 significant digits.
func Round(x float64, places int, mode Rounding) float64 {
	whole, frac := Fixed(x, places, mode)
	s := whole
	if s == "" {
		s = "0"
	}
	if frac != "" {
		s += "." + frac
	}
	r, _ := strconv.ParseFloat(s, 64)
	if x < 0 {
		r = -r
	}
	return r
}

// General renders a number the way Sheets shows it in General format and
// in text: up to 15 significant digits, no exponent for ordinary
// magnitudes.
func General(v float64) string {
	if v == 0 {
		return "0"
	}
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', 15, 64), 64)
	if a := math.Abs(r); a >= 1e21 || a < 1e-9 {
		return strconv.FormatFloat(r, 'E', -1, 64)
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// GeneralFit renders v in General format within width columns: as many
// digits as fit, rounding decimals first, then scientific notation, then
// #s.
func GeneralFit(v float64, width int) string {
	if width <= 0 {
		return ""
	}
	s := General(v)
	if strings.ContainsRune(s, 'E') {
		s = strconv.FormatFloat(v, 'f', -1, 64)
	}
	if len(s) <= width {
		return s
	}
	if f, ok := fewerDecimals(v, s, width); ok {
		return f
	}
	for prec := width; prec >= 0; prec-- {
		if e := strconv.FormatFloat(v, 'E', prec, 64); len(e) <= width {
			return e
		}
	}
	return strings.Repeat("#", width)
}

// fewerDecimals rounds v, written in full as s, to the decimals that fit
// in width, but never rounds a non-zero number down to zero.
func fewerDecimals(v float64, s string, width int) (string, bool) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 || dot > width {
		return "", false
	}
	for prec := max(width-dot-1, 0); prec >= 0; prec-- {
		f := strconv.FormatFloat(v, 'f', prec, 64)
		if len(f) > width {
			continue
		}
		if r, _ := strconv.ParseFloat(f, 64); r == 0 && v != 0 {
			return "", false
		}
		if strings.Contains(f, ".") {
			f = strings.TrimRight(strings.TrimRight(f, "0"), ".")
		}
		return f, true
	}
	return "", false
}
