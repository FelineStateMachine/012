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
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(src) {
				if src[j] == '"' {
					if j+1 < len(src) && src[j+1] == '"' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= len(src) {
				return "", false
			}
			b.WriteString(src[i : j+1])
			i = j
		case c == '#':
			if strings.HasPrefix(strings.ToUpper(src[i:]), "#REF!") {
				b.WriteString("#REF!")
				i += len("#REF!") - 1
				continue
			}
			return "", false // #AND#, #OR#, #NOT#
		case c == '.' && i+1 < len(src) && src[i+1] == '.':
			b.WriteByte(':')
			i++
		case c == '@' || isIdentByte(c) && !isDigitByte(c) && c != '.':
			j := i
			if c == '@' {
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
			if k < len(src) && src[k] == '(' || c == '@' {
				name = strings.ToUpper(name)
				if strings.HasPrefix(name, "JEV.") {
					return "", false
				}
				if xlfn[name] {
					name = "_xlfn." + name
				}
				if c == '@' && (k >= len(src) || src[k] != '(') {
					name += "()" // @PI
				}
			}
			b.WriteString(name)
			i = j - 1
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
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
