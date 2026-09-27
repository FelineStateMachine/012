package formula

import (
	"errors"
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
)

// Formulas are stored and parsed in en-US's syntax: 1.5 for a number, ,
// between arguments and ; between an array's rows. A locale whose
// decimal separator is a comma writes 1,5, puts ; between arguments and
// \ between the values of an array's row, as Sheets does: =ROUND(1,5; 0)
// and {1\2;3\4}. Function names stay in English. Localize and
// Delocalize translate at the edges, where formulas are shown and typed;
// each separator is one byte in both, so a position in one is the same
// position in the other (a parse error's, the caret's).

// Localize writes a formula (or any part of one) as stored in loc's
// syntax. Strings and sheet names are left alone.
func Localize(src string, loc *locale.Locale) string {
	return translate(src, loc, true)
}

// Delocalize reads a formula typed in loc's syntax into the syntax it is
// stored and parsed in. It works on unfinished formulas too.
func Delocalize(src string, loc *locale.Locale) string {
	return translate(src, loc, false)
}

// LocalizeError writes the message of a parse error in err (the error
// itself, or one it wraps) in loc's syntax, as the formula was typed:
// "Expected ; or ) in ROUND" where arguments are separated by ;, and
// the text typed where it names what it didn't expect. The position is
// the same in both syntaxes.
func LocalizeError(err error, loc *locale.Locale) error {
	var pe *ParseError
	if SameSyntax(loc) || !errors.As(err, &pe) {
		return err
	}
	msg := localMessage(pe, loc)
	if msg == pe.Msg {
		return err
	}
	if err == error(pe) {
		return &ParseError{Pos: pe.Pos, Msg: msg, src: pe.src}
	}
	return errors.New(strings.Replace(err.Error(), pe.Msg, msg, 1))
}

// localMessage is a parse error's message in loc's syntax.
func localMessage(pe *ParseError, loc *locale.Locale) string {
	typed := Localize(pe.src, loc)
	arg, col := string(loc.ArgSep()), string(loc.ColSep())
	switch msg := pe.Msg; {
	case strings.HasPrefix(msg, "Expected , or ) in "):
		return "Expected " + arg + " or )" + strings.TrimPrefix(msg, "Expected , or )")
	case msg == "Expected , ; or } in an array":
		return "Expected " + col + " ; or } in an array"
	case strings.HasPrefix(msg, "Wrong number of arguments to "):
		return strings.ReplaceAll(msg, ", ", arg+" ")
	}
	for _, lead := range []string{"Unexpected ", "Invalid number "} {
		tok, ok := strings.CutPrefix(pe.Msg, lead)
		if end := pe.Pos + len(tok); ok && pe.Pos >= 0 && end <= len(typed) && len(typed) == len(pe.src) {
			return lead + typed[pe.Pos:end]
		}
	}
	return pe.Msg
}

// SameSyntax reports whether loc writes formulas as they are stored.
func SameSyntax(loc *locale.Locale) bool {
	return loc == nil || loc.Decimal == '.' && loc.ArgSep() == ',' && loc.ColSep() == ','
}

// separators are the characters a translation swaps, from and to.
type separators struct {
	dec, arg, col       byte // in the syntax read
	toDec, toArg, toCol byte // in the syntax written
}

func translate(src string, loc *locale.Locale, out bool) string {
	if SameSyntax(loc) {
		return src
	}
	sep := separators{dec: '.', arg: ',', col: ',', toDec: loc.Decimal, toArg: loc.ArgSep(), toCol: loc.ColSep()}
	if !out {
		sep = separators{dec: loc.Decimal, arg: loc.ArgSep(), col: loc.ColSep(), toDec: '.', toArg: ',', toCol: ','}
	}
	b := []byte(src)
	var open []byte // the ( and { around the position, innermost last
	for i := 0; i < len(b); {
		switch c := b[i]; {
		case c == '"' || c == '\'':
			i = skipQuoted(b, i)
		case c == '@' || isIdentStart(c):
			i = skipIdent(b, i+1)
		case isDigit(c) || c == sep.dec && i+1 < len(b) && isDigit(b[i+1]):
			i = sep.number(b, i)
		case c == '(' || c == '{':
			open = append(open, c)
			i++
		case c == ')' || c == '}':
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
			i++
		default:
			inArray := len(open) > 0 && open[len(open)-1] == '{'
			switch {
			case inArray && c == sep.col:
				b[i] = sep.toCol
			case !inArray && c == sep.arg:
				b[i] = sep.toArg
			}
			i++
		}
	}
	return string(b)
}

// skipQuoted skips a string or quoted sheet name starting at i, with its
// quote doubled inside; an unclosed one runs to the end.
func skipQuoted(b []byte, i int) int {
	q := b[i]
	for i++; i < len(b); i++ {
		if b[i] != q {
			continue
		}
		if i+1 < len(b) && b[i+1] == q {
			i++
			continue
		}
		return i + 1
	}
	return len(b)
}

// skipIdent skips the rest of a reference, name or function name.
func skipIdent(b []byte, i int) int {
	for i < len(b) && (isIdentPart(b[i]) || b[i] >= 0x80) {
		i++
	}
	return i
}

// number rewrites the decimal separator of the number at i, returning
// where it ends: digits, one decimal separator, and an exponent.
func (sep separators) number(b []byte, i int) int {
	point := false
	for ; i < len(b); i++ {
		switch c := b[i]; {
		case isDigit(c):
		case c == sep.dec && !point && (i+1 >= len(b) || b[i+1] != sep.dec):
			b[i], point = sep.toDec, true
		case (c == 'e' || c == 'E') && exponentAt(b, i+1):
			i++
			if b[i] == '+' || b[i] == '-' {
				i++
			}
		default:
			return i
		}
	}
	return i
}

// exponentAt reports whether an exponent's sign or digits start at i.
func exponentAt(b []byte, i int) bool {
	if i < len(b) && (b[i] == '+' || b[i] == '-') {
		i++
	}
	return i < len(b) && isDigit(b[i])
}
