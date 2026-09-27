package numfmt

import (
	"math"
	"strconv"
	"strings"
)

// numLayout is where a number section puts each part of the number: the
// token indexes of its integer, decimal and exponent placeholders, its
// decimal point and exponent, and how it scales the value.
type numLayout struct {
	toks                 []ptok
	intPH, fracPH, expPH []int
	dot, exp             int // token index, or -1 for none
	percents             int // each % multiplies by 100
	general              bool
	// grouped is set by commas between the integer placeholders, unless
	// literals sit among them too (a phone number pattern).
	grouped bool
	scale   int // commas right after the last integer placeholder divide by 1000 each
}

func layoutNumber(toks []ptok) numLayout {
	l := numLayout{toks: toks, dot: -1, exp: -1}
	for i, t := range toks {
		switch t.kind {
		case ptDigit:
			l.addPlaceholder(i)
		case ptDot:
			if l.dot < 0 && l.exp < 0 {
				l.dot = i
			}
		case ptExp:
			if l.exp < 0 {
				l.exp = i
			}
		case ptPercent:
			l.percents++
		case ptGeneral:
			l.general = true
		}
	}
	if len(l.intPH) > 0 {
		first, last := l.intPH[0], l.intPH[len(l.intPH)-1]
		commas, literals := false, false
		for i := first + 1; i < last; i++ {
			commas = commas || toks[i].kind == ptComma
			literals = literals || toks[i].kind == ptLit
		}
		l.grouped = commas && !literals
		for i := last + 1; i < len(toks) && toks[i].kind == ptComma; i++ {
			l.scale++
		}
	}
	return l
}

// addPlaceholder files the digit placeholder at token i under the part of
// the number it falls in.
func (l *numLayout) addPlaceholder(i int) {
	switch {
	case l.exp >= 0:
		l.expPH = append(l.expPH, i)
	case l.dot >= 0:
		l.fracPH = append(l.fracPH, i)
	default:
		l.intPH = append(l.intPH, i)
	}
}

// numDigits are the digits of a number as a section shows it.
type numDigits struct {
	whole  string // integer digits, "" for zero
	frac   string // one digit per decimal placeholder
	exp    string // exponent digits, zero-padded to the exponent's placeholders
	expNeg bool
}

// digits rounds v to what the layout shows.
func (l *numLayout) digits(v float64) numDigits {
	switch {
	case l.general && len(l.intPH) == 0:
		// General shows its own decimals; placeholders after it get zeros.
		return numDigits{whole: General(v), frac: strings.Repeat("0", len(l.fracPH))}
	case l.exp >= 0:
		return l.scientific(v)
	}
	whole, frac := Fixed(v, len(l.fracPH), HalfUp)
	return numDigits{whole: whole, frac: frac}
}

// scientific splits v into a mantissa of one integer digit and an
// exponent.
func (l *numLayout) scientific(v float64) numDigits {
	e := 0
	if v != 0 {
		e = int(math.Floor(math.Log10(v)))
	}
	whole, frac := Fixed(v/math.Pow(10, float64(e)), len(l.fracPH), HalfUp)
	if whole == "10" { // rounding carried into another digit
		e++
		whole, frac = Fixed(v/math.Pow(10, float64(e)), len(l.fracPH), HalfUp)
	}
	exp := strconv.Itoa(max(e, -e))
	if len(exp) < len(l.expPH) {
		exp = strings.Repeat("0", len(l.expPH)-len(exp)) + exp
	}
	return numDigits{whole: whole, frac: frac, exp: exp, expNeg: e < 0}
}

// place spreads the digits over the integer and decimal placeholders,
// returning each placeholder's text by token index.
func (l *numLayout) place(d numDigits) []string {
	out := make([]string, len(l.toks))
	// Right to left; the leftmost placeholder takes any remaining digits.
	rem := d.whole
	for k := len(l.intPH) - 1; k >= 0; k-- {
		i := l.intPH[k]
		switch ph := l.toks[i].s; {
		case k == 0 && rem != "":
			out[i], rem = rem, ""
		case rem != "":
			out[i], rem = rem[len(rem)-1:], rem[:len(rem)-1]
		case ph == "0":
			out[i] = "0"
		case ph == "?":
			out[i] = " "
		}
	}
	// Trailing zeros disappear under # and turn to spaces under ?.
	trimming := true
	for k := len(l.fracPH) - 1; k >= 0; k-- {
		i := l.fracPH[k]
		digit := d.frac[k : k+1]
		switch ph := l.toks[i].s; {
		case trimming && digit == "0" && ph == "#":
		case trimming && digit == "0" && ph == "?":
			out[i] = " "
		default:
			trimming = false
			out[i] = digit
		}
	}
	return out
}

// formatNumber renders |v| with a number section; neg adds a minus sign.
func formatNumber(v float64, neg bool, toks []ptok) string {
	l := layoutNumber(toks)
	for range l.percents {
		v *= 100
	}
	for range l.scale {
		v /= 1000
	}
	d := l.digits(v)
	if neg && strings.Trim(d.whole+d.frac, "0") == "" {
		neg = false // don't show -0.00
	}
	placed := l.place(d)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, t := range toks {
		l.write(&b, i, t, d, placed)
	}
	return b.String()
}

// write writes token i of the section.
func (l *numLayout) write(b *strings.Builder, i int, t ptok, d numDigits, placed []string) {
	switch t.kind {
	case ptLit:
		b.WriteString(t.s)
	case ptDigit:
		l.writeDigit(b, i, d, placed)
	case ptDot:
		if i == l.dot {
			b.WriteByte('.')
		}
	case ptPercent:
		b.WriteByte('%')
	case ptExp:
		if i == l.exp {
			b.WriteString(expSign(t.s, d.expNeg))
		}
	case ptGeneral:
		if len(l.intPH) == 0 {
			b.WriteString(d.whole)
		}
	case ptComma:
		// Grouping and scaling commas are consumed; others are literal.
		if len(l.intPH) == 0 || i < l.intPH[0] || (l.dot >= 0 && i > l.dot) {
			b.WriteByte(',')
		}
	}
}

// writeDigit writes the digit placeholder at token i: the integer part
// grouped in thousands at the first integer placeholder, the exponent at
// the first exponent placeholder, otherwise the placeholder's own digit.
func (l *numLayout) writeDigit(b *strings.Builder, i int, d numDigits, placed []string) {
	inInt := len(l.intPH) > 0 && i >= l.intPH[0] && i <= l.intPH[len(l.intPH)-1]
	switch {
	case inInt && l.grouped && i == l.intPH[0]:
		var digits strings.Builder
		for _, p := range l.intPH {
			digits.WriteString(placed[p])
		}
		b.WriteString(group(digits.String()))
	case inInt:
		if !l.grouped {
			b.WriteString(placed[i])
		}
	case len(l.expPH) > 0 && i == l.expPH[0]:
		b.WriteString(d.exp)
	case len(l.expPH) > 0 && i > l.expPH[0]:
	default:
		b.WriteString(placed[i])
	}
}

// expSign writes an exponent marker: E- for negative exponents, E+ when
// the pattern asks for the sign.
func expSign(sign string, neg bool) string {
	switch {
	case neg:
		return "E-"
	case sign == "+":
		return "E+"
	}
	return "E"
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
