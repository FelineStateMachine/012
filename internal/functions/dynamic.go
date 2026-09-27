package functions

import (
	"strconv"

	"github.com/FelineStateMachine/012/internal/value"
)

// Functions that compute arrays, which spill from the cell into the
// cells to its right and below. Whole columns cost what they hold: FILTER
// and SORT of A:A work on its rows holding data, the blank rows past
// them being one fill (Array.Fill).

func init() {
	define(
		&FuncDef{Name: "ARRAYFORMULA", Args: "array_formula", Desc: "Compute a formula over arrays: ranges read whole, and functions of one value applied to each entry", Min: 1, Max: 1,
			eval: func(args []Node, get lookup) Value { return evalArr(args[0], get) }, arrays: takesArrays, format: inherit},
		&FuncDef{Name: "FILTER", Args: "range, condition1, [condition2, ...]", Desc: "The rows (or columns) of a range where every condition is true", Min: 2, Max: -1,
			eval: filter, arrays: takesArrays, format: inheritFrom(0)},
		&FuncDef{Name: "SORT", Args: "range, [sort_column], [is_ascending], [sort_column2, is_ascending2, ...]", Desc: "The rows of a range sorted by columns", Min: 1, Max: -1,
			eval: sortRows, arrays: takesArrays, format: inheritFrom(0)},
		&FuncDef{Name: "SORTN", Args: "range, [n], [display_ties_mode], [sort_column1, is_ascending1, ...]", Desc: "The first n rows of a range after sorting", Min: 1, Max: -1,
			eval: sortN, arrays: takesArrays, format: inheritFrom(0)},
		&FuncDef{Name: "UNIQUE", Args: "range, [by_column], [exactly_once]", Desc: "The distinct rows (or columns) of a range, in order", Min: 1, Max: 3,
			eval: unique, arrays: takesArrays, format: inheritFrom(0)},
		&FuncDef{Name: "SEQUENCE", Args: "rows, [columns], [start], [step]", Desc: "An array of numbers counting up from start by step", Min: 1, Max: 4,
			eval: sequence, arrays: takesArrays},
		&FuncDef{Name: "TRANSPOSE", Args: "array_or_range", Desc: "Rows as columns and columns as rows", Min: 1, Max: 1,
			eval: transpose, arrays: takesArrays, format: inheritFrom(0)},
		&FuncDef{Name: "FLATTEN", Args: "range1, [range2, ...]", Desc: "Every entry of ranges in one column, row by row", Min: 1, Max: -1,
			eval: flatten, arrays: takesArrays, format: inheritFrom(0)},
		&FuncDef{Name: "CHOOSECOLS", Args: "array, col_num1, [col_num2, ...]", Desc: "Columns of an array by position, negative from the end", Min: 2, Max: -1,
			eval: func(args []Node, get lookup) Value { return choose2(args, get, false) }, arrays: takesArrays, format: inheritFrom(0)},
		&FuncDef{Name: "CHOOSEROWS", Args: "array, row_num1, [row_num2, ...]", Desc: "Rows of an array by position, negative from the end", Min: 2, Max: -1,
			eval: func(args []Node, get lookup) Value { return choose2(args, get, true) }, arrays: takesArrays, format: inheritFrom(0)},
	)
}

// Evaluators for the table above, in its order.

func filter(args []Node, get lookup) Value {
	a, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	conds := make([]*Array, len(args)-1)
	for i, n := range args[1:] {
		if conds[i], err = arrayArg(n, get); err != nil {
			return *err
		}
	}
	byRow := conds[0].Cols == 1 && conds[0].Rows == a.Rows
	if !byRow && (conds[0].Rows != 1 || conds[0].Cols != a.Cols) {
		return value.ErrValue // FILTER has mismatched range sizes
	}
	if !byRow {
		a = a.transposed()
		for i, c := range conds {
			conds[i] = c.transposed()
		}
	}
	for _, c := range conds {
		if c.Rows != a.Rows || c.Cols != 1 {
			return value.ErrValue
		}
	}
	data := a.DRows
	for _, c := range conds {
		data = max(data, c.DRows)
	}
	keep, err := passing(conds, data)
	if err != nil {
		return *err
	}
	out := a.rows(keep)
	if a.Rows > data { // the rows past every data area: one fill
		if ok, err := passes1(conds, -1); err != nil {
			return *err
		} else if ok {
			out.Rows += a.Rows - data
			out.Fill = a.Fill
		}
	}
	if out.Rows == 0 {
		return value.ErrNA // no matches
	}
	if !byRow {
		out = out.transposed()
	}
	return get.arrayValue(out)
}

// passing is the rows below data where every condition is true.
func passing(conds []*Array, data int) ([]int, *Value) {
	var keep []int
	for r := range data {
		ok, err := passes1(conds, r)
		if err != nil {
			return nil, err
		}
		if ok {
			keep = append(keep, r)
		}
	}
	return keep, nil
}

// passes1 reports whether every condition is true in row r, or past
// their data with r < 0.
func passes1(conds []*Array, r int) (bool, *Value) {
	for _, c := range conds {
		v := c.Fill
		if r >= 0 {
			v = c.At(r, 0)
		}
		if v.Kind == value.Error {
			return false, errOf(v)
		}
		f, err := toNum(v)
		if err != nil {
			return false, err
		}
		if f == 0 {
			return false, nil
		}
	}
	return true, nil
}

// rows is a new array of the rows of a at indexes, as wide as a and
// holding as many columns of data.
func (a *Array) rows(indexes []int) *Array {
	out := &Array{Rows: len(indexes), Cols: a.Cols, DRows: len(indexes), DCols: a.DCols, Fill: a.Fill}
	out.V = make([]Value, len(indexes)*a.DCols)
	for i, r := range indexes {
		for c := range a.DCols {
			out.Set(i, c, a.At(r, c))
		}
	}
	return out
}

// transposed is a with rows and columns swapped.
func (a *Array) transposed() *Array {
	out := &Array{Rows: a.Cols, Cols: a.Rows, DRows: a.DCols, DCols: a.DRows, Fill: a.Fill, V: make([]Value, len(a.V))}
	for r := range a.DRows {
		for c := range a.DCols {
			out.Set(c, r, a.At(r, c))
		}
	}
	return out
}

func sortRows(args []Node, get lookup) Value {
	a, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	keys, err := sortKeys(a, args, 1, get)
	if err != nil {
		return *err
	}
	return get.arrayValue(sortedData(a, keys))
}

func sortN(args []Node, get lookup) Value {
	a, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	n, err := intArg(args, 1, 1, get)
	if err != nil {
		return *err
	}
	mode, err := intArg(args, 2, 0, get)
	if err != nil {
		return *err
	}
	if n < 0 || mode < 0 || mode > 3 {
		return value.ErrValue
	}
	keys, err := sortKeys(a, args, 3, get)
	if err != nil {
		return *err
	}
	sorted := sortedData(a, keys)
	sorted.Rows = sorted.DRows // rows past the data are blank: never among the first
	rows := topRows(sorted, keys, n, mode)
	if len(rows) == 0 {
		return value.ErrNA
	}
	return get.arrayValue(sorted.rows(rows))
}

func unique(args []Node, get lookup) Value {
	a, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	byCol, err := boolArg(args, 1, false, get)
	if err != nil {
		return *err
	}
	once, err := boolArg(args, 2, false, get)
	if err != nil {
		return *err
	}
	if byCol {
		a = a.transposed()
	}
	n := min(a.DRows+1, a.Rows) // the data rows, and one blank row past them
	groups := map[string][]int{}
	var order []string
	for r := range n {
		k := rowKey(a, r)
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	var keep []int
	for _, k := range order {
		g := groups[k]
		if once && (len(g) > 1 || g[0] == a.DRows && a.Rows > a.DRows+1) {
			continue // repeated, or the blank row, which repeats past the data
		}
		keep = append(keep, g[0])
	}
	if len(keep) == 0 {
		return value.ErrNA
	}
	out := a.rows(keep)
	if byCol {
		out = out.transposed()
	}
	return get.arrayValue(out)
}

// rowKey is a key equal for rows UNIQUE takes to be the same.
func rowKey(a *Array, r int) string {
	var b []byte
	for c := range a.Cols {
		v := a.At(r, c)
		if v.Kind == value.Text && v.Str == "" {
			v = Value{} // empty text and a blank are the same
		}
		b = append(b, byte(v.Kind))
		if v.Kind == value.Text || v.Kind == value.Error {
			b = append(b, v.Str...)
		} else {
			b = strconv.AppendFloat(b, v.Num, 'g', -1, 64)
		}
		b = append(b, 0)
	}
	return string(b)
}

func sequence(args []Node, get lookup) Value {
	rows, err := intArg(args, 0, 1, get)
	if err != nil {
		return *err
	}
	cols, err := intArg(args, 1, 1, get)
	if err != nil {
		return *err
	}
	start, err := optNum(args, 2, 1, get)
	if err != nil {
		return *err
	}
	step, err := optNum(args, 3, 1, get)
	if err != nil {
		return *err
	}
	if rows < 1 || cols < 1 || tooBig(rows, cols) {
		return value.ErrValue
	}
	out := NewArray(rows, cols)
	for i := range out.V {
		out.V[i] = num(start + float64(i)*step)
	}
	return get.arrayValue(out)
}

func transpose(args []Node, get lookup) Value {
	a, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	return get.arrayValue(a.transposed())
}

func flatten(args []Node, get lookup) Value {
	var vals []Value
	var fill Value
	extra := 0
	for i, n := range args {
		a, err := arrayArg(n, get)
		if err != nil {
			return *err
		}
		rows := a.Rows
		if i == len(args)-1 {
			rows = a.DRows // the rest is one fill
			extra, fill = (a.Rows-a.DRows)*a.Cols, a.Fill
		}
		if tooBig(len(vals)+rows, a.Cols) {
			return value.ErrValue
		}
		for r := range rows {
			for c := range a.Cols {
				vals = append(vals, a.At(r, c))
			}
		}
	}
	out := &Array{Rows: len(vals) + extra, Cols: 1, DRows: len(vals), DCols: 1, V: vals, Fill: fill}
	return get.arrayValue(out)
}

// choose2 is CHOOSEROWS (rows) or CHOOSECOLS.
func choose2(args []Node, get lookup, rows bool) Value {
	a, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	if !rows {
		a = a.transposed()
	}
	var pick []int
	for i := 1; i < len(args); i++ {
		v, err := arrayArg(args[i], get)
		if err != nil {
			return *err
		}
		for _, x := range v.V {
			f, err := toNum(x)
			if err != nil {
				return *err
			}
			k := int(f)
			if k < 0 {
				k = a.Rows + k + 1
			}
			if k < 1 || k > a.Rows {
				return value.ErrValue
			}
			pick = append(pick, k-1)
		}
	}
	out := a.rows(pick)
	if !rows {
		out = out.transposed()
	}
	return get.arrayValue(out)
}
