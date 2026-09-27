package sheet

import (
	"maps"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Column and row formats, as in Sheets: formatting whole columns or rows
// stores the format once for the line instead of on each of its million
// cells, and cells take it when they have none of their own. A cell's
// format and style come from the cell, else its row, else its column;
// each part (number format, text style) falls back on its own, so a
// currency cell in a bold column shows bold currency. A cell whose
// formatting differs from its line's in a way the fallback can't express
// (Automatic in a currency column) keeps its own formatting whole, marked
// by Style.own.

// lineFmt is the formatting of a column, a row, or what a cell shows.
type lineFmt struct {
	Format Format
	Style  Style
}

func (l lineFmt) IsZero() bool { return l == lineFmt{} }

// lineFormats are a sheet's column and row formats; nil maps when none.
type lineFormats struct {
	cols map[int]lineFmt
	rows map[int]lineFmt
}

// lineKey is a column (row false) or row of a sheet, for undo.
type lineKey struct {
	s   *Sheet
	row bool
	n   int
}

// line is a column's (row false) or row's formatting.
func (s *Sheet) line(row bool, n int) lineFmt {
	if row {
		return s.lines.rows[n]
	}
	return s.lines.cols[n]
}

// inherited is what a cell at a shows without formatting of its own: its
// row's format and style, else its column's.
func (s *Sheet) inherited(a Addr) lineFmt {
	if s.lines.rows == nil && s.lines.cols == nil {
		return lineFmt{}
	}
	l := s.lines.rows[a.Row]
	if c, ok := s.lines.cols[a.Col]; ok {
		if l.Format.IsZero() {
			l.Format = c.Format
		}
		if l.Style.IsZero() {
			l.Style = c.Style
		}
	}
	return l
}

// effective is the format (without what formulas infer) and style the
// cell at a shows: its own, falling back part by part on its line's.
func (s *Sheet) effective(a Addr) lineFmt {
	c := s.cells.get(a)
	if c != nil && c.Style.own {
		st := c.Style
		st.own = false
		return lineFmt{c.Format, st}
	}
	l := s.inherited(a)
	if c != nil {
		if !c.Format.IsZero() {
			l.Format = c.Format
		}
		if !c.Style.IsZero() {
			l.Style = c.Style
		}
	}
	return l
}

// CellFormat returns the number format the cell at a has, its own or
// its row's or column's, without the one a formula infers.
func (s *Sheet) CellFormat(a Addr) Format { return s.effective(a).Format }

// CellStyle returns the text style the cell at a shows: its own, or its
// row's or column's.
func (s *Sheet) CellStyle(a Addr) Style { return s.effective(a).Style }

// own is the formatting a cell stores to show want where it would
// otherwise show inh.
func own(want, inh lineFmt) (Format, Style) {
	f, st := want.Format, want.Style
	switch {
	case want == inh:
		return Format{}, Style{}
	case f.IsZero() && !inh.Format.IsZero(), st.IsZero() && !inh.Style.IsZero():
		st.own = true // Automatic, or plain text, over a formatted line
		return f, st
	}
	if f == inh.Format {
		f = Format{}
	}
	if st == inh.Style {
		st = Style{}
	}
	return f, st
}

// ColFormats and RowFormats return the columns and rows that have a
// format or style of their own, by index.
func (s *Sheet) ColFormats() map[int]Format { return lineFormatsOf(s.lines.cols) }
func (s *Sheet) RowFormats() map[int]Format { return lineFormatsOf(s.lines.rows) }

// ColStyles and RowStyles are the same for text styles.
func (s *Sheet) ColStyles() map[int]Style { return lineStylesOf(s.lines.cols) }
func (s *Sheet) RowStyles() map[int]Style { return lineStylesOf(s.lines.rows) }

func lineFormatsOf(m map[int]lineFmt) map[int]Format {
	out := map[int]Format{}
	for n, l := range m {
		if !l.Format.IsZero() {
			out[n] = l.Format
		}
	}
	return out
}

func lineStylesOf(m map[int]lineFmt) map[int]Style {
	out := map[int]Style{}
	for n, l := range m {
		if !l.Style.IsZero() {
			out[n] = l.Style
		}
	}
	return out
}

// LoadLineFormat gives a whole column (row false) or row a format and
// style, as an importer does, without recording undo or touching its
// cells.
func (s *Sheet) LoadLineFormat(row bool, n int, f Format, st Style) {
	if row && n >= 0 && n < MaxRows || !row && n >= 0 && n < MaxCols {
		s.setLine(row, n, lineFmt{f, st})
	}
}

func (s *Sheet) lineRect(row bool, from, to int) Rect {
	if row {
		return rowRect(from, to)
	}
	return colRect(from, to)
}

// setLine sets a column's or row's formatting, recording it for undo and,
// within a step, marking its cells changed, so formulas reading them infer
// their format again.
func (s *Sheet) setLine(row bool, n int, l lineFmt) {
	m := &s.lines.cols
	if row {
		m = &s.lines.rows
	}
	if (*m)[n] == l {
		return
	}
	if st := s.wb.hist.open; st != nil {
		k := lineKey{s, row, n}
		if _, seen := st.lines[k]; !seen {
			st.lines[k] = (*m)[n]
		}
	}
	if l.IsZero() {
		delete(*m, n)
		if len(*m) == 0 {
			*m = nil
		}
	} else {
		if *m == nil {
			*m = map[int]lineFmt{}
		}
		(*m)[n] = l
	}
	s.version++
	if s.wb.hist.open != nil {
		s.wb.hist.dirty = append(s.wb.hist.dirty, s.lineCells(row, n)...)
	}
}

// lineCells are the stored cells of a column or row.
func (s *Sheet) lineCells(row bool, n int) []loc {
	var out []loc
	for _, a := range s.cellsIn(s.lineRect(row, n, n)) {
		out = append(out, loc{s, a})
	}
	return out
}

// formatLines applies fn to the formatting of r, which is whole columns
// or rows: to their line formats and to the cells in r that show
// something else. It reports false, doing nothing, for any other range.
func (s *Sheet) formatLines(r Rect, fn func(*lineFmt)) bool {
	cols, rows := r.AllRows(), r.AllCols()
	if !cols && !rows {
		return false
	}
	// The cells that may show something other than their line: those
	// stored, and where a formatted line crosses r the other way.
	targets := s.cellsIn(r)
	crossing, across := s.lines.rows, Rect{From: Addr{Col: r.From.Col}, To: Addr{Col: r.To.Col}}
	if !cols {
		crossing, across = s.lines.cols, Rect{From: Addr{Row: r.From.Row}, To: Addr{Row: r.To.Row}}
	}
	if !(cols && rows) {
		for _, n := range slices.Sorted(maps.Keys(crossing)) {
			if len(targets) > materializeLimit {
				break
			}
			for i := range across.To.Col - across.From.Col + across.To.Row - across.From.Row + 1 {
				a := Addr{Col: across.From.Col + i, Row: n} // a column's cells in formatted row n
				if !cols {
					a = Addr{Col: n, Row: across.From.Row + i} // a row's cells in formatted column n
				}
				if s.cells.get(a) == nil {
					targets = append(targets, a)
				}
			}
		}
	}
	want := make([]lineFmt, len(targets))
	for i, a := range targets {
		want[i] = s.effective(a)
		fn(&want[i])
	}
	s.eachLine(r, fn)
	for i, a := range targets {
		s.placeFormat(a, want[i])
	}
	return true
}

// eachLine applies fn to the line formats of r's whole columns, or rows;
// for the whole sheet, to every column and to the rows that have one.
func (s *Sheet) eachLine(r Rect, fn func(*lineFmt)) {
	apply := func(row bool, n int, l lineFmt) {
		fn(&l)
		s.setLine(row, n, l)
	}
	if r.AllRows() {
		for c := r.From.Col; c <= r.To.Col; c++ {
			apply(false, c, s.lines.cols[c])
		}
		if r.AllCols() {
			for _, n := range slices.Sorted(maps.Keys(s.lines.rows)) {
				apply(true, n, s.lines.rows[n])
			}
		}
		return
	}
	for n := r.From.Row; n <= r.To.Row; n++ {
		apply(true, n, s.lines.rows[n])
	}
}

// placeFormat has the cell at a show want, giving it formatting of its
// own only where its lines don't already show it, and placing a copy so
// the change can be undone. A cell left with neither contents nor
// formatting is removed.
func (s *Sheet) placeFormat(a Addr, want lineFmt) {
	old := s.cells.get(a)
	c := old.clone()
	if c == nil {
		c = &Cell{}
	}
	c.Format, c.Style = own(want, s.inherited(a))
	switch {
	case c.Blank() && c.Format.IsZero() && c.Style.IsZero():
		if old != nil {
			s.place(a, nil)
		}
	case old == nil || c.Format != old.Format || c.Style != old.Style:
		s.place(a, c)
	}
}

// shiftLines moves column (or row) formats along with inserted or
// deleted columns (rows).
func (s *Sheet) shiftLines(rows bool, sp formula.Span) {
	m := s.lines.cols
	if rows {
		m = s.lines.rows
	}
	next := map[int]lineFmt{}
	for n, l := range m {
		if to, ok := sp.Point(n); ok {
			next[to] = l
		}
	}
	for _, n := range slices.Collect(maps.Keys(m)) {
		if _, ok := next[n]; !ok {
			s.setLine(rows, n, lineFmt{})
		}
	}
	for n, l := range next {
		s.setLine(rows, n, l)
	}
}
