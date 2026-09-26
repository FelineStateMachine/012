package sheet

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Number format patterns, as in Sheets' custom number formats and TEXT():
// "#,##0.00", "0.0%", "0.00E+00", "$#,##0;($#,##0)", "m/d/yyyy",
// "h:mm am/pm", "[h]:mm:ss". Up to four ;-separated sections apply to
// positive, negative, zero and text values.

type ptKind uint8

const (
	ptLit     ptKind = iota
	ptDigit          // 0, # or ?
	ptDot            // decimal point
	ptComma          // thousands separator or scaling
	ptPercent        //
	ptExp            // E+ or E-
	ptGeneral        // "General"
	ptText           // @
	ptFill           // *x: repeat x to fill the cell
	ptYear           // y, yy, yyyy
	ptMonth          // m..mmmmm: month, or minutes next to h or s
	ptDay            // d..dddd
	ptHour           // h, hh
	ptSecond         // s, ss
	ptAMPM           // AM/PM or A/P
	ptElapsed        // [h], [m], [s]
	ptFrac           // .0, .00 after seconds
)

type ptok struct {
	kind ptKind
	s    string // literal text, digit placeholder char, or the token as written
	n    int    // repeat count for date tokens
}

// splitSections splits a pattern on ; outside quotes and escapes.
func splitSections(pat string) []string {
	var secs []string
	start, inQuote := 0, false
	for i := 0; i < len(pat); i++ {
		switch c := pat[i]; {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
		case c == '\\':
			i++
		case c == ';':
			secs = append(secs, pat[start:i])
			start = i + 1
		}
	}
	return append(secs, pat[start:])
}

func lexPattern(sec string) []ptok {
	var toks []ptok
	lit := func(s string) { toks = append(toks, ptok{kind: ptLit, s: s}) }
	lower := strings.ToLower(sec)
	for i := 0; i < len(sec); {
		c := sec[i]
		lc := lower[i]
		switch {
		case c == '"':
			end := strings.IndexByte(sec[i+1:], '"')
			if end < 0 {
				end = len(sec) - i - 1
			}
			lit(sec[i+1 : i+1+end])
			i += end + 2
		case c == '\\' && i+1 < len(sec):
			_, w := utf8.DecodeRuneInString(sec[i+1:])
			lit(sec[i+1 : i+1+w])
			i += 1 + w
		case c == '_' && i+1 < len(sec):
			_, w := utf8.DecodeRuneInString(sec[i+1:])
			lit(" ")
			i += 1 + w
		case c == '*' && i+1 < len(sec):
			_, w := utf8.DecodeRuneInString(sec[i+1:])
			toks = append(toks, ptok{kind: ptFill, s: sec[i+1 : i+1+w]})
			i += 1 + w
		case c == '[':
			end := strings.IndexByte(sec[i:], ']')
			if end < 0 {
				lit(sec[i:])
				i = len(sec)
				break
			}
			inner := lower[i+1 : i+end]
			switch {
			case inner != "" && strings.Trim(inner, string(inner[0])) == "" && strings.ContainsAny(inner[:1], "hms"):
				toks = append(toks, ptok{kind: ptElapsed, s: inner[:1], n: len(inner)})
			case strings.HasPrefix(inner, "$"):
				// Currency like [$€-407]: show the symbol.
				sym, _, _ := strings.Cut(sec[i+2:i+end], "-")
				lit(sym)
			}
			// Colors and conditions are ignored.
			i += end + 1
		case c == '0' || c == '#' || c == '?':
			toks = append(toks, ptok{kind: ptDigit, s: string(c)})
			i++
		case c == '.':
			toks = append(toks, ptok{kind: ptDot})
			i++
		case c == ',':
			toks = append(toks, ptok{kind: ptComma})
			i++
		case c == '%':
			toks = append(toks, ptok{kind: ptPercent})
			i++
		case c == '@':
			toks = append(toks, ptok{kind: ptText})
			i++
		case (lc == 'e') && i+1 < len(sec) && (sec[i+1] == '+' || sec[i+1] == '-'):
			toks = append(toks, ptok{kind: ptExp, s: sec[i+1 : i+2]})
			i += 2
		case strings.HasPrefix(lower[i:], "am/pm"):
			toks = append(toks, ptok{kind: ptAMPM, s: "AM/PM"})
			i += 5
		case strings.HasPrefix(lower[i:], "a/p"):
			toks = append(toks, ptok{kind: ptAMPM, s: "A/P"})
			i += 3
		case strings.HasPrefix(lower[i:], "general"):
			toks = append(toks, ptok{kind: ptGeneral})
			i += 7
		case strings.IndexByte("ymdhs", lc) >= 0:
			j := i
			for j < len(sec) && lower[j] == lc {
				j++
			}
			kind := map[byte]ptKind{'y': ptYear, 'm': ptMonth, 'd': ptDay, 'h': ptHour, 's': ptSecond}[lc]
			toks = append(toks, ptok{kind: kind, s: sec[i:j], n: j - i})
			i = j
		default:
			_, w := utf8.DecodeRuneInString(sec[i:])
			lit(sec[i : i+w])
			i += w
		}
	}
	return toks
}

// isDatePattern reports whether a section formats dates and times.
func isDatePattern(toks []ptok) bool {
	for _, t := range toks {
		switch t.kind {
		case ptYear, ptMonth, ptDay, ptHour, ptSecond, ptAMPM, ptElapsed:
			return true
		}
	}
	return false
}

// FormatPattern renders v with a number format pattern, as TEXT() does.
func FormatPattern(v float64, pat string) string {
	secs := splitSections(pat)
	sec, neg := secs[0], v < 0
	switch {
	case len(secs) >= 3 && v == 0:
		sec = secs[2]
	case len(secs) >= 2 && v < 0 && strings.TrimSpace(secs[1]) != "":
		sec, v, neg = secs[1], -v, false
	}
	toks := lexPattern(sec)
	if isDatePattern(toks) {
		return formatDateToks(v, toks)
	}
	return formatNumToks(math.Abs(v), neg, toks)
}

func formatNumToks(v float64, neg bool, toks []ptok) string {
	// Find the parts of the number: integer placeholders, the decimal
	// point, decimal placeholders and an exponent.
	dot, exp := -1, -1
	var intPH, fracPH, expPH []int
	percents, general := 0, false
	for i, t := range toks {
		switch t.kind {
		case ptDigit:
			switch {
			case exp >= 0:
				expPH = append(expPH, i)
			case dot >= 0:
				fracPH = append(fracPH, i)
			default:
				intPH = append(intPH, i)
			}
		case ptDot:
			if dot < 0 && exp < 0 {
				dot = i
			}
		case ptExp:
			if exp < 0 {
				exp = i
			}
		case ptPercent:
			percents++
		case ptGeneral:
			general = true
		}
	}
	// Commas between integer placeholders group thousands; commas right
	// after the last one scale by 1000 each.
	grouping, scale := false, 0
	if len(intPH) > 0 {
		first, last := intPH[0], intPH[len(intPH)-1]
		for i := first; i < last; i++ {
			if toks[i].kind == ptComma {
				grouping = true
			}
		}
		for i := last + 1; i < len(toks) && toks[i].kind == ptComma; i++ {
			scale++
		}
	}
	for range percents {
		v *= 100
	}
	for range scale {
		v /= 1000
	}

	var intDigits, fracDigits, expText string
	expNeg := false
	switch {
	case general && len(intPH) == 0:
		intDigits = numString(v)
	case exp >= 0:
		e := 0
		if v != 0 {
			e = int(math.Floor(math.Log10(v)))
		}
		ip, fp := fixed(v/math.Pow(10, float64(e)), len(fracPH), roundHalfUp)
		if ip == "10" {
			e++
			ip, fp = fixed(v/math.Pow(10, float64(e)), len(fracPH), roundHalfUp)
		}
		intDigits, fracDigits = ip, fp
		expNeg = e < 0
		expText = strconv.Itoa(absInt(e))
		if len(expText) < len(expPH) {
			expText = strings.Repeat("0", len(expPH)-len(expText)) + expText
		}
	default:
		intDigits, fracDigits = fixed(v, len(fracPH), roundHalfUp)
	}
	if neg && strings.Trim(intDigits+fracDigits, "0") == "" {
		neg = false // don't show -0.00
	}

	// Fill integer placeholders right to left; the leftmost one takes any
	// remaining digits.
	intOut := make(map[int]string, len(intPH))
	rem := intDigits
	for k := len(intPH) - 1; k >= 0; k-- {
		ph := toks[intPH[k]].s
		switch {
		case k == 0 && rem != "":
			intOut[intPH[k]], rem = rem, ""
		case rem != "":
			intOut[intPH[k]], rem = rem[len(rem)-1:], rem[:len(rem)-1]
		case ph == "0":
			intOut[intPH[k]] = "0"
		case ph == "?":
			intOut[intPH[k]] = " "
		}
	}
	// Decimal placeholders: trailing zeros disappear under # and turn to
	// spaces under ?.
	fracOut := make(map[int]string, len(fracPH))
	trimming := true
	for k := len(fracPH) - 1; k >= 0; k-- {
		d := fracDigits[k : k+1]
		ph := toks[fracPH[k]].s
		switch {
		case trimming && d == "0" && ph == "#":
			fracOut[fracPH[k]] = ""
		case trimming && d == "0" && ph == "?":
			fracOut[fracPH[k]] = " "
		default:
			trimming = false
			fracOut[fracPH[k]] = d
		}
	}

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	literalsInInt := false
	for i := range toks {
		if len(intPH) > 0 && i > intPH[0] && i < intPH[len(intPH)-1] && toks[i].kind == ptLit {
			literalsInInt = true
		}
	}
	for i, t := range toks {
		switch t.kind {
		case ptLit:
			b.WriteString(t.s)
		case ptDigit:
			switch {
			case len(intPH) > 0 && i == intPH[0] && grouping && !literalsInInt:
				var digits strings.Builder
				for _, p := range intPH {
					digits.WriteString(intOut[p])
				}
				b.WriteString(group(digits.String()))
			case len(intPH) > 0 && i >= intPH[0] && i <= intPH[len(intPH)-1]:
				if !grouping || literalsInInt {
					b.WriteString(intOut[i])
				}
			case len(expPH) > 0 && i == expPH[0]:
				b.WriteString(expText)
			case len(expPH) > 0 && i > expPH[0]:
			default:
				b.WriteString(fracOut[i])
			}
		case ptDot:
			if i == dot {
				b.WriteByte('.')
			}
		case ptPercent:
			b.WriteByte('%')
		case ptExp:
			if i == exp {
				switch {
				case expNeg:
					b.WriteString("E-")
				case t.s == "+":
					b.WriteString("E+")
				default:
					b.WriteString("E")
				}
			}
		case ptGeneral:
			if len(intPH) == 0 {
				b.WriteString(intDigits)
			}
		case ptComma:
			// Grouping and scaling commas are consumed; others are literal.
			if len(intPH) == 0 || i < intPH[0] || (dot >= 0 && i > dot) {
				b.WriteByte(',')
			}
		}
	}
	return b.String()
}

// group inserts thousands separators into a run of digits (and leading
// spaces from ? placeholders).
func group(s string) string {
	lead := len(s) - len(strings.TrimLeft(s, " "))
	digits := s[lead:]
	if len(digits) <= 3 {
		return s
	}
	var b strings.Builder
	b.WriteString(s[:lead])
	first := len(digits) % 3
	if first > 0 {
		b.WriteString(digits[:first])
	}
	for i := first; i < len(digits); i += 3 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

type roundMode int

const (
	roundHalfUp roundMode = iota // ROUND and number formats
	roundUp                      // away from zero: ROUNDUP
	roundDown                    // toward zero: ROUNDDOWN
)

// fixed renders |x| rounded to n decimal places (n may be negative) as
// its integer digits (no leading zeros, "" for zero) and exactly n
// decimal digits. It works on the 15 significant digits spreadsheets
// show, so 1.005 rounds to 1.01 as users expect rather than to the 1.00
// its binary value would give.
func fixed(x float64, n int, mode roundMode) (intPart, frac string) {
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
		if mode == roundUp {
			return "1" + strings.Repeat("0", -n), strings.Repeat("0", nFrac)
		}
		return "", strings.Repeat("0", nFrac)
	}
	if keep < len(d) {
		var up bool
		switch mode {
		case roundHalfUp:
			up = d[keep] >= '5'
		case roundUp:
			up = strings.Trim(string(d[keep:]), "0") != ""
		}
		d = d[:keep]
		if up {
			i := len(d) - 1
			for ; i >= 0; i-- {
				if d[i] == '9' {
					d[i] = '0'
					continue
				}
				d[i]++
				break
			}
			if i < 0 {
				d = append([]byte{'1'}, d...)
				point++
			}
		}
	}
	for len(d) < point+nFrac {
		d = append(d, '0')
	}
	intPart = strings.TrimLeft(string(d[:point]), "0")
	frac = string(d[point : point+nFrac])
	return intPart, frac
}

// roundTo rounds x to places decimal places (negative places round to
// tens, hundreds...) with mode, on x's 15 significant digits.
func roundTo(x float64, places int, mode roundMode) float64 {
	ip, fp := fixed(x, places, mode)
	s := ip
	if s == "" {
		s = "0"
	}
	if fp != "" {
		s += "." + fp
	}
	r, _ := strconv.ParseFloat(s, 64)
	if x < 0 {
		r = -r
	}
	return r
}

// numString renders a number the way Sheets shows it in General format
// and in text: up to 15 significant digits, no exponent for ordinary
// magnitudes.
func numString(v float64) string {
	if v == 0 {
		return "0"
	}
	r, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', 15, 64), 64)
	if a := math.Abs(r); a >= 1e21 || a < 1e-9 {
		return strconv.FormatFloat(r, 'E', -1, 64)
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// formatDateToks renders serial v as a date, time or duration.
func formatDateToks(v float64, toks []ptok) string {
	fracDigits, ampm := 0, false
	for i, t := range toks {
		switch t.kind {
		case ptAMPM:
			ampm = true
		case ptSecond, ptElapsed:
			// .0, .00 or .000 right after seconds show fractions.
			if (t.kind == ptSecond || t.s == "s") && i+1 < len(toks) && toks[i+1].kind == ptDot {
				n := 0
				for j := i + 2; j < len(toks) && toks[j].kind == ptDigit && toks[j].s == "0"; j++ {
					n++
				}
				fracDigits = max(fracDigits, min(n, 3))
			}
		}
	}
	neg := false
	for _, t := range toks {
		if t.kind == ptElapsed && v < 0 {
			neg, v = true, -v
			break
		}
	}
	unit := math.Pow(10, float64(fracDigits))
	ticks := int64(math.Round(v * 86400 * unit)) // in 10^-fracDigits seconds
	perDay := int64(86400 * unit)
	days := floorDiv(ticks, perDay)
	inDay := ticks - days*perDay
	secs := inDay / int64(unit)
	sub := inDay % int64(unit)
	y, mo, d := civil(days)
	hour, minute, second := int(secs/3600), int(secs/60%60), int(secs%60)

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, t := range toks {
		switch t.kind {
		case ptLit:
			b.WriteString(t.s)
		case ptYear:
			if t.n <= 2 {
				b.WriteString(pad2(y % 100))
			} else {
				b.WriteString(strconv.Itoa(y))
			}
		case ptMonth:
			if t.n <= 2 && isMinute(toks, i) {
				b.WriteString(padN(minute, t.n))
				break
			}
			switch t.n {
			case 1, 2:
				b.WriteString(padN(mo, t.n))
			case 3:
				b.WriteString(monthNames[mo-1][:3])
			case 5:
				b.WriteString(monthNames[mo-1][:1])
			default:
				b.WriteString(monthNames[mo-1])
			}
		case ptDay:
			switch t.n {
			case 1, 2:
				b.WriteString(padN(d, t.n))
			case 3:
				b.WriteString(dayNames[weekday(days)][:3])
			default:
				b.WriteString(dayNames[weekday(days)])
			}
		case ptHour:
			h := hour
			if ampm {
				h = (h+11)%12 + 1
			}
			b.WriteString(padN(h, t.n))
		case ptSecond:
			b.WriteString(padN(second, t.n))
		case ptElapsed:
			total := ticks / int64(unit)
			switch t.s {
			case "h":
				b.WriteString(padN(int(total/3600), t.n))
			case "m":
				b.WriteString(padN(int(total/60), t.n))
			default:
				b.WriteString(padN(int(total), t.n))
			}
		case ptAMPM:
			pm := hour >= 12
			switch {
			case t.s == "A/P" && pm:
				b.WriteString("P")
			case t.s == "A/P":
				b.WriteString("A")
			case pm:
				b.WriteString("PM")
			default:
				b.WriteString("AM")
			}
		case ptDot:
			b.WriteByte('.')
			if secondsFraction(toks, i) {
				b.WriteString(padN(int(sub), fracDigits))
			}
		case ptDigit:
			// Fraction-of-second digits are written with the dot.
			if !afterSecondsDot(toks, i) {
				b.WriteString(t.s)
			}
		case ptComma:
			b.WriteByte(',')
		case ptPercent:
			b.WriteByte('%')
		}
	}
	return b.String()
}

// secondsFraction reports whether the dot at i starts fractions of a
// second (s.00).
func secondsFraction(toks []ptok, i int) bool {
	return i > 0 && i+1 < len(toks) && toks[i+1].kind == ptDigit &&
		(toks[i-1].kind == ptSecond || toks[i-1].kind == ptElapsed && toks[i-1].s == "s")
}

// afterSecondsDot reports whether the digit at i belongs to fractions of
// a second, which are written with the dot.
func afterSecondsDot(toks []ptok, i int) bool {
	j := i - 1
	for j >= 0 && toks[j].kind == ptDigit {
		j--
	}
	return j >= 0 && toks[j].kind == ptDot && secondsFraction(toks, j)
}

// isMinute resolves m and mm: minutes right after hours or before
// seconds, otherwise the month.
func isMinute(toks []ptok, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch toks[j].kind {
		case ptHour:
			return true
		case ptElapsed:
			return toks[j].s == "h"
		case ptYear, ptMonth, ptDay, ptSecond:
			j = -1
		}
	}
	for j := i + 1; j < len(toks); j++ {
		switch toks[j].kind {
		case ptSecond:
			return true
		case ptElapsed:
			return toks[j].s == "s"
		case ptYear, ptMonth, ptDay, ptHour:
			return false
		}
	}
	return false
}

var (
	monthNames = [...]string{"January", "February", "March", "April", "May", "June", "July",
		"August", "September", "October", "November", "December"}
	dayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
)

func pad2(v int) string { return padN(v, 2) }

func padN(v, n int) string {
	s := strconv.Itoa(v)
	if len(s) < n {
		s = strings.Repeat("0", n-len(s)) + s
	}
	return s
}
