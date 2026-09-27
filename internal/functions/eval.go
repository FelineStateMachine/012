package functions

import (
	"math"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Eval computes a formula, reading the cells it refers to through get.
func Eval(n Node, get *Reader) Value { return eval(n, get) }

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
		// A range outside a function: Sheets uses the top-left cell here
		// for single-cell ranges and #VALUE! otherwise.
		if n.Rect.From == n.Rect.To {
			return get.cell(n.Sheet, n.Rect.From)
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
		v := funcOf(n).call(n.Args, get)
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
