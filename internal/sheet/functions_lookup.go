package sheet

import (
	"strings"
)

func init() {
	define(
		&FuncDef{Name: "VLOOKUP", Args: "search_key, range, index, [is_sorted]", Desc: "Find a key in the first column and return a value from its row", Min: 3, Max: 4,
			eval: func(args []Node, get lookup) Value { return tableLookup(args, get, true) }},
		&FuncDef{Name: "HLOOKUP", Args: "search_key, range, index, [is_sorted]", Desc: "Find a key in the first row and return a value from its column", Min: 3, Max: 4,
			eval: func(args []Node, get lookup) Value { return tableLookup(args, get, false) }},
		&FuncDef{Name: "MATCH", Args: "search_key, range, [search_type]", Desc: "Position of a key in a row or column", Min: 2, Max: 3,
			eval: func(args []Node, get lookup) Value {
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
				var i int
				switch {
				case kind == 0:
					i = findExact(key, m.size(), m.at, true)
				case kind > 0:
					i = findSorted(key, m.size(), m.at, 1)
				default:
					i = findSorted(key, m.size(), m.at, -1)
				}
				if i < 0 {
					return ErrNA
				}
				return num(float64(i + 1))
			}},
		&FuncDef{Name: "INDEX", Args: "reference, [row], [column]", Desc: "The value at a row and column of a range", Min: 1, Max: 3,
			eval: func(args []Node, get lookup) Value {
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
			}, format: inheritFrom(0)},
		&FuncDef{Name: "XLOOKUP", Args: "search_key, lookup_range, result_range, [missing_value], [match_mode], [search_mode]", Desc: "Find a key and return the matching entry of another range", Min: 3, Max: 6,
			eval: xlookup, format: inheritFrom(2)},
		&FuncDef{Name: "CHOOSE", Args: "index, choice1, [choice2, ...]", Desc: "The choice at a position", Min: 2, Max: -1,
			eval: func(args []Node, get lookup) Value {
				i, err := intArg(args, 0, 0, get)
				if err != nil {
					return *err
				}
				if i < 1 || i >= len(args) {
					return ErrValue
				}
				return eval(args[i], get)
			}},
		&FuncDef{Name: "ROWS", Args: "range", Desc: "Number of rows in a range", Min: 1, Max: 1,
			eval: func(args []Node, get lookup) Value { return num(float64(matrixArg(args[0], get).rows)) }},
		&FuncDef{Name: "COLUMNS", Args: "range", Desc: "Number of columns in a range", Min: 1, Max: 1,
			eval: func(args []Node, get lookup) Value { return num(float64(matrixArg(args[0], get).cols)) }},
	)
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
	n, across := m.rows, m.cols
	at := func(i int) Value { return m.cell(i, 0) }
	if !vertical {
		n, across = m.cols, m.rows
		at = func(i int) Value { return m.cell(0, i) }
	}
	switch {
	case idx < 1:
		return ErrValue
	case idx > across:
		return ErrRef
	}
	var i int
	if sorted {
		i = findSorted(key, n, at, 1)
	} else {
		i = findExact(key, n, at, true)
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

// findExact returns the first index whose value equals key, or -1.
func findExact(key Value, n int, at func(int) Value, wild bool) int {
	for i := range n {
		if lookupEqual(key, at(i), wild) {
			return i
		}
	}
	return -1
}

// findSorted is the approximate match of sorted lookups: with dir 1 the
// last value <= key in ascending data, with dir -1 the last value >= key
// in descending data. Values of another kind are skipped.
func findSorted(key Value, n int, at func(int) Value, dir int) int {
	found := -1
	for i := range n {
		v := at(i)
		if v.Kind != key.Kind {
			continue
		}
		if c := compare(v, key) * dir; c > 0 {
			break
		}
		found = i
	}
	return found
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
	order := make([]int, n)
	for i := range order {
		order[i] = i
		if search < 0 {
			order[i] = n - 1 - i
		}
	}
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
