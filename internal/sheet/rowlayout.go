package sheet

import (
	"bytes"
	"maps"
	"slices"
	"strconv"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Row layout: what makes a row of the grid more than one line of text.
// A row is as tall as its height, in lines, when one is set by hand
// (Format > Row height, or dragging the row's border), kept per sheet as
// column widths are; otherwise the grid grows it to fit the text its
// cells wrap. A horizontal border takes a line of its own above the row
// it tops. Cells whose own style wraps text or draws borders are indexed
// by row (shapers), so the grid measures the rows it shows at the cost
// of those cells, and a sheet without any costs nothing.

// MaxRowHeight caps a row's height in lines.
const MaxRowHeight = 50

// shapes reports whether a style changes the layout of its row: text
// that wraps, or borders.
func (st Style) shapes() bool { return st.Wrap == WrapOn || !st.Borders.IsZero() }

// shaperIndex holds the columns of each row whose cells' own styles
// shape it, and whether any line format does, as of a version.
type shaperIndex struct {
	rows      map[int]map[int]struct{}
	linesAt   uint64 // the version the rest was worked out at, plus one
	lineShape bool   // a line format wraps or draws borders
	colRules  bool   // a column's format draws a top or bottom edge
}

// trackShape keeps the index as the cell at a becomes c.
func (s *Sheet) trackShape(a Addr, c *Cell) {
	_, was, _ := s.cells.look(a)
	had, has := was.shapes(), c != nil && c.Style.shapes()
	switch {
	case has && !had:
		if s.shapers.rows == nil {
			s.shapers.rows = map[int]map[int]struct{}{}
		}
		row := s.shapers.rows[a.Row]
		if row == nil {
			row = map[int]struct{}{}
			s.shapers.rows[a.Row] = row
		}
		row[a.Col] = struct{}{}
	case had && !has:
		row := s.shapers.rows[a.Row]
		delete(row, a.Col)
		if len(row) == 0 {
			delete(s.shapers.rows, a.Row)
		}
	}
}

// linesShape reports whether any line format wraps or draws borders.
func (s *Sheet) linesShape() bool {
	if x := &s.shapers; x.linesAt != s.version+1 {
		x.linesAt, x.lineShape, x.colRules = s.version+1, s.lines.sheet.Style.shapes(), false
		for _, l := range s.lines.cols {
			x.lineShape = x.lineShape || l.Style.shapes()
			x.colRules = x.colRules || l.Style.Borders.horizontal()
		}
		for _, l := range s.lines.rows {
			x.lineShape = x.lineShape || l.Style.shapes()
		}
	}
	return s.shapers.lineShape
}

// horizontal reports whether b draws its top or bottom edge.
func (b Borders) horizontal() bool { return b.Top() != LineNone || b.Bottom() != LineNone }

// Shaped reports whether any row may be other than one line of text
// under the one above: something wraps or draws borders, a row has a
// height of its own, or cells are merged. The grid lays out a sheet that
// isn't shaped a line per row without asking more.
func (s *Sheet) Shaped() bool {
	return len(s.shapers.rows) > 0 || len(s.heights) > 0 || len(s.view.merges) > 0 || s.linesShape()
}

// Version changes whenever anything the grid draws may have: cells,
// their values and formats, widths, heights and merges.
func (s *Sheet) Version() uint64 { return s.version }

// shapersIn calls fn with the columns of row whose cells may wrap or have
// borders: those indexed, and when line formats shape, those of
// formatted columns and, for a shaping row or sheet format, every cell
// the row holds.
func (s *Sheet) shapersIn(row int, fn func(col int)) {
	for c := range s.shapers.rows[row] {
		fn(c)
	}
	if !s.linesShape() {
		return
	}
	if s.lines.sheet.Style.shapes() || s.lines.rows[row].Style.shapes() {
		for a := range s.cells.inRange(rowRect(row, row)) {
			if _, seen := s.shapers.rows[row][a.Col]; !seen {
				fn(a.Col)
			}
		}
		return
	}
	for c, l := range s.lines.cols {
		if _, seen := s.shapers.rows[row][c]; !seen && l.Style.shapes() {
			fn(c)
		}
	}
}

// BorderedIn returns the blank cells of r whose own formatting draws
// borders, in no particular order, at the cost of the cells that wrap or
// draw borders.
func (s *Sheet) BorderedIn(r Rect) []Addr {
	var out []Addr
	for row, cols := range s.shapers.rows {
		if row < r.From.Row || row > r.To.Row {
			continue
		}
		for c := range cols {
			a := Addr{Col: c, Row: row}
			if _, st, _ := s.cells.look(a); r.Contains(a) && !s.cells.filledAt(a) && !st.Borders.IsZero() {
				out = append(out, a)
			}
		}
	}
	return out
}

// WrappedIn returns the columns of row whose cells wrap text they hold,
// outside merged cells, in no particular order.
func (s *Sheet) WrappedIn(row int) []int {
	var out []int
	s.shapersIn(row, func(c int) {
		a := Addr{Col: c, Row: row}
		if !s.cells.filledAt(a) || s.CellStyle(a).Wrap != WrapOn {
			return
		}
		if _, merged := s.MergeAt(a); !merged {
			out = append(out, c)
		}
	})
	return out
}

// RuleAbove reports whether a border lies on the edge above row: the top
// of one of its cells or the bottom of one in the row before, or a
// column's, row's or the sheet's format drawing either.
func (s *Sheet) RuleAbove(row int) bool {
	if s.linesShape() && (s.shapers.colRules || s.lines.sheet.Style.Borders.horizontal() ||
		s.lines.rows[row].Style.Borders.Top() != LineNone || row > 0 && s.lines.rows[row-1].Style.Borders.Bottom() != LineNone) {
		return true
	}
	found := false
	s.shapersIn(row, func(c int) {
		found = found || s.CellStyle(Addr{Col: c, Row: row}).Borders.Top() != LineNone
	})
	if row > 0 && !found {
		s.shapersIn(row-1, func(c int) {
			found = found || s.CellStyle(Addr{Col: c, Row: row - 1}).Borders.Bottom() != LineNone
		})
	}
	return found
}

// RowHeight returns the height set for row by hand, in lines, and false
// when it has none and fits its contents.
func (s *Sheet) RowHeight(row int) (int, bool) {
	h, ok := s.heights[row]
	return h, ok
}

// SetRowHeight sets rows from..to to h lines, as one undo step; h 0
// has them fit their contents again, as Sheets' Fit to data. Heights are
// kept row by row, so a range of more than materializeLimit rows (whole
// columns) stops at the last row holding a cell.
func (s *Sheet) SetRowHeight(from, to, h int) {
	label := "row height"
	if h <= 0 {
		label = "fit rows to data"
	}
	if to-from >= materializeLimit {
		used, _ := s.cells.bounds(rowRect(from, to))
		to = max(from, used.To.Row)
	}
	s.change(label, rowRect(from, to), func() {
		for r := from; r <= to; r++ {
			s.setHeight(r, h)
		}
	})
}

// ColsWidth is the width of columns from..to together, at the cost of
// the columns with a width of their own.
func (s *Sheet) ColsWidth(from, to int) int {
	w := (to - from + 1) * DefaultWidth
	for c, cw := range s.widths {
		if c >= from && c <= to {
			w += cw - DefaultWidth
		}
	}
	return w
}

// Heights returns the rows that have a height set by hand.
func (s *Sheet) Heights() map[int]int { return maps.Clone(s.heights) }

// LoadRowHeight sets a row's height as an importer does, without
// recording undo.
func (s *Sheet) LoadRowHeight(row, h int) {
	if row >= 0 && row < MaxRows {
		s.setHeight(row, h)
	}
}

func (s *Sheet) setHeight(row, h int) {
	h = min(h, MaxRowHeight)
	if old, ok := s.heights[row]; ok == (h > 0) && old == h {
		return
	}
	s.recordHeight(row)
	s.version++
	if h <= 0 {
		delete(s.heights, row)
		return
	}
	if s.heights == nil {
		s.heights = map[int]int{}
	}
	s.heights[row] = h
}

// rowKey is a row of a sheet, for undo.
type rowKey struct {
	s   *Sheet
	row int
}

// recordHeight saves row's height before its first change in the open
// step; 0 stands for none.
func (s *Sheet) recordHeight(row int) {
	st := s.wb.hist.open
	if st == nil {
		return
	}
	k := rowKey{s, row}
	if _, seen := st.heights[k]; !seen {
		st.heights[k] = s.heights[row]
	}
}

// shiftHeights moves row heights along with inserted or deleted rows.
func (s *Sheet) shiftHeights(sp formula.Span) {
	next := map[int]int{}
	for r, h := range s.heights {
		if to, ok := sp.Point(r); ok {
			next[to] = h
		}
	}
	for _, r := range slices.Collect(maps.Keys(s.heights)) {
		if _, ok := next[r]; !ok {
			s.setHeight(r, 0)
		}
	}
	for r, h := range next {
		s.setHeight(r, h)
	}
}

// heightsIn is the heights of rows lo..hi, by offset from lo.
func (s *Sheet) heightsIn(lo, hi int) map[int]int {
	var out map[int]int
	for r, h := range s.heights {
		if r >= lo && r <= hi {
			if out == nil {
				out = map[int]int{}
			}
			out[r-lo] = h
		}
	}
	return out
}

// writeHeights writes the "heights" field: rows by number, in lines.
func (s *Sheet) writeHeights(b *bytes.Buffer, indent string) {
	if len(s.heights) == 0 {
		return
	}
	b.WriteString(indent + `"heights": {`)
	for i, r := range slices.Sorted(maps.Keys(s.heights)) {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(strconv.Itoa(r+1)) + ": " + strconv.Itoa(s.heights[r]))
	}
	b.WriteString("},\n")
}
