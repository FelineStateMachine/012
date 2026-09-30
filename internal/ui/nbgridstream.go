package ui

import (
	"context"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// A cell run as a stream prints rows until it's stopped, and its output
// grows a few times a second. Its grid takes the rows that arrived
// under those it has (nbview.View.Carry) rather than being made again,
// so what the reader has done in it stays: the pointer, the selection,
// the rows as sorted (rows arriving go under them) and the filter, which
// covers the new rows when it covered the table to its end. Columns
// widen to fit the new rows' text as fitting the table fits it, never
// narrowing, and not while the grid has the keys, so the table doesn't
// shift under the pointer: they widen once it's left. A column the
// reader resized keeps its width.

// Append adds more, a NUON table of n rows, under the grid's rows
// (nbview.Grid); data is the whole output, which a grid not made yet
// is made from.
func (g *outGrid) Append(more []byte, n int, data []byte) {
	g.data = data
	if g.child == nil {
		g.rows += n
		return
	}
	s := g.child.sheet
	from, cols := g.rows, g.cols
	rows, all, err := fileio.AppendTable(context.Background(), s, from, cols, more)
	if err != nil {
		g.child.note = "Rows the stream printed aren't a table: " + err.Error()
	}
	g.rows, g.cols = from+rows, max(all, cols)
	g.followFilter(from, cols)
	g.measure(from, cols)
	g.widen()
}

// followFilter has the child's filter cover the rows and columns that
// arrived when it covered the table's last row, of from, and its last
// column, of cols, and applies it to them.
func (g *outGrid) followFilter(from, cols int) {
	s := g.child.sheet
	f := s.Filter()
	if f == nil {
		return
	}
	if f.Range.To.Row >= from {
		f.Range.To.Row = max(f.Range.To.Row, g.rows)
	}
	if f.Range.To.Col >= cols-1 {
		f.Range.To.Col = max(f.Range.To.Col, g.cols-1)
	}
	s.LoadFilter(f)
}

// measure widens need to the text of the rows after row from, and of
// the header of the columns after the first cols.
func (g *outGrid) measure(from, cols int) {
	s := g.child.sheet
	for c := len(g.fit); c < g.cols; c++ {
		g.fit, g.need = append(g.fit, s.ColWidth(c)), append(g.need, 0)
	}
	for c := range g.cols {
		first := from + 1
		if c >= cols {
			first = 0
		}
		w := g.need[c]
		for r := first; r <= g.rows; r++ {
			w = max(w, sheet.FitWidth(s.ShownText(sheet.Addr{Col: c, Row: r})))
		}
		g.need[c] = w
	}
}

// widen widens the columns to what their text needs, unless the grid
// has the keys: a column keeps a width the reader gave it.
func (g *outGrid) widen() {
	if g.Entered() {
		return
	}
	s := g.child.sheet
	for c, w := range g.need {
		if now := s.ColWidth(c); now == g.fit[c] && w > now {
			s.LoadColWidth(c, w)
			g.fit[c] = s.ColWidth(c)
		}
	}
}
