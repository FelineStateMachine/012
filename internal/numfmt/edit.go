package numfmt

import "strings"

// AdjustDecimals adds (delta > 0) or removes zeros after the decimal
// point of every section of a number pattern, as Sheets' Increase and
// Decrease decimal places do. Date and time patterns are returned
// unchanged.
func AdjustDecimals(pat string, delta int) string {
	secs := splitSections(pat)
	for i, sec := range secs {
		if isDatePattern(lexPattern(sec)) {
			return pat
		}
		secs[i] = adjustSection(sec, delta)
	}
	return strings.Join(secs, ";")
}

func adjustSection(sec string, delta int) string {
	// Find the last digit placeholder outside quotes; decimals follow a '.'.
	last, dot := -1, -1
	inQuote := false
	for i := 0; i < len(sec); i++ {
		switch c := sec[i]; {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
		case c == '\\' || c == '_' || c == '*':
			i++
		case c == '.' && dot < 0:
			dot = i
		case c == '0' || c == '#' || c == '?':
			last = i
		case c == 'E' || c == 'e':
			if last >= 0 {
				// Stop at an exponent: its digits aren't decimals.
				i = len(sec)
			}
		}
	}
	if last < 0 {
		return sec
	}
	switch {
	case delta > 0 && dot < 0:
		return sec[:last+1] + "." + strings.Repeat("0", delta) + sec[last+1:]
	case delta > 0:
		return sec[:last+1] + strings.Repeat("0", delta) + sec[last+1:]
	case dot < 0 || last < dot:
		return sec
	}
	decimals := last - dot
	remove := min(-delta, decimals)
	if remove == decimals { // drop the point too
		return sec[:dot] + sec[last+1:]
	}
	return sec[:last+1-remove] + sec[last+1:]
}

// Accounting lays out Sheets' Accounting format with dec decimals in
// width columns: the $ at the left, the number right-aligned with room
// for a closing parenthesis, negatives in parentheses and zero as a dash.
// It reports false when the number doesn't fit.
func Accounting(v float64, dec, width int) (string, bool) {
	whole, frac := Fixed(v, dec, HalfUp)
	var num string
	switch {
	case strings.Trim(whole+frac, "0") == "":
		num = "-" + strings.Repeat(" ", dec) + " "
	case v < 0:
		num = "(" + Format(-v, "#,##0"+decimals(dec)) + ")"
	default:
		num = Format(v, "#,##0"+decimals(dec)) + " "
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
