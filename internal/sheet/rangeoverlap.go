package sheet

// overlapping calls fn with each formula one of whose ranges shares a
// cell with r, as readers does for one cell; a formula may come more
// than once. It walks r's columns or the indexed ones, whichever are
// fewer.
func (x *rangeIndex) overlapping(r Rect, fn func(Addr)) {
	if r.From == r.To {
		x.readers(r.From, fn)
		return
	}
	if r.To.Col-r.From.Col < len(x.byCol) {
		for c := r.From.Col; c <= r.To.Col; c++ {
			if iv := x.byCol[c]; iv != nil {
				iv.overlap(r, fn)
			}
		}
	} else {
		for c, iv := range x.byCol {
			if c >= r.From.Col && c <= r.To.Col {
				iv.overlap(r, fn)
			}
		}
	}
	x.wide.overlap(r, fn)
}

// overlap calls fn with the users of every range sharing a cell with r.
func (iv *intervals) overlap(r Rect, fn func(Addr)) {
	if len(iv.users) == 0 {
		return
	}
	if iv.sorted == nil {
		iv.build()
	}
	iv.queryRect(0, len(iv.sorted), r, fn)
}

// queryRect is query for the rows of r rather than one row.
func (iv *intervals) queryRect(lo, hi int, r Rect, fn func(Addr)) {
	for lo < hi {
		mid := (lo + hi) / 2
		if iv.maxEnd[mid] < r.From.Row {
			return // nothing below reaches r's rows
		}
		iv.queryRect(lo, mid, r, fn)
		g := iv.sorted[mid]
		if g.From.Row > r.To.Row {
			return // this and everything after start below r
		}
		if _, over := intersectRect(g, r); over {
			for u := range iv.users[g] {
				fn(u)
			}
		}
		lo = mid + 1
	}
}
