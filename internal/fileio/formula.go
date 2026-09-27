package fileio

import (
	"strings"
)

// 012 reads Sheets' formula syntax and some of 1-2-3's (@SUM, A1..B3,
// #AND#). Excel files store formulas without the leading =, and prefix
// functions newer than Excel 2007 with _xlfn.

// xlfn lists 012's functions that Excel stores with the _xlfn. prefix.
var xlfn = map[string]bool{
	"CONCAT": true, "DAYS": true, "IFNA": true, "IFS": true, "SWITCH": true,
	"TEXTJOIN": true, "XLOOKUP": true, "XOR": true,
}

// toExcelFormula translates a formula entry to Excel's syntax, or
// reports false when Excel has no equivalent (JEV functions, 1-2-3's
// #AND# operators), in which case the value is exported instead.
func toExcelFormula(input string) (string, bool) {
	src := strings.TrimPrefix(input, "=")
	var b strings.Builder
	for i := 0; i < len(src); {
		next, ok := excelToken(&b, src, i)
		if !ok {
			return "", false
		}
		i = next
	}
	return b.String(), true
}

// excelToken writes the token starting at src[i] in Excel's syntax and
// returns the index just past it, or false when Excel has no equivalent.
func excelToken(b *strings.Builder, src string, i int) (int, bool) {
	c := src[i]
	switch {
	case c == '\'':
		// A quoted sheet name, 'Q3 plan'!A1, is the same in Excel.
		j := quoteEnd(src, i)
		b.WriteString(src[i:j])
		return j, true
	case c == '"':
		j := stringEnd(src, i)
		if j < 0 {
			return 0, false
		}
		b.WriteString(src[i:j])
		return j, true
	case c == '#':
		if p := prefixFold(src[i:], "#REF!"); p > 0 {
			b.WriteString("#REF!")
			return i + p, true
		}
		return 0, false // #AND#, #OR#, #NOT#
	case c == '.' && i+1 < len(src) && src[i+1] == '.':
		b.WriteByte(':') // 1-2-3's A1..B3
		return i + 2, true
	case c == '@' || isIdentByte(c) && !isDigitByte(c) && c != '.':
		return excelName(b, src, i)
	}
	b.WriteByte(c)
	return i + 1, true
}

// stringEnd returns the index just past the string literal starting at
// s[i], where a quote inside is written twice, or -1 if it isn't closed.
func stringEnd(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		if s[j] == '"' {
			if j+1 < len(s) && s[j+1] == '"' {
				j++
				continue
			}
			return j + 1
		}
	}
	return -1
}

// excelName writes the name starting at src[i]: a function name is
// upper-cased and given Excel's _xlfn. prefix where it needs one, and
// 1-2-3's @PI becomes PI(); references and named ranges stay as they are.
func excelName(b *strings.Builder, src string, i int) (int, bool) {
	at := src[i] == '@'
	j := i
	if at {
		j++
	}
	start := j
	for j < len(src) && isIdentByte(src[j]) && !strings.HasPrefix(src[j:], "..") {
		j++
	}
	name := src[start:j]
	k := j
	for k < len(src) && src[k] == ' ' {
		k++
	}
	call := k < len(src) && src[k] == '('
	if call || at {
		name = strings.ToUpper(name)
		if strings.HasPrefix(name, "JEV.") {
			return 0, false
		}
		if xlfn[name] {
			name = "_xlfn." + name
		}
		if at && !call {
			name += "()" // @PI
		}
	}
	b.WriteString(name)
	return j, true
}

// fromExcelFormula translates an Excel formula (without its =) to a 012
// entry. The result may still fail to parse, e.g. with references to
// other sheets; the importer then keeps the cell's value.
func fromExcelFormula(f string) string {
	f = strings.TrimPrefix(f, "=")
	var b strings.Builder
	b.WriteByte('=')
	inStr := false
	for i := 0; i < len(f); i++ {
		c := f[i]
		if c == '\'' && !inStr {
			j := quoteEnd(f, i)
			b.WriteString(f[i:j])
			i = j - 1
			continue
		}
		if c == '"' {
			inStr = !inStr
		}
		if !inStr {
			rest := f[i:]
			if p := prefixFold(rest, "_xlfn."); p > 0 {
				i += p - 1
				continue
			}
			if p := prefixFold(rest, "_xlws."); p > 0 {
				i += p - 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// quoteEnd returns the index just past the quoted sheet name starting at
// s[i], where a quote inside is written twice.
func quoteEnd(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		if s[j] == '\'' {
			if j+1 < len(s) && s[j+1] == '\'' {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(s)
}

// prefixFold returns len(prefix) if s starts with it, ignoring case.
func prefixFold(s, prefix string) int {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return len(prefix)
	}
	return 0
}

func isIdentByte(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '$' || c == '.' || isDigitByte(c)
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }
