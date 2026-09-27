package sheet

import (
	"cmp"
	"slices"
)

// Tracing, as Excel's Trace Precedents and Trace Dependents: which cells
// a formula reads, and which formulas read a cell, on any sheet. Only
// direct links are followed; tracing again from a found cell goes a level
// further.

// Target is a traced cell or range and the sheet it is on.
type Target struct {
	Sheet *Sheet
	Range Rect
}

// Precedents returns the cells and ranges the formula at a reads, in the
// order the formula mentions them, with named ranges resolved and
// repeats dropped. References to sheets that don't exist are left out. It
// is empty for anything but a formula.
func (s *Sheet) Precedents(a Addr) []Target {
	c := s.cells[a]
	if c == nil || !c.IsFormula() {
		return nil
	}
	var out []Target
	add := func(sheet string, r Rect) {
		t := Target{s.wb.resolve(s, sheet), r}
		if t.Sheet != nil && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	walkRefs(s.bound(c), func(sheet string, a Addr) { add(sheet, Rect{a, a}) }, add)
	return out
}

// Dependents returns the formula cells that read a directly, through a
// reference, a range or a named range: those on this sheet first in
// row-major order, then those on other sheets in tab order.
func (s *Sheet) Dependents(a Addr) []Target {
	w := s.wb
	seen := map[loc]bool{}
	for d := range s.dependents[a] {
		seen[loc{s, d}] = true
	}
	for u := range s.rangeUsers.candidates(a.Col) {
		for _, r := range s.cells[u].ranges {
			if r.Contains(a) {
				seen[loc{s, u}] = true
			}
		}
	}
	for k, users := range w.nameUsers {
		if n, ok := w.names[k]; ok && !n.Lost && n.Sheet == s && n.Range.Contains(a) {
			for u := range users {
				seen[u] = true
			}
		}
	}
	for u := range w.crossUsers {
		if w.crossReads(u, loc{s, a}) {
			seen[u] = true
		}
	}
	out := make([]Target, 0, len(seen))
	for l := range seen {
		out = append(out, Target{l.s, Rect{l.a, l.a}})
	}
	rank := func(t *Sheet) int {
		if t == s {
			return -1
		}
		return w.Index(t)
	}
	slices.SortFunc(out, func(x, y Target) int {
		return cmp.Or(cmp.Compare(rank(x.Sheet), rank(y.Sheet)),
			cmp.Compare(x.Range.From.Row, y.Range.From.Row), cmp.Compare(x.Range.From.Col, y.Range.From.Col))
	})
	return out
}
