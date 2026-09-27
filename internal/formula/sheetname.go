package formula

import "strings"

// Sheet names in references: Sheet2!A1, 'Q3 plan'!B2:C9.

// SheetKey is how sheet names compare: ignoring case, as in Sheets.
func SheetKey(name string) string { return strings.ToUpper(name) }

// QuoteSheet writes a sheet name as a formula needs it: bare when it reads
// as a plain identifier (Sheet2), otherwise in single quotes with quotes
// doubled, e.g. 'Q3 plan'.
func QuoteSheet(name string) string {
	bare := name != "" && (isLetter(name[0]) || name[0] == '_') && !looksLikeCell(strings.ToUpper(name))
	for i := 0; bare && i < len(name); i++ {
		c := name[i]
		bare = isLetter(c) || isDigit(c) || c == '_' || c == '.'
	}
	if bare {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", "''") + "'"
}

// looksLikeCell reports whether an upper-case name reads as a cell in
// Excel, A1 to XFD1048576 or R1C1, so a sheet of that name is quoted
// everywhere its formulas may go. Sheet1 doesn't: SHEET is no column.
func looksLikeCell(k string) bool {
	digits := strings.TrimLeft(k, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if n := len(k) - len(digits); n >= 1 && n <= 3 && digits != "" && strings.Trim(digits, "0123456789") == "" {
		return true
	}
	return (strings.HasPrefix(k, "R") || strings.HasPrefix(k, "C")) && LooksLikeRef(k)
}

// quotedName reads a quoted sheet name starting at the quote at src[i] and
// returns it unquoted with the index just past the closing quote.
func quotedName(src string, i int) (string, int, bool) {
	var b strings.Builder
	for j := i + 1; j < len(src); j++ {
		if src[j] != '\'' {
			b.WriteByte(src[j])
			continue
		}
		if j+1 < len(src) && src[j+1] == '\'' {
			b.WriteByte('\'')
			j++
			continue
		}
		return b.String(), j + 1, b.Len() > 0
	}
	return "", len(src), false
}

// SplitSheet splits a reference such as "Sheet2!A1:B3" or "'Q3 plan'!B2"
// into the sheet name, unquoted, and the rest. Without a sheet, sheet is
// "" and rest is s.
func SplitSheet(s string) (sheet, rest string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "'") {
		if name, end, ok := quotedName(s, 0); ok && end < len(s) && s[end] == '!' {
			return name, s[end+1:]
		}
		return "", s
	}
	if i := strings.LastIndexByte(s, '!'); i > 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

// Qualified writes r on sheet as a formula would: Sheet2!A1:B3.
func Qualified(sheet string, r Rect) string {
	return QuoteSheet(sheet) + "!" + r.String()
}
