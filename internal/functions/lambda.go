package functions

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// LET names values within a formula, and LAMBDA makes a function of
// names, called with arguments right away (LAMBDA(x, x*2)(3)), through a
// name LET gave it, or by MAP, REDUCE, BYROW, BYCOL, SCAN and MAKEARRAY
// for each entry of an array. The parser turns the names into Local
// nodes (formula), so named ranges never replace them.

func init() {
	define(
		&FuncDef{Name: "LET", Args: "name1, value_expression1, [name2, value_expression2, ...], formula_expression", Desc: "Name values for use in a formula; an error counts only where its name is used", Min: 3, Max: -1, step: 2,
			eval: let, arrays: takesArrays, binds: formula.BindLet, format: func(args []Node, infer func(Node) Format) Format { return infer(args[len(args)-1]) }},
		&FuncDef{Name: "LAMBDA", Args: "[name, ...], formula_expression", Desc: "A function of names, called with values: LAMBDA(x, x*2)(3)", Min: 1, Max: -1,
			eval: makeLambda, arrays: takesArrays, binds: formula.BindLambda},
		&FuncDef{Name: "MAP", Args: "array1, [array2, ...], LAMBDA", Desc: "Each entry of arrays passed to a LAMBDA", Min: 2, Max: -1,
			eval: mapArrays, arrays: takesArrays},
		&FuncDef{Name: "REDUCE", Args: "initial_value, array_or_range, LAMBDA", Desc: "An array folded into one value by a LAMBDA of the total so far and each entry", Min: 3, Max: 3,
			eval: func(args []Node, get lookup) Value { return fold(args, get, false) }, arrays: takesArrays},
		&FuncDef{Name: "SCAN", Args: "initial_value, array_or_range, LAMBDA", Desc: "The running totals of REDUCE, one for each entry", Min: 3, Max: 3,
			eval: func(args []Node, get lookup) Value { return fold(args, get, true) }, arrays: takesArrays},
		&FuncDef{Name: "BYROW", Args: "array_or_range, LAMBDA", Desc: "Each row of an array passed to a LAMBDA, one value per row", Min: 2, Max: 2,
			eval: func(args []Node, get lookup) Value { return byLine(args, get, true) }, arrays: takesArrays},
		&FuncDef{Name: "BYCOL", Args: "array_or_range, LAMBDA", Desc: "Each column of an array passed to a LAMBDA, one value per column", Min: 2, Max: 2,
			eval: func(args []Node, get lookup) Value { return byLine(args, get, false) }, arrays: takesArrays},
		&FuncDef{Name: "MAKEARRAY", Args: "rows, columns, LAMBDA", Desc: "An array of a size, each entry a LAMBDA of its row and column", Min: 3, Max: 3,
			eval: makeArray, arrays: takesArrays},
	)
}

func let(args []Node, get lookup) Value {
	outer := len(get.scope)
	defer func() { get.scope = get.scope[:outer] }()
	last := len(args) - 1
	for i := 0; i < last; i += 2 {
		// An error is the result only where a name bound to it is used:
		// LET(x, 1/0, 5) is 5, as in Sheets.
		get.scope = append(get.scope, get.bind(args[i].(formula.Local).Name, args[i+1]))
	}
	return eval(args[last], get)
}

func makeLambda(args []Node, get lookup) Value {
	l := &lambda{body: args[len(args)-1], scope: append([]binding(nil), get.scope...)}
	for _, a := range args[:len(args)-1] {
		l.params = append(l.params, a.(formula.Local).Name)
	}
	return get.handle(l)
}

func mapArrays(args []Node, get lookup) Value {
	last := len(args) - 1
	l, err := lambdaArg(args[last], get, last)
	if err != nil {
		return *err
	}
	arrs := make([]*Array, last)
	var s shape
	for i, a := range args[:last] {
		if arrs[i], err = arrayArg(a, get); err != nil {
			return *err
		}
		s.fit(arrs[i])
	}
	s.drows, s.dcols = s.rows, s.cols
	out, ok := s.newArray()
	if !ok {
		return value.ErrValue
	}
	vals := make([]Value, last)
	for r := range s.rows {
		for c := range s.cols {
			for i, a := range arrs {
				vals[i] = a.bcast(r, c)
			}
			out.Set(r, c, get.single(get.call(l, vals...)))
		}
	}
	return get.arrayValue(out)
}

// fold is REDUCE, or SCAN with running: the LAMBDA called with the total
// so far and each entry, row by row.
func fold(args []Node, get lookup, running bool) Value {
	acc := eval1(args[0], get)
	a, err := arrayArg(args[1], get)
	if err != nil {
		return *err
	}
	l, err := lambdaArg(args[2], get, 2)
	if err != nil {
		return *err
	}
	if tooBig(a.Rows, a.Cols) {
		return value.ErrValue
	}
	var out *Array
	if running {
		out = NewArray(a.Rows, a.Cols)
	}
	for r := range a.Rows {
		for c := range a.Cols {
			acc = get.call(l, acc, a.At(r, c))
			if running {
				acc = get.single(acc)
				out.Set(r, c, acc)
			}
		}
	}
	if running {
		return get.arrayValue(out)
	}
	return acc
}

// byLine is BYROW (rows) or BYCOL: the LAMBDA called with each line of
// the array, one value for each.
func byLine(args []Node, get lookup, rows bool) Value {
	a, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	l, err := lambdaArg(args[1], get, 1)
	if err != nil {
		return *err
	}
	n, width := a.Rows, a.Cols
	if !rows {
		n, width = a.Cols, a.Rows
	}
	if tooBig(n, width) {
		return value.ErrValue
	}
	out := NewArray(n, 1)
	if !rows {
		out = NewArray(1, n)
	}
	for i := range n {
		line := NewArray(1, width)
		if !rows {
			line = NewArray(width, 1)
		}
		for j := range width {
			if rows {
				line.Set(0, j, a.At(i, j))
			} else {
				line.Set(j, 0, a.At(j, i))
			}
		}
		out.V[i] = get.single(get.call(l, get.arrayValue(line)))
	}
	return get.arrayValue(out)
}

func makeArray(args []Node, get lookup) Value {
	rows, err := intArg(args, 0, 0, get)
	if err != nil {
		return *err
	}
	cols, err := intArg(args, 1, 0, get)
	if err != nil {
		return *err
	}
	l, err := lambdaArg(args[2], get, 2)
	if err != nil {
		return *err
	}
	if rows < 1 || cols < 1 || tooBig(rows, cols) {
		return value.ErrValue
	}
	out := NewArray(rows, cols)
	for r := range rows {
		for c := range cols {
			out.Set(r, c, get.single(get.call(l, num(float64(r+1)), num(float64(c+1)))))
		}
	}
	return get.arrayValue(out)
}
