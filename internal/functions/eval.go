package functions

import (
	"math"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Eval computes a formula where one value is wanted, reading the cells
// it refers to through get. A range where one value is wanted reads as
// the cell in the row or column last given to EvalAt (see intersect),
// and an array as its first entry.
func Eval(n Node, get *Reader) Value { return EvalAt(n, get, get.here) }

// EvalAt computes the formula in the cell at as one value, so that a
// range used where one value is wanted reads as the cell in at's row or
// column (implicit intersection, as Sheets and Excel do). Evaluations
// nest, one cell's formula reading another's through the same Reader, so
// the cell before is given back afterwards.
func EvalAt(n Node, get *Reader, at Addr) Value {
	v, _ := evalTop(n, get, at, false)
	return v
}

// EvalCell computes the formula in the cell at, as EvalAt, and the array
// it computes when that is several values: the cell shows the first, and
// the engine spills the array from it. A 1x1 array is one value. A range
// that is the whole formula is an array, as Sheets spills =B2:B9.
func EvalCell(n Node, get *Reader, at Addr) (Value, *Array) {
	if r, ok := n.(formula.Range); ok {
		n = formula.Call{Fn: arrayFormula, Args: []Node{r}}
	}
	return evalTop(n, get, at, true)
}

// evalTop evaluates a cell's formula with a fresh evaluation state,
// saving the state of the formula it nests in (a cell read while another
// is evaluated).
func evalTop(n Node, get *Reader, at Addr, arrays bool) (Value, *Array) {
	nested := get.nest > 0
	var saved evalState
	if nested {
		saved = get.evalState
	}
	get.evalState = evalState{here: at, wantArr: arrays}
	get.nest++
	v := eval(n, get)
	var a *Array
	if v.Kind == value.Array {
		v, a = get.first(v), get.arrayOf(v)
	}
	if get.nest--; get.nest == 0 && len(get.arena) > 0 {
		clear(get.arena)
		get.arena = get.arena[:0]
	}
	if nested {
		get.evalState = saved
	}
	return v, a
}

// Reset forgets the evaluations in progress, which the engine abandoned
// part way (its evaluate.go), so the next starts afresh.
func (rd *Reader) Reset() {
	rd.evalState, rd.nest = evalState{}, 0
	clear(rd.arena)
	rd.arena = rd.arena[:0]
}

func eval(n Node, get lookup) Value {
	switch n := n.(type) {
	case formula.Num:
		return num(n.V)
	case formula.Str:
		return Value{Kind: value.Text, Str: n.V}
	case formula.Bool:
		return boolean(n.V)
	case formula.Ref:
		return get.cell(n.Sheet, n.Addr)
	case formula.Name:
		return value.ErrName
	case formula.RefErr:
		return value.ErrRef
	case formula.Empty:
		return Value{}
	case formula.Range:
		if get.lift > 0 {
			return get.rangeArray(n.Sheet, n.Rect)
		}
		if a, ok := get.intersect(n.Rect); ok {
			return get.cell(n.Sheet, a)
		}
		return value.ErrValue
	case formula.Unary:
		return evalUnary(n, get, false)
	case decUnary:
		return evalUnary(formula.Unary(n), get, true)
	case formula.Binary:
		return evalBinary(n, get, false)
	case decBinary:
		return evalBinary(formula.Binary(n), get, true)
	case formula.Call:
		// In an array context a function of one value is mapped over the
		// arrays it's given (lift.go); otherwise its arguments are read as
		// it reads them, and what it returns is one value unless the
		// caller asked for an array.
		*get.depth++ // see the engine's evaluate.go; operators count in theirs
		f := funcOf(n)
		var v Value
		if get.lift > 0 && f.arrays != takesArrays {
			v = liftCall(f, n.Args, get)
		} else {
			want := get.wantArr
			get.wantArr = f.arrays != liftScalar
			v = f.call(n.Args, get)
			get.wantArr = want
		}
		*get.depth--
		if v.Kind == value.Array {
			return get.reduce(v)
		}
		return v
	}
	return get.reduce(evalOther(n, get))
}

// evalOther evaluates what formulas rarely hold: array literals, names
// LET and LAMBDA bind, LAMBDA calls, and arguments standing in for
// arrays being mapped over.
func evalOther(n Node, get lookup) Value {
	switch n := n.(type) {
	case formula.Array:
		return get.literal(n)
	case formula.Local:
		return get.local(n.Name)
	case formula.Invoke:
		return get.invoke(n)
	case liftArg:
		return n.one(get)
	}
	return value.ErrValue
}

// evalUnary computes a prefix operator or the postfix %; with dec, % is
// decimal (decimal.go).
func evalUnary(n formula.Unary, get lookup, dec bool) Value {
	*get.depth++
	x := eval(n.X, get)
	*get.depth--
	if x.Kind == value.Array {
		return get.reduce(get.elementwise(x, Value{}, func(x, _ Value) Value { return unaryOp(n.Op, x, dec) }))
	}
	return unaryOp(n.Op, x, dec)
}

func unaryOp(op string, x Value, dec bool) Value {
	if x.Kind == value.Error {
		return x
	}
	switch op {
	case "-":
		f, err := toNum(x)
		if err != nil {
			return *err
		}
		return num(-f)
	case "#NOT#":
		f, err := toNum(x)
		if err != nil {
			return *err
		}
		return boolean(f == 0)
	case "%":
		f, err := toNum(x)
		if err != nil {
			return *err
		}
		if dec {
			if v, ok := decArith("/", f, 100); ok {
				return v
			}
		}
		return num(f / 100)
	}
	return x // unary + is identity
}

// evalBinary computes a binary operator; with dec, arithmetic is decimal
// (decimal.go).
func evalBinary(n formula.Binary, get lookup, dec bool) Value {
	*get.depth++
	l, r := eval(n.L, get), eval(n.R, get)
	*get.depth--
	switch {
	case l.Kind == value.Array || r.Kind == value.Array:
		return get.reduce(get.elementwise(l, r, func(l, r Value) Value { return binaryOp(n.Op, l, r, dec) }))
	case l.Kind == value.Error:
		return l
	case r.Kind == value.Error:
		return r
	case !dec && l.Kind == value.Number && r.Kind == value.Number:
		// Arithmetic on numbers, the commonest case, at once.
		switch n.Op {
		case "+":
			return num(l.Num + r.Num)
		case "-":
			return num(l.Num - r.Num)
		case "*":
			return num(l.Num * r.Num)
		}
	}
	return binaryOp(n.Op, l, r, dec)
}

func binaryOp(op string, l, r Value, dec bool) Value {
	if l.Kind == value.Error {
		return l
	}
	if r.Kind == value.Error {
		return r
	}
	switch op {
	case "&":
		return Value{Kind: value.Text, Str: text(l) + text(r)}
	case "=", "<>", "<", ">", "<=", ">=":
		return boolean(cmpResult(op, compare(l, r)))
	}
	a, err := toNum(l)
	if err != nil {
		return *err
	}
	b, err := toNum(r)
	if err != nil {
		return *err
	}
	if dec {
		if v, ok := decArith(op, a, b); ok {
			return v
		}
	}
	switch op {
	case "+":
		return num(a + b)
	case "-":
		return num(a - b)
	case "*":
		return num(a * b)
	case "/":
		if b == 0 {
			return value.ErrDiv0
		}
		return num(a / b)
	case "^":
		return num(math.Pow(a, b))
	case "#AND#":
		return boolean(a != 0 && b != 0)
	case "#OR#":
		return boolean(a != 0 || b != 0)
	}
	return value.ErrValue
}

// intersect is the cell a range stands for where one value is wanted
// (Intersect), in the formula being evaluated.
func (rd *Reader) intersect(r Rect) (Addr, bool) {
	return Intersect(r, rd.here)
}

// Intersect is the cell of r a formula in the cell at reads where it
// wants one value: a single cell is itself; a single column gives the
// cell in at's row, a single row the cell in at's column, when that lies
// within r (implicit intersection, on whatever sheet r is on). Any other
// range has no such cell, which is #VALUE!.
func Intersect(r Rect, at Addr) (Addr, bool) {
	switch {
	case r.From == r.To:
		return r.From, true
	case r.From.Col == r.To.Col && at.Row >= r.From.Row && at.Row <= r.To.Row:
		return Addr{Col: r.From.Col, Row: at.Row}, true
	case r.From.Row == r.To.Row && at.Col >= r.From.Col && at.Col <= r.To.Col:
		return Addr{Col: at.Col, Row: r.From.Row}, true
	}
	return Addr{}, false
}
