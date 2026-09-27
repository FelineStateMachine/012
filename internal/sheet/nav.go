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

// Stats summarizes the values in r, as shown in the status line for a
// selection. Count is non-blank cells, Nums is numeric cells.
type Stats struct {
	Sum         float64
	Count, Nums int
}

// statsCache is the last RangeStats result: the status line asks for the
// selection's statistics on every frame, and over a whole sheet of two
// million cells they take tens of milliseconds.
type statsCache struct {
	r       Rect
	version uint64
	st      Stats
	ok      bool
}

// RangeStats computes Stats over r. The result is kept until a cell or
// value changes.
func (s *Sheet) RangeStats(r Rect) Stats {
	if c := s.stats; c.ok && c.r == r && c.version == s.version {
		return c.st
	}
	var st Stats
	add := func(c *Cell) {
		st.Count++
		if c.Value.Kind == Number {
			st.Nums++
			st.Sum += c.Value.Num
		}
	}
	for _, c := range s.cells.inRange(r) {
		if !c.Blank() {
			add(c)
		}
	}
	s.stats = statsCache{r: r, version: s.version, st: st, ok: true}
	return st
}
