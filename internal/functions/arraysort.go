package functions

import (
	"slices"

	"github.com/FelineStateMachine/012/internal/value"
)

// Sorting and comparing the rows of arrays, for SORT, SORTN and UNIQUE
// (dynamic.go).

// sortKey is a column to sort by and its direction.
type sortKey struct {
	col  *Array // one column, aligned with the rows
	desc bool
}

// sortKeys reads SORT's (sort_column, is_ascending) pairs from args[i]
// on: a column by position in a, or a range of its own, aligned with it.
// With none, the first column goes up.
func sortKeys(a *Array, args []Node, i int, get lookup) ([]sortKey, *Value) {
	if !given(args, i) && len(args) <= i+1 {
		return []sortKey{{col: a.column(0)}}, nil
	}
	var keys []sortKey
	for ; i < len(args); i += 2 {
		k, err := sortKeyArg(a, args[i], get)
		if err != nil {
			return nil, err
		}
		asc, err := boolArg(args, i+1, true, get)
		if err != nil {
			return nil, err
		}
		k.desc = !asc
		keys = append(keys, k)
	}
	return keys, nil
}

func sortKeyArg(a *Array, n Node, get lookup) (sortKey, *Value) {
	col, err := arrayArg(n, get)
	switch {
	case err != nil:
		return sortKey{}, err
	case col.Rows == 1 && col.Cols == 1:
		i, err := toNum(col.At(0, 0))
		if err != nil {
			return sortKey{}, err
		}
		if i < 1 || int(i) > a.Cols {
			return sortKey{}, &value.ErrValue
		}
		return sortKey{col: a.column(int(i) - 1)}, nil
	case col.Cols != 1 || col.Rows != a.Rows:
		return sortKey{}, &value.ErrValue
	}
	return sortKey{col: col}, nil
}

// column is column c of a, as an array of one column.
func (a *Array) column(c int) *Array {
	out := &Array{Rows: a.Rows, Cols: 1, Fill: a.Fill}
	if c < a.DCols {
		out.DRows, out.DCols = a.DRows, 1
		out.V = make([]Value, a.DRows)
		for r := range a.DRows {
			out.V[r] = a.At(r, c)
		}
	}
	return out
}

// sortOrder orders rows 0..n-1 by keys, stably: numbers, then text, then
// booleans, ascending or not, and blanks last either way, as Sheets
// sorts.
func sortOrder(n int, keys []sortKey) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int {
		for _, k := range keys {
			if d := sortCompare(k.col.At(x, 0), k.col.At(y, 0), k.desc); d != 0 {
				return d
			}
		}
		return 0
	})
	return order
}

// sortCompare orders two values for sorting, blanks last.
func sortCompare(a, b Value, desc bool) int {
	ab, bb := a.Kind == value.Empty, b.Kind == value.Empty
	switch {
	case ab || bb:
		return boolOrder(ab) - boolOrder(bb)
	case a.Kind == value.Error || b.Kind == value.Error:
		return boolOrder(a.Kind == value.Error) - boolOrder(b.Kind == value.Error)
	}
	d := compare(a, b)
	if desc {
		d = -d
	}
	return d
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

// sortedData sorts a's rows that hold data (every row past them is
// blank, and blanks sort last), keeping the blank rows as its fill.
func sortedData(a *Array, keys []sortKey) *Array {
	data := a.DRows
	for _, k := range keys {
		data = max(data, k.col.DRows)
	}
	data = min(data, a.Rows)
	out := a.rows(sortOrder(data, keys))
	out.Rows, out.Fill = a.Rows, a.Fill
	return out
}

// topRows picks SORTN's rows of sorted, by its display_ties_mode: 0 the
// first n; 1 the first n and any tied with the last; 2 the first n
// distinct rows; 3 the rows of the first n distinct keys.
func topRows(sorted *Array, keys []sortKey, n, mode int) []int {
	keyed := sortedKeys(sorted, keys)
	var out []int
	distinct := 0
	for r := range sorted.Rows {
		switch mode {
		case 0:
			if len(out) == n {
				return out
			}
		case 1:
			if len(out) >= n && !sameRow(keyed, r, out[len(out)-1]) {
				return out
			}
		case 2:
			if slices.ContainsFunc(out, func(p int) bool { return sameRow(sorted, r, p) }) {
				continue
			}
			if len(out) == n {
				return out
			}
		case 3:
			if len(out) == 0 || !sameRow(keyed, r, out[len(out)-1]) {
				distinct++
			}
			if distinct > n {
				return out
			}
		}
		out = append(out, r)
	}
	return out
}

// sortedKeys is the sort key columns of sorted's rows side by side, for
// comparing rows by their keys.
func sortedKeys(sorted *Array, keys []sortKey) *Array {
	out := NewArray(sorted.Rows, len(keys))
	for i, k := range keys {
		for r := range sorted.Rows {
			out.Set(r, i, k.col.At(r, 0))
		}
	}
	return out
}

// sameRow reports whether rows x and y of a hold the same values: text
// compared exactly, as UNIQUE does.
func sameRow(a *Array, x, y int) bool {
	for c := range a.Cols {
		if !sameValue(a.At(x, c), a.At(y, c)) {
			return false
		}
	}
	return true
}

// sameValue reports whether two values are the same for UNIQUE: of one
// kind and equal, text case included, as Sheets compares them.
func sameValue(a, b Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind == value.Text || a.Kind == value.Error {
		return a.Str == b.Str
	}
	return a.Num == b.Num
}
