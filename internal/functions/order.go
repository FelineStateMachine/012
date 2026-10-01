package functions

import (
	"cmp"
	"math"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Order statistics: MEDIAN, MODE, PERCENTILE, QUARTILE, LARGE and SMALL
// read their numbers once, in ascending order, picking the ones they
// need as they pass. On a sheet the numbers are held and sorted; over a
// linked source's column, computing a StreamCall, the Book sorts them
// on disk (Sorter), so a source of any size takes the memory of a sort
// run rather than of its column. Either way the same code picks, so the
// results are the same.

// Sorted is numbers read in ascending order, once.
type Sorted interface {
	// Len counts the numbers.
	Len() int
	// Next is the next number, ties in the order they were read, with
	// its position in that order; false at the end, or on an error,
	// which Err says.
	Next() (v float64, pos int64, ok bool)
	Err() error
	Close() error
}

// Sorter is a Book that sorts the numbers of a range of a paged sheet
// (a linked source's column) on disk: its numbers, skipping the cells
// holding anything else, as a range's are read by SUM, or the first
// error in it.
type Sorter interface {
	SortedNums(sheet string, r Rect) (Sorted, *Value)
}

// orderFuncs are the functions that read their numbers in order: from
// every argument (true: MEDIAN, MODE) or from the first, the rest being
// what to pick.
var orderFuncs = map[string]bool{
	"MEDIAN": true, "MODE": true, "MODE.SNGL": true, "LARGE": false, "SMALL": false,
	"PERCENTILE": false, "PERCENTILE.INC": false, "PERCENTILE.EXC": false,
	"QUARTILE": false, "QUARTILE.INC": false, "QUARTILE.EXC": false,
}

// sortsSource reports whether c is an order statistic of one range of a
// source, its other arguments single values: the Book sorts that range
// on disk, so it streams.
func sortsSource(c StreamCall) bool {
	all, ok := orderFuncs[c.Fn]
	if !ok || len(c.Args) == 0 || c.Args[0].Sheet == "" || all && len(c.Args) > 1 {
		return false
	}
	for _, a := range c.Args[1:] {
		if a.Sheet != "" {
			return false
		}
	}
	return true
}

// held is numbers held in memory, sorted.
type held struct {
	x []posNum
	i int
}

type posNum struct {
	v   float64
	pos int64
}

func (h *held) Len() int     { return len(h.x) }
func (h *held) Err() error   { return nil }
func (h *held) Close() error { return nil }
func (h *held) Next() (float64, int64, bool) {
	if h.i >= len(h.x) {
		return 0, 0, false
	}
	h.i++
	return h.x[h.i-1].v, h.x[h.i-1].pos, true
}

// sortedNums is the numbers of args in ascending order, with aggregate
// semantics (nums): sorted by the Book on disk when computing a
// StreamCall over one range of a source, else held.
func sortedNums(args []Node, get lookup) (Sorted, *Value) {
	if s, ok := get.book.(Sorter); ok && get.stream && len(args) == 1 {
		if r, ok := get.refOf(args[0]).(formula.Range); ok && r.Sheet != "" {
			return s.SortedNums(r.Sheet, r.Rect)
		}
	}
	x, err := nums(args, get)
	if err != nil {
		return nil, err
	}
	h := &held{x: make([]posNum, len(x))}
	for i, v := range x {
		h.x[i] = posNum{v, int64(i)}
	}
	slices.SortFunc(h.x, func(a, b posNum) int { return cmp.Or(cmpFloat(a.v, b.v), cmp.Compare(a.pos, b.pos)) })
	return h, nil
}

// pickAt reads the numbers at the ascending indexes ks of s, in order.
func pickAt(s Sorted, ks ...int) ([]float64, *Value) {
	out := make([]float64, 0, len(ks))
	for i := 0; len(out) < len(ks); i++ {
		v, _, ok := s.Next()
		if !ok {
			return nil, &value.ErrRef // the source failed as it was read
		}
		for len(out) < len(ks) && ks[len(out)] == i {
			out = append(out, v)
		}
	}
	return out, nil
}

// withSorted passes the numbers of the first argument, or with all of
// every argument (MEDIAN, MODE), sorted, to f with the rest.
func withSorted(all bool, f func(s Sorted, rest []Node, get lookup) Value) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		upto := 1
		if all {
			upto = len(args)
		}
		s, err := sortedNums(args[:upto], get)
		if err != nil {
			return *err
		}
		defer s.Close()
		return f(s, args[upto:], get)
	}
}

func medianOf(s Sorted, _ []Node, _ lookup) Value {
	n := s.Len()
	if n == 0 {
		return value.ErrNum
	}
	if n%2 == 1 {
		x, err := pickAt(s, n/2)
		if err != nil {
			return *err
		}
		return num(x[0])
	}
	x, err := pickAt(s, n/2-1, n/2)
	if err != nil {
		return *err
	}
	return num((x[0] + x[1]) / 2)
}

// modeOf is the most common number, the one read first on a tie, or
// #N/A when none repeats: one pass over the runs of equal numbers.
func modeOf(s Sorted, _ []Node, _ lookup) Value {
	best, bestN, bestPos := 0.0, 1, int64(math.MaxInt64)
	cur, n, first := 0.0, 0, int64(0)
	end := func() {
		if n > bestN || n == bestN && n > 1 && first < bestPos {
			best, bestN, bestPos = cur, n, first
		}
	}
	for {
		v, pos, ok := s.Next()
		if !ok {
			break
		}
		if n > 0 && v == cur {
			n++
			continue
		}
		end()
		cur, n, first = v, 1, pos
	}
	if s.Err() != nil {
		return value.ErrRef
	}
	end()
	if bestN < 2 {
		return value.ErrNA
	}
	return num(best)
}

// nthOf builds LARGE and SMALL: the kth largest or smallest number.
func nthOf(largest bool) func(Sorted, []Node, lookup) Value {
	return func(s Sorted, rest []Node, get lookup) Value {
		n, err := numArg(rest[0], get)
		if err != nil {
			return *err
		}
		k := int(math.Ceil(n))
		if k < 1 || k > s.Len() {
			return value.ErrNum
		}
		if largest {
			k = s.Len() - k + 1
		}
		x, err := pickAt(s, k-1)
		if err != nil {
			return *err
		}
		return num(x[0])
	}
}

// percentileOf builds PERCENTILE.INC (exc false) and PERCENTILE.EXC,
// and with quart QUARTILE.INC and QUARTILE.EXC, whose argument counts
// quarters.
func percentileOf(exc, quart bool) func(Sorted, []Node, lookup) Value {
	return func(s Sorted, rest []Node, get lookup) Value {
		p, err := numArg(rest[0], get)
		if err != nil {
			return *err
		}
		if quart {
			q := math.Trunc(p)
			if q < 0 || q > 4 || exc && (q < 1 || q > 3) {
				return value.ErrNum
			}
			p = q / 4
		}
		return percentile(s, p, exc)
	}
}

// percentile interpolates between the numbers either side of rank p of
// s: (n-1)p counting from 0, or (n+1)p counting from 1 for the
// exclusive kind, which has no rank before the first or past the last.
func percentile(s Sorted, p float64, exc bool) Value {
	n := s.Len()
	if n == 0 || p < 0 || p > 1 || exc && (p <= 0 || p >= 1) {
		return value.ErrNum
	}
	h := float64(n-1) * p
	if exc {
		h = float64(n+1)*p - 1
		if h < 0 || h > float64(n-1) {
			return value.ErrNum
		}
	}
	lo := int(math.Floor(h))
	if lo >= n-1 {
		x, err := pickAt(s, n-1)
		if err != nil {
			return *err
		}
		return num(x[0])
	}
	x, err := pickAt(s, lo, lo+1)
	if err != nil {
		return *err
	}
	return num(x[0] + (h-float64(lo))*(x[1]-x[0]))
}
