package fileio

import (
	"strconv"
	"strings"
)

// shiftFormula moves the relative references of an Excel formula by
// dCol columns and dRow rows, as Excel does to copy a shared formula
// from its first cell to the others: A1 and A$1 move, $A$1 doesn't, and
// whole columns (A:B) and rows (1:2) move along their axis. Text,
// quoted sheet names and function names stay as they are. A reference
// moved off the sheet becomes #REF!.
func shiftFormula(f string, dCol, dRow int) string {
	if dCol == 0 && dRow == 0 {
		return f
	}
	var b strings.Builder
	for i := 0; i < len(f); {
		c := f[i]
		switch {
		case c == '"':
			j := stringEnd(f, i)
			if j < 0 {
				j = len(f)
			}
			b.WriteString(f[i:j])
			i = j
		case c == '\'':
			j := quoteEnd(f, i)
			b.WriteString(f[i:j])
			i = j
		case c == '[':
			// A structured reference or an external book's index.
			j := strings.IndexByte(f[i:], ']')
			if j < 0 {
				j = len(f) - i - 1
			}
			b.WriteString(f[i : i+j+1])
			i += j + 1
		case isRefByte(c):
			j := i
			for j < len(f) && isRefByte(f[j]) {
				j++
			}
			if word := f[i:j]; j < len(f) && f[j] == '(' {
				b.WriteString(word) // a function
			} else {
				b.WriteString(shiftRef(word, dCol, dRow))
			}
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func isRefByte(c byte) bool { return isIdentByte(c) || c == ':' || c == '!' }

// refPart is one side of a reference: a cell (A1), a column (A) or a
// row (1), each part with or without $.
type refPart struct {
	prefix         string // a sheet name and !, if any
	col, row       int    // from 1; 0 when absent
	absCol, absRow bool
}

// shiftRef moves a reference such as A1, $A1:B$2, Sheet1!A1 or A:B, and
// returns anything else (a name, a number) unchanged.
func shiftRef(word string, dCol, dRow int) string {
	sides := strings.Split(word, ":")
	if len(sides) > 2 {
		return word
	}
	var parts [2]refPart
	for i, s := range sides {
		p, ok := parseRefPart(s)
		if !ok {
			return word
		}
		parts[i] = p
	}
	first := parts[0]
	for _, p := range parts[1:len(sides)] {
		if (p.col > 0) != (first.col > 0) || (p.row > 0) != (first.row > 0) {
			return word // mixed kinds: not a reference
		}
	}
	if len(sides) == 1 && (first.col == 0 || first.row == 0) {
		return word // a lone column or row is a name or a number
	}
	out := make([]string, len(sides))
	for i, p := range parts[:len(sides)] {
		if p.col > 0 && !p.absCol {
			p.col += dCol
		}
		if p.row > 0 && !p.absRow {
			p.row += dRow
		}
		if first.col > 0 && (p.col < 1 || p.col > excelCols) || first.row > 0 && (p.row < 1 || p.row > excelRows) {
			return first.prefix + "#REF!"
		}
		out[i] = p.String()
	}
	return strings.Join(out, ":")
}

// parseRefPart parses [Sheet!][$]COL[$]ROW, [$]COL or [$]ROW.
func parseRefPart(s string) (refPart, bool) {
	var p refPart
	if k := strings.LastIndexByte(s, '!'); k >= 0 {
		p.prefix, s = s[:k+1], s[k+1:]
	}
	i := 0
	dollar := i < len(s) && s[i] == '$'
	if dollar {
		i++
	}
	letters := i
	for i < len(s) && isLetter(s[i]) && i-letters < 3 {
		p.col = p.col*26 + int(upper(s[i])-'A'+1)
		i++
	}
	if i == letters {
		p.absRow = dollar // no column: a row, and the $ is the row's
	} else {
		p.absCol = dollar
		if i < len(s) && s[i] == '$' {
			p.absRow = true
			i++
		}
	}
	digits := i
	for i < len(s) && isDigitByte(s[i]) && i-digits < 7 {
		p.row = p.row*10 + int(s[i]-'0')
		i++
	}
	switch {
	case i != len(s), i == letters, p.col > excelCols, p.row > excelRows:
		return p, false
	case i > digits && p.row == 0, i == digits && p.absRow:
		return p, false // row 0, or a $ with no row after it
	}
	return p, true
}

func (p refPart) String() string {
	var b strings.Builder
	b.WriteString(p.prefix)
	if p.col > 0 {
		if p.absCol {
			b.WriteByte('$')
		}
		b.WriteString(excelColName(p.col))
	}
	if p.row > 0 {
		if p.absRow {
			b.WriteByte('$')
		}
		b.WriteString(strconv.Itoa(p.row))
	}
	return b.String()
}

// excelColName is the letters of column col, from 1 (A) to 16384 (XFD).
func excelColName(col int) string {
	var b [3]byte
	i := len(b)
	for col > 0 && i > 0 {
		col--
		i--
		b[i] = byte('A' + col%26)
		col /= 26
	}
	return string(b[i:])
}
