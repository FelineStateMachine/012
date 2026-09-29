package suggest

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/formula"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Tables in formula suggestions: typing =Sa offers the table Sales with
// the named ranges, and once its "[" is typed, Sales[A offers its
// columns (Amount) and, after a #, the items (#All, #Headers ...). The
// context line shows the table's columns with the one being typed
// marked, as it shows a function's arguments.

// items are the items a structured reference names rows by.
var items = []struct{ name, desc string }{
	{"#All", "The whole table, header row included"},
	{"#Data", "The data rows, as the table's name alone"},
	{"#Headers", "The header row"},
	{"#This Row", "The formula's own row, as @ before a column"},
}

// tableSuggestion is the suggestion for a table formulas can read.
func tableSuggestion(sh *sheet.Sheet, t sheet.TableInfo) Suggestion {
	where := "table, no rows yet"
	if t.Shown() {
		where = "table, " + t.Range.String()
		if t.Sheet != sh {
			where = "table, " + sheet.Qualified(t.Sheet.Name(), t.Range)
		}
	}
	return Suggestion{Name: t.Name, Detail: where, Desc: "Table " + t.Name + ": " + strings.Join(t.Cols, ", ") + "; type [ for its columns"}
}

// columnSuggestions lists what may go in the brackets of the table
// named table, for word typed there: its columns starting with it,
// then from two letters on those containing it, and the items after a
// #. A column is offered as a structured reference writes it, its [ ] #
// and ' escaped.
func columnSuggestions(sh *sheet.Sheet, table, word string) []Suggestion {
	t, ok := sh.Book().LookupTable(table)
	if !ok {
		return nil
	}
	w := strings.ToUpper(word)
	var prefix, inner []Suggestion
	if strings.HasPrefix(w, "#") {
		for _, it := range items {
			if strings.HasPrefix(strings.ToUpper(it.name), w) {
				prefix = append(prefix, Suggestion{Name: it.name, Detail: "item", Desc: it.desc, Column: true})
			}
		}
		return prefix
	}
	for i, col := range t.Cols {
		s := Suggestion{Name: sheet.EscapeColumn(col), Detail: "column " + sheet.ColName(t.Range.From.Col+i),
			Desc: "The " + col + " column of " + t.Name, Column: true}
		switch name := strings.ToUpper(col); {
		case strings.HasPrefix(name, w):
			prefix = append(prefix, s)
		case len(w) >= 2 && strings.Contains(name, w):
			inner = append(inner, s)
		}
	}
	return append(prefix, inner...)
}

// InTable reports the table whose brackets the caret is in, if any: buf
// is the formula as it's parsed (see Host.Stored).
func InTable(buf []rune, pos int) (string, bool) {
	c := formula.ScanCaret(buf, pos)
	return c.Table, c.Table != ""
}

// TableLine puts the columns of the table whose brackets the caret is
// in on the context line, Sales[Region, Units, Amount], with the column
// being typed marked and the items after, and the keys that apply
// (hints) as room allows. It reports false outside a table's brackets
// or for a table formulas can't find.
func TableLine(th *theme.Theme, width int, sh *sheet.Sheet, buf []rune, pos int, hints string) (left, right string, ok bool) {
	c := formula.ScanCaret(buf, pos)
	t, found := sh.Book().LookupTable(c.Table)
	if c.Table == "" || !found {
		return "", "", false
	}
	typed := strings.TrimPrefix(c.Word, "@")
	var b strings.Builder
	b.WriteString(th.Key.Render(t.Name) + "[")
	marked := false
	for i, col := range t.Cols {
		if i > 0 {
			b.WriteString(", ")
		}
		if !marked && typed != "" && strings.HasPrefix(strings.ToUpper(col), strings.ToUpper(typed)) {
			b.WriteString(th.Argument.Render(col))
			marked = true
			continue
		}
		b.WriteString(col)
	}
	b.WriteString("]")
	full := b.String() + "   " + th.Muted.Render("#All #Data #Headers, @ for this row")
	for _, try := range [][2]string{{full, hints}, {b.String(), hints}, {full, ""}} {
		if ansi.StringWidth(try[0])+3+ansi.StringWidth(try[1]) <= width {
			return try[0], try[1], true
		}
	}
	return ansi.Truncate(b.String(), width, "…"), "", true
}
