package sheet

import "slices"

// spillsIntoItself finds whether the array at a would spill over a cell
// its formula reads, other than a itself, directly or through other
// formulas and the arrays they spill: spilling would change what
// computed it, a circular dependency, whose values would depend on the
// order cells were computed in. It returns why, or "", and the other
// arrays on the way, which are part of the cycle and give way too.
//
// It walks the formulas that read the area's cells, and those that read
// them or what they spill, looking for the anchor: the cells a
// recalculation after the spill would visit anyway. An array blocked by
// a cycle counts as spilling over the cells it needs, so the arrays of
// a cycle stay blocked together whichever was computed first.
//
// A region whose table is in area gives way to the array
// (regionsGiveWay), so what reads it by name reads #REF! once the array
// spills, whatever the array holds: the walk doesn't follow it, or the
// array would be blocked or not by whether the region's rows arrived
// before it spilled. A formula reading the table's cells, though, leads
// the walk from each of them, those outside area too: spilling empties
// them, the anchor showing #REF!, which may make the array smaller and
// free the region, spilling again once it's sent its rows, without
// end. The table is the one shown or, blocked by arrays alone, the one
// it needs, so the array is blocked either way (regionMoved has it
// checked again).
func (s *Sheet) spillsIntoItself(a Addr, area Rect) (string, []loc) {
	if area.From == area.To {
		return "", nil
	}
	look := s.wb.readerLookup()
	look.named = slices.DeleteFunc(look.named, func(nu namedUsers) bool { return nu.region && nu.s == s && overlaps(nu.r, area) })
	w := &spillWalk{look: look, target: loc{s, a}, seen: map[loc]bool{{s, a}: true}, parent: -1}
	if w.cells(s, area, a, loc{}, true) {
		return circular(w.origin), nil
	}
	for _, r := range s.regions.list {
		if t, ok := s.heldByArrays(r); ok && overlaps(t, area) && w.cells(s, t, a, loc{}, true) {
			return "Circular dependency: the array would spill over " + r.Name + "'s table, whose cells its formula reads", nil
		}
	}
	for i := 0; i < len(w.queue); i++ {
		u := w.queue[i]
		w.origin, w.parent = w.nodes[i].from, i
		// An array's cells first, so a cycle through them is found as
		// one, and the array blocked with it, rather than through the
		// anchor alone.
		if sp := u.s.spills[u.a]; sp != nil && (sp.why == "" || sp.circular) {
			w.cells(u.s, sp.area, u.a, u, false)
		}
		if !w.found {
			w.look.each(u, w.visitVia(loc{}))
		}
		if w.found {
			return circular(w.origin), w.arrays()
		}
	}
	return "", nil
}

// spillCycle is spillsIntoItself for the array at a, whose spill was
// old, holding it blocked once a cycle has blocked it twice in the
// recalculation, freed between. Arrays whose sizes follow what they read
// can go around a cycle without settling: blocked, one shows #REF!,
// which the other reads, and its smaller array frees the first, which
// spills and blocks it again. Held, they end blocked together, as they
// are when the cycle is found in one go, whichever came first.
func (s *Sheet) spillCycle(a Addr, area Rect, old *spill) (string, []loc) {
	l, w := loc{s, a}, s.wb
	why, cycle := s.spillsIntoItself(a, area)
	blocked := old != nil && old.circular
	checked := w.checkedCycle(l)
	switch {
	case why != "" && (!blocked || !checked):
		w.noteCycle(l)
	case why == "" && blocked && w.spillCycles[l] >= 2:
		why = old.why
	}
	return why, cycle
}

// checkedCycle reports whether the array at l was checked for a cycle
// already in the recalculation, and notes that it has been.
func (w *Workbook) checkedCycle(l loc) bool {
	if _, ok := w.spillCycles[l]; ok {
		return true
	}
	if w.spillCycles == nil {
		w.spillCycles = map[loc]int{}
	}
	w.spillCycles[l] = 0
	return false
}

// cycleMoved reports whether the arrays are to be checked again, the
// array whose spill was old found blocked by a cycle (circ) or not: a
// cycle through arrays began or ended at it, which may take in or let go
// of others, or it is blocked still, the first time in the
// recalculation: something changed on the way around its cycle, as a
// formula typed reading another array's cells, which may have closed a
// cycle through another array, which isn't computed again for it.
func (w *Workbook) cycleMoved(circ bool, old *spill) bool {
	switch was := old != nil && old.circular; {
	case circ != was:
		return true
	case circ && !w.spillRechecked:
		w.spillRechecked = true
		return true
	}
	return false
}

// noteCycle counts the array at l found blocked by a cycle, from spilling
// or not, in the recalculation.
func (w *Workbook) noteCycle(l loc) {
	if w.spillCycles == nil {
		w.spillCycles = map[loc]int{}
	}
	w.spillCycles[l]++
}

// spillWalk is the state of spillsIntoItself: the formulas reached, in
// the order found, each with how it was reached.
type spillWalk struct {
	look   readerLookup
	target loc
	seen   map[loc]bool
	queue  []loc
	nodes  []walkNode
	origin Addr // the cell of the area the walk is on
	parent int  // the node the walk is on, -1 at the start
	via    loc  // the array whose cells the walk is on, if any
	found  bool
	last   int // the node the target was reached from
}

// walkNode is how the walk reached a formula: from which cell of the
// area, from which node, and through the cells of which array.
type walkNode struct {
	from   Addr
	parent int
	via    loc
}

// visitVia returns what the walk does with each formula it reaches
// through the cells of the array via (none when zero).
func (w *spillWalk) visitVia(via loc) func(loc) {
	w.via = via
	return w.visit
}

func (w *spillWalk) visit(u loc) {
	switch {
	case w.found:
	case u == w.target:
		w.found, w.last = true, len(w.nodes)
		w.nodes = append(w.nodes, walkNode{w.origin, w.parent, w.via})
	case !w.seen[u]:
		w.seen[u] = true
		w.queue = append(w.queue, u)
		w.nodes = append(w.nodes, walkNode{w.origin, w.parent, w.via})
	}
}

// cells visits the formulas reading the cells of area on s other than
// skip, the cells of the array via, and reports whether one leads to
// the target. At the start, each cell is its own origin.
func (w *spillWalk) cells(s *Sheet, area Rect, skip Addr, via loc, start bool) bool {
	visit := w.visitVia(via)
	for row := area.From.Row; row <= area.To.Row; row++ {
		for col := area.From.Col; col <= area.To.Col; col++ {
			at := Addr{Col: col, Row: row}
			if at == skip {
				continue
			}
			if start {
				w.origin = at
			}
			if w.look.each(loc{s, at}, visit); w.found {
				return true
			}
		}
	}
	return false
}

// arrays are the arrays whose cells the path to the target went
// through.
func (w *spillWalk) arrays() []loc {
	var out []loc
	for i := w.last; i >= 0; i = w.nodes[i].parent {
		if v := w.nodes[i].via; v.s != nil {
			out = append(out, v)
		}
	}
	return out
}

func circular(at Addr) string {
	return "Circular dependency: the array would spill into " + at.String() + ", which its formula reads"
}

// recheckArrays returns every array's anchor but l's, spilling or
// blocked by a cycle, to compute again: a cycle through arrays that
// begins or ends at l may take in or let go of others, which checking
// each again finds, whichever order they're in.
func (w *Workbook) recheckArrays(l loc) []loc {
	var out []loc
	for _, s := range w.sheets {
		for b, sp := range s.spills {
			if (loc{s, b}) != l && (sp.why == "" || sp.circular) {
				sp.stale = true
				out = append(out, loc{s, b})
			}
		}
	}
	return out
}

// recheckCircular returns the anchors of the arrays blocked by a cycle,
// to compute again, when an array stops spilling or spills over other
// cells: the cycle may have gone through its cells, and the arrays
// found it blocked in the same pass, before it changed, which nothing
// they read may tell them.
func (w *Workbook) recheckCircular() []loc {
	if w.circArrays <= 0 {
		return nil
	}
	var out []loc
	for _, s := range w.sheets {
		for b, sp := range s.spills {
			if sp.circular {
				sp.stale = true
				out = append(out, loc{s, b})
			}
		}
	}
	return out
}

// forgetSpill drops the anchor at a's spill for good, its formula
// computing one value or none, returning the cells that changed. When a
// cycle blocked it, it counted as spilling over the cells it needed, so
// the arrays blocked by a cycle are checked again.
func (s *Sheet) forgetSpill(a Addr) []loc {
	old := s.spills[a]
	changed := s.dropSpill(a)
	if old != nil && old.circular {
		changed = append(changed, s.wb.recheckCircular()...)
	}
	return changed
}

// blockCircular blocks the arrays at l, part of a cycle with another,
// and returns the cells that changed, with the anchors, to compute
// again, finding the cycle themselves.
func blockCircular(arrays []loc) []loc {
	var changed []loc
	for _, l := range arrays {
		sp := l.s.spills[l.a]
		if sp == nil || sp.circular {
			continue
		}
		area := sp.area
		changed = append(changed, l.s.dropSpill(l.a)...)
		l.s.setSpill(l.a, &spill{area: area, why: "Circular dependency through another array", circular: true})
		l.s.wb.noteCycle(l)
		changed = append(changed, l)
	}
	return changed
}
