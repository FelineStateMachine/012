package sheet

// UsedRange returns the smallest range from A1 covering every non-blank
// cell, and false if the sheet is empty.
func (s *Sheet) UsedRange() (Rect, bool) {
	var last Addr
	found := false
	for a, c := range s.cells {
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
// stops at the edge of the worksheet.
func (s *Sheet) Edge(a Addr, dc, dr int) Addr {
	filled := func(a Addr) bool { return !s.cells[a].Blank() }
	next := Addr{Col: a.Col + dc, Row: a.Row + dr}
	if !next.Valid() {
		return a
	}
	if filled(a) && filled(next) {
		for {
			n := Addr{Col: next.Col + dc, Row: next.Row + dr}
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
		n := Addr{Col: next.Col + dc, Row: next.Row + dr}
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

// RangeStats computes Stats over r, visiting whichever is smaller: the
// cells in r or the non-blank cells in the sheet.
func (s *Sheet) RangeStats(r Rect) Stats {
	var st Stats
	add := func(c *Cell) {
		st.Count++
		if c.Value.Kind == Number {
			st.Nums++
			st.Sum += c.Value.Num
		}
	}
	area := (r.To.Col - r.From.Col + 1) * (r.To.Row - r.From.Row + 1)
	if area <= len(s.cells) {
		for row := r.From.Row; row <= r.To.Row; row++ {
			for col := r.From.Col; col <= r.To.Col; col++ {
				if c := s.cells[Addr{Col: col, Row: row}]; !c.Blank() {
					add(c)
				}
			}
		}
		return st
	}
	for a, c := range s.cells {
		if r.Contains(a) && !c.Blank() {
			add(c)
		}
	}
	return st
}
