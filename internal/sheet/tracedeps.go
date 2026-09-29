package sheet

import (
	"cmp"
	"slices"
)

// Dependents, for tracing (trace.go): the formulas reading a cell, found
// through the indexes recalculation keeps, and Reads, which asks the
// same of one formula, so what draws the sheet can mark the dependents it
// shows without listing them all.

// DependentLinks returns the formulas that read the cell at a directly:
// through a reference, a range, a named range or a region's name. When
// the formula at a spills, the cells it spills into come first, and the
// formulas reading any of them count too. Formulas on this sheet come in
// row-major order, then those on other sheets in tab order. With max > 0
// at most max formulas are found, and more reports that there were
// others: a cell read by a million formulas costs what max does.
func (s *Sheet) DependentLinks(a Addr, max int) (links []Link, more bool) {
	subject := Rect{From: a, To: a}
	if area, ok := s.SpillArea(a); ok && area != subject {
		subject = area
		links = append(links, Link{Target: Target{s, area}, Kind: LinkSpill})
	}
	f := &depFinder{max: max, seen: map[loc]struct{}{}}
	s.readersOf(subject, f)
	found := make([]Link, 0, len(f.seen))
	for l := range f.seen {
		found = append(found, Link{Target: Target{l.s, Rect{From: l.a, To: l.a}}})
	}
	w := s.wb
	rank := func(t *Sheet) int {
		if t == s {
			return -1
		}
		return w.Index(t)
	}
	slices.SortFunc(found, func(x, y Link) int {
		return cmp.Or(cmp.Compare(rank(x.Sheet), rank(y.Sheet)),
			cmp.Compare(x.Range.From.Row, y.Range.From.Row), cmp.Compare(x.Range.From.Col, y.Range.From.Col))
	})
	return append(links, found...), f.more
}

// depFinder collects dependents, up to max when it's positive.
type depFinder struct {
	max  int
	seen map[loc]struct{}
	more bool
}

func (f *depFinder) add(l loc) {
	if _, ok := f.seen[l]; ok || f.more {
		return
	}
	if f.max > 0 && len(f.seen) >= f.max {
		f.more = true
		return
	}
	f.seen[l] = struct{}{}
}

// readersOf adds the formulas that read any cell of r, on s.
func (s *Sheet) readersOf(r Rect, f *depFinder) {
	s.refReaders(r, f)
	s.rangeUsers.overlapping(r, func(u Addr) { f.add(loc{s, u}) })
	w := s.wb
	for k, users := range w.nameUsers {
		if f.more || !w.namesOverlap(k, s, r) {
			continue
		}
		for u := range users {
			if f.add(u); f.more {
				break
			}
		}
	}
	for u := range w.crossUsers {
		if f.more {
			return
		}
		if c := u.s.cells.get(u.a); c != nil && w.xrefsOverlap(c, s, r) {
			f.add(u)
		}
	}
}

// refReaders adds the formulas on s with a single-cell reference into
// r, walking whichever is smaller: r's cells or the cells referenced.
func (s *Sheet) refReaders(r Rect, f *depFinder) {
	add := func(users map[Addr]struct{}) {
		for u := range users {
			if f.add(loc{s, u}); f.more {
				return
			}
		}
	}
	if n := (r.To.Row - r.From.Row + 1) * (r.To.Col - r.From.Col + 1); n <= len(s.dependents) {
		for row := r.From.Row; row <= r.To.Row && !f.more; row++ {
			for col := r.From.Col; col <= r.To.Col; col++ {
				add(s.dependents[Addr{Col: col, Row: row}])
			}
		}
		return
	}
	for a, users := range s.dependents {
		if f.more {
			return
		}
		if r.Contains(a) {
			add(users)
		}
	}
}

// namesOverlap reports whether the name with key k, a named range, a
// table (a region's too) or a region's (nu.sales), stands for cells of
// r on s.
func (w *Workbook) namesOverlap(k string, s *Sheet, r Rect) bool {
	if nm, ok := w.names[k]; ok {
		_, over := intersectRect(nm.Range, r)
		return !nm.Gone() && nm.Sheet == s && over
	}
	if v, ok := w.findTable(k); ok {
		_, over := intersectRect(v.r, r)
		return v.ok && v.s == s && over
	}
	t, table, ok := w.regionName(k)
	if !ok || t != s || table == (Rect{}) {
		return false
	}
	_, over := intersectRect(table, r)
	return over
}

// xrefsOverlap reports whether c reads cells of r on s through a
// reference that names s.
func (w *Workbook) xrefsOverlap(c *Cell, s *Sheet, r Rect) bool {
	for _, x := range c.xrefs {
		if _, over := intersectRect(x.r, r); over && w.byKey[x.key] == s {
			return true
		}
	}
	return false
}

// Reads reports whether the formula at u, on s, reads any cell of r on
// t directly: through a reference, a range, a named range or a region's
// name. It costs the formula's references, so asking it of every cell
// on screen is cheap.
func (s *Sheet) Reads(u Addr, t *Sheet, r Rect) bool {
	c, _, _ := s.cells.peek(u)
	if c == nil || !c.IsFormula() {
		return false
	}
	w := s.wb
	if t == s {
		for _, a := range c.refs {
			if r.Contains(a) {
				return true
			}
		}
		for _, g := range c.ranges {
			if _, over := intersectRect(g, r); over {
				return true
			}
		}
	}
	if w.xrefsOverlap(c, t, r) {
		return true
	}
	for _, k := range c.names {
		if w.namesOverlap(k, t, r) {
			return true
		}
	}
	return false
}
