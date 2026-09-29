package sheet

import (
	"slices"

	"github.com/FelineStateMachine/012/internal/functions"
)

// Running aggregates: within one recalculation, the SUM-like functions
// (SUM, AVERAGE, COUNT, COUNTA, MIN, MAX, PRODUCT) share what they read
// of a range. For each run of columns and first row read more than once,
// the aggregate is kept as a checkpoint after every row with data, read
// forward only as far as some formula has asked. A thousand SUM(A1:A8192) read column A
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
	cum  []functions.Agg
	next int // every row before next has been read

	errRow int
	err    *Value // the first error met, which ends the run

	busy bool // being extended: a formula it reads may ask for it again
}

type aggMemo struct {
	w *Workbook
	m map[aggKey]*runAgg
}

// RangeAgg returns the aggregate of the range r on the named sheet, or
// false when it should be read directly: no recalculation is running, the
// range is short, its sheet doesn't exist, or a circular reference makes
// the order of evaluation matter.
func (rd *reader) RangeAgg(sheet string, r Rect) (functions.Agg, *Value, bool) {
	memo := rd.memo
	if memo == nil || denseReads || r.To.Row-r.From.Row+1 < memoRows || memo.w.Circular {
		return functions.Agg{}, nil, false
	}
	t := rd.sheet(sheet)
	if t == nil {
		return functions.Agg{}, nil, false
	}
	if _, ok := t.pagedRegion(); ok {
		return functions.Agg{}, nil, false
	}
	key := aggKey{t, r.From.Col, r.To.Col, r.From.Row}
	run := memo.m[key]
	if run == nil {
		// A range read once (a SUM of each named region) is read directly;
		// only the second read of a run starts sharing it.
		memo.m[key] = &runAgg{next: r.From.Row}
		return functions.Agg{}, nil, false
	}
	if run.err == nil && r.To.Row >= run.next {
		if run.busy {
			return functions.Agg{}, nil, false
		}
		circular := memo.w.Circular
		run.extend(rd, t, key, r.To.Row)
		if memo.w.Circular != circular {
			delete(memo.m, key) // it read a cell of a cycle
		}
	}
	return run.through(r.To.Row)
}

// Fold adds the cells of r to s as SUM-like functions read a range; see
// functions.Book. The engine walks the cells and adds each as it reads
// it, so a range costs no more than a call to Agg.Add per cell.
func (rd *reader) Fold(sheet string, r Rect, s functions.Agg) (functions.Agg, *Value) {
	t := rd.sheet(sheet)
	if t == nil {
		return s, s.Add(ErrRef, false)
	}
	if reg, ok := t.pagedRegion(); ok {
		return rd.pagedFold(t, reg, r, s)
	}
	var e *Value
	add := func(a Addr) bool {
		e = s.Add(rd.read(t, a), false)
		return e == nil
	}
	switch {
	case denseReads:
		for a := r.From; a.Row <= r.To.Row && add(a); {
			if a.Col++; a.Col > r.To.Col {
				a = Addr{Col: r.From.Col, Row: a.Row + 1}
			}
		}
	case r.From.Col == r.To.Col:
		t.cells.colScan(r.From.Col, r.From.Row, r.To.Row, func(row int) bool {
			return add(Addr{Col: r.From.Col, Row: row})
		})
	default:
		for a := range t.cells.keysIn(r) {
			if !add(a) {
				break
			}
		}
	}
	return s, e
}

// extend reads the run's columns from its next row through to.
func (run *runAgg) extend(rd *reader, t *Sheet, key aggKey, to int) {
	run.busy = true
	defer func() { run.busy = false }()
	cur, row := functions.NewAgg(), -1
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
		if e := cur.Add(rd.read(t, a), false); e != nil {
			run.errRow, run.err = a.Row, e
			return false
		}
		return true
	}
	if key.c0 == key.c1 {
		t.cells.colScan(key.c0, run.next, to, func(r int) bool { return visit(Addr{Col: key.c0, Row: r}) })
	} else {
		for a := range t.cells.keysIn(Rect{From: Addr{Col: key.c0, Row: run.next}, To: Addr{Col: key.c1, Row: to}}) {
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
func (run *runAgg) through(to int) (functions.Agg, *Value, bool) {
	if run.err != nil && to >= run.errRow {
		return functions.Agg{}, run.err, true
	}
	i, found := slices.BinarySearch(run.rows, to)
	if found {
		return run.cum[i], nil, true
	}
	if i == 0 {
		return functions.NewAgg(), nil, true
	}
	return run.cum[i-1], nil, true
}
