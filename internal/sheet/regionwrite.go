package sheet

// Writing regions' cells (see region.go). Every region's cells go
// through writeTable: a header and rows under it, written as spilled
// cells are, only those that differ, the region's cells it no longer
// needs cleared. Rows arrive from live operations (live.go). A table
// that would overwrite other contents isn't shown: the region's first
// cell says where it's blocked, and clearing that cell has its source
// send it again.

// meta is the region with key k's meta, made when missing.
func (s *Sheet) meta(k string) *regionMeta {
	if s.regionMeta == nil {
		s.regionMeta = map[string]*regionMeta{}
	}
	me := s.regionMeta[k]
	if me == nil {
		me = &regionMeta{}
		s.regionMeta[k] = me
	}
	return me
}

// regionTouched notes that c is being placed at a: a region blocked by
// it tries again, and one whose table it lands in (a blank cell of it,
// pasted over) finds it in its way, its source sending its rows again
// either way.
func (s *Sheet) regionTouched(a Addr, c *Cell) {
	for _, r := range s.regions.list {
		me := s.regionMeta[nameKey(r.Name)]
		switch {
		case me == nil:
		case me.why != "" && me.need.Contains(a):
			me.stale = true
		case me.why == "" && me.has && me.written.Contains(a) && !c.Blank():
			me.stale, me.placed = true, true
		}
	}
}

// ownerOf is the region whose cell the cell at a is, and its meta.
func (s *Sheet) ownerOf(a Addr) (Region, *regionMeta, bool) {
	for _, r := range s.regions.list {
		if me := s.regionMeta[nameKey(r.Name)]; me != nil && s.regionOwns(a, me) {
			return r, me, true
		}
	}
	return Region{}, nil, false
}

// settleRegions writes the cells of the sheets whose regions changed,
// then recalculates what reads them.
func (w *Workbook) settleRegions() {
	if w.settling {
		return
	}
	w.settling = true
	defer func() { w.settling = false }()
	var changed []loc
	for _, s := range w.sheets {
		if s.regionsStale {
			s.regionsStale = false
			changed = append(changed, s.writeRegions()...)
		}
	}
	if len(changed) > 0 {
		w.recalc(changed)
	}
}

// writeRegions empties the regions that are to be fed again and clears
// the cells of regions gone, returning the cells that changed and the
// formulas naming regions gone.
func (s *Sheet) writeRegions() []loc {
	var changed []loc
	for k, me := range s.regionMeta {
		if s.regionIndex(k) < 0 {
			changed = append(changed, s.clearOwned(me, noRect)...)
			if t, ok := me.table(); ok {
				changed = append(changed, s.regionMoved(t, noRect, true)...)
			}
			delete(s.regionMeta, k)
			s.wakeAllRegions()
			for u := range s.wb.nameUsers[nameKey(regionPrefix+k)] {
				changed = append(changed, u) // the name reads nothing now
			}
		}
	}
	for _, r := range s.regions.list {
		if me := s.meta(nameKey(r.Name)); me.reread && !r.File.Paged {
			me.reread = false
			changed = append(changed, s.emptyRegion(r, me)...)
		}
	}
	return changed
}

// emptyRegion clears the region r's cells, outside the undo history,
// to be fed again whole, which it is marked stale for. The formulas
// naming it read #REF! until then, so they're among the cells returned.
func (s *Sheet) emptyRegion(r Region, me *regionMeta) []loc {
	changed := s.clearOwned(me, noRect)
	if t, ok := me.table(); ok {
		changed = append(changed, s.regionMoved(t, noRect, true)...)
	}
	if me.has {
		changed = s.regionReaders(r, changed)
	}
	me.has, me.rows, me.cols, me.why = false, 0, 0, ""
	me.data, me.stale = 0, true
	return changed
}

// writeTable makes the region r show header and rows under it: with
// reset, rows replace every row; otherwise they follow those shown, and
// nil rows show already. It writes only the cells that differ and
// clears the region's cells it no longer needs; when a cell it needs
// holds something else, it shows only why. It returns the cells that
// changed, with the formulas naming the region when its table moved or
// changed shape.
func (s *Sheet) writeTable(r Region, me *regionMeta, header LiveRow, rows []LiveRow, reset bool) []loc {
	cols, nrows := tableSize(me, header, rows, reset)
	was, had, shape := me.written, me.has, [2]int{me.rows, me.cols}
	before, _ := me.table()
	need := s.tableArea(r, cols, nrows)
	changed := s.makeRoomAt(r)
	whole := me.why != "" || me.placed // it may not hold every cell it wrote
	me.why, me.placed = "", false
	switch {
	case need.To.Row >= MaxRows || need.To.Col >= MaxCols:
		me.why = "can't show: it would go past the edge of the sheet"
	default:
		at, blocked, later := s.inTheWay(r, need, me, whole)
		if blocked {
			me.why = "can't show: it would overwrite data in " + at.String()
			break
		}
		for _, q := range later {
			changed = append(changed, s.giveWay(q, need)...)
		}
	}
	if me.why != "" {
		me.need, me.rows, me.cols = need, 0, 0
		need, header, rows, cols, nrows = Rect{From: r.At, To: r.At}, nil, nil, 0, 0
	}
	changed = append(changed, s.clearOwned(me, need)...)
	me.written, me.has = need, true
	if me.why != "" || nrows == 0 && me.err != "" {
		changed = s.writeRegionCell(r.At, LiveCell{V: ErrRef}, me, changed)
	}
	o := r.At
	if cols > 0 {
		changed = s.writeRow(o, header, cols, true, me, changed)
	}
	for i, row := range rows {
		// A row written over one that showed clears what it doesn't
		// fill; one below the rows shown has nothing to clear.
		if row != nil {
			changed = s.writeRow(Addr{Col: o.Col, Row: o.Row + 1 + i}, row, cols, i+1 < me.rows || reset, me, changed)
		}
	}
	me.rows, me.cols = nrows, cols
	if !me.fitted && nrows > 1 {
		me.fitted = true
		s.fitTable(o, nrows, cols)
	}
	for _, c := range changed {
		s.freedFor(c.a)
	}
	// Blocked, what's in its way may have gone but for arrays, which then
	// find it (heldByArrays).
	if after, _ := me.table(); !had || after != before || me.why != "" {
		changed = append(changed, s.regionMoved(before, after, had)...)
	}
	if !had || was != me.written || shape != [2]int{me.rows, me.cols} { // moved, or its table changed shape
		changed = s.regionReaders(r, changed)
	}
	return changed
}

// regionReaders adds the formulas naming the region r to changed.
func (s *Sheet) regionReaders(r Region, changed []loc) []loc {
	for _, k := range []string{nameKey(r.FormulaName()), nameKey(r.Name)} {
		for u := range s.wb.nameUsers[k] {
			changed = append(changed, u)
		}
	}
	return changed
}

// tableSize is how many columns and rows, header included, a table
// takes to show header and rows: none when there's nothing to show.
func tableSize(me *regionMeta, header LiveRow, rows []LiveRow, reset bool) (cols, nrows int) {
	cols = len(header)
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if !reset {
		cols = max(cols, me.cols)
	}
	if cols > 0 {
		nrows = 1 + len(rows)
	}
	return cols, nrows
}

// tableArea is the cells r takes to show a table of cols by nrows, or
// its anchor alone for an empty one.
func (s *Sheet) tableArea(r Region, cols, nrows int) Rect {
	end := r.At.Row + max(nrows, 1) - 1
	return Rect{From: r.At, To: Addr{Col: r.At.Col + max(cols, 1) - 1, Row: end}}
}

// writeRow writes a table's row at at, cols cells wide, adding the cells
// that changed to changed. With all, the cells the row leaves blank are
// cleared; otherwise they're blank already.
func (s *Sheet) writeRow(at Addr, row LiveRow, cols int, all bool, me *regionMeta, changed []loc) []loc {
	for c := range cols {
		var lc LiveCell
		if c < len(row) {
			lc = row[c]
		}
		if lc.V.Kind == Empty && !all {
			continue
		}
		changed = s.writeRegionCell(Addr{Col: at.Col + c, Row: at.Row}, lc, me, changed)
	}
	return changed
}

// writeRegionCell makes the cell at a show c as the region's, keeping
// its formatting and note, when it's free for the region to write,
// adding it to changed if it changed.
func (s *Sheet) writeRegionCell(a Addr, c LiveCell, me *regionMeta, changed []loc) []loc {
	if !s.writable(a, me) {
		return changed
	}
	if s.writeSpilled(a, c.V, c.F) {
		changed = append(changed, loc{s, a})
	}
	return changed
}

// regionCell reports whether a region holds the cell at a.
func (s *Sheet) regionCell(a Addr) bool {
	if len(s.regions.list) == 0 {
		return false
	}
	_, _, ok := s.ownerOf(a)
	return ok
}

// fresh is the parts of need the region with meta me doesn't hold: all
// of it, or when need grows from what it holds, the strips to its right
// and below, so rows arriving cost what they add.
func fresh(need Rect, me *regionMeta) []Rect {
	w := me.written
	if !me.has || w.From != need.From {
		return []Rect{need}
	}
	var out []Rect
	if need.To.Col > w.To.Col {
		out = append(out, Rect{From: Addr{Col: w.To.Col + 1, Row: need.From.Row}, To: need.To})
	}
	if need.To.Row > w.To.Row {
		out = append(out, Rect{From: Addr{Col: need.From.Col, Row: w.To.Row + 1}, To: Addr{Col: min(need.To.Col, w.To.Col), Row: need.To.Row}})
	}
	return out
}

// writable reports whether the cell at a is free for the region with
// meta me to write.
func (s *Sheet) writable(a Addr, me *regionMeta) bool {
	if s.regionOwns(a, me) {
		return true
	}
	_, spilled := s.SpillAnchor(a) // an array's blank cell is its own
	return !spilled && !s.cells.filledAt(a)
}

// regionsGiveWay empties the regions whose tables are in area, where an
// array spills: an array spills over a region's table, whichever came
// first, as it does when the file opens and arrays spill before regions
// are sent their rows. Sent again, a region finds the array in its way.
func (s *Sheet) regionsGiveWay(area Rect) []loc {
	var changed []loc
	for _, r := range s.regions.list {
		if me := s.regionMeta[nameKey(r.Name)]; me != nil && me.has && overlaps(me.written, area) {
			changed = append(changed, s.emptyRegion(r, me)...)
		}
	}
	return changed
}

// regionOwns reports whether the cell at a is one the region with meta
// me wrote: a derived cell of what it wrote last that no formula
// spilled.
func (s *Sheet) regionOwns(a Addr, me *regionMeta) bool {
	if !me.has || !me.written.Contains(a) || s.cells.derivedAt(a) != slotSpill {
		return false
	}
	_, spilled := s.SpillAnchor(a)
	return !spilled
}

// clearOwned clears the cells the region with meta me wrote outside
// keep, turning them back into what they were: blank, or formatting
// only.
func (s *Sheet) clearOwned(me *regionMeta, keep Rect) []loc {
	if !me.has || keep.Contains(me.written.From) && keep.Contains(me.written.To) {
		return nil
	}
	var gone []Addr
	for a := range s.cells.anyKeysIn(me.written) {
		if !keep.Contains(a) && s.regionOwns(a, me) {
			gone = append(gone, a)
		}
	}
	changed := make([]loc, len(gone))
	for i, a := range gone {
		s.setDerived(a, s.cells.get(a).leftover())
		changed[i] = loc{s, a}
	}
	s.wakeRegions(me, gone)
	return changed
}
