package sheet

// UsedRange returns the smallest range from A1 covering every non-blank
// cell, and false if the sheet is empty.
func (s *Sheet) UsedRange() (Rect, bool) {
	b, ok := s.cells.filled.bounds(Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}})
	return Rect{To: b.To}, ok
}

// FilledBounds returns the smallest range holding every non-blank cell
// of r, and false if there is none.
func (s *Sheet) FilledBounds(r Rect) (Rect, bool) { return s.cells.filled.bounds(r) }

// Edge returns where a data-edge jump (Ctrl+arrow in Excel, End+arrow in
// 1-2-3) from a in direction (dc, dr) lands. Inside a block of filled cells
// it stops at the block's last cell; from a blank cell, or at the end of a
// block, it goes to the next filled cell. With nothing filled ahead it
// stops at the edge of the worksheet. Rows the filter hides are skipped.
// Blank stretches are jumped over through the index of filled cells, so
// a jump costs what the block it walks holds, not the empty rows past it.
func (s *Sheet) Edge(a Addr, dc, dr int) Addr {
	filled := func(a Addr) bool { return s.cells.filled.has(a) }
	step := func(a Addr) Addr {
		if dr != 0 {
			r, _ := s.NextShownRow(a.Row, dr) // off the sheet when there is none
			return Addr{Col: a.Col, Row: r}
		}
		return Addr{Col: a.Col + dc, Row: a.Row}
	}
	next := step(a)
	if !next.Valid() {
		return a
	}
	if filled(a) && filled(next) {
		for {
			n := step(next)
			if !n.Valid() || !filled(n) {
				return next
			}
			next = n
		}
	}
	if f, ok := s.nextFilled(a, dc, dr); ok {
		return f
	}
	return s.sheetEdge(a, dc, dr)
}

// nextFilled is the first filled cell past a in direction (dc, dr), on
// a row the filter doesn't hide.
func (s *Sheet) nextFilled(a Addr, dc, dr int) (Addr, bool) {
	fl := &s.cells.filled
	if dr != 0 {
		for row := a.Row + dr; ; {
			r, ok := fl.nextRow(a.Col, row, dr)
			if !ok {
				return Addr{}, false
			}
			if !s.RowHidden(r) {
				return Addr{Col: a.Col, Row: r}, true
			}
			row = r + dr
		}
	}
	limit := MaxCols - 1
	if dc < 0 {
		limit = 0
	}
	c, ok := s.NextFilledCol(a.Row, a.Col+dc, dc, limit)
	return Addr{Col: c, Row: a.Row}, ok
}

// NextFilledCol returns the nearest column of row, from col on in
// direction dir (1 or -1) and no further than limit, whose cell has
// contents, looking only at the columns that hold any.
func (s *Sheet) NextFilledCol(row, col, dir, limit int) (int, bool) {
	fl := &s.cells.filled
	lo, hi := col, limit
	if dir < 0 {
		lo, hi = limit, col
	}
	if lo > hi {
		return 0, false
	}
	cols := fl.colsIn(max(lo, 0), min(hi, MaxCols-1))
	for i := range cols {
		c := cols[i]
		if dir < 0 {
			c = cols[len(cols)-1-i]
		}
		if fl.has(Addr{Col: c, Row: row}) {
			return c, true
		}
	}
	return 0, false
}

// sheetEdge is the last cell from a in direction (dc, dr): the sheet's
// last (or first) column, or its last (or first) row the filter doesn't
// hide, or a itself when every row past it is hidden.
func (s *Sheet) sheetEdge(a Addr, dc, dr int) Addr {
	switch {
	case dc > 0:
		a.Col = MaxCols - 1
	case dc < 0:
		a.Col = 0
	case dr > 0:
		if r, ok := s.NextShownRow(MaxRows, -1); ok && r > a.Row {
			a.Row = r
		}
	default:
		if r, ok := s.NextShownRow(-1, 1); ok && r < a.Row {
			a.Row = r
		}
	}
	return a
}

// colRect is the range covering whole columns from..to.
func colRect(from, to int) Rect {
	return Rect{From: Addr{Col: from}, To: Addr{Col: to, Row: MaxRows - 1}}
}

// rowRect is the range covering whole rows from..to.
func rowRect(from, to int) Rect {
	return Rect{From: Addr{Row: from}, To: Addr{Col: MaxCols - 1, Row: to}}
}

func union(a, b Rect) Rect {
	return Rect{
		From: Addr{Col: min(a.From.Col, b.From.Col), Row: min(a.From.Row, b.From.Row)},
		To:   Addr{Col: max(a.To.Col, b.To.Col), Row: max(a.To.Row, b.To.Row)},
	}
}
