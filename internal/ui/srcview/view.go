// Package srcview is a linked source's tab: a window of the source's
// rows drawn as a sheet with a frozen header row, the row numbers and a
// scrollbar over the whole source, however many millions of rows it
// has. The rows come from the Host a page at a time as the window moves
// (paged.Pages); rows on their way show as a dim ellipsis. The view
// owns the active cell and the scroll; the UI owns the rest.
package srcview

import (
	"strconv"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Column is one of the source's columns.
type Column struct {
	Name   string
	Format sheet.Format
}

// Host is what the view needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Locale() *locale.Locale
	// Columns are the source's columns, left to right.
	Columns() []Column
	// Rows counts the rows the tab shows, false while they're being
	// counted (a sort or a filter being applied).
	Rows() (int64, bool)
	// Row is the tab's row i: its number in the source, counting from
	// 0, and its values, false while its page is on its way.
	Row(i int64) (int64, []sheet.LiveCell, bool)
	// Want asks for the rows from through to.
	Want(from, to int64)
}

// View is a source's tab.
type View struct {
	h             Host
	width, height int   // from the column letters down to the last row line
	top, cur      int64 // the first row shown, the active row: of the tab's rows
	col, left     int   // the active column, the first shown
	widths        []int
	fitted        bool // widths fitted to the first rows read
}

// New is a view of what h shows.
func New(h Host) *View { return &View{h: h} }

// The view's lines: the column letters, the header row of names, the
// divider under it, then the rows.
const (
	letterLine = iota
	namesLine
	dividerLine
	firstRow
)

// minWidth and maxWidth bound a column's fitted width.
const (
	minWidth = 6
	maxWidth = 30
)

// Resize gives the view width columns and height lines.
func (v *View) Resize(width, height int) {
	v.width, v.height = width, height
	v.clamp()
}

// Lines are how many rows show.
func (v *View) Lines() int { return max(v.height-firstRow, 1) }

// Active is the active cell: its row among the tab's, its column.
func (v *View) Active() (int64, int) { return v.cur, v.col }

// Top is the first row shown.
func (v *View) Top() int64 { return v.top }

// Cell is the active cell's number in the source and value, with false
// while its row is on its way.
func (v *View) Cell() (int64, sheet.LiveCell, bool) {
	num, row, ok := v.h.Row(v.cur)
	if !ok || v.col >= len(row) {
		return num, sheet.LiveCell{}, ok
	}
	return num, row[v.col], true
}

// Reset goes back to the first row, as when the rows are sorted or
// filtered again.
func (v *View) Reset() {
	v.top, v.cur = 0, 0
	v.clamp()
}

// rows counts the rows, 0 while they're being counted.
func (v *View) rows() int64 {
	n, _ := v.h.Rows()
	return n
}

// cols counts the columns.
func (v *View) cols() int { return len(v.h.Columns()) }

// Move moves the active cell by rows and cols, keeping it in the
// window.
func (v *View) Move(rows int64, cols int) {
	v.cur += rows
	v.col += cols
	v.clamp()
	v.follow()
}

// MoveTo makes row and col the active cell, row -1 meaning the last.
func (v *View) MoveTo(row int64, col int) {
	if row < 0 {
		row = v.rows() - 1
	}
	if col < 0 {
		col = v.cols() - 1
	}
	v.cur, v.col = row, col
	v.clamp()
	v.follow()
}

// Scroll moves the window d rows, leaving the active cell where it is
// unless it would leave the window.
func (v *View) Scroll(d int64) {
	v.top += d
	v.clamp()
	if v.cur < v.top {
		v.cur = v.top
	}
	if last := v.top + int64(v.Lines()) - 1; v.cur > last {
		v.cur = last
	}
	v.clamp()
}

// ScrollTo shows the rows from top on.
func (v *View) ScrollTo(top int64) { v.Scroll(top - v.top) }

// clamp keeps the active cell and the window in the rows and columns.
func (v *View) clamp() {
	n := v.rows()
	v.cur = max(min(v.cur, n-1), 0)
	v.top = max(min(v.top, n-int64(v.Lines())), 0)
	v.col = max(min(v.col, v.cols()-1), 0)
	v.left = max(min(v.left, v.col), 0)
}

// follow scrolls the active cell into view.
func (v *View) follow() {
	lines := int64(v.Lines())
	switch {
	case v.cur < v.top:
		v.top = v.cur
	case v.cur >= v.top+lines:
		v.top = v.cur - lines + 1
	}
	for v.col > v.left && v.col >= v.left+v.fits(v.left) {
		v.left++
	}
	v.clamp()
}

// gutter is the width of the row numbers: the widest a row shows as,
// counting the header's row 1, and room.
func (v *View) gutter() int {
	return max(6, len(strconv.FormatInt(v.rows()+1, 10))+2)
}

// width is column c's width.
func (v *View) colWidth(c int) int {
	if c < len(v.widths) {
		return v.widths[c]
	}
	return sheet.DefaultWidth
}

// fits is how many columns from left fit beside the row numbers and the
// scrollbar, at least one.
func (v *View) fits(left int) int {
	room := v.width - v.gutter() - 1
	n := 0
	for c := left; c < v.cols(); c++ {
		if room -= v.colWidth(c); room < 0 {
			break
		}
		n++
	}
	return max(n, 1)
}

// fit sizes the columns to their names and the rows first read, once.
func (v *View) fit() {
	cols := v.h.Columns()
	if v.fitted || len(cols) == 0 {
		return
	}
	widths := make([]int, len(cols))
	for c, col := range cols {
		widths[c] = max(len(sheet.ColName(c)), len([]rune(col.Name)))
	}
	_, fitted := v.h.Rows() // fitted again once the rows are counted
	for i := v.top; i < v.top+int64(v.Lines()) && i < v.rows(); i++ {
		_, row, ok := v.h.Row(i)
		if !ok {
			fitted = false // fitted again once the window's rows are here
			break
		}
		for c, cell := range row {
			if c < len(widths) && cell.V.Kind != sheet.Empty {
				text := sheet.FormatTextIn(cell.V, v.format(c, cell), v.h.Locale())
				widths[c] = max(widths[c], len([]rune(text)))
			}
		}
	}
	for c := range widths {
		widths[c] = min(max(widths[c]+2, minWidth), maxWidth)
	}
	v.widths, v.fitted = widths, fitted
}

// Prefetch asks for the rows shown and a window's worth either side, so
// scrolling finds them read.
func (v *View) Prefetch() {
	v.clamp()
	lines := int64(v.Lines())
	v.h.Want(v.top-lines, v.top+2*lines)
}

// format is what a cell of column c shows in: its own, or its column's.
func (v *View) format(c int, cell sheet.LiveCell) sheet.Format {
	if !cell.F.IsZero() {
		return cell.F
	}
	if cols := v.h.Columns(); c < len(cols) {
		return cols[c].Format
	}
	return sheet.Format{}
}

// Refit sizes the columns again once rows are read, as when the source
// is read again.
func (v *View) Refit() { v.fitted = false }
