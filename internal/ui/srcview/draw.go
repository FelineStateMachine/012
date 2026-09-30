package srcview

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Draw draws the view's lines: the column letters, the header row of
// the columns' names, the divider under it, then the rows, the
// scrollbar at the right edge. It asks the host for the rows shown and
// a page either side, so scrolling finds them read.
func (v *View) Draw() []string {
	v.Prefetch()
	lines := v.Lines()
	v.fit()
	v.follow()
	out := []string{v.letters(), v.names(), v.divider()}
	bar := v.scrollbar(lines)
	for i := range lines {
		out = append(out, v.rowLine(v.top+int64(i))+bar[i])
	}
	return out
}

// shown are the columns on screen.
func (v *View) shown() []int {
	n := v.fits(v.left)
	out := make([]int, 0, n)
	for c := v.left; c < v.left+n && c < v.cols(); c++ {
		out = append(out, c)
	}
	return out
}

// pad fills what's left of the width after used columns.
func (v *View) pad(used int) string { return strings.Repeat(" ", max(v.width-used, 0)) }

// letters is the column header: the columns' letters, as a sheet's.
func (v *View) letters() string {
	th := v.h.Theme()
	var b strings.Builder
	used := v.gutter()
	b.WriteString(th.Header.Render(strings.Repeat(" ", used)))
	for _, c := range v.shown() {
		w := v.colWidth(c)
		style := th.Header
		if c == v.col {
			style = th.HeaderActive
		}
		b.WriteString(style.Render(theme.Center(sheet.ColName(c), w)))
		used += w
	}
	return b.String() + th.Header.Render(v.pad(used))
}

// names is the header row, the sheet's row 1: the columns' names, bold
// and underlined as a table's header.
func (v *View) names() string {
	th := v.h.Theme()
	var b strings.Builder
	b.WriteString(th.RowHeader.Render(theme.PadLeft("1", v.gutter()-1) + " "))
	used := v.gutter()
	cols := v.h.Columns()
	for _, c := range v.shown() {
		w := v.colWidth(c)
		name := ansi.Truncate(cols[c].Name, w-1, "…")
		b.WriteString(th.TableHeader.Render(theme.PadRight(" "+name, w)))
		used += w
	}
	return b.String() + v.pad(used)
}

// divider is the line under the frozen header row.
func (v *View) divider() string {
	return v.h.Theme().FrozenLine.Render(strings.Repeat("─", v.width))
}

// rowLine draws row i of the tab: its number, the sheet's row the
// source's row is (under the header), and its cells.
func (v *View) rowLine(i int64) string {
	th := v.h.Theme()
	used := v.gutter()
	if i >= v.rows() {
		return th.RowHeader.Render(strings.Repeat(" ", used)) + v.pad(used+1)
	}
	num, row, ok := v.h.Row(i)
	hdr := th.RowHeader
	if i == v.cur {
		hdr = th.HeaderActive
	}
	label := "…"
	if ok {
		label = strconv.FormatInt(num+2, 10)
	}
	var b strings.Builder
	b.WriteString(hdr.Render(theme.PadLeft(label, used-1) + " "))
	for _, c := range v.shown() {
		w := v.colWidth(c)
		b.WriteString(v.cell(i, c, w, row, ok))
		used += w
	}
	return b.String() + v.pad(used+1)
}

// cell draws column c of row i, w wide.
func (v *View) cell(i int64, c, w int, row []sheet.LiveCell, ok bool) string {
	th := v.h.Theme()
	base := th.Cell
	if i == v.cur && c == v.col {
		base = th.Pointer
	}
	switch {
	case !ok:
		if c == v.left {
			return th.Muted.Inherit(base).Render(theme.PadRight(" …", w))
		}
		return base.Render(strings.Repeat(" ", w))
	case c >= len(row):
		return base.Render(strings.Repeat(" ", w))
	}
	cell := row[c]
	text, align := sheet.DisplayIn(cell.V, v.format(c, cell), w, v.h.Locale())
	text = ansi.Truncate(text, w-1, "")
	switch align {
	case sheet.AlignRight, sheet.AlignFill:
		text = theme.PadLeft(text, w-1) + " "
	case sheet.AlignCenter:
		text = theme.Center(text, w)
	default:
		text = " " + theme.PadRight(text, w-1)
	}
	if cell.V.Kind == sheet.Error {
		return th.ErrorCell.Inherit(base).Render(text)
	}
	return base.Render(text)
}

// scrollbar is the scrollbar's column beside each of lines rows: a
// track, and a thumb as long as the share of the rows shown and as far
// down as the window is, drawn with other characters so it reads
// without color.
func (v *View) scrollbar(lines int) []string {
	th := v.h.Theme()
	out := make([]string, lines)
	start, size := v.thumb(lines)
	for i := range out {
		if i >= start && i < start+size {
			out[i] = th.Key.Render("┃")
		} else {
			out[i] = th.Border.Render("│")
		}
	}
	return out
}

// thumb is where the scrollbar's thumb starts and how long it is, over
// lines lines.
func (v *View) thumb(lines int) (start, size int) {
	n := v.rows()
	if n <= int64(lines) {
		return 0, lines
	}
	size = max(int(int64(lines)*int64(lines)/n), 1)
	room := int64(lines - size)
	last := n - int64(lines)
	return int(v.top * room / max(last, 1)), size
}

// Hit is what the screen position x, y of the view is: a cell (its row
// and column), the scrollbar, or nothing.
type Hit struct {
	Row      int64
	Col      int
	Cell     bool
	Bar      bool
	BarLines int // how many lines the scrollbar has, for ScrollBar
	BarAt    int // the line of the scrollbar hit
}

// HitAt says what x, y (from the view's first line) is.
func (v *View) HitAt(x, y int) Hit {
	lines := v.Lines()
	if y < firstRow || y >= firstRow+lines {
		return Hit{}
	}
	if x == v.width-1 {
		return Hit{Bar: true, BarLines: lines, BarAt: y - firstRow}
	}
	i := v.top + int64(y-firstRow)
	if i >= v.rows() {
		return Hit{}
	}
	used := v.gutter()
	for _, c := range v.shown() {
		if w := v.colWidth(c); x >= used && x < used+w {
			return Hit{Row: i, Col: c, Cell: true}
		}
		used += v.colWidth(c)
	}
	return Hit{}
}

// ScrollBar moves the window to where line at of a scrollbar of lines
// lines stands for: a click on the track, or the thumb dragged there.
func (v *View) ScrollBar(at, lines int) {
	n := v.rows() - int64(v.Lines())
	if n <= 0 || lines <= 1 {
		return
	}
	v.ScrollTo(int64(at) * n / int64(lines-1))
}

// DragBar moves the window to where the scrollbar's line at y (from the
// view's first line) stands for, as the thumb is dragged.
func (v *View) DragBar(y int) {
	lines := v.Lines()
	v.ScrollBar(min(max(y-firstRow, 0), lines-1), lines)
}

// ColX is the screen column column col starts at, from the view's
// left edge: the row numbers' width when it isn't shown.
func (v *View) ColX(col int) int {
	x := v.gutter()
	for _, c := range v.shown() {
		if c == col {
			return x
		}
		x += v.colWidth(c)
	}
	return v.gutter()
}
