package nbview

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// An output's window: at most window of its rows (a grid's rows under
// its column header, a list's items, text's lines) from the row it's
// scrolled to, then a line saying what's left out. Whole shows every row
// instead, and hidden folds the output to one line, as Jupyter's
// collapsed outputs. The window's height doesn't change as it scrolls,
// so nothing under it moves.

// fold is how a cell's output shows.
type fold struct {
	hidden bool // o: folded to one line
	whole  bool // O: every row, no window
	scroll int  // the window's first row, but a grid's, which the grid keeps
}

// items is how many rows the output scrolls through at width.
func (sh *shown) items(width int) int {
	switch sh.kind {
	case outList:
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
	return n, footer
}

// height is how many lines the output takes at width.
func (sh *shown) height(whole bool, width int) int {
	switch {
	case sh.kind == outNone:
		return 0
	case sh.isGrid():
		_, n := sh.gridRows(whole)
		lines := 1 + max(n, 1)
		if n < sh.grid.Rows() || sh.grid.Hidden(width) > 0 {
			lines++
		}
		return lines
	}
	n, footer := sh.rowsShown(whole, width)
	if footer {
		n++
	}
	return n
}

// maxScroll is how far the window scrolls: 0 when it shows every row.
func (sh *shown) maxScroll(whole bool, width int) int {
	n, _ := sh.rowsShown(whole, width)
	return max(sh.items(width)-n, 0)
}

// line draws line i of the output as f shows it, at width.
func (sh *shown) line(th *theme.Theme, loc *locale.Locale, i int, f fold, width int) string {
	if sh.isGrid() {
		return sh.gridLine(th, i, f, width)
	}
	n, _ := sh.rowsShown(f.whole, width)
	scroll := min(max(f.scroll, 0), sh.maxScroll(f.whole, width))
	if i < n {
		return sh.item(th, loc, scroll+i, width)
	}
	return sh.footer(th, scroll, n, width)
}

// item draws row i: a list's item, a line of text, or the one value.
func (sh *shown) item(th *theme.Theme, loc *locale.Locale, i, width int) string {
	switch sh.kind {
	case outList:
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

// footer says what the window leaves out: the rows under it, or where
// it's scrolled to.
func (sh *shown) footer(th *theme.Theme, scroll, n, width int) string {
	unit := "line"
	if sh.kind == outList {
		unit = "item"
	}
	total := sh.items(width)
	part := more(total-n, "more "+unit)
	if scroll > 0 {
		part = unit + "s " + grouped(scroll+1) + " to " + grouped(scroll+n) + " of " + grouped(total)
	}
	return ansi.Truncate(th.Muted.Render("… "+part+"  (O shows all)"), width, "…")
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
