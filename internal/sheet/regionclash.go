package sheet

// Regions in each other's way (see region.go). What a region shows
// can't depend on the order their rows arrived in, or a file would open
// differently from how it was saved, its regions sent their rows in
// another order. So, as for arrays (spill.go):
//
//   - a region's anchor is in the way of every other region, whether
//     it shows a table, why it can't, or nothing yet, as an array's
//     formula is in the way of other arrays;
//   - when two tables need the same cells, the region whose anchor
//     comes first, row by row, gets them, whichever arrived first: the
//     other gives way, showing why at its anchor.
//
// A region blocked by another tries again, its source sending its rows,
// when a cell it needs is freed, or a region on its sheet goes.

// inTheWay returns a cell of need, the cells the region r with meta me
// needs, holding something r didn't write and that keeps it from
// showing, and the regions whose tables are to give way to it. Unless
// whole is set, only what r didn't hold is looked at: nothing else is
// placed in a region's cells without it knowing (regionTouched).
func (s *Sheet) inTheWay(r Region, need Rect, me *regionMeta, whole bool) (at Addr, blocked bool, later []Region) {
	// An array's cells are in the way even where it spilled blanks, as
	// another array's are (spill.go).
	for a, sp := range s.spills {
		if x, ok := intersectRect(sp.area, need); ok && sp.why == "" {
			if x.From != a {
				return x.From, true, nil
			}
			return a, true, nil
		}
	}
	for _, q := range s.regions.list {
		if q.Name != r.Name && need.Contains(q.At) {
			return q.At, true, nil
		}
	}
	parts := []Rect{need}
	if !whole {
		parts = fresh(need, me)
	}
	for _, part := range parts {
		if at, blocked = s.filledIn(part, r, me, &later); blocked {
			return at, true, nil
		}
	}
	return Addr{}, false, later
}

// filledIn returns a cell of part holding something the region r with
// meta me didn't write that keeps it from showing, adding to later the
// regions whose cells there come after r's anchor.
func (s *Sheet) filledIn(part Rect, r Region, me *regionMeta, later *[]Region) (Addr, bool) {
	for a := range s.cells.anyKeysIn(part) {
		if !s.cells.filledAt(a) || s.regionOwns(a, me) {
			continue
		}
		q, _, ok := s.ownerOf(a)
		if !ok || before(q.At, r.At) {
			return a, true
		}
		if !regionListed(*later, q) {
			*later = append(*later, q)
		}
	}
	return Addr{}, false
}

// before reports whether the cell at a comes before the cell at b, row
// by row.
func before(a, b Addr) bool { return a.Row < b.Row || a.Row == b.Row && a.Col < b.Col }

func regionListed(list []Region, r Region) bool {
	for _, x := range list {
		if x.Name == r.Name {
			return true
		}
	}
	return false
}

// makeRoomAt has the regions whose tables hold r's anchor give way to
// it, and returns the cells that changed.
func (s *Sheet) makeRoomAt(r Region) []loc {
	var changed []loc
	for _, q := range s.regions.list {
		if me := s.regionMeta[nameKey(q.Name)]; q.Name != r.Name && me != nil && me.why == "" && s.regionOwns(r.At, me) {
			changed = append(changed, s.giveWay(q, Rect{From: r.At, To: r.At})...)
		}
	}
	return changed
}

// giveWay has the region q, showing a table, show why it can't instead:
// another region needs cells of it in taken. It returns the cells that
// changed, with the formulas naming q.
func (s *Sheet) giveWay(q Region, taken Rect) []loc {
	me := s.meta(nameKey(q.Name))
	at, _ := intersectRect(me.written, taken)
	need, anchor := me.written, Rect{From: q.At, To: q.At}
	changed := s.clearOwned(me, anchor)
	me.why = "can't show: it would overwrite data in " + at.From.String()
	me.need, me.written, me.rows, me.cols = need, anchor, 0, 0
	changed = s.writeRegionCell(q.At, LiveCell{V: ErrRef}, me, changed)
	return s.regionReaders(q, changed)
}

// wakeRegions marks the regions other than me blocked from showing by a
// cell of gone, which me has just cleared, to be sent again.
func (s *Sheet) wakeRegions(me *regionMeta, gone []Addr) {
	for _, other := range s.regionMeta {
		if other == me || other.why == "" {
			continue
		}
		for _, a := range gone {
			if other.need.Contains(a) {
				other.stale = true
				break
			}
		}
	}
}

// wakeAllRegions marks every region of the sheet blocked from showing
// to be sent again, as when a region goes: its anchor, which may have
// blocked them, goes with it.
func (s *Sheet) wakeAllRegions() {
	for _, me := range s.regionMeta {
		if me.why != "" {
			me.stale = true
		}
	}
}

// table is the cells the region with meta me takes: its table shown or,
// blocked, the one it needs; none before its rows first arrive.
func (me *regionMeta) table() (Rect, bool) {
	switch {
	case me == nil || !me.has:
		return Rect{}, false
	case me.why != "":
		return me.need, true
	}
	return me.written, true
}

// regionMoved returns the anchors of the arrays over the region's
// table, before and after it changed, spilling or blocked by a cycle,
// to compute again: an array reading the table's cells is blocked by
// it (spillsIntoItself) wherever the table goes, which the cells it
// reads may not tell it.
func (s *Sheet) regionMoved(before, after Rect, had bool) []loc {
	var out []loc
	for b, sp := range s.spills {
		if (sp.why == "" || sp.circular) && (had && overlaps(sp.area, before) || overlaps(sp.area, after)) {
			sp.stale = true
			out = append(out, loc{s, b})
		}
	}
	return out
}

// heldByArrays is the region r's table, when an array spilling over it
// decides whether it shows: shown, or blocked by arrays alone, which it
// would show without.
func (s *Sheet) heldByArrays(r Region) (Rect, bool) {
	me := s.regionMeta[nameKey(r.Name)]
	t, ok := me.table()
	if !ok || me.why == "" {
		return t, ok
	}
	if t.To.Row >= MaxRows || t.To.Col >= MaxCols {
		return Rect{}, false
	}
	for a := range s.cells.anyKeysIn(t) {
		if a == r.At || !s.cells.filledAt(a) {
			continue
		}
		if _, spilled := s.SpillAnchor(a); !spilled || s.cells.derivedAt(a) != slotSpill {
			return Rect{}, false
		}
	}
	for _, q := range s.regions.list {
		if q.Name != r.Name && t.Contains(q.At) {
			return Rect{}, false
		}
	}
	return t, true
}
