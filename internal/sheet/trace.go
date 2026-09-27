package sheet

import "slices"

// Tracing, as Excel's Trace Precedents and Trace Dependents: which cells
// a formula reads, and which formulas read a cell. Only direct links are
// followed; tracing again from a found cell goes a level further.

// Precedents returns the cells and ranges the formula at a reads, in the
// order the formula mentions them, with named ranges resolved and
// repeats dropped. It is empty for anything but a formula.
func (s *Sheet) Precedents(a Addr) []Rect {
	c := s.cells[a]
	if c == nil || !c.IsFormula() {
		return nil
	}
	var out []Rect
	add := func(r Rect) {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	walkRefs(s.bound(c), func(a Addr) { add(Rect{a, a}) }, add)
	return out
}

// Dependents returns the formula cells that read a directly, through a
// reference, a range or a named range, in row-major order.
func (s *Sheet) Dependents(a Addr) []Addr {
	seen := map[Addr]bool{}
	for d := range s.dependents[a] {
		seen[d] = true
	}
	for u := range s.rangeUsers.candidates(a.Col) {
		for _, r := range s.cells[u].ranges {
			if r.Contains(a) {
				seen[u] = true
			}
		}
	}
	for k, users := range s.nameUsers {
		if n, ok := s.names[k]; ok && !n.Lost && n.Range.Contains(a) {
			for u := range users {
				seen[u] = true
			}
		}
	}
	out := make([]Addr, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sortAddrs(out)
	return out
}
