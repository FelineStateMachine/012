package sheet

import (
	"cmp"
	"slices"
)

// rangeIndex finds the formulas whose range references contain a cell.
// Recalculation asks it about every changed cell; scanning every range
// user instead made a change cost O(changed x range users), so filling a
// column of 8192 running totals checked 67 million ranges.
//
// Ranges up to wideCols columns are indexed under each column they cover,
// wider ones (whole rows) once; within each, an interval tree over rows
// finds the ranges holding a row in O(log n + found), and formulas with
// the same range (a thousand SUM(A:A)) share one entry. Only the columns
// that ranges cover take memory.
type rangeIndex struct {
	byCol map[int]*intervals
	wide  intervals
}

// wideCols is the widest range indexed column by column.
const wideCols = 32

// add indexes the formula at a under the ranges it reads.
func (x *rangeIndex) add(a Addr, ranges []Rect) {
	for _, r := range ranges {
		if r.To.Col-r.From.Col >= wideCols {
			x.wide.add(r, a)
			continue
		}
		if x.byCol == nil {
			x.byCol = make(map[int]*intervals)
		}
		for c := max(r.From.Col, 0); c <= min(r.To.Col, MaxCols-1); c++ {
			iv := x.byCol[c]
			if iv == nil {
				iv = &intervals{}
				x.byCol[c] = iv
			}
			iv.add(r, a)
		}
	}
}

// remove undoes add for the same ranges.
func (x *rangeIndex) remove(a Addr, ranges []Rect) {
	for _, r := range ranges {
		if r.To.Col-r.From.Col >= wideCols {
			x.wide.remove(r, a)
			continue
		}
		for c := max(r.From.Col, 0); c <= min(r.To.Col, MaxCols-1); c++ {
			if iv := x.byCol[c]; iv != nil {
				iv.remove(r, a)
				if len(iv.users) == 0 {
					delete(x.byCol, c)
				}
			}
		}
	}
}

// readers calls fn with each formula one of whose ranges contains a; a
// formula with two such ranges may come twice.
func (x *rangeIndex) readers(a Addr, fn func(Addr)) {
	if iv := x.byCol[a.Col]; iv != nil {
		iv.stab(a, fn)
	}
	x.wide.stab(a, fn)
}

// intervals holds ranges and the formulas reading each, with a tree over
// their rows built when first asked after a change.
type intervals struct {
	users  map[Rect]map[Addr]struct{}
	sorted []Rect // by first row; nil when stale
	maxEnd []int  // the last row of any range in the subtree rooted at each index
}

func (iv *intervals) add(r Rect, a Addr) {
	if iv.users == nil {
		iv.users = make(map[Rect]map[Addr]struct{})
	}
	u := iv.users[r]
	if u == nil {
		u = make(map[Addr]struct{})
		iv.users[r] = u
		iv.sorted = nil
	}
	u[a] = struct{}{}
}

func (iv *intervals) remove(r Rect, a Addr) {
	u := iv.users[r]
	if u == nil {
		return
	}
	delete(u, a)
	if len(u) == 0 {
		delete(iv.users, r)
		iv.sorted = nil
	}
}

// stab calls fn with the users of every range containing a.
func (iv *intervals) stab(a Addr, fn func(Addr)) {
	if len(iv.users) == 0 {
		return
	}
	if iv.sorted == nil {
		iv.build()
	}
	iv.query(0, len(iv.sorted), a, fn)
}

func (iv *intervals) build() {
	iv.sorted = iv.sorted[:0]
	for r := range iv.users {
		iv.sorted = append(iv.sorted, r)
	}
	slices.SortFunc(iv.sorted, func(p, q Rect) int {
		if c := cmp.Compare(p.From.Row, q.From.Row); c != 0 {
			return c
		}
		if c := cmp.Compare(p.To.Row, q.To.Row); c != 0 {
			return c
		}
		if c := cmp.Compare(p.From.Col, q.From.Col); c != 0 {
			return c
		}
		return cmp.Compare(p.To.Col, q.To.Col)
	})
	iv.maxEnd = slices.Grow(iv.maxEnd[:0], len(iv.sorted))[:len(iv.sorted)]
	iv.fill(0, len(iv.sorted))
}

// fill computes maxEnd for the implicit tree over sorted[lo:hi], whose
// root is its middle element, and returns it.
func (iv *intervals) fill(lo, hi int) int {
	if lo >= hi {
		return -1
	}
	mid := (lo + hi) / 2
	m := max(iv.sorted[mid].To.Row, iv.fill(lo, mid), iv.fill(mid+1, hi))
	iv.maxEnd[mid] = m
	return m
}

func (iv *intervals) query(lo, hi int, a Addr, fn func(Addr)) {
	for lo < hi {
		mid := (lo + hi) / 2
		if iv.maxEnd[mid] < a.Row {
			return // nothing below reaches the row
		}
		iv.query(lo, mid, a, fn)
		r := iv.sorted[mid]
		if r.From.Row > a.Row {
			return // this and everything after start below the row
		}
		if r.Contains(a) {
			for u := range iv.users[r] {
				fn(u)
			}
		}
		lo = mid + 1
	}
}
