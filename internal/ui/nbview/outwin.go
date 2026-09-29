package nbview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// An output's window: a table's header, then at most window of its rows
// (a list's items, a record's fields, text's lines) from the row it's
// scrolled to, then a line saying what's left out. Whole shows every row
// instead, and hidden folds the output to one line, as Jupyter's
// collapsed outputs. The window's height doesn't change as it scrolls,
// so nothing under it moves.

// fold is how a cell's output shows.
type fold struct {
	hidden bool // o: folded to one line
	whole  bool // O: every row, no window
	scroll int  // the window's first row
}

// head is how many lines stay above the rows: a table's header.
func (sh *shown) head() int {
	if sh.kind == outTable {
		return 1
	}
	return 0
}

// items is how many rows the output scrolls through at width.
func (sh *shown) items(width int) int {
	switch sh.kind {
	case outTable, outRecord, outList:
		return sh.total
	case outText, outError:
		return len(sh.wrapped(width))
	case outNone:
		return 0
	}
	return 1
}

// rowsShown is how many rows show, and whether a line saying what's left
// out follows them.
func (sh *shown) rowsShown(whole bool, width int) (n int, footer bool) {
	n = sh.items(width)
	if !whole && n > window {
		n, footer = window, true
	}
	if sh.kind == outTable && sh.hiddenCols(width) > 0 {
		footer = true
	}
	return n, footer
}

// height is how many lines the output takes at width.
func (sh *shown) height(whole bool, width int) int {
	if sh.kind == outNone {
		return 0
	}
	n, footer := sh.rowsShown(whole, width)
	if footer {
		n++
	}
	return sh.head() + n
}

// maxScroll is how far the window scrolls: 0 when it shows every row.
func (sh *shown) maxScroll(whole bool, width int) int {
	n, _ := sh.rowsShown(whole, width)
	return max(sh.items(width)-n, 0)
}

// line draws line i of the output as f shows it, at width.
func (sh *shown) line(th *theme.Theme, loc *locale.Locale, i int, f fold, width int) string {
	if i < sh.head() {
		return sh.tableHead(th, width)
	}
	i -= sh.head()
	n, _ := sh.rowsShown(f.whole, width)
	scroll := min(max(f.scroll, 0), sh.maxScroll(f.whole, width))
	if i < n {
		return sh.item(th, loc, scroll+i, width)
	}
	return sh.footer(th, scroll, n, width)
}

// item draws row i: a table's row, a field or an item, a line of text,
// or the one value.
func (sh *shown) item(th *theme.Theme, loc *locale.Locale, i, width int) string {
	switch sh.kind {
	case outTable:
		return sh.tableRow(loc, i, width)
	case outRecord, outList:
		return sh.fieldLine(th, loc, i, width)
	case outText, outError:
		return sh.textLine(th, i, width)
	case outValue:
		return ansi.Truncate(cellText(sh.rows[0][0], loc), width, "…")
	case outUnsaved:
		return th.Muted.Render("not saved; run to see")
	case outBad:
		return th.Warning.Render(ansi.Truncate(sh.text[0], width, "…"))
	}
	return ""
}

// footer says what the window leaves out: the rows under it (or where
// it's scrolled to) and a table's columns past the width.
func (sh *shown) footer(th *theme.Theme, scroll, n, width int) string {
	unit := map[outKind]string{outTable: "row", outRecord: "field", outList: "item"}[sh.kind]
	if unit == "" {
		unit = "line"
	}
	total := sh.items(width)
	var parts []string
	switch {
	case n >= total:
	case scroll == 0:
		parts = append(parts, more(total-n, "more "+unit))
	default:
		parts = append(parts, unit+"s "+grouped(scroll+1)+" to "+grouped(scroll+n)+" of "+grouped(total))
	}
	if sh.kind == outTable {
		if c := sh.hiddenCols(width); c > 0 {
			parts = append(parts, more(c, "more column"))
		}
	}
	hint := "O shows all"
	if n >= total {
		hint = ""
	}
	if sh.kind == outTable {
		hint = strings.TrimPrefix(hint+", Enter opens it", ", ")
	}
	return ansi.Truncate(th.Muted.Render("… "+strings.Join(parts, ", ")+"  ("+hint+")"), width, "…")
}

// tableHead is a table's header row.
func (sh *shown) tableHead(th *theme.Theme, width int) string {
	var b strings.Builder
	for c := range sh.shownCols(width) {
		if c > 0 {
			b.WriteString("  ")
		}
		name := ansi.Truncate(sh.cols[c], sh.fit[c], "…")
		b.WriteString(th.OutputHead.Render(name) + strings.Repeat(" ", sh.fit[c]-ansi.StringWidth(name)))
	}
	return b.String()
}

// tableRow is a table's row i.
func (sh *shown) tableRow(loc *locale.Locale, i, width int) string {
	if i >= len(sh.rows) {
		return ""
	}
	row := sh.rows[i]
	var b strings.Builder
	for c := range sh.shownCols(width) {
		if c > 0 {
			b.WriteString("  ")
		}
		var lc sheet.LiveCell
		if c < len(row) {
			lc = row[c]
		}
		b.WriteString(pad(ansi.Truncate(cellText(lc, loc), sh.fit[c], "…"), sh.fit[c], align(lc)))
	}
	return b.String()
}

// textLine is line i of text or an error, wrapped at width.
func (sh *shown) textLine(th *theme.Theme, i, width int) string {
	lines := sh.wrapped(width)
	if i >= len(lines) {
		return ""
	}
	if sh.kind == outError {
		if i == 0 {
			return th.Warning.Render(lines[i])
		}
		return th.Muted.Render(lines[i])
	}
	return lines[i]
}
