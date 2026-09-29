// Package numfmt renders numbers the way spreadsheets show them: number
// format patterns as in Sheets' custom number formats and TEXT(),
// General's shortest form, rounding on the 15 significant digits users
// see, and the serial day numbers dates and times are stored as. It knows
// nothing of cells or sheets.
package numfmt

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/locale"
)

// Number format patterns: "#,##0.00", "0.0%", "0.00E+00",
// "$#,##0;($#,##0)", "m/d/yyyy", "h:mm am/pm", "[h]:mm:ss". Up to four
// ;-separated sections apply to positive, negative, zero and text values.

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

// Format renders v with a number format pattern, as TEXT() does.
func Format(v float64, pat string) string { return FormatIn(v, pat, locale.Canonical) }

// FormatIn renders v with a number format pattern as shown in loc: the
// pattern is written as in en-US ("#,##0.00") and the number shows loc's
// decimal and thousands separators ("1.234,50" in de-DE).
func FormatIn(v float64, pat string, loc *locale.Locale) string {
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
		return formatDate(v, toks, patternNames(sec, loc))
	}
	if l, ok := layoutFraction(toks); ok {
		return formatFraction(math.Abs(v), neg, l, loc)
	}
	return formatNumber(math.Abs(v), neg, toks, loc)
}

// patternNames are the names of months and days a date pattern shows:
// those of the locale its [$-407] tag names, as in Excel, or else loc's.
func patternNames(sec string, loc *locale.Locale) *locale.Names {
	for rest := sec; ; {
		i := strings.Index(rest, "[$")
		if i < 0 {
			return loc.Names()
		}
		rest = rest[i+2:]
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return loc.Names()
		}
		_, hex, ok := strings.Cut(rest[:end], "-")
		if id, err := strconv.ParseUint(hex, 16, 32); ok && err == nil {
			if l, found := locale.ByLCID(uint16(id)); found {
				return l.Names()
			}
		}
		rest = rest[end:]
	}
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

// punctuation are the one-character tokens other than digit placeholders,
// by character; ptLit for none. Arrays rather than maps: patterns are
// lexed for every formatted cell drawn.
var punctuation = [256]ptKind{'.': ptDot, ',': ptComma, '%': ptPercent, '@': ptText}

// words are the tokens spelled with letters, matched ignoring case.
var words = []struct {
	word string
	tok  ptok
}{
	{"am/pm", ptok{kind: ptAMPM, s: "AM/PM"}},
	{"a/p", ptok{kind: ptAMPM, s: "A/P"}},
	{"general", ptok{kind: ptGeneral}},
}

// dateLetters are the letters that repeat to make date and time tokens.
var dateLetters = [256]ptKind{'y': ptYear, 'm': ptMonth, 'd': ptDay, 'h': ptHour, 's': ptSecond}

// patternLexer splits one section of a pattern into tokens.
type patternLexer struct {
	sec   string
	lower string // sec with ASCII letters lower-cased, at the same byte offsets
	i     int
	toks  []ptok
}

func lexPattern(sec string) []ptok {
	lx := patternLexer{sec: sec, lower: asciiLower(sec)}
	for lx.i < len(sec) {
		lx.next()
	}
	return lx.toks
}

// emit adds t, which was written in the next width bytes.
func (lx *patternLexer) emit(t ptok, width int) {
	lx.toks = append(lx.toks, t)
	lx.i += width
}

func (lx *patternLexer) lit(s string, width int) { lx.emit(ptok{kind: ptLit, s: s}, width) }

func (lx *patternLexer) next() {
	c, lc := lx.sec[lx.i], lx.lower[lx.i]
	switch {
	case c == '"':
		lx.quoted()
	case c == '[':
		lx.bracketed()
	case (c == '\\' || c == '_' || c == '*') && lx.i+1 < len(lx.sec):
		lx.escaped(c)
	case c == '0' || c == '#' || c == '?':
		lx.emit(ptok{kind: ptDigit, s: lx.sec[lx.i : lx.i+1]}, 1)
	case punctuation[c] != ptLit:
		lx.emit(ptok{kind: punctuation[c]}, 1)
	case lc == 'e' && lx.i+1 < len(lx.sec) && (lx.sec[lx.i+1] == '+' || lx.sec[lx.i+1] == '-'):
		lx.emit(ptok{kind: ptExp, s: lx.sec[lx.i+1 : lx.i+2]}, 2)
	case lx.word(): // emitted
	case dateLetters[lc] != ptLit:
		j := lx.i
		for j < len(lx.sec) && lx.lower[j] == lc {
			j++
		}
		lx.emit(ptok{kind: dateLetters[lc], s: lx.sec[lx.i:j], n: j - lx.i}, j-lx.i)
	default:
		_, w := utf8.DecodeRuneInString(lx.sec[lx.i:])
		lx.lit(lx.sec[lx.i:lx.i+w], w)
	}
}

// quoted reads "text", shown as is; an unclosed quote runs to the end.
func (lx *patternLexer) quoted() {
	end := strings.IndexByte(lx.sec[lx.i+1:], '"')
	if end < 0 {
		end = len(lx.sec) - lx.i - 1
	}
	lx.lit(lx.sec[lx.i+1:lx.i+1+end], end+2)
}

// escaped reads the character after \ (shown as is), _ (a space as wide
// as it) or * (repeated to fill the cell).
func (lx *patternLexer) escaped(c byte) {
	_, w := utf8.DecodeRuneInString(lx.sec[lx.i+1:])
	next := lx.sec[lx.i+1 : lx.i+1+w]
	switch c {
	case '\\':
		lx.lit(next, 1+w)
	case '_':
		lx.lit(" ", 1+w)
	default:
		lx.emit(ptok{kind: ptFill, s: next}, 1+w)
	}
}

// bracketed reads [h], [mm] and [ss] (elapsed time) and [$€-407]
// (a currency symbol). Colors and conditions are ignored; an unclosed
// bracket is literal text.
func (lx *patternLexer) bracketed() {
	end := strings.IndexByte(lx.sec[lx.i:], ']')
	if end < 0 {
		lx.lit(lx.sec[lx.i:], len(lx.sec)-lx.i)
		return
	}
	inner := lx.lower[lx.i+1 : lx.i+end]
	switch {
	case inner != "" && strings.Trim(inner, inner[:1]) == "" && strings.ContainsAny(inner[:1], "hms"):
		lx.toks = append(lx.toks, ptok{kind: ptElapsed, s: inner[:1], n: len(inner)})
	case strings.HasPrefix(inner, "$"):
		sym, _, _ := strings.Cut(lx.sec[lx.i+2:lx.i+end], "-")
		lx.toks = append(lx.toks, ptok{kind: ptLit, s: sym})
	}
	lx.i += end + 1
}

// word reads AM/PM, A/P or General, reporting whether one was there.
func (lx *patternLexer) word() bool {
	for _, w := range words {
		if strings.HasPrefix(lx.lower[lx.i:], w.word) {
			lx.emit(w.tok, len(w.word))
			return true
		}
	}
	return false
}

// asciiLower lower-cases ASCII letters only, keeping byte offsets.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
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

// hasKind reports whether any token is of kind k.
func hasKind(toks []ptok, k ptKind) bool {
	for _, t := range toks {
		if t.kind == k {
			return true
		}
	}
	return false
}
