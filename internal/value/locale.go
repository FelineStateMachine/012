package value

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
)

// Entries are stored as typed in en-US, the canonical locale, so a sheet
// reads the same in every locale. What is typed in another locale is
// rewritten to that form as it's entered (Canonicalize), and shown in
// the locale's form when edited again (Localize): 1.234,5 in de-DE is
// stored as 1,234.5, 26.09.2026 as 09/26/2026 and 12,50 € as $12.50.
// Both keep what was typed as far as they can (grouping, leading zeros,
// ISO dates, month names, times), and the value is the same either way.

// ParseValueIn is ParseValue for an entry typed in loc.
func ParseValueIn(s string, loc *locale.Locale) (float64, Format, bool) {
	if loc.IsCanonical() {
		return ParseValue(s)
	}
	c, ok := Canonicalize(s, loc)
	if !ok {
		return 0, Format{}, false
	}
	return ParseValue(c)
}

// Canonicalize rewrites a number, date or time typed in loc as it would
// be typed in en-US, reporting false when s isn't one in loc.
func Canonicalize(s string, loc *locale.Locale) (string, bool) {
	if loc.IsCanonical() {
		_, _, ok := ParseValue(s)
		return s, ok
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return s, false
	}
	if c, ok := canonicalNumber(t, loc); ok {
		if _, _, ok := parseNumberFormat(c); ok {
			return c, true
		}
	}
	if c := canonicalDateTime(t, loc); c != "" {
		if _, _, ok := ParseDateTime(c); ok {
			return c, true
		}
	}
	return s, false
}

// Localize is the inverse of Canonicalize: a number, date or time entry
// as stored, written as typed in loc. Anything else is returned as is.
func Localize(s string, loc *locale.Locale) string {
	if loc.IsCanonical() {
		return s
	}
	t := strings.TrimSpace(s)
	if _, _, ok := parseNumberFormat(t); ok {
		return localNumber(t, loc)
	}
	if _, _, ok := ParseDateTime(t); ok {
		return localDateTime(t, loc)
	}
	return s
}

// spaces are the spaces that may stand between a number and its
// currency symbol or percent sign.
const spaces = "   "

// canonicalNumber rewrites a number typed in loc with en-US's
// separators, $ for loc's currency symbol before it and % after it. It
// reports false for characters that are no part of a number in loc,
// such as the other locale's separators.
func canonicalNumber(t string, loc *locale.Locale) (string, bool) {
	sign := ""
	if t[0] == '-' || t[0] == '+' {
		sign, t = t[:1], t[1:]
	}
	cur := false
	if rest, ok := strings.CutPrefix(t, loc.Currency); ok {
		t, cur = strings.TrimLeft(rest, spaces), true
	} else if rest, ok := strings.CutSuffix(t, loc.Currency); ok {
		t, cur = strings.TrimRight(rest, spaces), true
	}
	pct := false
	if rest, ok := strings.CutSuffix(t, "%"); ok {
		t, pct = strings.TrimRight(rest, spaces), true
	}
	if t == "" {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(t) + 3)
	b.WriteString(sign)
	if cur {
		b.WriteByte('$')
	}
	for _, r := range t {
		switch {
		case r == rune(loc.Decimal):
			b.WriteByte('.')
		case loc.IsGroup(r):
			b.WriteByte(',')
		case r >= '0' && r <= '9', r == 'e', r == 'E', r == '+', r == '-':
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	if pct {
		b.WriteByte('%')
	}
	return b.String(), true
}

// localNumber writes a number entry as stored (en-US) as typed in loc.
func localNumber(t string, loc *locale.Locale) string {
	sign := ""
	if t[0] == '-' || t[0] == '+' {
		sign, t = t[:1], t[1:]
	}
	t, cur := strings.CutPrefix(t, "$")
	var b strings.Builder
	for i := 0; i < len(t); i++ {
		switch c := t[i]; c {
		case '.':
			b.WriteByte(loc.Decimal)
		case ',':
			b.WriteString(loc.Group)
		default:
			b.WriteByte(c)
		}
	}
	num := b.String()
	if !cur {
		return sign + num
	}
	gap := ""
	if loc.Space {
		gap = " "
	}
	if loc.After {
		return sign + num + gap + loc.Currency
	}
	return sign + loc.Currency + gap + num
}

// canonicalDateTime rewrites a date typed with numbers in loc's order
// (26.09.2026, 26/9) in en-US's (9/26/2026), keeping any time after it.
// Other dates and times are returned as they are; "" is a date of
// numbers that can't be one in loc.
func canonicalDateTime(t string, loc *locale.Locale) string {
	head, tail, spaced := strings.Cut(t, " ")
	date, shaped := canonicalDate(head, loc)
	if !shaped {
		return t
	}
	if date == "" || !spaced {
		return date
	}
	if loc.Decimal == ',' {
		tail = strings.Replace(tail, ",", ".", 1) // 14:30:15,5
	}
	return date + " " + tail
}

// canonicalDate rewrites a date of numbers typed in loc in en-US's
// order. shaped is false when s isn't numbers and separators; the date
// is "" when they can't make a date in loc.
func canonicalDate(s string, loc *locale.Locale) (date string, shaped bool) {
	sep := dateSep(s, loc)
	if sep == 0 {
		return "", false
	}
	if sep == '.' {
		s = strings.TrimSuffix(s, ".") // 26.09.2026.
	}
	p := strings.Split(s, string(sep))
	if len(p) < 2 || len(p) > 3 || strings.Contains(s, string(sep)+string(sep)) || p[0] == "" || p[len(p)-1] == "" {
		return "", true
	}
	if len(p) == 3 && len(p[0]) == 4 { // the year first, in every locale
		switch {
		case loc.Order == locale.YMD:
			return p[1] + "/" + p[2] + "/" + p[0], true
		case sep == '.':
			return p[0] + "/" + p[1] + "/" + p[2], true
		}
		return s, true
	}
	var d, m, y string
	switch {
	case loc.Order == locale.DMY:
		d, m = p[0], p[1]
	case loc.Order == locale.YMD && len(p) == 3:
		y, m, d = p[0], p[1], p[2]
	default:
		m, d = p[0], p[1]
	}
	if len(p) == 3 && y == "" {
		y = p[2]
	}
	if y == "" {
		return m + "/" + d, true
	}
	return m + "/" + d + "/" + y, true
}

// dateSep is the separator of a date of numbers typed in loc: / and -
// everywhere, . where loc writes dates with it. It is 0 when s is
// anything else, or mixes separators.
func dateSep(s string, loc *locale.Locale) byte {
	var sep byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case isDigit(c):
			continue
		case c != '/' && c != '-' && (c != '.' || loc.DateSep != '.'):
			return 0
		case sep != 0 && c != sep:
			return 0
		}
		sep = c
	}
	return sep
}

// localDateTime writes a date or time entry as stored (en-US) as typed
// in loc: a date of numbers in loc's order with its separator. ISO
// dates, month names and times stay as they are.
func localDateTime(t string, loc *locale.Locale) string {
	head, tail, spaced := strings.Cut(t, " ")
	sep := dateSep(head, locale.Canonical)
	if sep == 0 {
		return t
	}
	p := strings.Split(head, string(sep))
	if len(p[0]) == 4 || len(p) < 2 {
		return t // the year first: as typed, in every locale
	}
	m, d := p[0], p[1]
	parts := []string{m, d}
	switch {
	case loc.Order == locale.DMY:
		parts = []string{d, m}
		if len(p) == 3 {
			parts = append(parts, p[2])
		}
	case loc.Order == locale.YMD && len(p) == 3:
		parts = []string{p[2], m, d}
	case len(p) == 3:
		parts = append(parts, p[2])
	}
	sep = loc.DateSep
	if len(parts) == 2 && sep == loc.Decimal {
		sep = '/' // 26.9 would be a number in de-CH
	}
	date := strings.Join(parts, string(sep))
	if !spaced {
		return date
	}
	if loc.Decimal == ',' {
		tail = strings.Replace(tail, ".", ",", 1)
	}
	return date + " " + tail
}

// currencyCode is the Currency format's pattern in loc, with dec
// decimals: "$"#,##0.00 in en-US, #,##0.00" €" in de-DE.
func currencyCode(loc *locale.Locale, dec string) string {
	sym := loc.Currency
	if loc.Space {
		if loc.After {
			sym = " " + sym
		} else {
			sym += " "
		}
	}
	if loc.After {
		return "#,##0" + dec + `"` + sym + `"`
	}
	return `"` + sym + `"#,##0` + dec
}

// CodeIn is Code as shown in loc: the Currency format with loc's symbol
// and the Date, Time and Date time formats in loc's order. Patterns of
// their own are the same in every locale.
func (f Format) CodeIn(loc *locale.Locale) string {
	if loc.IsCanonical() || f.Pattern != "" && (f.Kind == FmtCustom || f.Kind.IsTime()) {
		return f.Code()
	}
	switch f.Kind {
	case FmtCurrency:
		dec := ""
		if f.Decimals > 0 {
			dec = "." + strings.Repeat("0", min(f.Decimals, MaxDecimals))
		}
		return currencyCode(loc, dec)
	case FmtDate:
		return loc.Date
	case FmtTime:
		return loc.Time
	case FmtDateTime:
		return loc.DateTime()
	}
	return f.Code()
}
