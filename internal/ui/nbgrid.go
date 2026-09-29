package ui

import (
	"bytes"
	"context"
	"slices"
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
)

// A notebook cell's table or record output is drawn and worked as 012's
// own grid. Each such output gets a Model of its own, the grid's child,
// over a workbook of one sheet holding the output as importing its NUON
// makes it (as sending it to a sheet does): the header row, frozen, then
// the rows, each value in the format of its type, the columns fitted.
// The output's window shows the child's column header, named after the
// table's columns rather than lettered, and its rows under the header,
// numbered from 1; the child draws them with the grid's own code
// (view.go), so borders, formats, the pointer, the selection, the copy
// marker and search matches look as on a sheet. While the output is
// entered, keys, the mouse and commands go to the child (nbgridin.go),
// which refuses anything that would change a value: sorting, filtering,
// finding, selecting, copying and resizing columns act on its copy, and
// the output itself never changes.

// outGrids are the outputs drawn as grids, and the one entered.
type outGrids struct {
	in *outGrid // the grid with the keys, or nil
}

// outGrid is an output's grid.
type outGrid struct {
	parent *Model
	id     int    // the cell's
	data   []byte // the output as a table, NUON (nbview.Host.Grid)
	rows   int    // its rows, until the child counts them
	child  *Model // made the first time the grid is drawn
	err    string // why the output couldn't be read as a table
	cols   int    // the table's columns
	// win is the rows the window shows, as last drawn, which the child
	// is sized to.
	win int
	// shown are the rows showing when a filter hides some, for the
	// sheet's version shownAt; nil when every row shows.
	shown   []int
	shownAt uint64
	// frame is the parent's frame the child last drew for.
	frame int
}

var _ nbview.Grid = (*outGrid)(nil)

// Grid makes cell id's output a grid (nbview.Host).
func (h nbHost) Grid(id int, data []byte, rows int) nbview.Grid {
	return &outGrid{parent: h.m, id: id, data: data, rows: rows}
}

// ready makes the child if it isn't yet, and sizes it to a window of
// rows rows width wide: reports false if the output isn't a table.
func (g *outGrid) ready(width, rows int) bool {
	if g.child == nil && g.err == "" {
		g.build()
	}
	if g.child == nil {
		return false
	}
	c := g.child
	if f := g.parent.nb.frame; g.frame != f {
		g.frame = f
		c.th = g.parent.th
		clear(c.painted)
		clear(c.mergeLines)
		clear(c.shaded)
	}
	// An output shown whole draws its rows one at a time wherever they
	// are, so the child needn't be taller than a screen.
	g.win = min(max(rows, 1), maxWin)
	// The frozen header and its divider take two of the child's lines,
	// and at least two rows keep the header frozen (grid.frozen).
	c.width, c.height = width, gridTop+max(g.win, 2)+2+1
	c.clampView()
	return true
}

// build reads the output into the child's workbook.
func (g *outGrid) build() {
	res, err := fileio.ImportReader(context.Background(), "nu", bytes.NewReader(g.data), fileio.Options{})
	if err != nil {
		g.err = err.Error()
		return
	}
	s := res.Sheet
	s.Book().SetLocale(g.parent.book().LocaleTag())
	s.LoadFrozen(1, 0)
	s.LoadFitWidths()
	s.ClearHistory()
	used, _ := s.UsedRange()
	g.cols, g.rows = used.To.Col+1, used.To.Row
	c := New(s, "")
	c.out, c.named = g, true
	c.th = g.parent.th
	c.prefs = g.parent.prefs
	c.prefs.vim = false // vim's keys edit cells, and an output's are read-only
	c.term = terminal{} // no images: its sheet has no charts, and the program draws the screen
	c.cur, c.top = sheet.Addr{Row: 1}, 1
	g.child = c
}

// table is the table on the child's sheet, its header row included.
func (g *outGrid) table() sheet.Rect {
	return sheet.Rect{To: sheet.Addr{Col: max(g.cols-1, 0), Row: max(g.rows, 1)}}
}

// Rows is how many rows show: those the filter keeps.
func (g *outGrid) Rows() int {
	if g.child == nil {
		return g.rows
	}
	if s := g.child.sheet; s.HiddenRows() > 0 {
		return len(g.shownRows())
	}
	return g.rows
}

// shownRows are the rows a filter keeps, worked out once for each
// version of the sheet.
func (g *outGrid) shownRows() []int {
	s := g.child.sheet
	if g.shown != nil && g.shownAt == s.Version() {
		return g.shown
	}
	g.shown = g.shown[:0]
	for r := 1; r <= g.rows; r++ {
		if !s.RowHidden(r) {
			g.shown = append(g.shown, r)
		}
	}
	g.shownAt = s.Version()
	return g.shown
}

// row is the sheet row of the i-th row showing.
func (g *outGrid) row(i int) (int, bool) {
	if i < 0 || i >= g.Rows() {
		return 0, false
	}
	if g.child == nil || g.child.sheet.HiddenRows() == 0 {
		return i + 1, true
	}
	return g.shownRows()[i], true
}

// index is where sheet row r is among the rows showing: the first
// showing at or after it.
func (g *outGrid) index(r int) int {
	if g.child == nil || g.child.sheet.HiddenRows() == 0 {
		return max(r-1, 0)
	}
	i, _ := slices.BinarySearch(g.shownRows(), r)
	return i
}

// Top is the first row the window shows.
func (g *outGrid) Top() int {
	if g.child == nil {
		return 0
	}
	return min(g.index(g.child.top), max(g.Rows()-g.win, 0))
}

// Scroll scrolls the window d rows, reporting whether it moved.
func (g *outGrid) Scroll(d int) bool {
	if g.child == nil && !g.ready(g.parent.nbGridWidth(), 10) {
		return false
	}
	top := g.Top()
	to := min(max(top+d, 0), max(g.Rows()-g.win, 0))
	if to == top {
		return false
	}
	r, _ := g.row(to)
	g.child.top = r
	return true
}

// Line draws line i of the window: the column header, or a row.
func (g *outGrid) Line(i, from, rows, width int) string {
	if !g.ready(width, rows) {
		if i == 0 {
			return g.parent.th.Warning.Render(ansi.Truncate("The output isn't a table: "+g.err, width, "…"))
		}
		return ""
	}
	c := g.child
	if rows > g.win {
		// Shown whole: the row numbers are as wide as the last one's,
		// which the child measures from its top.
		top := c.top
		c.top, _ = g.row(max(g.Rows()-g.win, 0))
		defer func() { c.top = top }()
	}
	var line string
	if i == 0 {
		line = c.headerRow()
	} else {
		r, ok := g.row(from + i - 1)
		if !ok {
			return ""
		}
		line = c.gridRow(r)
	}
	return ansi.Truncate(line, min(width, g.right()), "")
}

// maxWin is the most rows the child is sized to show.
const maxWin = 500

// right is where the table's last column ends on the child's screen:
// nothing is drawn past it.
func (g *outGrid) right() int {
	c := g.child
	if g.cols <= c.left {
		return c.width
	}
	return c.colStart(g.cols)
}

// Hidden is how many columns don't show whole at width.
func (g *outGrid) Hidden(width int) int {
	if !g.ready(width, g.win) {
		return 0
	}
	c := g.child
	shown := 0
	for col := c.left; col < g.cols && c.colStart(col)+c.sheet.ColWidth(col) <= width; col++ {
		shown++
	}
	return g.cols - shown
}

// Entered reports whether the grid has the keys.
func (g *outGrid) Entered() bool { return g.parent.nb.out.in == g }

// unfocused reports whether m is an output's grid without the keys,
// which draws neither its active cell nor its selection.
func (m *Model) unfocused() bool { return m.out != nil && !m.out.Entered() }

// colLabel is column c's name in the column header, w wide: its letters,
// or on an output's grid the table's name for it, cut to fit beside
// room more columns (a filter's button).
func (g *grid) colLabel(c, w, room int) string {
	if !g.named {
		return sheet.ColName(c)
	}
	return ansi.Truncate(g.colName(c), max(w-2-room, 1), "…")
}

// colName names column c in messages: its letters, or the table's name
// for it on an output's grid.
func (g *grid) colName(c int) string {
	if !g.named {
		return sheet.ColName(c)
	}
	return g.sheet.ShownText(sheet.Addr{Col: c})
}

// rowLabel is row r's number: on an output's grid, counted from the
// first row under the header.
func (g *grid) rowLabel(r int) string {
	if g.named {
		return strconv.Itoa(r)
	}
	return strconv.Itoa(r + 1)
}

// nbGridWidth is how wide the notebook shown draws outputs' grids.
func (m *Model) nbGridWidth() int {
	if v := m.nbView(); v != nil {
		return v.GridWidth()
	}
	return m.width
}
