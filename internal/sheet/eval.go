package sheet

import (
	"math"

	"github.com/FelineStateMachine/012/internal/formula"
)

// lookup is how a formula reads other cells (see reader): one at a time,
// or a whole range at once, so reading a range needn't cost a lookup per
// cell and can later be served from column blocks or cached aggregates.
// It is a concrete type rather than an interface so the callbacks given
// to cells stay on the stack.
type lookup = *reader

func eval(n Node, get lookup) Value {
	switch n := n.(type) {
	case formula.Num:
		return num(n.V)
	case formula.Str:
		return Value{Kind: Text, Str: n.V}
	case formula.Bool:
		return boolean(n.V)
	case formula.Ref:
		return get.cell(n.Sheet, n.Addr)
	case formula.Name:
		return ErrName
	case formula.RefErr:
		return ErrRef
	case formula.Empty:
		return Value{}
	case formula.Range:
		// A range outside a function: Sheets uses the top-left cell here
		// for single-cell ranges and #VALUE! otherwise.
		if n.Rect.From == n.Rect.To {
			return get.cell(n.Sheet, n.Rect.From)
		}
		return ErrValue
	case formula.Unary:
		return evalUnary(n, get, false)
	case decUnary:
		return evalUnary(formula.Unary(n), get, true)
	case formula.Binary:
		return evalBinary(n, get, false)
	case decBinary:
		return evalBinary(formula.Binary(n), get, true)
	case formula.Call:
		get.w.depth++ // see evaluate.go; operators count in theirs
		v := funcOf(n).call(n.Args, get)
		get.w.depth--
		return v
	}
	return ErrValue
}

// evalUnary computes a prefix operator or the postfix %; with dec, % is
// decimal (decimal.go).
func evalUnary(n formula.Unary, get lookup, dec bool) Value {
	get.w.depth++
	x := eval(n.X, get)
	get.w.depth--
	if x.Kind == Error {
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
	get.w.depth++
	l, r := eval(n.L, get), eval(n.R, get)
	get.w.depth--
	if l.Kind == Error {
		return l
	}
	if r.Kind == Error {
		return r
	}
	switch n.Op {
	case "&":
		return Value{Kind: Text, Str: text(l) + text(r)}
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
			return ErrDiv0
		}
		return num(a / b)
	case "^":
		return num(math.Pow(a, b))
	case "#AND#":
		return boolean(a != 0 && b != 0)
	case "#OR#":
		return boolean(a != 0 || b != 0)
	}
	return ErrValue
}
