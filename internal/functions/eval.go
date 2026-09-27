package functions

import (
	"math"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Eval computes a formula, reading the cells it refers to through get.
// A range where one value is wanted reads as the cell in the row or
// column last given to EvalAt (see intersect).
func Eval(n Node, get *Reader) Value { return eval(n, get) }

// EvalAt computes the formula in the cell at, so that a range used where
// one value is wanted reads as the cell in at's row or column (implicit
// intersection, as Sheets and Excel do). Evaluations nest, one cell's
// formula reading another's through the same Reader, so the cell before
// is given back afterwards.
func EvalAt(n Node, get *Reader, at Addr) Value {
	outer := get.here
	get.here = at
	v := eval(n, get)
	get.here = outer
	return v
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
		*get.depth++ // see the engine's evaluate.go; operators count in theirs
		arrays := get.arrays
		get.arrays = 0 // a function's own arguments take one value again
		v := funcOf(n).call(n.Args, get)
		get.arrays = arrays
		*get.depth--
		return v
	}
	return value.ErrValue
}

// evalUnary computes a prefix operator or the postfix %; with dec, % is
// decimal (decimal.go).
func evalUnary(n formula.Unary, get lookup, dec bool) Value {
	*get.depth++
	x := eval(n.X, get)
	*get.depth--
	if x.Kind == value.Error {
		return x
	}
	switch n.Op {
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
	if l.Kind == value.Error {
		return l
	}
	if r.Kind == value.Error {
		return r
	}
	switch n.Op {
	case "&":
		return Value{Kind: value.Text, Str: text(l) + text(r)}
	case "=", "<>", "<", ">", "<=", ">=":
		c := compare(l, r)
		switch n.Op {
		case "=":
			return boolean(c == 0)
		case "<>":
			return boolean(c != 0)
		case "<":
			return boolean(c < 0)
		case ">":
			return boolean(c > 0)
		case "<=":
			return boolean(c <= 0)
		}
		return boolean(c >= 0)
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
		if v, ok := decArith(n.Op, a, b); ok {
			return v
		}
	}
	switch n.Op {
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
// (Intersect), in the formula being evaluated. Inside an argument that
// takes ranges (evalArray) only a single cell does.
func (rd *Reader) intersect(r Rect) (Addr, bool) {
	if rd.arrays > 0 && r.From != r.To {
		return Addr{}, false
	}
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

// evalArray computes an expression given to an argument that takes
// ranges (SUM(B2:B4*2), SUMPRODUCT(A1:A3*B1:B3)). Sheets computes those
// over whole arrays, which 012 doesn't, so ranges in them stay #VALUE!
// rather than reading one cell each.
func evalArray(n Node, get lookup) Value {
	get.arrays++
	v := eval(n, get)
	get.arrays--
	return v
}
