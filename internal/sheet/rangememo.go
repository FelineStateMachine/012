package sheet

import "slices"

// Running aggregates: within one recalculation, the SUM-like functions
// (SUM, AVERAGE, COUNT, COUNTA, MIN, MAX, PRODUCT) share what they read
// of a range. For each run of columns and first row, the aggregate is
// kept as a checkpoint after every row with data, read forward only as
// far as some formula has asked. A thousand SUM(A1:A8192) read column A
// once; 8192 running totals SUM($A$1:An) extend the same run one row at
// a time, so they cost O(n) reads instead of O(n^2). Each result is
// accumulated in the same order as reading the range directly, so it is
// the same to the last bit.

// memoRows is the shortest range, in rows, whose aggregate is shared:
// shorter ones are read directly.
const memoRows = 16

type aggKey struct {
	s          *Sheet
	c0, c1, r0 int
}

// runAgg is a run: the rows read so far with data, and the aggregate
// through each.
type runAgg struct {
	rows []int
	cum  []agg
	next int // every row before next has been read

	errRow int
	err    *Value // the first error met, which ends the run

	busy bool // being extended: a formula it reads may ask for it again
}

type aggMemo struct {
	w *Workbook
	m map[aggKey]*runAgg
}

// rangeAgg returns the aggregate of the range r on the named sheet, or
// false when it should be read directly: no recalculation is running, the
// range is short, its sheet doesn't exist, or a circular reference makes
// the order of evaluation matter.
func (rd *reader) rangeAgg(sheet string, r Rect) (agg, *Value, bool) {
	memo := rd.memo
	if memo == nil || denseReads || r.To.Row-r.From.Row+1 < memoRows || memo.w.Circular {
		return agg{}, nil, false
	}
	t := rd.sheet(sheet)
	if t == nil {
		return agg{}, nil, false
	}
	key := aggKey{t, r.From.Col, r.To.Col, r.From.Row}
	run := memo.m[key]
	if run == nil {
		run = &runAgg{next: r.From.Row}
		memo.m[key] = run
	}
	if run.err == nil && r.To.Row >= run.next {
		if run.busy {
			return agg{}, nil, false
		}
		circular := memo.w.Circular
		run.extend(rd, t, key, r.To.Row)
		if memo.w.Circular != circular {
			delete(memo.m, key) // it read a cell of a cycle
		}
	}
	return run.through(r.To.Row)
}

// extend reads the run's columns from its next row through to.
func (run *runAgg) extend(rd *reader, t *Sheet, key aggKey, to int) {
	run.busy = true
	defer func() { run.busy = false }()
	cur, row := newAgg(), -1
	if n := len(run.cum); n > 0 {
		cur = run.cum[n-1]
	}
	visit := func(a Addr) bool {
		if a.Row != row {
			if row >= 0 {
				run.rows, run.cum = append(run.rows, row), append(run.cum, cur)
			}
			row, run.next = a.Row, a.Row
		}
		if e := cur.add(rd.read(t, a), false); e != nil {
			run.errRow, run.err = a.Row, e
			return false
		}
		return true
	}
	if key.c0 == key.c1 {
		t.cells.colScan(key.c0, run.next, to, func(r int) bool { return visit(Addr{Col: key.c0, Row: r}) })
	} else {
		for a := range t.cells.inRange(Rect{From: Addr{Col: key.c0, Row: run.next}, To: Addr{Col: key.c1, Row: to}}) {
			if !visit(a) {
				break
			}
		}
	}
	if run.err != nil {
		return
	}
	if row >= 0 {
		run.rows, run.cum = append(run.rows, row), append(run.cum, cur)
	}
	run.next = to + 1
}

// through is the aggregate from the run's first row through row to.
func (run *runAgg) through(to int) (agg, *Value, bool) {
	if run.err != nil && to >= run.errRow {
		return agg{}, run.err, true
	}
	i, found := slices.BinarySearch(run.rows, to)
	if found {
		return run.cum[i], nil, true
	}
	if i == 0 {
		return newAgg(), nil, true
	}
	return run.cum[i-1], nil, true
}
