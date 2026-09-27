package ui

import (
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
)

// Rows on screen. Each row shown is a band of screen lines: a rule line
// first when a border lies along its top, then as many lines as its
// cells take (rowtext.Shapes), the last row cut off at the bottom of the
// grid. A sheet nothing shapes has a line per row. Scrolling moves a row
// at a time, so the top row always shows from its first line.

// band is a row's place on screen.
type band struct {
	row   int  // the row, or divider
	y     int  // the screen line it starts on
	rule  bool // a border line comes first
	shown int  // lines of the row's cells on screen
	lines int  // lines the row's cells take
}

// top is the screen line of the band's first line of cells.
func (b band) top() int {
	if b.rule {
		return b.y + 1
	}
	return b.y
}

// end is the screen line after the band.
func (b band) end() int { return b.top() + b.shown }

// divider marks the line between frozen and scrolling rows.
const divider = -1

// shape returns how row r is laid out.
func (g *grid) shape(r int) rowtext.Shape { return g.shapes.Row(g.sheet, r) }

// span is how many screen lines row r takes, its rule line included.
func (g *grid) span(r int) int {
	sh := g.shape(r)
	if sh.Rule {
		return sh.Lines + 1
	}
	return sh.Lines
}

// frozenLines counts the screen lines frozen rows take: those the filter
// doesn't hide.
func (g *grid) frozenLines(rows int) int {
	n := 0
	for r := range rows {
		if !g.sheet.RowHidden(r) {
			n += g.span(r)
		}
	}
	return n
}

// scrollLines is how many screen lines the scrolling rows have below the
// frozen ones and their divider.
func (g *grid) scrollLines() int {
	fr, _ := g.frozen()
	if fr == 0 {
		return g.visibleRows()
	}
	return max(g.visibleRows()-g.frozenLines(fr)-1, 1)
}

// scrollRows is how many scrolling rows fit below the frozen ones and
// their divider, at least one: a page, for Page Down.
func (g *grid) scrollRows() int {
	if !g.sheet.Shaped() {
		return g.scrollLines()
	}
	n := 0
	for _, b := range g.bands() {
		if b.row != divider && b.row >= g.top && b.shown == b.lines {
			n++
		}
	}
	return max(n, 1)
}

// bands lays out the rows shown, top to bottom: frozen rows, the
// divider, then scrolling rows from top, skipping rows the filter hides.
// Rows past the end of the sheet are left off.
func (g *grid) bands() []band {
	n := g.visibleRows()
	out := make([]band, 0, n)
	y, shaped := gridTop, g.sheet.Shaped()
	add := func(r int) bool {
		sh := rowtext.Shape{Lines: 1}
		if shaped {
			sh = g.shape(r)
		}
		b := band{row: r, y: y, rule: sh.Rule, lines: sh.Lines}
		b.shown = min(sh.Lines, gridTop+n-b.top())
		if b.shown <= 0 {
			return false
		}
		out = append(out, b)
		y = b.end()
		return y < gridTop+n
	}
	fr, _ := g.frozen()
	for r := 0; r < fr; r++ {
		if !g.sheet.RowHidden(r) {
			add(r)
		}
	}
	if fr > 0 {
		out = append(out, band{row: divider, y: y, shown: 1, lines: 1})
		y++
	}
	r, ok := g.top, y < gridTop+n
	if g.sheet.RowHidden(r) {
		r, ok = g.sheet.NextShownRow(r, 1)
	}
	for ; ok && add(r); r, ok = g.sheet.NextShownRow(r, 1) {
	}
	return out
}

// screenRows returns the rows shown, top to bottom, with divider where
// the frozen rows end.
func (g *grid) screenRows() []int {
	bands := g.bands()
	out := make([]int, len(bands))
	for i, b := range bands {
		out[i] = b.row
	}
	return out
}

// rowAt returns the row on screen line y, its rule line included, or
// false for the divider and lines outside the grid.
func (g *grid) rowAt(y int) (int, bool) {
	for _, b := range g.bands() {
		if y >= b.y && y < b.end() {
			return b.row, b.row != divider
		}
	}
	return 0, false
}

// bandOf returns the band of row r, or false if it isn't on screen.
func (g *grid) bandOf(r int) (band, bool) {
	for _, b := range g.bands() {
		if b.row == r {
			return b, true
		}
	}
	return band{}, false
}

// rowY returns the screen line of row r's first line of cells, or false
// if it isn't on screen.
func (g *grid) rowY(r int) (int, bool) {
	b, ok := g.bandOf(r)
	return b.top(), ok
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

// scrollTo moves the viewport the minimum amount needed to show a. Cells
// in the frozen panes are always on screen.
func (g *grid) scrollTo(a sheet.Addr) {
	g.clampView()
	fr, fc := g.frozen()
	if a.Row >= fr {
		if a.Row < g.top {
			g.top = a.Row
		} else {
			// Walk up from a over as many rows as fit; if that passes
			// g.top, a is already on screen.
			r, used, room := a.Row, g.span(a.Row), g.scrollLines()
			for r > g.top {
				prev := g.stepRow(r, -1)
				if prev == r || used+g.span(prev) > room {
					break
				}
				r, used = prev, used+g.span(prev)
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

// lineOf returns the screen line where row r starts, counting down (or
// up) from the top of the scrolling rows, as far as limit lines past the
// grid; rows beyond read as limit away. Charts, which float over the
// scrolling rows, are placed with it.
func (g *grid) lineOf(r, limit int) int {
	y := gridTop
	if !g.sheet.Shaped() {
		return clamp(y+r-g.top, -limit, g.height+limit)
	}
	for k := g.top; k < r && y < g.height+limit; k = g.stepRow(k, 1) {
		if next := g.stepRow(k, 1); next == k {
			break
		}
		y += g.span(k)
	}
	for k := g.top; k > r && y > -limit; {
		prev := g.stepRow(k, -1)
		if prev == k {
			break
		}
		k = prev
		y -= g.span(k)
	}
	return y
}

// rowAtLine is the inverse of lineOf: the row whose lines hold screen
// line y, counting from the top of the scrolling rows, for lines past
// the grid too.
func (g *grid) rowAtLine(y int) int {
	if !g.sheet.Shaped() {
		return max(g.top+y-gridTop, 0)
	}
	r, at := g.top, gridTop
	for at+g.span(r) <= y {
		next := g.stepRow(r, 1)
		if next == r {
			break
		}
		at, r = at+g.span(r), next
	}
	for at > y {
		prev := g.stepRow(r, -1)
		if prev == r {
			break
		}
		r = prev
		at -= g.span(r)
	}
	return r
}

// cellPos returns the screen position of a visible cell's left edge, on
// its first line of text.
func (g *grid) cellPos(a sheet.Addr) (x, y int) {
	y, _ = g.rowY(a.Row)
	return g.colStart(a.Col), y
}

// dragTarget maps a position during a drag to a cell, clamping to the
// visible grid, and reports which way to autoscroll when it's outside.
// Dragging from the scrolling area into the frozen panes scrolls back
// toward them, as in Sheets, until the two meet; anchor is where the
// drag started.
func (g *grid) dragTarget(x, y int, anchor sheet.Addr) (a sheet.Addr, dc, dr int) {
	bands := g.bands()
	fr, _ := g.frozen()
	firstRow := g.visibleRow(fr)
	scrolledDown := g.top > firstRow
	var last, bottom int
	for _, b := range bands {
		if b.row != divider {
			last = b.row
		}
		bottom = b.end()
	}
	switch row, ok := g.rowAt(y); {
	case ok && (row >= fr || !scrolledDown || anchor.Row < fr):
		a.Row = row
	case y >= bottom:
		a.Row, dr = last, 1
	case y >= gridTop && !scrolledDown: // the divider, with nothing scrolled
		a.Row = g.top
	case scrolledDown && anchor.Row >= fr:
		a.Row, dr = g.top, -1
	default: // above the grid
		a.Row = bands[0].row
		if scrolledDown {
			dr = -1
		}
	}
	a.Col, dc = g.dragCol(x, anchor)
	return clampAddr(a), dc, dr
}

// dragCol is dragTarget's column for x.
func (g *grid) dragCol(x int, anchor sheet.Addr) (col, dc int) {
	_, fc := g.frozen()
	scrolledRight := g.left > fc
	cols := g.visibleCols(g.left)
	right := g.colStart(g.left + cols)
	switch c, _, ok := g.colSpan(x); {
	case x >= right:
		return g.left + cols - 1, 1
	case ok && (c >= fc || !scrolledRight || anchor.Col < fc):
		return c, 0
	case x >= g.hdrW() && !scrolledRight:
		return g.left, 0
	case scrolledRight && anchor.Col >= fc:
		return g.left, -1
	}
	// Over the row numbers.
	col = 0
	if fc == 0 {
		col = g.left
	}
	if scrolledRight || fc == 0 && g.left > 0 {
		dc = -1
	}
	return col, dc
}
