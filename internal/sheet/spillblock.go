package sheet

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
func (s *Sheet) spillsIntoItself(a Addr, area Rect) (string, []loc) {
	if area.From == area.To {
		return "", nil
	}
	w := &spillWalk{look: s.wb.readerLookup(), target: loc{s, a}, seen: map[loc]bool{{s, a}: true}, parent: -1}
	if w.cells(s, area, a, loc{}, true) {
		return circular(w.origin), nil
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
		changed = append(changed, l)
	}
	return changed
}
