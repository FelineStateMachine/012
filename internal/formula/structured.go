package formula

import (
	"strings"
)

// Structured references name a table's cells by its columns' names, as
// Excel's and Sheets' tables do: Sales[Amount] is the Amount column's
// data rows, Sales[#All] the whole table, header row included,
// Sales[@Amount] the Amount cell in the formula's own row, and
// Sales[[#Headers],[Amount]:[Units]] the headers of three columns. The
// parser only reads them; the engine finds the table and turns the
// reference into a range when it evaluates the formula, so tables that
// grow or columns that move are followed without rewriting formulas.

// TableItems are the rows of a table a structured reference names.
type TableItems uint8

const (
	ItemHeaders TableItems = 1 << iota // the header row: [#Headers]
	ItemData                           // the data rows: [#Data], or nothing
	ItemTotals                         // a totals row: [#Totals]
	ItemThisRow                        // the formula's own row: [#This Row] or @
	// ItemAll is the whole table: [#All].
	ItemAll = ItemHeaders | ItemData | ItemTotals
)

// itemWords are the item keywords, in the order they're printed.
var itemWords = []struct {
	item TableItems
	word string
}{{ItemAll, "#All"}, {ItemHeaders, "#Headers"}, {ItemData, "#Data"}, {ItemTotals, "#Totals"}, {ItemThisRow, "#This Row"}}

// TableRef is a structured reference: Table[Column], Table[#All],
// Table[@Column], Table[[#Headers],[First]:[Last]].
type TableRef struct {
	Table string // the table's name as written
	// Items are the rows named; zero means the data rows, as when no
	// item is written.
	Items TableItems
	// From and To are the first and last column named, as written: ""
	// for every column, To "" for From alone.
	From, To string
}

// Rows are the rows the reference names, with no item meaning the
// data rows.
func (t TableRef) Rows() TableItems {
	if t.Items == 0 {
		return ItemData
	}
	return t.Items
}

// Cols are the first and last column the reference names, the same
// for one column, or "" for every column.
func (t TableRef) Cols() (string, string) {
	if t.To == "" {
		return t.From, t.From
	}
	return t.From, t.To
}

// tableRef parses a structured reference token: the table's name and
// its bracketed part, written as src, which starts at pos.
func tableRef(src string, pos int) (TableRef, error) {
	i := strings.IndexByte(src, '[')
	t := TableRef{Table: src[:i]}
	inner := strings.TrimSpace(src[i+1 : len(src)-1])
	fail := func(msg string) (TableRef, error) {
		return TableRef{}, &ParseError{Pos: pos + i, Msg: msg}
	}
	switch {
	case inner == "":
	case inner[0] == '@':
		t.Items = ItemThisRow
		rest := strings.TrimSpace(inner[1:])
		if rest == "" {
			break
		}
		if rest[0] != '[' {
			t.From = unescapeColumn(rest)
			break
		}
		items, from, to, msg := specList(rest)
		if msg != "" || items != 0 {
			return fail(cmpOr(msg, "Only columns can follow @"))
		}
		t.From, t.To = from, to
	case inner[0] == '#':
		it, ok := itemWord(inner)
		if !ok {
			return fail("Unknown table item " + inner + ": use #All, #Data, #Headers, #Totals or #This Row")
		}
		t.Items = it
	case inner[0] == '[':
		items, from, to, msg := specList(inner)
		if msg != "" {
			return fail(msg)
		}
		t.Items, t.From, t.To = items, from, to
	default:
		t.From = unescapeColumn(inner)
	}
	if msg := t.check(); msg != "" {
		return fail(msg)
	}
	return t, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// check reports why the items can't go together, or "".
func (t TableRef) check() string {
	switch it := t.Items; {
	case it&ItemThisRow != 0 && it != ItemThisRow:
		return "#This Row can't be combined with other items"
	case it&ItemHeaders != 0 && it&ItemTotals != 0 && it&ItemData == 0:
		return "#Headers and #Totals need #Data between them"
	}
	if t.From == "" && t.To != "" {
		return "Expected a column before :"
	}
	return ""
}

// itemWord reads an item keyword, ignoring case and spaces around it.
func itemWord(s string) (TableItems, bool) {
	s = strings.TrimSpace(s)
	for _, w := range itemWords {
		if strings.EqualFold(s, w.word) {
			return w.item, true
		}
	}
	return 0, false
}

// specList reads a list of bracketed items, [#Headers],[Amount]:[Units],
// separated by , or ; and returns the items, the columns, and what's
// wrong with it, if anything.
func specList(s string) (items TableItems, from, to, msg string) {
	cols := 0
	for i := 0; ; {
		i = skipSpaces(s, i)
		text, next, ok := bracketed(s, i)
		if !ok {
			return 0, "", "", "Expected [ and ] around each item"
		}
		if strings.HasPrefix(strings.TrimSpace(text), "#") {
			it, ok := itemWord(text)
			if !ok {
				return 0, "", "", "Unknown table item " + strings.TrimSpace(text) + ": use #All, #Data, #Headers, #Totals or #This Row"
			}
			items |= it
		} else {
			if cols++; cols > 1 {
				return 0, "", "", "Name one column, or a range of them as [First]:[Last]"
			}
			from = unescapeColumn(text)
			if j := skipSpaces(s, next); j < len(s) && s[j] == ':' {
				var last string
				if last, next, ok = bracketed(s, skipSpaces(s, j+1)); !ok {
					return 0, "", "", "Expected [column] after :"
				}
				to = unescapeColumn(last)
			}
		}
		i = skipSpaces(s, next)
		if i >= len(s) {
			return items, from, to, ""
		}
		if s[i] != ',' && s[i] != ';' {
			return 0, "", "", "Expected , between a table's items"
		}
		i++
	}
}

// bracketed reads [text] at s[i], with ' escaping the character after
// it, and returns the text as written and the index past the "]".
func bracketed(s string, i int) (string, int, bool) {
	if i >= len(s) || s[i] != '[' {
		return "", i, false
	}
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\'':
			j++
		case '[':
			return "", j, false
		case ']':
			return s[i+1 : j], j + 1, true
		}
	}
	return "", len(s), false
}

func skipSpaces(s string, i int) int {
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

// bracketEnd returns the index just past the "]" closing the "[" at
// src[i], or -1: brackets nest, and ' escapes the character after it.
func bracketEnd(src string, i int) int {
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '\'':
			j++
		case '[':
			depth++
		case ']':
			if depth--; depth == 0 {
				return j + 1
			}
		}
	}
	return -1
}

// escaped are the characters a column's name escapes with ' in a
// structured reference.
const escaped = "[]#'"

// unescapeColumn reads a column's name as written, trimmed, with its
// escapes removed.
func unescapeColumn(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "'") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// EscapeColumn writes a column's name as a structured reference holds
// it, with ' before each of [ ] # and '.
func EscapeColumn(name string) string {
	if !strings.ContainsAny(name, escaped) {
		return name
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if strings.IndexByte(escaped, name[i]) >= 0 {
			b.WriteByte('\'')
		}
		b.WriteByte(name[i])
	}
	return b.String()
}

// plainColumn reports whether a column's name can stand alone in the
// brackets, Sales[Unit Price], rather than in brackets of its own,
// Sales[[Total, EUR]]: nothing but letters, digits, _, spaces inside
// and characters beyond ASCII.
func plainColumn(name string) bool {
	if name == "" || name[0] == ' ' || name[len(name)-1] == ' ' {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; !isLetter(c) && !isDigit(c) && c != '_' && c != ' ' && c < 0x80 {
			return false
		}
	}
	return true
}

// String writes the reference as 012 prints it: Sales[Amount],
// Sales[@Amount], Sales[[#Headers],[Amount]].
func (t TableRef) String() string { return t.write(false) }

// Excel writes the reference as Excel's files hold it, where the
// formula's own row is [#This Row] rather than @.
func (t TableRef) Excel() string { return t.write(true) }

func (t TableRef) write(excel bool) string {
	var b strings.Builder
	b.WriteString(t.Table)
	b.WriteByte('[')
	cols := t.columns()
	switch {
	case t.Items == ItemThisRow && !excel:
		b.WriteByte('@')
		if cols != "" && t.To == "" && plainColumn(t.From) && !strings.Contains(t.From, " ") {
			b.WriteString(t.From)
		} else {
			b.WriteString(cols)
		}
	case t.Items == 0 && cols == "":
	case t.Items == 0 && t.To == "" && plainColumn(t.From):
		b.WriteString(t.From)
	case t.Items == 0:
		b.WriteString(cols)
	case cols == "" && t.single():
		b.WriteString(t.words()[0])
	default:
		for i, w := range t.words() {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString("[" + w + "]")
		}
		if cols != "" {
			b.WriteString("," + cols)
		}
	}
	b.WriteByte(']')
	return b.String()
}

// columns writes the columns named in brackets, [From] or
// [From]:[To], or "" for every column.
func (t TableRef) columns() string {
	if t.From == "" {
		return ""
	}
	s := "[" + EscapeColumn(t.From) + "]"
	if t.To != "" {
		s += ":[" + EscapeColumn(t.To) + "]"
	}
	return s
}

// single reports whether the reference names one item keyword.
func (t TableRef) single() bool { return len(t.words()) == 1 }

// words are the item keywords the reference names.
func (t TableRef) words() []string {
	var out []string
	left := t.Items
	for _, w := range itemWords {
		if left&w.item == w.item && w.item != 0 {
			out = append(out, w.word)
			left &^= w.item
		}
	}
	return out
}

// ExcelTableRef writes a structured reference, the table's name and
// its brackets as written in src, in Excel's file syntax, or reports
// false when it doesn't parse.
func ExcelTableRef(src string) (string, bool) {
	t, err := tableRef(src, 0)
	if err != nil {
		return "", false
	}
	return t.Excel(), true
}

// WalkTables calls fn for every structured reference in n.
func WalkTables(n Node, fn func(TableRef)) {
	switch n := n.(type) {
	case TableRef:
		fn(n)
	case Unary, Binary, Call, Array, Invoke:
		EachChild(n, func(k Node) { WalkTables(k, fn) })
	}
}
