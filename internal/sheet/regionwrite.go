package sheet

// Writing regions' cells (see region.go). Every region's cells go
// through writeTable: a header and rows under it, written as spilled
// cells are, only those that differ, the region's cells it no longer
// needs cleared. Rows arrive from live operations (live.go); a command
// region's are its RegionData, written again after any change that
// touched its sheet's regions (settleRegions). A table that would
// overwrite other contents isn't shown: a command region's label, or a
// linked file's first cell, says where it's blocked, and clearing that
// cell has it shown (a linked file is read again for it).

// SetRegionStatus sets what a region's label says it's doing, such as
// "Running…", "" for nothing. It isn't an undo step.
func (s *Sheet) SetRegionStatus(name, status string) {
	k := nameKey(name)
	if s.regionIndex(k) < 0 {
		return
	}
	me := s.meta(k)
	if me.status == status {
		return
	}
	me.status = status
	s.regionsStale = true
	if s.wb.hist.open == nil {
		s.wb.settleRegions()
	}
}

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

// regionTouched notes that the cell at a is being placed: a region
// blocked by it tries again, a command region from its table, a linked
// file by being read again.
func (s *Sheet) regionTouched(a Addr) {
	for _, r := range s.regions.list {
		me := s.regionMeta[nameKey(r.Name)]
		if me == nil || me.why == "" || !me.need.Contains(a) {
			continue
		}
		if r.Linked() {
			me.stale = true
		} else {
			s.regionsStale = true
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

// writeRegions writes every command region's table on s from its data,
// empties the linked files that are to be read again, and clears the
// cells of regions gone, returning the cells that changed and the
// formulas naming a region whose table moved.
func (s *Sheet) writeRegions() []loc {
	var changed []loc
	for k, me := range s.regionMeta {
		if s.regionIndex(k) < 0 {
			changed = append(changed, s.clearOwned(me, noRect)...)
			delete(s.regionMeta, k)
		}
	}
	// Tables first leave the cells they no longer need, so regions that
	// moved past each other don't find each other in the way.
	type table struct {
		header LiveRow
		rows   []LiveRow
	}
	tables := make([]table, len(s.regions.list))
	for i, r := range s.regions.list {
		me := s.meta(nameKey(r.Name))
		switch {
		case !r.Linked():
			header, rows := s.dataRows(r)
			tables[i] = table{header, rows}
			cols, nrows := tableSize(me, header, rows, true)
			keep := s.tableArea(r, cols, nrows)
			changed = append(changed, s.clearOwned(me, keep)...)
			if was := me.written; me.has {
				me.written, me.has = intersectRect(me.written, keep) // what it holds now
				if me.written != was {
					changed = s.regionUsers(r, changed) // its table shrank
				}
			}
		case me.reread:
			me.reread = false
			changed = append(changed, s.emptyRegion(r, me)...)
		}
	}
	for i, r := range s.regions.list {
		if r.Linked() {
			continue
		}
		me := s.meta(nameKey(r.Name))
		changed = append(changed, s.writeTable(r, me, tables[i].header, tables[i].rows, true)...)
		if tables[i].header == nil && me.why == "" {
			me.written = s.regionArea(r) // not run: its place is kept
		}
	}
	return changed
}

// dataRows is a command region's table as rows, the header first, the
// rest in the region's order; nil when it hasn't run.
func (s *Sheet) dataRows(r Region) (LiveRow, []LiveRow) {
	d := s.regions.data[nameKey(r.Name)]
	if d == nil || d.Rows == 0 {
		return nil, nil
	}
	order := sortedTable(d, r.Sort)
	row := func(i int) LiveRow {
		out := make(LiveRow, d.Cols)
		for c := range d.Cols {
			v, f := d.At(order[i], c)
			out[c] = LiveCell{V: v, F: f}
		}
		return out
	}
	rows := make([]LiveRow, d.Rows-1)
	for i := range rows {
		rows[i] = row(i + 1)
	}
	return row(0), rows
}

// emptyRegion clears a region's cells, outside the undo history: a
// linked file's, to be read again whole, which it is marked stale for.
func (s *Sheet) emptyRegion(r Region, me *regionMeta) []loc {
	changed := s.clearOwned(me, noRect)
	me.has, me.rows, me.cols, me.why = false, 0, 0, ""
	if r.Linked() {
		me.data, me.stale = 0, true
	}
	return changed
}

// writeTable makes the region r show header and rows under it: with
// reset, rows replace every row; otherwise they follow those shown, and
// nil rows show already. It writes only the cells that differ and
// clears the region's cells it no longer needs; when a cell it needs
// holds something else, it shows only why. It returns the cells that
// changed, with the formulas naming the region when its table moved.
func (s *Sheet) writeTable(r Region, me *regionMeta, header LiveRow, rows []LiveRow, reset bool) []loc {
	cols, nrows := tableSize(me, header, rows, reset)
	was, had := me.written, me.has
	need := s.tableArea(r, cols, nrows)
	me.why = ""
	switch {
	case need.To.Row >= MaxRows || need.To.Col >= MaxCols:
		me.why = "can't show: it would go past the edge of the sheet"
	default:
		if at, ok := s.inTheWay(need, me); ok {
			me.why = "can't show: it would overwrite data in " + at.String()
		}
	}
	var changed []loc
	if me.why != "" {
		me.need, me.rows, me.cols = need, 0, 0
		need, header, rows, cols, nrows = Rect{From: r.At, To: r.At}, nil, nil, 0, 0
	}
	changed = append(changed, s.clearOwned(me, need)...)
	me.written, me.has = need, true
	if r.labelled() {
		changed = s.writeRegionCell(r.At, LiveCell{V: Value{Kind: Text, Str: s.regionLabel(r)}}, me, changed)
		changed = s.clearBesideLabel(r, need, me, changed)
	} else if me.why != "" || nrows == 0 && me.err != "" {
		changed = s.writeRegionCell(r.At, LiveCell{V: ErrRef}, me, changed)
	}
	o := tableOrigin(r)
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
	if r.Linked() && !me.fitted && nrows > 1 {
		me.fitted = true
		s.fitTable(o, nrows, cols)
	}
	for _, c := range changed {
		s.freedFor(c.a)
	}
	if !had || was != me.written {
		changed = s.regionUsers(r, changed)
	}
	return changed
}

// clearBesideLabel clears the cells of the label line, beside the label,
// that the region wrote as part of its table before it moved down onto
// them (undone and redone, a region may find its old rows there).
func (s *Sheet) clearBesideLabel(r Region, need Rect, me *regionMeta, changed []loc) []loc {
	line := Rect{From: Addr{Col: r.At.Col + 1, Row: r.At.Row}, To: Addr{Col: need.To.Col, Row: r.At.Row}}
	if line.To.Col < line.From.Col {
		return changed
	}
	var gone []Addr
	for a := range s.cells.anyKeysIn(line) {
		if s.cells.filledAt(a) && s.regionOwns(a, me) {
			gone = append(gone, a)
		}
	}
	for _, a := range gone {
		s.setDerived(a, s.cells.get(a).leftover())
		changed = append(changed, loc{s, a})
	}
	return changed
}

// regionUsers appends the formulas naming r to changed, to recalculate
// when its table moves or changes size.
func (s *Sheet) regionUsers(r Region, changed []loc) []loc {
	for u := range s.wb.nameUsers[nameKey(r.FormulaName())] {
		changed = append(changed, u)
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

// tableArea is the cells r takes to show a table of cols by nrows: its
// label line and the table, or its anchor alone for an empty linked
// file.
func (s *Sheet) tableArea(r Region, cols, nrows int) Rect {
	o := tableOrigin(r)
	end := o.Row + nrows - 1
	if nrows == 0 {
		end = r.At.Row
	}
	return Rect{From: r.At, To: Addr{Col: o.Col + max(cols, 1) - 1, Row: end}}
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

// regionLabel is what a command region's label line says: its name and
// command, and what it's doing.
func (s *Sheet) regionLabel(r Region) string {
	label := r.Name + "  " + r.Command
	if st := s.RegionStatus(r.Name); st != "" {
		label += "   " + st
	}
	return label
}

// inTheWay returns a cell of need holding something the region with
// meta me didn't write. Only what the region didn't hold is looked at:
// nothing else can be placed in a region's cells.
func (s *Sheet) inTheWay(need Rect, me *regionMeta) (Addr, bool) {
	for _, part := range fresh(need, me) {
		for a := range s.cells.anyKeysIn(part) {
			if s.cells.filledAt(a) && !s.regionOwns(a, me) {
				return a, true
			}
		}
	}
	return Addr{}, false
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
	return !s.cells.filledAt(a) || s.regionOwns(a, me)
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
	return changed
}
