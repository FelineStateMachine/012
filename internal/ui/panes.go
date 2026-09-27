package ui

import (
	"012/internal/sheet"
)

// The viewport: frozen rows and columns stay on screen at the top and
// left while the rest scrolls, split from it by a thin line like a tmux
// pane border, and rows the filter hides are skipped. m.top and m.left
// are the first scrolling row and column, never inside the frozen area.
// Everything that maps between rows or columns and the screen goes
// through here, so drawing, navigation and the mouse agree.

// frozen returns the frozen rows and columns that fit on screen: frozen
// panes too big for the window are shown only in part, leaving room for
// at least one scrolling row and column.
func (m *Model) frozen() (rows, cols int) {
	rows, cols = m.sheet.Frozen()
	for rows > 0 && m.frozenLines(rows)+1 >= m.visibleRows() {
		rows--
	}
	w := 0
	for c := range cols {
		w += m.sheet.ColWidth(c)
		if rowHdrW+w+1+3 > m.width {
			cols = c
			break
		}
	}
	return rows, cols
}

// frozenLines counts the screen lines frozen rows take: those the filter
// doesn't hide.
func (m *Model) frozenLines(rows int) int {
	n := 0
	for r := range rows {
		if !m.sheet.RowHidden(r) {
			n++
		}
	}
	return n
}

// scrollRows is how many scrolling rows fit below the frozen ones and
// their divider.
func (m *Model) scrollRows() int {
	fr, _ := m.frozen()
	if fr == 0 {
		return m.visibleRows()
	}
	return max(m.visibleRows()-m.frozenLines(fr)-1, 1)
}

// divider marks the line between frozen and scrolling rows in
// screenRows.
const divider = -1

// screenRows returns the row shown on each grid line, top to bottom:
// frozen rows, the divider, then scrolling rows from m.top, skipping
// rows the filter hides. Rows past the end of the sheet are left off.
func (m *Model) screenRows() []int {
	n := m.visibleRows()
	out := make([]int, 0, n)
	fr, _ := m.frozen()
	for r := 0; r < fr; r++ {
		if !m.sheet.RowHidden(r) {
			out = append(out, r)
		}
	}
	if fr > 0 {
		out = append(out, divider)
	}
	for r := m.top; len(out) < n && r < sheet.MaxRows; r++ {
		if !m.sheet.RowHidden(r) {
			out = append(out, r)
		}
	}
	return out
}

// rowAt returns the row on screen line y, or false for the divider and
// lines outside the grid.
func (m *Model) rowAt(y int) (int, bool) {
	rows := m.screenRows()
	if i := y - gridTop; i >= 0 && i < len(rows) && rows[i] != divider {
		return rows[i], true
	}
	return 0, false
}

// rowY returns the screen line of row r, or false if it isn't on screen.
func (m *Model) rowY(r int) (int, bool) {
	for i, row := range m.screenRows() {
		if row == r {
			return gridTop + i, true
		}
	}
	return 0, false
}

// stepRow moves n visible rows from r (up for negative n), skipping
// hidden rows and stopping at the edges of the sheet.
func (m *Model) stepRow(r, n int) int {
	d := 1
	if n < 0 {
		d, n = -1, -n
	}
	for ; n > 0; n-- {
		next := r + d
		for next >= 0 && next < sheet.MaxRows && m.sheet.RowHidden(next) {
			next += d
		}
		if next < 0 || next >= sheet.MaxRows {
			break
		}
		r = next
	}
	return r
}

// visibleRow returns r, or the nearest row above it the filter doesn't
// hide (the header row always shows), so the active cell stays in the
// data when its row is filtered out.
func (m *Model) visibleRow(r int) int {
	if !m.sheet.RowHidden(r) {
		return r
	}
	if prev := m.stepRow(r, -1); prev != r {
		return prev
	}
	return m.stepRow(r, 1)
}

// scrollX is the screen x where the scrolling columns start: after the
// row numbers, the frozen columns and their divider.
func (m *Model) scrollX() int {
	_, fc := m.frozen()
	x := rowHdrW
	for c := range fc {
		x += m.sheet.ColWidth(c)
	}
	if fc > 0 {
		x++
	}
	return x
}

// clampView keeps the scrolling area out of the frozen panes, e.g. after
// freezing more rows than were scrolled past.
func (m *Model) clampView() {
	fr, fc := m.frozen()
	m.top = clamp(m.top, fr, sheet.MaxRows-1)
	m.left = clamp(m.left, fc, sheet.MaxCols-1)
}

// scrollTo moves the viewport the minimum amount needed to show a. Cells
// in the frozen panes are always on screen.
func (m *Model) scrollTo(a sheet.Addr) {
	m.clampView()
	fr, fc := m.frozen()
	if a.Row >= fr {
		if a.Row < m.top {
			m.top = a.Row
		} else {
			// Walk up from a over as many visible rows as fit; if that
			// passes m.top, a is already on screen.
			r, n := a.Row, 1
			for r > m.top && n < m.scrollRows() {
				r = m.stepRow(r, -1)
				n++
			}
			m.top = max(m.top, r)
		}
	}
	if a.Col >= fc {
		if a.Col < m.left {
			m.left = a.Col
		}
		for a.Col >= m.left+m.visibleCols(m.left) {
			m.left++
		}
	}
	m.clampView()
}
