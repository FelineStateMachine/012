package sheet

// Formulas infer their format from the cells they read (=B5*2 shows
// currency when B5 does), blank ones included. A blank cell has no entry
// of its own to mark changed when its column, row or the sheet is
// formatted, so the formulas reading anywhere in the line are marked
// instead, found through the indexes recalculation uses: the cost is the
// formulas, not the line's million cells.

// readersIn returns the formulas, on any sheet, that read a cell of r on
// s: by reference, through a range, a named range, or naming s.
func (s *Sheet) readersIn(r Rect) []loc {
	seen := map[loc]struct{}{}
	add := func(l loc) { seen[l] = struct{}{} }
	for a, users := range s.dependents {
		if r.Contains(a) {
			for u := range users {
				add(loc{s, u})
			}
		}
	}
	s.rangeUsers.usersIn(r, func(u Addr) { add(loc{s, u}) })
	w := s.wb
	for _, nu := range w.namedInUse() {
		if nu.s == s && overlaps(nu.r, r) {
			for u := range nu.users {
				add(u)
			}
		}
	}
	for u := range w.crossUsers {
		if c := u.s.cells.get(u.a); c != nil {
			for _, x := range c.xrefs {
				if w.byKey[x.key] == s && overlaps(x.r, r) {
					add(u)
				}
			}
		}
	}
	out := make([]loc, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	return out
}

// usersIn calls fn with the formulas one of whose ranges overlaps r; a
// formula may come more than once.
func (x *rangeIndex) usersIn(r Rect, fn func(Addr)) {
	each := func(iv *intervals) {
		for rg, users := range iv.users {
			if overlaps(rg, r) {
				for u := range users {
					fn(u)
				}
			}
		}
	}
	for c, iv := range x.byCol {
		if c >= r.From.Col && c <= r.To.Col {
			each(iv)
		}
	}
	each(&x.wide)
}

// overlaps reports whether two ranges share a cell.
func overlaps(p, q Rect) bool {
	return p.From.Col <= q.To.Col && q.From.Col <= p.To.Col && p.From.Row <= q.To.Row && q.From.Row <= p.To.Row
}
