package sheet

import (
	"iter"
	"slices"
	"strings"
)

func init() {
	define(
		&FuncDef{Name: "VLOOKUP", Args: "search_key, range, index, [is_sorted]", Desc: "Find a key in the first column and return a value from its row", Min: 3, Max: 4,
			eval: func(args []Node, get lookup) Value { return tableLookup(args, get, true) }},
		&FuncDef{Name: "HLOOKUP", Args: "search_key, range, index, [is_sorted]", Desc: "Find a key in the first row and return a value from its column", Min: 3, Max: 4,
			eval: func(args []Node, get lookup) Value { return tableLookup(args, get, false) }},
		&FuncDef{Name: "MATCH", Args: "search_key, range, [search_type]", Desc: "Position of a key in a row or column", Min: 2, Max: 3,
			eval: match},
		&FuncDef{Name: "INDEX", Args: "reference, [row], [column]", Desc: "The value at a row and column of a range", Min: 1, Max: 3,
			eval: index, format: inheritFrom(0)},
		&FuncDef{Name: "XLOOKUP", Args: "search_key, lookup_range, result_range, [missing_value], [match_mode], [search_mode]", Desc: "Find a key and return the matching entry of another range", Min: 3, Max: 6,
			eval: xlookup, format: inheritFrom(2)},
		&FuncDef{Name: "CHOOSE", Args: "index, choice1, [choice2, ...]", Desc: "The choice at a position", Min: 2, Max: -1,
			eval: choose},
		&FuncDef{Name: "ROWS", Args: "range", Desc: "Number of rows in a range", Min: 1, Max: 1,
			eval: func(args []Node, get lookup) Value { return num(float64(matrixArg(args[0], get).rows)) }},
		&FuncDef{Name: "COLUMNS", Args: "range", Desc: "Number of columns in a range", Min: 1, Max: 1,
			eval: func(args []Node, get lookup) Value { return num(float64(matrixArg(args[0], get).cols)) }},
	)
}

// Evaluators for the table above, in its order.

func match(args []Node, get lookup) Value {
	key := eval(args[0], get)
	if key.Kind == Error {
		return key
	}
	m := matrixArg(args[1], get)
	kind, err := intArg(args, 2, 1, get)
	switch {
	case err != nil:
		return *err
	case !m.vector():
		return ErrNA
	}
	q := lineSeq(m, get, m.cols == 1)
	var i int
	switch {
	case kind == 0:
		i = q.findExact(key, true)
	case kind > 0:
		i = q.findSorted(key, 1)
	default:
		i = q.findSorted(key, -1)
	}
	if i < 0 {
		return ErrNA
	}
	return num(float64(i + 1))
}

func index(args []Node, get lookup) Value {
	m := matrixArg(args[0], get)
	row, err := intArg(args, 1, 0, get)
	if err != nil {
		return *err
	}
	col, err := intArg(args, 2, 0, get)
	if err != nil {
		return *err
	}
	// In a single row, a lone index counts along the row.
	if m.rows == 1 && len(args) == 2 {
		row, col = 1, row
	}
	if row < 0 || col < 0 {
		return ErrValue
	}
	if row == 0 && m.rows == 1 {
		row = 1
	}
	if col == 0 && m.cols == 1 {
		col = 1
	}
	switch {
	case row == 0 || col == 0:
		return ErrValue // a whole row or column: an array we can't show
	case row > m.rows || col > m.cols:
		return ErrRef
	}
	return m.cell(row-1, col-1)
}

func choose(args []Node, get lookup) Value {
	i, err := intArg(args, 0, 0, get)
	if err != nil {
		return *err
	}
	if i < 1 || i >= len(args) {
		return ErrValue
	}
	return eval(args[i], get)
}

// tableLookup is VLOOKUP (vertical) and HLOOKUP: search the first column
// (or row), then return the index-th entry across.
func tableLookup(args []Node, get lookup, vertical bool) Value {
	key := eval(args[0], get)
	if key.Kind == Error {
		return key
	}
	m := matrixArg(args[1], get)
	idx, err := intArg(args, 2, 0, get)
	if err != nil {
		return *err
	}
	sorted, err := boolArg(args, 3, true, get)
	if err != nil {
		return *err
	}
	q := lineSeq(m, get, vertical)
	across := m.cols
	if !vertical {
		across = m.rows
	}
	switch {
	case idx < 1:
		return ErrValue
	case idx > across:
		return ErrRef
	}
	var i int
	if sorted {
		i = q.findSorted(key, 1)
	} else {
		i = q.findExact(key, true)
	}
	if i < 0 {
		return ErrNA
	}
	if vertical {
		return m.cell(i, idx-1)
	}
	return m.cell(idx-1, i)
}

// lookupEqual reports whether a lookup key matches v: same kind, text
// case-insensitive, with wildcards when allowed.
func lookupEqual(key, v Value, wild bool) bool {
	if key.Kind != v.Kind {
		return false
	}
	if key.Kind == Text {
		if wild && hasWildcards(key.Str) {
			return wildMatch(key.Str, v.Str)
		}
		return strings.EqualFold(key.Str, v.Str)
	}
	return key.Num == v.Num
}

// seq is the entries of a row or column searched by a lookup: n of them,
// of which only the first data may hold data. With sparse, only those in
// stored hold anything; every other entry is blank.
type seq struct {
	n, data int
	at      func(int) Value
	blank   Value
	stored  iter.Seq[int] // the entries holding cells, in order, when sparse
	sparse  bool
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
	q := seq{n: line.size(), data: min(line.dataLen(), line.size()), at: line.at, blank: line.blank}
	t := get.sheet(line.sheet)
	switch {
	case !line.ref:
		q.sparse, q.stored = true, func(yield func(int) bool) { yield(0) }
	case denseReads:
	case t == nil:
		q.sparse, q.stored = true, func(func(int) bool) {}
	default:
		q.sparse = true
		r := Rect{From: line.origin, To: Addr{Col: line.origin.Col + line.cols - 1, Row: line.origin.Row + line.rows - 1}}
		q.stored = func(yield func(int) bool) {
			if vertical {
				t.cells.colScan(r.From.Col, r.From.Row, r.To.Row, func(row int) bool { return yield(row - r.From.Row) })
				return
			}
			for a := range t.cells.inRange(r) {
				if !yield(a.Col - r.From.Col) {
					return
				}
			}
		}
	}
	return q
}

// findExact returns the first index whose value equals key, or -1.
func (q seq) findExact(key Value, wild bool) int {
	if q.sparse {
		blank := lookupEqual(key, q.blank, wild)
		next := 0 // the first entry not looked at
		for i := range q.stored {
			if i > next && blank {
				return next // a blank before this cell
			}
			if lookupEqual(key, q.at(i), wild) {
				return i
			}
			next = i + 1
		}
		if blank && next < q.n {
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
func (q seq) findSorted(key Value, dir int) int {
	found := -1
	if q.sparse && key.Kind != q.blank.Kind { // blanks are skipped: only the cells count
		for i := range q.stored {
			v := q.at(i)
			if v.Kind != key.Kind {
				continue
			}
			if compare(v, key)*dir > 0 {
				return found
			}
			found = i
		}
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
func (q seq) order(reverse bool) []int {
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
	out = slices.Collect(q.stored)
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

func xlookup(args []Node, get lookup) Value {
	key := eval(args[0], get)
	if key.Kind == Error {
		return key
	}
	look := matrixArg(args[1], get)
	res := matrixArg(args[2], get)
	mode, err := intArg(args, 4, 0, get)
	if err != nil {
		return *err
	}
	search, err := intArg(args, 5, 1, get)
	if err != nil {
		return *err
	}
	switch {
	case !look.vector():
		return ErrValue
	case mode < -1 || mode > 2 || search == 0 || search < -2 || search > 2:
		return ErrValue
	}
	// The result range lines up with the lookup range along its length.
	n := look.size()
	var pick func(i int) Value
	switch {
	case look.cols == 1 && res.rows == n:
		pick = func(i int) Value { return res.cell(i, 0) }
	case look.rows == 1 && res.cols == n:
		pick = func(i int) Value { return res.cell(0, i) }
	default:
		return ErrValue
	}
	order := lineSeq(look, get, look.cols == 1).order(search < 0)
	best := -1
	for _, i := range order {
		v := look.at(i)
		if lookupEqual(key, v, mode == 2) {
			best = i
			break
		}
		if mode == 0 || mode == 2 || v.Kind != key.Kind {
			continue
		}
		// Next smaller (-1) or next larger (1): keep the closest.
		c := compare(v, key)
		if c*mode > 0 && (best < 0 || compare(v, look.at(best))*mode < 0) {
			best = i
		}
	}
	if best < 0 {
		if given(args, 3) {
			return eval(args[3], get)
		}
		return ErrNA
	}
	return pick(best)
}
