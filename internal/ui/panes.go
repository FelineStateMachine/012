package ui

import (
	"strconv"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// The viewport: frozen rows and columns stay on screen at the top and
// left while the rest scrolls, split from it by a thin line like a tmux
// pane border, and rows the filter hides are skipped. top and left
// are the first scrolling row and column, never inside the frozen area.
// Everything that maps between rows or columns and the screen goes
// through here, so drawing, navigation and the mouse agree.

// frozen returns the frozen rows and columns that fit on screen: frozen
// panes too big for the window are shown only in part, leaving room for
// at least one scrolling row and column.
func (g *grid) frozen() (rows, cols int) {
	rows, cols = g.sheet.Frozen()
	for rows > 0 && g.frozenLines(rows)+1 >= g.visibleRows() {
		rows--
	}
	w := 0
	for c := range cols {
		w += g.sheet.ColWidth(c)
		if g.hdrW()+w+1+3 > g.width {
			cols = c
			break
		}
	}
	return rows, cols
}

// hdrW is the width of the row numbers: the widest number on screen with
// a space either side, and at least minRowHdrW, so it widens past row
// 9999 as Excel's does.
func (g *grid) hdrW() int {
	last := g.top + g.visibleRows()
	if g.sheet.HiddenRows() > 0 {
		last = g.top
		for range g.visibleRows() {
			r, ok := g.sheet.NextShownRow(last, 1)
			if !ok {
				break
			}
			last = r
		}
		last++
	}
	return max(minRowHdrW, len(strconv.Itoa(min(last, sheet.MaxRows)))+2)
}

// scrollX is the screen x where the scrolling columns start: after the
// row numbers, the frozen columns and their divider.
func (g *grid) scrollX() int {
	_, fc := g.frozen()
	x := g.hdrW()
	for c := range fc {
		x += g.sheet.ColWidth(c)
	}
	if fc > 0 {
		x++
	}
	return x
}

// clampView keeps the scrolling area out of the frozen panes, e.g. after
// freezing more rows than were scrolled past.
func (g *grid) clampView() {
	fr, fc := g.frozen()
	g.top = clamp(g.top, fr, sheet.MaxRows-1)
	g.left = clamp(g.left, fc, sheet.MaxCols-1)
}

func (g *grid) visibleRows() int {
	return max(g.height-gridTop-1, 1)
}

// visibleCols returns how many whole scrolling columns fit starting at
// left.
func (g *grid) visibleCols(left int) int {
	n, w := 0, g.scrollX()
	for c := left; c < sheet.MaxCols; c++ {
		w += g.sheet.ColWidth(c)
		if w > g.width {
			break
		}
		n++
	}
	return max(n, 1)
}

// colSpan returns the visible column under x and the x where it starts:
// a frozen column or a scrolling one, but not the divider between them.
func (g *grid) colSpan(x int) (col, start int, ok bool) {
	_, fc := g.frozen()
	cx := g.hdrW()
	for c := 0; c < fc; c++ {
		w := g.sheet.ColWidth(c)
		if x >= cx && x < cx+w {
			return c, cx, true
		}
		cx += w
	}
	cx = g.scrollX()
	for c := g.left; c < sheet.MaxCols && cx < g.width; c++ {
		w := g.sheet.ColWidth(c)
		if x >= cx && x < cx+w {
			return c, cx, true
		}
		cx += w
	}
	return 0, 0, false
}

// colStart returns the screen x where column c starts, which may be off
// screen (or under the frozen columns, for a scrolling column left of
// left).
func (g *grid) colStart(c int) int {
	_, fc := g.frozen()
	if c < fc {
		x := g.hdrW()
		for k := range c {
			x += g.sheet.ColWidth(k)
		}
		return x
	}
	x := g.scrollX()
	if c >= g.left {
		for k := g.left; k < c; k++ {
			x += g.sheet.ColWidth(k)
		}
		return x
	}
	for k := c; k < g.left; k++ {
		x -= g.sheet.ColWidth(k)
	}
	return x
}

// clampBox keeps a w by h box at x, y on screen, shifting it left and up
// as needed.
func (g *grid) clampBox(x, y, w, h int) (int, int) {
	return max(min(x, g.width-w), 0), max(min(y, g.height-h), 0)
}
