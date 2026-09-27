package functions

import "slices"

// seq is the entries of a row or column searched by a lookup: n of them,
// of which only the first data may hold data. With sparse, only the
// entries holding cells (entries, positions) hold anything; every other
// entry is blank.
type seq struct {
	n, data int
	line    matrix // the row or column
	blank   Value
	sparse  bool

	get      lookup
	r        Rect // the line's cells, when it is a range
	vertical bool
}

// lineSeq is the first column (vertical) or row of m, as a lookup
// searches it: through the cells it holds, when it is a range.
func lineSeq(m matrix, get lookup, vertical bool) seq {
	line := m
	if m.ref {
		to := Addr{Col: m.origin.Col, Row: m.origin.Row + m.rows - 1}
		if !vertical {
			to = Addr{Col: m.origin.Col + m.cols - 1, Row: m.origin.Row}
		}
		line = rectMatrix(m.sheet, Rect{From: m.origin, To: to}, get)
	}
	return seq{
		n: line.size(), data: min(line.dataLen(), line.size()), line: line, blank: line.blank,
		sparse: !line.ref || !get.dense,
		get:    get, vertical: vertical,
		r: Rect{From: line.origin, To: Addr{Col: line.origin.Col + line.cols - 1, Row: line.origin.Row + line.rows - 1}},
	}
}

// at is entry i's value.
func (q *seq) at(i int) Value { return q.line.at(i) }

// value is the value of entry i, whose cell is at a when the line is a
// range.
func (q *seq) value(i int, a Addr) Value {
	if !q.line.ref {
		return q.at(i)
	}
	return q.get.cell(q.line.sheet, a)
}

// index is the entry of the line's cell at a.
func (q *seq) index(a Addr) int {
	if q.vertical {
		return a.Row - q.r.From.Row
	}
	return a.Col - q.r.From.Col
}

// entries calls fn with the entries holding cells, in order, and where
// their cells are, until fn returns false. It evaluates nothing: a search
// reads the value of each entry it reaches (q.value), and no more.
func (q *seq) entries(fn func(i int, a Addr) bool) {
	if !q.line.ref {
		fn(0, Addr{})
		return
	}
	q.get.stored(q.line.sheet, q.r, func(a Addr) bool { return fn(q.index(a), a) })
}

// positions is the entries holding cells, in order.
func (q *seq) positions() []int {
	if !q.line.ref {
		return []int{0}
	}
	var out []int
	q.get.stored(q.line.sheet, q.r, func(a Addr) bool {
		out = append(out, q.index(a))
		return true
	})
	return out
}

// findExact returns the first index whose value equals key, or -1.
func (q *seq) findExact(key Value, wild bool) int {
	if q.sparse {
		blank := lookupEqual(key, q.blank, wild)
		next, found := 0, -1 // next is the first entry not looked at
		q.entries(func(i int, a Addr) bool {
			switch {
			case i > next && blank:
				found = next // a blank before this cell
			case lookupEqual(key, q.value(i, a), wild):
				found = i
			default:
				next = i + 1
				return true
			}
			return false
		})
		switch {
		case found >= 0:
			return found
		case blank && next < q.n:
			return next
		}
		return -1
	}
	for i := range q.data {
		if lookupEqual(key, q.at(i), wild) {
			return i
		}
	}
	if q.data < q.n && lookupEqual(key, q.blank, wild) {
		return q.data
	}
	return -1
}

// findSorted is the approximate match of sorted lookups: with dir 1 the
// last value <= key in ascending data, with dir -1 the last value >= key
// in descending data. Values of another kind are skipped.
func (q *seq) findSorted(key Value, dir int) int {
	found := -1
	if q.sparse && key.Kind != q.blank.Kind { // blanks are skipped: only the cells count
		q.entries(func(i int, a Addr) bool {
			v := q.value(i, a)
			if v.Kind != key.Kind {
				return true
			}
			if compare(v, key)*dir > 0 {
				return false
			}
			found = i
			return true
		})
		return found
	}
	for i := range q.data {
		v := q.at(i)
		if v.Kind != key.Kind {
			continue
		}
		if c := compare(v, key) * dir; c > 0 {
			return found
		}
		found = i
	}
	if v := q.blank; q.data < q.n && v.Kind == key.Kind && compare(v, key)*dir <= 0 {
		found = q.n - 1 // the blanks past the data all match; the last wins
	}
	return found
}

// order is the entries a search visits, first to last (last to first
// with reverse): those that hold cells, and of the blanks, which are all
// the same, only the first met.
func (q *seq) order(reverse bool) []int {
	var out []int
	if !q.sparse {
		for i := range q.data {
			out = append(out, i)
		}
		if q.data < q.n {
			out = append(out, q.data)
		}
		if reverse {
			slices.Reverse(out)
			if q.data < q.n {
				out[0] = q.n - 1
			}
		}
		return out
	}
	out = q.positions()
	gap := -1 // the first blank in search order
	if !reverse {
		for gap = 0; gap < len(out) && out[gap] == gap; gap++ {
		}
	} else {
		for gap = q.n - 1; len(out) > 0 && q.n-1-gap < len(out) && out[len(out)-1-(q.n-1-gap)] == gap; gap-- {
		}
	}
	if gap >= 0 && gap < q.n {
		i, _ := slices.BinarySearch(out, gap)
		out = slices.Insert(out, i, gap)
	}
	if reverse {
		slices.Reverse(out)
	}
	return out
}
