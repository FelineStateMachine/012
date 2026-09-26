package sheet

import (
	"math"
	"strconv"
	"strings"
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
		return strconv.FormatFloat(v.Num, 'f', -1, 64)
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
// 1 or 0, numeric text is its number, other text is #VALUE!.
func toNum(v Value) (float64, *Value) {
	switch v.Kind {
	case Empty:
		return 0, nil
	case Number, Bool:
		return v.Num, nil
	case Text:
		if n, ok := ParseNumber(v.Str); ok {
			return n, nil
		}
		return 0, &ErrValue
	}
	return 0, &v
}

// lookup resolves the current value of a referenced cell.
type lookup func(Addr) Value

func eval(n Node, get lookup) Value {
	switch n := n.(type) {
	case numLit:
		return num(n.v)
	case strLit:
		return Value{Kind: Text, Str: n.v}
	case boolLit:
		return boolean(n.v)
	case refNode:
		return get(n.a)
	case nameNode:
		return ErrName
	case rangeNode:
		// A range outside a function: Sheets uses the top-left cell here
		// for single-cell ranges and #VALUE! otherwise.
		if n.r.From == n.r.To {
			return get(n.r.From)
		}
		return ErrValue
	case unaryNode:
		x := eval(n.x, get)
		if x.Kind == Error {
			return x
		}
		switch n.op {
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
		}
		return x // unary + is identity
	case binaryNode:
		return evalBinary(n, get)
	case callNode:
		return n.fn.call(n.args, get)
	}
	return ErrValue
}

func evalBinary(n binaryNode, get lookup) Value {
	l, r := eval(n.l, get), eval(n.r, get)
	if l.Kind == Error {
		return l
	}
	if r.Kind == Error {
		return r
	}
	switch n.op {
	case "&":
		return Value{Kind: Text, Str: text(l) + text(r)}
	case "=", "<>", "<", ">", "<=", ">=":
		c := compare(l, r)
		switch n.op {
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
	switch n.op {
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
