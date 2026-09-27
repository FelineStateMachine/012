package sheet

// UsedRange returns the smallest range from A1 covering every non-blank
// cell, and false if the sheet is empty.
func (s *Sheet) UsedRange() (Rect, bool) {
	var last Addr
	found := false
	for a, c := range s.cells.all() {
		if c.Blank() {
			continue
		}
		found = true
		last.Col = max(last.Col, a.Col)
		last.Row = max(last.Row, a.Row)
	}
	return Rect{To: last}, found
}

// Edge returns where a data-edge jump (Ctrl+arrow in Excel, End+arrow in
// 1-2-3) from a in direction (dc, dr) lands. Inside a block of filled cells
// it stops at the block's last cell; from a blank cell, or at the end of a
// block, it goes to the next filled cell. With nothing filled ahead it
// stops at the edge of the worksheet. Rows the filter hides are skipped.
func (s *Sheet) Edge(a Addr, dc, dr int) Addr {
	filled := func(a Addr) bool { return !s.cells.get(a).Blank() }
	step := func(a Addr) Addr {
		n := Addr{Col: a.Col + dc, Row: a.Row + dr}
		for dr != 0 && n.Valid() && s.RowHidden(n.Row) {
			n.Row += dr
		}
		return n
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
	for {
		if filled(next) {
			return next
		}
		n := step(next)
		if !n.Valid() {
			return next
		}
		next = n
	}
}
