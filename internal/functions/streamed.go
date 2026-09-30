package functions

import "github.com/FelineStateMachine/012/internal/value"

// Functions over aligned ranges computing a StreamCall (stream.go). On
// a sheet they gather the positions of the cells their ranges hold and
// visit those (masked.go), which over a source of ten million rows
// would hold ten million positions. Computing a StreamCall they read
// their ranges instead a window of rows at a time, every cell of the
// window's rows, blank or not, testing and adding as they go, so they
// hold one window whatever the source's size. Blank cells meet
// criteria or add nothing as they would when accounted for at once, so
// the results are the same.

// streamRows is how many rows of each range a window holds.
const streamRows = 2048

// streamable reports whether same-sized matrices are read a window at
// a time: computing a StreamCall, every one a range of a sheet that
// exists.
func streamable(get lookup, ms ...matrix) bool {
	if !get.stream {
		return false
	}
	for _, m := range ms {
		if !m.ref || m.blank.Kind == value.Error {
			return false
		}
	}
	return true
}

// windows calls fn with each window of the rows of same-sized ranges
// that may hold data, the values of each range's window row by row (the
// slices are reused), until fn returns false. It returns how many cells
// of each range the windows cover; the rest are blank in all of them.
func windows(get lookup, ms []matrix, fn func(vals [][]Value) bool) int {
	rows, cols := 0, ms[0].cols
	for _, m := range ms {
		rows = max(rows, min(m.dataRows, m.rows))
	}
	bufs := make([][]Value, len(ms))
	for r0 := 0; r0 < rows; r0 += streamRows {
		n := min(streamRows, rows-r0)
		for i, m := range ms {
			bufs[i] = m.window(get, r0, n, bufs[i])
		}
		if !fn(bufs) {
			break
		}
	}
	return rows * cols
}

// window reads n rows of the range m from its row r0 into buf, every
// cell of them, blanks as blank values.
func (m matrix) window(get lookup, r0, n int, buf []Value) []Value {
	size := n * m.cols
	if cap(buf) < size {
		buf = make([]Value, size)
	}
	buf = buf[:size]
	clear(buf)
	top := m.origin.Row + r0
	r := Rect{From: Addr{Col: m.origin.Col, Row: top}, To: Addr{Col: m.origin.Col + m.cols - 1, Row: top + n - 1}}
	get.cells(m.sheet, r, func(a Addr, v Value) bool {
		buf[(a.Row-top)*m.cols+a.Col-m.origin.Col] = v
		return true
	})
	return buf
}

// passAll reports whether the cell at i of each window meets its
// criterion.
func passAll(vals [][]Value, cs []criterion, i int) bool {
	for j, c := range cs {
		if !c.test(vals[j][i]) {
			return false
		}
	}
	return true
}

// blanksPass reports whether blank cells meet every criterion.
func blanksPass(cs []criterion) bool {
	for _, c := range cs {
		if !c.test(Value{}) {
			return false
		}
	}
	return true
}

// countStream is COUNTIFS over windows.
func countStream(get lookup, ms []matrix, cs []criterion) Value {
	n := 0
	covered := windows(get, ms, func(vals [][]Value) bool {
		for i := range vals[0] {
			if passAll(vals, cs, i) {
				n++
			}
		}
		return true
	})
	if blanksPass(cs) {
		n += ms[0].size() - covered
	}
	return num(float64(n))
}

// sumStream calls add with the numbers of val where the criteria on ms
// pass, over windows, or returns the first error there.
func sumStream(get lookup, val matrix, ms []matrix, cs []criterion, add func(float64)) *Value {
	var err *Value
	windows(get, append([]matrix{val}, ms...), func(vals [][]Value) bool {
		for i, v := range vals[0] {
			if !passAll(vals[1:], cs, i) {
				continue
			}
			switch v.Kind {
			case value.Error:
				err = &v
				return false
			case value.Number:
				add(v.Num)
			}
		}
		return true
	})
	return err
}

// averageStream is AVERAGEIFS over windows.
func averageStream(get lookup, val matrix, ms []matrix, cs []criterion) Value {
	sum, n := 0.0, 0
	if e := sumStream(get, val, ms, cs, func(f float64) { sum += f; n++ }); e != nil {
		return *e
	}
	if n == 0 {
		return value.ErrDiv0
	}
	return num(sum / float64(n))
}

// blankStream is COUNTBLANK over windows.
func blankStream(get lookup, m matrix) Value {
	n := 0
	covered := windows(get, []matrix{m}, func(vals [][]Value) bool {
		for _, v := range vals[0] {
			if v.Kind == value.Empty || v.Kind == value.Text && v.Str == "" {
				n++
			}
		}
		return true
	})
	return num(float64(n + m.size() - covered))
}

// productStream calls term with the factors of each entry of
// same-sized ranges, over windows, as sumProductTerms does.
func productStream(get lookup, ms []matrix, term func(fs []float64)) *Value {
	var err *Value
	fs := make([]float64, len(ms))
	windows(get, ms, func(vals [][]Value) bool {
		for i := range vals[0] {
			for j := range ms {
				switch v := vals[j][i]; v.Kind {
				case value.Error:
					err = &v
					return false
				case value.Number, value.Bool:
					fs[j] = v.Num
				default:
					fs[j] = 0
				}
			}
			term(fs)
		}
		return true
	})
	return err
}

// seekStream is XLOOKUP's search of a sparse line computing a
// StreamCall: one pass over the cells it holds, forwards whichever way
// the search goes, keeping what the search in order would find (the
// first exact match, or the last searching backwards; else the closest
// value, the first or last of equals) without listing the entries. ok
// is false when a blank would match the key, which the search in order
// handles.
func (q *seq) seekStream(key Value, mode int, reverse bool) (int, bool) {
	wild := mode == 2
	if lookupEqual(key, q.blank, wild) {
		return 0, false
	}
	exact, best := -1, -1
	var bestV Value
	c := q.walk()
	defer c.close()
	for a, ok := c.next(); ok; a, ok = c.next() {
		i, v := q.index(a), q.value(a)
		switch {
		case lookupEqual(key, v, wild):
			if !reverse {
				return i, true
			}
			exact = i
		case exact >= 0 || mode == 0 || wild || v.Kind != key.Kind:
		case compare(v, key)*mode > 0 && (best < 0 || closer(v, bestV, mode, reverse)):
			best, bestV = i, v
		}
	}
	if exact >= 0 {
		return exact, true
	}
	return best, true
}

// closer reports whether v is closer to the key than the best so far,
// in XLOOKUP's next smaller (-1) or next larger (1) mode: equals keep
// the first met, or the last searching backwards.
func closer(v, best Value, mode int, reverse bool) bool {
	d := compare(v, best) * mode
	return d < 0 || reverse && d == 0
}
