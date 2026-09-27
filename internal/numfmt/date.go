package numfmt

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/locale"
)

// clock is a serial broken into the fields date and time patterns show.
type clock struct {
	toks                 []ptok
	year, month, day     int
	weekday              int // 0 for Sunday
	hour, minute, second int
	ampm                 bool  // hours count 1 to 12
	elapsed              int64 // whole seconds since day 0, for [h], [m] and [s]
	fracDigits           int   // decimals of a second shown
	sub                  int   // the fraction of the second, in fracDigits digits
	names                *locale.Names
}

func newClock(v float64, toks []ptok, names *locale.Names) clock {
	c := clock{toks: toks, ampm: hasKind(toks, ptAMPM), fracDigits: secondDecimals(toks), names: names}
	unit := math.Pow(10, float64(c.fracDigits))
	ticks := int64(math.Round(v * 86400 * unit)) // in 10^-fracDigits seconds
	perDay := int64(86400 * unit)
	days := floorDiv(ticks, perDay)
	inDay := ticks - days*perDay
	secs := inDay / int64(unit)
	c.sub = int(inDay % int64(unit))
	c.elapsed = ticks / int64(unit)
	c.year, c.month, c.day = Civil(days)
	c.weekday = Weekday(days)
	c.hour, c.minute, c.second = int(secs/3600), int(secs/60%60), int(secs%60)
	return c
}

// secondDecimals is how many decimals of a second a pattern shows: .0,
// .00 or .000 right after seconds.
func secondDecimals(toks []ptok) int {
	n := 0
	for i, t := range toks {
		seconds := t.kind == ptSecond || t.kind == ptElapsed && t.s == "s"
		if !seconds || i+1 >= len(toks) || toks[i+1].kind != ptDot {
			continue
		}
		zeros := 0
		for j := i + 2; j < len(toks) && toks[j].kind == ptDigit && toks[j].s == "0"; j++ {
			zeros++
		}
		n = max(n, min(zeros, 3))
	}
	return n
}

// formatDate renders serial v as a date, time or duration, with the
// names of months and days in names' language.
func formatDate(v float64, toks []ptok, names *locale.Names) string {
	var b strings.Builder
	if v < 0 && hasKind(toks, ptElapsed) {
		b.WriteByte('-') // a negative duration
		v = -v
	}
	c := newClock(v, toks, names)
	for i := range toks {
		c.write(&b, i)
	}
	return b.String()
}

// write writes token i of the pattern.
func (c *clock) write(b *strings.Builder, i int) {
	switch t := c.toks[i]; t.kind {
	case ptLit:
		b.WriteString(t.s)
	case ptYear:
		if t.n <= 2 {
			b.WriteString(padN(c.year%100, 2))
		} else {
			b.WriteString(strconv.Itoa(c.year))
		}
	case ptMonth:
		b.WriteString(c.monthText(t.n, i))
	case ptDay:
		b.WriteString(c.dayText(t.n))
	case ptHour:
		b.WriteString(padN(c.hour12(), t.n))
	case ptSecond:
		b.WriteString(padN(c.second, t.n))
	case ptElapsed:
		b.WriteString(c.elapsedText(t))
	case ptAMPM:
		b.WriteString(c.meridiem(t.s))
	case ptDot:
		b.WriteByte('.')
		if secondsFraction(c.toks, i) {
			b.WriteString(padN(c.sub, c.fracDigits))
		}
	case ptDigit:
		// Fraction-of-second digits are written with the dot.
		if !afterSecondsDot(c.toks, i) {
			b.WriteString(t.s)
		}
	case ptComma:
		b.WriteByte(',')
	case ptPercent:
		b.WriteByte('%')
	}
}

// monthText writes m to mmmmm at token i: minutes (see isMinute), the
// month's number, or its name abbreviated, as its first letter, or
// spelled out, in the form a date with a day of the month takes where
// the language has one ("26 września", "wrzesień 2026").
func (c *clock) monthText(n, i int) string {
	switch {
	case n <= 2 && isMinute(c.toks, i):
		return padN(c.minute, n)
	case n <= 2:
		return padN(c.month, n)
	case n == 3:
		return c.names.Short[c.month-1]
	}
	name := c.names.Month(c.month, !hasDayOfMonth(c.toks))
	if n == 5 {
		r, _ := utf8.DecodeRuneInString(name)
		return strings.ToUpper(string(r))
	}
	return name
}

// hasDayOfMonth reports whether a pattern shows the day of the month.
func hasDayOfMonth(toks []ptok) bool {
	for _, t := range toks {
		if t.kind == ptDay && t.n <= 2 {
			return true
		}
	}
	return false
}

// dayText writes d to dddd: the day of the month or of the week.
func (c *clock) dayText(n int) string {
	switch n {
	case 1, 2:
		return padN(c.day, n)
	case 3:
		return c.names.DaysShort[c.weekday]
	}
	return c.names.Days[c.weekday]
}

func (c *clock) hour12() int {
	if c.ampm {
		return (c.hour+11)%12 + 1
	}
	return c.hour
}

// elapsedText writes [h], [m] or [s]: the whole duration in that unit.
func (c *clock) elapsedText(t ptok) string {
	switch t.s {
	case "h":
		return padN(int(c.elapsed/3600), t.n)
	case "m":
		return padN(int(c.elapsed/60), t.n)
	}
	return padN(int(c.elapsed), t.n)
}

// meridiem writes AM/PM or A/P, or the language's words for them.
func (c *clock) meridiem(style string) string {
	pm := c.hour >= 12
	switch {
	case c.names.AM != "" && pm:
		return c.names.PM
	case c.names.AM != "":
		return c.names.AM
	case style == "A/P" && pm:
		return "P"
	case style == "A/P":
		return "A"
	case pm:
		return "PM"
	}
	return "AM"
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

func padN(v, n int) string {
	s := strconv.Itoa(v)
	if len(s) < n {
		s = strings.Repeat("0", n-len(s)) + s
	}
	return s
}
