package sheet

import "github.com/FelineStateMachine/012/internal/formula"

// Where regions are (see region.go): the cells they cover and their
// tables, finding them by cell and by the name formulas use, and keeping
// them in step with inserted and deleted lines and with each other on a
// notebook sheet.

// noRect contains no cell.
var noRect = Rect{From: Addr{Col: -1, Row: -1}, To: Addr{Col: -1, Row: -1}}

// regionArea is the cells a region's definition reserves: a command
// region's label line and table as last shown, at least one column
// wide; a linked file's first cell.
func (s *Sheet) regionArea(r Region) Rect {
	rows := r.Rows
	if r.Linked() {
		rows = 0
	}
	return Rect{From: r.At, To: Addr{Col: min(r.At.Col+max(r.Cols, 1)-1, MaxCols-1), Row: min(r.At.Row+rows, MaxRows-1)}}
}

// tableOrigin is where a region's table starts: under a command
// region's label line, at a linked file's anchor.
func tableOrigin(r Region) Addr {
	if r.labelled() {
		return Addr{Col: r.At.Col, Row: r.At.Row + 1}
	}
	return r.At
}

// RegionTable returns the cells of a region's table, header row
// included, when it has one.
func (s *Sheet) RegionTable(name string) (Rect, bool) {
	i := s.regionIndex(nameKey(name))
	if i < 0 {
		return Rect{}, false
	}
	r := s.regions.list[i]
	rows, cols := r.Rows, r.Cols
	if r.Linked() {
		me := s.regionMeta[nameKey(r.Name)]
		if me == nil || me.why != "" {
			return Rect{}, false
		}
		rows, cols = me.rows, me.cols
	}
	o := tableOrigin(r)
	if rows == 0 || cols == 0 || o.Row+rows-1 >= MaxRows {
		return Rect{}, false
	}
	return Rect{From: o, To: Addr{Col: min(o.Col+cols-1, MaxCols-1), Row: o.Row + rows - 1}}, true
}

// covered is the cells a region holds: those it wrote, or while it's
// yet to write them, those its definition reserves.
func (s *Sheet) covered(r Region) Rect {
	if me := s.regionMeta[nameKey(r.Name)]; me != nil && me.has {
		return me.written
	}
	return s.regionArea(r)
}

// RegionAt returns the region covering the cell at a, and whether a is on
// its label line.
func (s *Sheet) RegionAt(a Addr) (Region, bool, bool) {
	for _, r := range s.regions.list {
		if s.covered(r).Contains(a) {
			return r, r.labelled() && a.Row == r.At.Row, true
		}
	}
	return Region{}, false, false
}

// InRegion returns a cell of r a region covers, and the region.
func (s *Sheet) InRegion(r Rect) (Addr, Region, bool) {
	for _, reg := range s.regions.list {
		if overlap, ok := intersectRect(s.covered(reg), r); ok {
			return overlap.From, reg, true
		}
	}
	return Addr{}, Region{}, false
}

// shiftRegions keeps the sheet's regions in step with rows or columns
// inserted or deleted, before the cells move: a region the lines reach
// is emptied first, so cells moving where it was aren't taken for its
// own, and moves and resizes as a range would; one whose anchor is
// deleted goes. Once the change ends, command regions are written again
// from their tables and linked files read again.
func (s *Sheet) shiftRegions(rows bool, sp formula.Span) {
	if len(s.regions.list) == 0 {
		return
	}
	cell, rng := formula.AxisMaps(rows, sp)
	st := s.regionsCopy()
	st.list = st.list[:0]
	for _, r := range s.regions.list {
		area := s.covered(r)
		end := area.To.Col
		if rows {
			end = area.To.Row
		}
		if end < sp.At {
			st.list = append(st.list, r) // before the lines: untouched
			continue
		}
		me := s.meta(nameKey(r.Name))
		for _, c := range s.emptyRegion(r, me) {
			s.wb.markDirty(c)
		}
		at, ok := cell(r.At)
		moved, ok2 := rng(s.regionArea(r))
		if !ok || !ok2 {
			delete(st.data, nameKey(r.Name))
			continue
		}
		r.At, r.Rows = at, moved.To.Row-at.Row
		if r.Cols > 0 {
			r.Cols = moved.To.Col - at.Col + 1
		}
		if r.Linked() {
			r.Rows, r.Cols = 0, 0
		}
		st.list = append(st.list, r)
	}
	if !sameRegions(st, s.regions) {
		s.putRegions(st)
	}
	s.regionsStale = true
}

// RegionLabels returns the label lines of the command regions on row,
// each as wide as its table, for drawing them.
func (s *Sheet) RegionLabels(row int) []Rect {
	var out []Rect
	for _, r := range s.regions.list {
		if r.labelled() && r.At.Row == row {
			c := s.covered(r)
			out = append(out, Rect{From: r.At, To: Addr{Col: c.To.Col, Row: row}})
		}
	}
	return out
}

// CommandTables returns the tables of the command regions that cross
// row, which are drawn as the sheet's own cells are: their label line
// says whose they are, where a linked file's values show in italics as
// an array's spilled values do.
func (s *Sheet) CommandTables(row int) []Rect {
	var out []Rect
	for _, r := range s.regions.list {
		if t, ok := s.RegionTable(r.Name); ok && r.labelled() && t.From.Row <= row && row <= t.To.Row {
			out = append(out, t)
		}
	}
	return out
}

// spanOf is n rows inserted at row at, or deleted when n is negative.
func spanOf(at, n int) formula.Span { return formula.Span{At: at, N: n, Size: MaxRows} }

// regionName resolves a name formulas use, nu.r1, to the region's sheet
// and table.
func (w *Workbook) regionName(k string) (*Sheet, Rect, bool) {
	if len(k) <= len(regionPrefix) || k[:len(regionPrefix)] != nameKey(regionPrefix) {
		return nil, Rect{}, false
	}
	for _, s := range w.sheets {
		if i := s.regionIndex(k[len(regionPrefix):]); i >= 0 {
			r, ok := s.RegionTable(s.regions.list[i].Name)
			if !ok {
				return s, Rect{}, true
			}
			return s, r, true
		}
	}
	return nil, Rect{}, false
}

// regionsInUse are the regions formulas name, with their tables, for
// finding what reads a changed cell.
func (w *Workbook) regionsInUse() []namedUsers {
	var out []namedUsers
	for _, s := range w.sheets {
		for _, r := range s.regions.list {
			users := w.nameUsers[nameKey(r.FormulaName())]
			if len(users) == 0 {
				continue
			}
			if t, ok := s.RegionTable(r.Name); ok {
				out = append(out, namedUsers{s, t, users})
			}
		}
	}
	return out
}

// fitRegion makes room under the region with key k for a table of rows
// rows: inserting rows below its table, or deleting those it no longer
// needs when nothing else is on them.
func (s *Sheet) fitRegion(k string, rows int) {
	r := s.regions.list[s.regionIndex(k)]
	end := r.At.Row + r.Rows // the table's last row
	switch {
	case rows > r.Rows:
		below := rowRect(end+1, MaxRows-1)
		for range s.cells.anyKeysIn(below) {
			_ = s.insert(true, end+1, rows-r.Rows) // rows pushed past the edge: the table is cut instead
			return
		}
	case rows < r.Rows:
		from := r.At.Row + rows + 1
		spare := rowRect(from, end)
		area := s.regionArea(r)
		for a := range s.cells.anyKeysIn(spare) {
			if !area.Contains(a) {
				return // something else is on those rows
			}
		}
		s.restructure(true, spanOf(from, -(end-from+1)))
	}
}

// nextRegionRow is where a new region's label goes on a notebook: below
// the last cell and region, a row left free, or row 1 on an empty sheet.
func (s *Sheet) nextRegionRow() int {
	next := 0
	if used, ok := s.UsedRange(); ok {
		next = used.To.Row + 2
	}
	for _, r := range s.regions.list {
		next = max(next, r.At.Row+r.Rows+2)
	}
	return min(next, MaxRows-1)
}
