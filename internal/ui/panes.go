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

// frozenLines counts the screen lines frozen rows take: those the filter
// doesn't hide.
func (g *grid) frozenLines(rows int) int {
	n := 0
	for r := range rows {
		if !g.sheet.RowHidden(r) {
			n++
		}
	}
	return n
}

// scrollRows is how many scrolling rows fit below the frozen ones and
// their divider.
func (g *grid) scrollRows() int {
	fr, _ := g.frozen()
	if fr == 0 {
		return g.visibleRows()
	}
	return max(g.visibleRows()-g.frozenLines(fr)-1, 1)
}

// divider marks the line between frozen and scrolling rows in
// screenRows.
const divider = -1

// screenRows returns the row shown on each grid line, top to bottom:
// frozen rows, the divider, then scrolling rows from top, skipping
// rows the filter hides. Rows past the end of the sheet are left off.
func (g *grid) screenRows() []int {
	n := g.visibleRows()
	out := make([]int, 0, n)
	fr, _ := g.frozen()
	for r := 0; r < fr; r++ {
		if !g.sheet.RowHidden(r) {
			out = append(out, r)
		}
	}
	if fr > 0 {
		out = append(out, divider)
	}
	r, ok := g.top, true
	if g.sheet.RowHidden(r) {
		r, ok = g.sheet.NextShownRow(r, 1)
	}
	for ; ok && len(out) < n; r, ok = g.sheet.NextShownRow(r, 1) {
		out = append(out, r)
	}
	return out
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

// rowAt returns the row on screen line y, or false for the divider and
// lines outside the grid.
func (g *grid) rowAt(y int) (int, bool) {
	rows := g.screenRows()
	if i := y - gridTop; i >= 0 && i < len(rows) && rows[i] != divider {
		return rows[i], true
	}
	return 0, false
}

// rowY returns the screen line of row r, or false if it isn't on screen.
func (g *grid) rowY(r int) (int, bool) {
	for i, row := range g.screenRows() {
		if row == r {
			return gridTop + i, true
		}
	}
	return 0, false
}

// stepRow moves n visible rows from r (up for negative n), skipping
// hidden rows and stopping at the edges of the sheet.
func (g *grid) stepRow(r, n int) int {
	d := 1
	if n < 0 {
		d, n = -1, -n
	}
	for ; n > 0; n-- {
		next, ok := g.sheet.NextShownRow(r, d)
		if !ok {
			break
		}
		r = next
	}
	return r
}

// visibleRow returns r, or the nearest row above it the filter doesn't
// hide (the header row always shows), so the active cell stays in the
// data when its row is filtered out.
func (g *grid) visibleRow(r int) int {
	if !g.sheet.RowHidden(r) {
		return r
	}
	if prev := g.stepRow(r, -1); prev != r {
		return prev
	}
	return g.stepRow(r, 1)
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

// scrollTo moves the viewport the minimum amount needed to show a. Cells
// in the frozen panes are always on screen.
func (g *grid) scrollTo(a sheet.Addr) {
	g.clampView()
	fr, fc := g.frozen()
	if a.Row >= fr {
		if a.Row < g.top {
			g.top = a.Row
		} else {
			// Walk up from a over as many visible rows as fit; if that
			// passes g.top, a is already on screen.
			r, n := a.Row, 1
			for r > g.top && n < g.scrollRows() {
				r = g.stepRow(r, -1)
				n++
			}
			g.top = max(g.top, r)
		}
	}
	if a.Col >= fc {
		if a.Col < g.left {
			g.left = a.Col
		}
		for a.Col >= g.left+g.visibleCols(g.left) {
			g.left++
		}
	}
	g.clampView()
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

// cellPos returns the screen position of a visible cell's left edge.
func (g *grid) cellPos(a sheet.Addr) (x, y int) {
	y, _ = g.rowY(a.Row)
	return g.colStart(a.Col), y
}

// clampBox keeps a w by h box at x, y on screen, shifting it left and up
// as needed.
func (g *grid) clampBox(x, y, w, h int) (int, int) {
	return max(min(x, g.width-w), 0), max(min(y, g.height-h), 0)
}

// dragTarget maps a position during a drag to a cell, clamping to the
// visible grid, and reports which way to autoscroll when it's outside.
// Dragging from the scrolling area into the frozen panes scrolls back
// toward them, as in Sheets, until the two meet; anchor is where the
// drag started.
func (g *grid) dragTarget(x, y int, anchor sheet.Addr) (a sheet.Addr, dc, dr int) {
	rows := g.screenRows()
	fr, fc := g.frozen()
	firstRow := g.visibleRow(fr)
	scrolledDown := g.top > firstRow
	var last int
	for _, r := range rows {
		if r != divider {
			last = r
		}
	}
	switch row, ok := g.rowAt(y); {
	case ok && (row >= fr || !scrolledDown || anchor.Row < fr):
		a.Row = row
	case y >= gridTop+len(rows):
		a.Row, dr = last, 1
	case y >= gridTop && !scrolledDown: // the divider, with nothing scrolled
		a.Row = g.top
	case scrolledDown && anchor.Row >= fr:
		a.Row, dr = g.top, -1
	default: // above the grid
		a.Row = rows[0]
		if scrolledDown {
			dr = -1
		}
	}
	scrolledRight := g.left > fc
	cols := g.visibleCols(g.left)
	right := g.colStart(g.left + cols)
	switch col, _, ok := g.colSpan(x); {
	case x >= right:
		a.Col, dc = g.left+cols-1, 1
	case ok && (col >= fc || !scrolledRight || anchor.Col < fc):
		a.Col = col
	case x >= g.hdrW() && !scrolledRight:
		a.Col = g.left
	case scrolledRight && anchor.Col >= fc:
		a.Col, dc = g.left, -1
	default: // over the row numbers
		a.Col = 0
		if fc == 0 {
			a.Col = g.left
		}
		if scrolledRight || fc == 0 && g.left > 0 {
			dc = -1
		}
	}
	return clampAddr(a), dc, dr
}
