package sheet

import "slices"

// restore puts back the before-images a step holds and returns the step
// that would put back what they replaced, keeping st's ID (on the redo
// stack an ID names the state the step leads back to), and the cells
// whose values may have changed. Undo and redo swap steps with it; a
// change taken back inside another's step (Try) restores its own.
func (w *Workbook) restore(st *step) (inv *step, changed []loc) {
	inv = newStep(st.label, st.sheet, st.focus)
	inv.id, inv.shifts = st.id, st.shifts
	// The sheet list goes first, so cells land on attached sheets.
	if st.sheets != nil {
		inv.sheets = w.sheetList()
		w.setSheets(st.sheets)
	}
	for s, r := range st.regions {
		changed = append(changed, s.emptyMoved(r)...)
	}
	for s, img := range st.cells {
		changed = append(changed, s.emptyUnder(img)...)
	}
	for s, img := range st.cells {
		img.each(func(a Addr, c *Cell) {
			inv.keep(s, a)
			s.place(a, c)
			changed = append(changed, loc{s, a})
		})
	}
	for k, width := range st.widths {
		inv.widths[k] = k.s.widths[k.col]
		k.s.setWidth(k.col, width)
	}
	for k, h := range st.heights {
		inv.keepHeight(k, k.s.heights[k.row])
		k.s.setHeight(k.row, h)
	}
	for k, l := range st.lines {
		inv.lines[k] = k.s.line(k.row, k.n)
		k.s.setLine(k.row, k.n, l)
		changed = append(changed, k.s.lineChanged(k.row, k.n)...)
	}
	for k, n := range st.names {
		inv.names[k] = w.namePtr(k)
		changed = append(changed, w.putName(k, n)...)
	}
	changed = append(changed, w.restoreSheetParts(st, inv)...)
	return inv, changed
}

// restoreSheetParts is restore's part for what a sheet keeps whole: its
// charts, pivot, rules and view, and the workbook's settings and macros.
// It returns the formulas reading the tables it changed.
func (w *Workbook) restoreSheetParts(st, inv *step) (changed []loc) {
	for s, charts := range st.charts {
		inv.charts[s] = s.charts
		s.charts = slices.Clone(charts)
	}
	for s, p := range st.pivots {
		inv.pivots[s] = s.pivot.def
		s.pivot.def, s.pivot.stale = p, true
	}
	for s, r := range st.regions {
		inv.regions[s] = s.regions
		s.putRegionsBack(r)
	}
	for s, r := range st.rules {
		inv.rules[s] = s.rules
		s.rules = r
		s.looks.reset()
	}
	if st.settings != nil {
		cur := w.settings
		inv.settings, w.settings = &cur, *st.settings
		// Every formula computes differently in the other arithmetic.
		w.structural = w.structural || cur.decimal != w.decimal
	}
	if st.macros != nil {
		cur := w.macros
		inv.macros, w.macros = &cur, slices.Clone(*st.macros)
	}
	for s, v := range st.views {
		cur := s.view
		inv.views[s] = &cur
		changed = append(changed, w.tableUsers(cur.tables, v.tables)...)
		s.view = *v
		s.hidden.valid = false
		if !slices.Equal(cur.merges, v.merges) {
			s.version++
			s.respill(Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}})
		}
	}
	return changed
}

// Try runs fn, a change, so that it can be taken back whole: after fn
// and a recalculation, takeBack says whether to. Outside any step, fn's
// change is a step of its own and taking it back discards it, as
// Discard does; inside a step another change opened (a macro's run),
// what fn changed is put back within that step, which goes on. It
// reports whether fn's change stayed, and fn's error. A paste or fill
// checked against validation after the fact runs through it.
func (w *Workbook) Try(fn func() error, takeBack func() bool) (bool, error) {
	h := &w.hist
	if h.open == nil {
		before := w.StateID()
		err := fn()
		if err == nil && w.StateID() != before && takeBack() {
			w.Discard()
			return false, nil
		}
		return true, err
	}
	outer := h.open
	h.open = newStep(outer.label, outer.sheet, outer.focus)
	err := fn()
	child := h.open
	h.open = outer
	w.Settle()
	if err == nil && takeBack() {
		_, changed := w.restore(child)
		w.recalcSwapped(changed)
		return false, nil
	}
	outer.join(child)
	return true, err
}

// join adds to st the before-images of child, a step taken inside it,
// that st doesn't hold: st began earlier, so its own come first.
func (st *step) join(child *step) {
	for s, img := range child.cells {
		img.each(func(a Addr, c *Cell) {
			mine := st.cells[s]
			if mine == nil {
				mine = &image{}
				st.cells[s] = mine
				st.cellBytes += imageBytes
			}
			switch {
			case mine.seen(a):
			case c == nil:
				st.cellBytes += mine.put(a, slot{}, true)
			default:
				st.cellBytes += mine.keepRich(a, c)
			}
		})
	}
	joinMap(st.widths, child.widths)
	if child.heights != nil {
		if st.heights == nil {
			st.heights = map[rowKey]int{}
		}
		joinMap(st.heights, child.heights)
	}
	joinMap(st.lines, child.lines)
	joinMap(st.names, child.names)
	joinMap(st.views, child.views)
	joinMap(st.charts, child.charts)
	joinMap(st.pivots, child.pivots)
	joinMap(st.rules, child.rules)
	joinMap(st.regions, child.regions)
	st.sheets = cmpOrPtr(st.sheets, child.sheets)
	st.settings = cmpOrPtr(st.settings, child.settings)
	st.macros = cmpOrPtr(st.macros, child.macros)
}

// joinMap adds child's entries to st where st has none.
func joinMap[K comparable, V any](st, child map[K]V) {
	for k, v := range child {
		if _, ok := st[k]; !ok {
			st[k] = v
		}
	}
}

// cmpOrPtr is a, or b when a is nil.
func cmpOrPtr[T any](a, b *T) *T {
	if a != nil {
		return a
	}
	return b
}
