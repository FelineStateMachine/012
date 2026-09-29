package sheet

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
)

// Where regions are (see region.go): the cells they cover and their
// tables, finding them by cell and by the name formulas use, and keeping
// them in step with inserted and deleted lines.

// noRect contains no cell.
var noRect = Rect{From: Addr{Col: -1, Row: -1}, To: Addr{Col: -1, Row: -1}}

// RegionTable returns the cells of a region's table, header row
// included, when it has one.
func (s *Sheet) RegionTable(name string) (Rect, bool) {
	i := s.regionIndex(nameKey(name))
	if i < 0 {
		return Rect{}, false
	}
	r := s.regions.list[i]
	if r.File.Paged {
		t, _, ok := s.sourceTable(r)
		return t, ok
	}
	me := s.regionMeta[nameKey(r.Name)]
	if me == nil || me.why != "" || me.rows == 0 || me.cols == 0 {
		return Rect{}, false
	}
	o := r.At
	return Rect{From: o, To: Addr{Col: min(o.Col+me.cols-1, MaxCols-1), Row: min(o.Row+me.rows-1, MaxRows-1)}}, true
}

// covered is the cells a region holds: those it wrote, or while it's
// yet to write them, its anchor.
func (s *Sheet) covered(r Region) Rect {
	if me := s.regionMeta[nameKey(r.Name)]; me != nil && me.has {
		return me.written
	}
	return Rect{From: r.At, To: r.At}
}

// RegionAt returns the region covering the cell at a.
func (s *Sheet) RegionAt(a Addr) (Region, bool) {
	for _, r := range s.regions.list {
		if s.covered(r).Contains(a) {
			return r, true
		}
	}
	return Region{}, false
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
// own, and its anchor moves as a cell would; one whose anchor is deleted
// goes. Once the change ends, the regions emptied are fed again.
func (s *Sheet) shiftRegions(rows bool, sp formula.Span) {
	if len(s.regions.list) == 0 {
		return
	}
	cell, _ := formula.AxisMaps(rows, sp)
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
		for _, c := range s.emptyRegion(r, s.meta(nameKey(r.Name))) {
			s.wb.markDirty(c)
		}
		at, ok := cell(r.At)
		if !ok {
			continue
		}
		r.At = at
		st.list = append(st.list, r)
	}
	if !sameRegions(st, s.regions) {
		s.putRegions(st)
	}
	s.regionsStale = true
}

// regionName resolves a name formulas use, nu.sales, to the region's
// sheet and table.
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
				out = append(out, namedUsers{s: s, r: t, users: users, region: true})
			}
		}
	}
	return out
}

// boundRegion is a name in a formula that isn't a named range: a
// region's table (nu.r1), #REF! while it has none, or the name as
// written, which shows #NAME?.
func (s *Sheet) boundRegion(nn formula.Name) Node {
	t, r, ok := s.wb.regionName(nameKey(nn.Name))
	switch {
	case !ok:
		return nn
	case r == (Rect{}) && t.IsSource() && t.sourceErr(t.regions.list[0]) == "":
		return functions.Const(Pending) // its host is opening it
	case r == (Rect{}):
		return formula.RefErr{}
	}
	sheet := ""
	if t != s {
		sheet = t.name
	}
	const fixed = formula.AbsCol | formula.AbsRow
	return formula.Range{Rect: r, Abs: [2]formula.Abs{fixed, fixed}, Sheet: sheet}
}
