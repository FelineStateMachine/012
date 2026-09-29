package nbview

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// A table (a list of records) or a record is drawn by the UI as its own
// grid, which the output's window shows: the grid's column header, named
// after the table's columns, then as many of its rows as the window
// holds. The UI owns everything about the grid (its cells, formats,
// widths, the active cell, the selection, sorting and filtering); the
// notebook places it, says how many rows show, and scrolls it.

// Grid is an output drawn as the UI's grid.
type Grid interface {
	// Rows is how many rows the grid has showing: the rows a filter
	// keeps, the header not counted.
	Rows() int
	// Top is the first row the window shows, counted among those.
	Top() int
	// Scroll scrolls the window d rows, reporting whether it moved.
	Scroll(d int) bool
	// Line draws line i of a window of rows rows at width, showing the
	// rows from row from on: the column header for 0, then the rows.
	Line(i, from, rows, width int) string
	// Hidden is how many of the grid's columns don't show at width.
	Hidden(width int) int
	// Entered reports whether the grid has the keys: its active cell
	// and selection show, and arrows move them.
	Entered() bool
}

// gridData is what the UI's grid of an output holds, as NUON: a table as
// it is, and a record as a table of its fields, a column of their names
// and one of their values.
func gridData(v nuon.Value, data []byte) []byte {
	if v.Kind != nuon.Record {
		return data
	}
	list := nuon.Value{Kind: nuon.List}
	for _, f := range v.Fields {
		list.List = append(list.List, nuon.Value{Kind: nuon.Record, Fields: []nuon.Field{
			{Key: "field", Value: nuon.StringValue(f.Key)}, {Key: "value", Value: f.Value}}})
	}
	return nuon.Append(nil, list)
}

// entered reports whether the output's grid has the keys.
func (sh *shown) entered() bool { return sh.grid != nil && sh.grid.Entered() }

// gridRows is how many of a grid's rows show, and from which: all of
// them from the first when the output shows whole (unless it's entered,
// when it scrolls in its window), or a window's worth from where it's
// scrolled.
func (sh *shown) gridRows(whole bool) (from, n int) {
	total := sh.grid.Rows()
	if whole && !sh.entered() {
		return 0, total
	}
	return sh.grid.Top(), min(total, window)
}

// gridLine draws line i of a grid's window.
func (sh *shown) gridLine(th *theme.Theme, i int, f fold, width int) string {
	from, n := sh.gridRows(f.whole)
	switch {
	case i <= n:
		return sh.grid.Line(i, from, max(n, 1), width)
	case n == 0 && i == 1:
		return th.Muted.Render("no rows: the filter hides them all")
	}
	return sh.gridFooter(th, from, n, width)
}

// gridFooter says what a grid's window leaves out.
func (sh *shown) gridFooter(th *theme.Theme, from, n, width int) string {
	total := sh.grid.Rows()
	unit := "row"
	if sh.kind == outRecord {
		unit = "field"
	}
	var parts []string
	switch {
	case n >= total:
	case from == 0:
		parts = append(parts, more(total-n, "more "+unit))
	default:
		parts = append(parts, unit+"s "+grouped(from+1)+" to "+grouped(from+n)+" of "+grouped(total))
	}
	if c := sh.grid.Hidden(width); c > 0 {
		parts = append(parts, more(c, "more column"))
	}
	var hints []string
	if n < total && !sh.entered() {
		hints = append(hints, "O shows all")
	}
	if !sh.entered() {
		hints = append(hints, "Enter works in it")
	}
	line := "… " + strings.Join(parts, ", ")
	if len(hints) > 0 {
		line += "  (" + strings.Join(hints, ", ") + ")"
	}
	return th.Muted.Render(line)
}

// SelectedGrid is the selected output's grid, if it's selected and drawn
// as one.
func (v *View) SelectedGrid() Grid {
	c, ok := v.Cell()
	if !ok || !v.onOut {
		return nil
	}
	return v.shown(c).grid
}

// Window is how many rows an output's window shows.
const Window = window

// Shows reports whether grid g is the selected output's, or shown
// full-screen.
func (v *View) Shows(g Grid) bool {
	if v.full != nil {
		return v.full.Grid() == g
	}
	return g != nil && v.SelectedGrid() == g
}

// SelectGrid selects the output grid g is drawn in.
func (v *View) SelectGrid(g Grid) {
	for i, c := range v.h.Cells() {
		if sh := v.shown(c); g != nil && sh.grid == g {
			v.StopEdit()
			v.Select(i, true)
			return
		}
	}
}

// GridAt is the grid at column x of line y of the body, with where that
// is in the grid: gy 0 its column header, then its rows as the window
// shows them.
func (v *View) GridAt(x, y int) (g Grid, gx, gy int, ok bool) {
	if v.full != nil {
		if g = v.full.Grid(); g == nil || y < 0 || y >= v.height {
			return nil, 0, 0, false
		}
		return g, x, y, true
	}
	h, ok := v.hitAt(y)
	if !ok || h.kind != rowOut || x < textX {
		return nil, 0, 0, false
	}
	c := v.h.Cells()[h.cell]
	f := v.foldOf(c.ID)
	sh := v.shown(c)
	if f.hidden || !sh.isGrid() {
		return nil, 0, 0, false
	}
	if _, n := sh.gridRows(f.whole); h.r > n {
		return nil, 0, 0, false
	}
	return sh.grid, x - textX, h.r, true
}

// GridOrigin is where grid g's column header is drawn in the body: its
// first column and line, which may be off screen.
func (v *View) GridOrigin(g Grid) (x, y int, ok bool) {
	if v.full != nil {
		return 0, 0, v.full.Grid() == g
	}
	for i, c := range v.h.Cells() {
		if sh := v.shown(c); g != nil && sh.grid == g {
			b := v.blockOf(i, c)
			return textX, v.startOf(i) + 2*b.box + b.src - v.top, true
		}
	}
	return 0, 0, false
}

// GridWidth is how wide an output's grid is drawn: the output's width,
// or the body's full-screen.
func (v *View) GridWidth() int {
	if v.full != nil {
		return v.width
	}
	return v.outWidth()
}

// FollowOutput scrolls so the selected output's window shows, as far as
// it fits: its grid's active cell moved, or the grid changed its height.
func (v *View) FollowOutput() {
	if v.full == nil && v.onOut {
		v.follow()
	}
}
