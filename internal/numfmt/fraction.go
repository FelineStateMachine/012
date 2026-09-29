package numfmt

import (
	"math"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
)

// Fraction patterns: "# ?/?" shows 1.75 as 1 3/4, "# ??/??" allows
// denominators up to 99, "# ?/8" shows eighths, and "?/?" with no whole
// part shows 7/4. Placeholders pad as in numbers: ? with a space, 0 with
// a zero, # with nothing.

// fracLayout is where a fraction section puts each part: the tokens of
// the whole number (with anything before it), those between it and the
// numerator, the numerator's and denominator's placeholders, and the
// rest.
type fracLayout struct {
	whole, gap, num, den, rest []ptok
	fixed                      int // the denominator written in the pattern, or 0
}

// layoutFraction finds a fraction in a number section: digit
// placeholders, a /, and placeholders or digits for the denominator.
func layoutFraction(toks []ptok) (fracLayout, bool) {
	slash := -1
	for i := 1; i+1 < len(toks); i++ {
		if toks[i].kind == ptLit && toks[i].s == "/" && toks[i-1].kind == ptDigit && denToken(toks[i+1]) {
			slash = i
			break
		}
	}
	if slash < 0 {
		return fracLayout{}, false
	}
	start := slash
	for start > 0 && toks[start-1].kind == ptDigit {
		start--
	}
	end := slash + 1
	for end < len(toks) && denToken(toks[end]) {
		end++
	}
	l := fracLayout{num: toks[start:slash], den: toks[slash+1 : end], rest: toks[end:]}
	last := start - 1
	for last >= 0 && toks[last].kind != ptDigit {
		last--
	}
	l.whole, l.gap = toks[:last+1], toks[last+1:start]
	var d strings.Builder
	for _, t := range l.den {
		d.WriteString(t.s)
	}
	if n, err := strconv.Atoi(d.String()); err == nil && n > 0 {
		l.fixed = n
	}
	return l, true
}

// denToken reports whether t can be part of a denominator: a digit
// placeholder or a digit written out.
func denToken(t ptok) bool {
	return t.kind == ptDigit || t.kind == ptLit && len(t.s) == 1 && t.s[0] >= '0' && t.s[0] <= '9'
}

// hasWhole reports whether the section shows a whole number before the
// fraction.
func (l fracLayout) hasWhole() bool { return hasKind(l.whole, ptDigit) }

// formatFraction renders |v| with a fraction section as shown in loc;
// neg adds a minus sign.
func formatFraction(v float64, neg bool, l fracLayout, loc *locale.Locale) string {
	whole := math.Floor(v)
	n, d := l.approximate(v - whole)
	if n == d {
		whole, n = whole+1, 0
	}
	if !l.hasWhole() {
		n += int(whole) * d
		whole = 0
	}
	var b strings.Builder
	if neg && (whole != 0 || n != 0) {
		b.WriteByte('-')
	}
	w := ""
	if l.hasWhole() {
		w = formatNumber(whole, false, l.whole, loc)
	} else {
		w = literals(l.whole)
	}
	switch {
	case n == 0 && l.hasWhole():
		if strings.TrimSpace(w) == "" {
			w = "0"
		}
		b.WriteString(w)
	case n == 0 && !l.hasWhole() && l.fixed == 0:
		b.WriteString(w + "0/1")
	default:
		b.WriteString(w)
		if strings.TrimSpace(w) != "" {
			b.WriteString(literals(l.gap))
		}
		b.WriteString(padDigits(strconv.Itoa(n), l.num, true))
		b.WriteByte('/')
		if l.fixed > 0 {
			b.WriteString(strconv.Itoa(l.fixed))
		} else {
			b.WriteString(padDigits(strconv.Itoa(d), l.den, false))
		}
	}
	b.WriteString(literals(l.rest))
	return b.String()
}

// approximate is the fraction n/d nearest f (0 <= f < 1): with the
// pattern's denominator, or the nearest with a denominator of as many
// digits as its placeholders, the smallest such denominator on a tie.
func (l fracLayout) approximate(f float64) (n, d int) {
	if l.fixed > 0 {
		return int(math.Round(f * float64(l.fixed))), l.fixed
	}
	maxD := int(math.Pow10(min(len(l.den), 5))) - 1
	n, d = int(math.Round(f)), 1
	best := math.Abs(f - float64(n))
	for q := 2; q <= maxD && best > 0; q++ {
		p := math.Round(f * float64(q))
		if e := math.Abs(f - p/float64(q)); e < best {
			n, d, best = int(p), q, e
		}
	}
	return n, d
}

// padDigits fills the placeholders phs with digits: those it has room
// for are padded, a ? with a space and a 0 with a zero, on the left of a
// numerator (right) or the right of a denominator (left).
func padDigits(digits string, phs []ptok, right bool) string {
	var pad strings.Builder
	for i := len(digits); i < len(phs); i++ {
		switch phs[i].s {
		case "?":
			pad.WriteByte(' ')
		case "0":
			pad.WriteByte('0')
		}
	}
	if right {
		return pad.String() + digits
	}
	return digits + pad.String()
}

// literals is the text of the literal tokens among toks.
func literals(toks []ptok) string {
	var b strings.Builder
	for _, t := range toks {
		if t.kind == ptLit {
			b.WriteString(t.s)
		}
	}
	return b.String()
}
