package sheet

import (
	"math"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/numfmt"
)

// Kind is the type of a computed cell value.
type Kind int

const (
	Empty Kind = iota
	Number
	Text
	Bool
	Error
)

// Value is the computed contents of a cell.
type Value struct {
	Kind Kind
	Num  float64 // numbers, and booleans as 1 or 0
	Str  string  // text, or the error code such as #DIV/0!
}

// String renders a value the way a cell shows it in General format.
func (v Value) String() string {
	switch v.Kind {
	case Number:
		return numfmt.General(v.Num)
	case Bool:
		if v.Num != 0 {
			return "TRUE"
		}
		return "FALSE"
	}
	return v.Str
}

// Error values, using Google Sheets codes.
var (
	ErrDiv0  = Value{Kind: Error, Str: "#DIV/0!"}
	ErrValue = Value{Kind: Error, Str: "#VALUE!"}
	ErrName  = Value{Kind: Error, Str: "#NAME?"}
	ErrNA    = Value{Kind: Error, Str: "#N/A"}
	ErrNum   = Value{Kind: Error, Str: "#NUM!"}
	ErrRef   = Value{Kind: Error, Str: "#REF!"} // also circular references
)

func num(v float64) Value {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ErrNum
	}
	return Value{Kind: Number, Num: v}
}

func boolean(b bool) Value {
	if b {
		return Value{Kind: Bool, Num: 1}
	}
	return Value{Kind: Bool}
}

// toNum coerces v for arithmetic as Sheets does: blanks are 0, booleans
// 1 or 0, numeric text (including dates such as "2026-09-26") is its
// number, other text is #VALUE!.
// errOf returns a pointer to a copy of v, for the *Value error results of
// argument helpers. Taking &v of a parameter directly would move it to
// the heap on every call, erroneous or not: one allocation per cell read
// by SUM over a range.
func errOf(v Value) *Value { return &v }

func toNum(v Value) (float64, *Value) {
	switch v.Kind {
	case Empty:
		return 0, nil
	case Number, Bool:
		return v.Num, nil
	case Text:
		if n, _, ok := ParseValue(v.Str); ok {
			return n, nil
		}
		return 0, &ErrValue
	}
	return 0, errOf(v)
}

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
	case formula.Unary, decUnary, formula.Binary, decBinary, formula.Call:
		get.w.depth++
		v := evalNested(n, get)
		get.w.depth--
		return v
	}
	return ErrValue
}

// evalNested computes an operator or a call, which evaluate their
// operands: eval counts the nesting around it (see evaluate.go).
func evalNested(n Node, get lookup) Value {
	switch n := n.(type) {
	case formula.Unary:
		return evalUnary(n, get, false)
	case decUnary:
		return evalUnary(formula.Unary(n), get, true)
	case formula.Binary:
		return evalBinary(n, get, false)
	case decBinary:
		return evalBinary(formula.Binary(n), get, true)
	case formula.Call:
		return funcOf(n).call(n.Args, get)
	}
	return ErrValue
}

// evalUnary computes a prefix operator or the postfix %; with dec, % is
// decimal (decimal.go).
func evalUnary(n formula.Unary, get lookup, dec bool) Value {
	x := eval(n.X, get)
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
	l, r := eval(n.L, get), eval(n.R, get)
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

// text converts a value for string concatenation.
func text(v Value) string {
	if v.Kind == Empty {
		return ""
	}
	return v.String()
}

// compare orders values like Sheets: numbers < text < booleans, text is
// case-insensitive, and a blank equals 0 or "".
func compare(l, r Value) int {
	rank := func(v Value) int {
		switch v.Kind {
		case Text:
			return 1
		case Bool:
			return 2
		}
		return 0
	}
	if l.Kind == Empty && r.Kind == Text {
		l = Value{Kind: Text}
	}
	if r.Kind == Empty && l.Kind == Text {
		r = Value{Kind: Text}
	}
	if d := rank(l) - rank(r); d != 0 {
		return d
	}
	if l.Kind == Text {
		return strings.Compare(strings.ToUpper(l.Str), strings.ToUpper(r.Str))
	}
	switch {
	case l.Num < r.Num:
		return -1
	case l.Num > r.Num:
		return 1
	}
	return 0
}
