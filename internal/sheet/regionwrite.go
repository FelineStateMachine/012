package sheet

import (
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Writing regions' cells (see region.go): after a change that touched
// regions, each stale sheet's labels and tables are written again, only
// the cells that differ, and cells no region covers any more are
// cleared. A table that would overwrite other contents isn't shown; its
// label says where it's blocked, and clearing that cell shows it.

// regionArea is the cells a region covers: its label line and table, at
// least one column wide.
func (s *Sheet) regionArea(r Region) Rect {
	return Rect{From: r.At, To: Addr{Col: min(r.At.Col+max(r.Cols, 1)-1, MaxCols-1), Row: min(r.At.Row+r.Rows, MaxRows-1)}}
}

// RegionTable returns the cells of a region's table, header row
// included, when it has one.
func (s *Sheet) RegionTable(name string) (Rect, bool) {
	i := s.regionIndex(nameKey(name))
	if i < 0 {
		return Rect{}, false
	}
	r := s.regions.list[i]
	if r.Rows == 0 || r.Cols == 0 || r.At.Row+1 >= MaxRows {
		return Rect{}, false
	}
	area := s.regionArea(r)
	area.From.Row++
	return area, true
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
			return r, a.Row == r.At.Row, true
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

// regionTouched has the regions write their cells again after the cell
// at a is placed in their way or over their cells.
func (s *Sheet) regionTouched(a Addr) {
	for _, r := range s.regions.list {
		if s.regionArea(r).Contains(a) || s.blockedAt(nameKey(r.Name), a) {
			s.regionsStale = true
			return
		}
	}
}

// blockedAt reports whether the region with key k isn't shown because of
// the cell at a's row and columns.
func (s *Sheet) blockedAt(k string, a Addr) bool {
	me := s.regionMeta[k]
	if me == nil || me.why == "" {
		return false
	}
	i := s.regionIndex(k)
	if i < 0 {
		return false
	}
	r := s.regions.list[i]
	d := s.regions.data[k]
	if d == nil {
		return false
	}
	need := Rect{From: r.At, To: Addr{Col: r.At.Col + max(d.Cols, 1) - 1, Row: r.At.Row + d.Rows}}
	return need.Contains(a)
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

// regionCell is a cell a region writes: its value and the format it
// comes in.
type regionCell struct {
	v Value
	f Format
}

// writeRegions writes every region's cells on s and clears those no
// region covers any more, returning the cells that changed and the
// formulas naming a region whose table moved.
func (s *Sheet) writeRegions() []loc {
	var old []Rect
	for k, me := range s.regionMeta {
		if me.has {
			old = append(old, me.written)
		}
		if s.regionIndex(k) < 0 {
			delete(s.regionMeta, k)
		}
	}
	want := map[Addr]regionCell{}
	var changed []loc
	for _, r := range s.regions.list {
		changed = append(changed, s.planRegion(r, want)...)
	}
	for _, area := range old {
		changed = append(changed, s.clearRegion(area, want)...)
	}
	for a, c := range want {
		if s.writeRegionCell(a, c) {
			changed = append(changed, loc{s, a})
		}
	}
	return changed
}

// planRegion adds the cells r writes to want, recording where they are,
// and returns the formulas to recalculate because its table moved.
func (s *Sheet) planRegion(r Region, want map[Addr]regionCell) []loc {
	k := nameKey(r.Name)
	me := s.meta(k)
	d := s.regions.data[k]
	me.why = ""
	area := s.regionArea(r)
	if d != nil {
		need := Rect{From: r.At, To: Addr{Col: r.At.Col + max(d.Cols, 1) - 1, Row: r.At.Row + d.Rows}}
		if need.To.Row >= MaxRows || need.To.Col >= MaxCols {
			me.why = "can't show: it would go past the edge of the sheet"
		} else if at, ok := s.inTheWay(need, me); ok {
			me.why = "can't show: it would overwrite data in " + at.String()
		} else {
			area = need
		}
	}
	if me.why != "" {
		area = Rect{From: r.At, To: r.At} // the label says why
	}
	var moved []loc
	if !me.has || me.written != area {
		for u := range s.wb.nameUsers[nameKey(r.FormulaName())] {
			moved = append(moved, u)
		}
	}
	me.written, me.has = area, true
	if !s.writable(r.At, me) {
		return moved
	}
	want[r.At] = regionCell{v: Value{Kind: Text, Str: s.regionLabel(r)}}
	if d == nil || me.why != "" {
		return moved
	}
	order := sortedTable(d, r.Sort)
	for row := range d.Rows {
		for col := range d.Cols {
			v, f := d.At(order[row], col)
			if v.Kind != Empty {
				want[Addr{Col: r.At.Col + col, Row: r.At.Row + 1 + row}] = regionCell{v, f}
			}
		}
	}
	return moved
}

// regionLabel is what a region's label line says: its name and command,
// and what it's doing.
func (s *Sheet) regionLabel(r Region) string {
	label := r.Name + "  " + r.Command
	if st := s.RegionStatus(r.Name); st != "" {
		label += "   " + st
	}
	return label
}

// inTheWay returns a cell of need holding something the region with
// meta me didn't write.
func (s *Sheet) inTheWay(need Rect, me *regionMeta) (Addr, bool) {
	for a := range s.cells.anyKeysIn(need) {
		if s.cells.filledAt(a) && !s.regionOwns(a, me) {
			return a, true
		}
	}
	return Addr{}, false
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

// clearRegion clears the cells a region wrote in area that want doesn't
// cover.
func (s *Sheet) clearRegion(area Rect, want map[Addr]regionCell) []loc {
	var gone []Addr
	for a := range s.cells.anyKeysIn(area) {
		if _, ok := want[a]; ok || s.cells.derivedAt(a) != slotSpill {
			continue
		}
		if _, spilled := s.SpillAnchor(a); !spilled {
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

// writeRegionCell makes the cell at a show c as a region's, keeping its
// formatting and note, and reports whether it changed.
func (s *Sheet) writeRegionCell(a Addr, c regionCell) bool {
	was, l, kind := s.cells.derivedOf(a)
	if kind == slotSpill && was == c.v && l.auto == c.f {
		return false
	}
	return s.writeSpilled(a, c.v, c.f)
}

// sortedTable is the order of d's rows: the header first, then the rest
// by keys.
func sortedTable(d *RegionData, keys []SortKey) []int {
	order := make([]int, d.Rows)
	for i := range order {
		order[i] = i
	}
	if len(keys) == 0 || d.Rows < 3 {
		return order
	}
	slices.SortStableFunc(order[1:], func(i, j int) int {
		for _, k := range keys {
			a, _ := d.At(i, k.Col)
			b, _ := d.At(j, k.Col)
			if c := compareKey(a, b, k.Desc); c != 0 {
				return c
			}
		}
		return 0
	})
	return order
}

// compareKey orders two values of a sort key, blanks last either way.
func compareKey(a, b Value, desc bool) int {
	switch {
	case a.Kind == Empty && b.Kind == Empty:
		return 0
	case a.Kind == Empty:
		return 1
	case b.Kind == Empty:
		return -1
	case desc:
		return -sortCompare(a, b)
	}
	return sortCompare(a, b)
}

// shiftRegions keeps the sheet's regions in step with rows or columns
// inserted or deleted: a region moves and resizes as a range would, and
// one whose label line was deleted goes.
func (s *Sheet) shiftRegions(rows bool, sp formula.Span) {
	if len(s.regions.list) == 0 {
		return
	}
	cell, rng := formula.AxisMaps(rows, sp)
	st := s.regionsCopy()
	st.list = st.list[:0]
	for _, r := range s.regions.list {
		at, ok := cell(r.At)
		area, ok2 := rng(s.regionArea(r))
		if !ok || !ok2 {
			delete(st.data, nameKey(r.Name))
			continue
		}
		r.At, r.Rows = at, area.To.Row-at.Row
		if r.Cols > 0 {
			r.Cols = area.To.Col - at.Col + 1
		}
		st.list = append(st.list, r)
	}
	for _, me := range s.regionMeta {
		if w, ok := rng(me.written); ok && me.has {
			me.written = w
		}
	}
	if !sameRegions(st, s.regions) {
		s.putRegions(st)
	}
	s.regionsStale = true
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
