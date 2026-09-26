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
	Label
	Error
)

// Value is the computed contents of a cell.
type Value struct {
	Kind Kind
	Num  float64
	Str  string // label text or error name (ERR, NA)
}

var (
	errValue = Value{Kind: Error, Str: "ERR"}
	naValue  = Value{Kind: Error, Str: "NA"}
)

func num(v float64) Value {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return errValue
	}
	return Value{Kind: Number, Num: v}
}

func boolNum(b bool) Value {
	if b {
		return num(1)
	}
	return num(0)
}

// asNum converts v for arithmetic. Like 1-2-3, labels and blanks count as 0.
func asNum(v Value) float64 {
	if v.Kind == Number {
		return v.Num
	}
	return 0
}

func asStr(v Value) string {
	switch v.Kind {
	case Label:
		return v.Str
	case Number:
		return strconv.FormatFloat(v.Num, 'f', -1, 64)
	}
	return ""
}

// lookup resolves the current value of a referenced cell.
type lookup func(Addr) Value

func eval(n Node, get lookup) Value {
	switch n := n.(type) {
	case numLit:
		return num(n.v)
	case strLit:
		return Value{Kind: Label, Str: n.v}
	case refNode:
		return get(n.a)
	case rangeNode:
		// A bare range outside a function is an error in 1-2-3.
		return errValue
	case unaryNode:
		x := eval(n.x, get)
		if x.Kind == Error {
			return x
		}
		switch n.op {
		case "-":
			return num(-asNum(x))
		case "#NOT#":
			return boolNum(asNum(x) == 0)
		}
		// Unary + is identity, so string formulas like +B1&"x" work.
		return x
	case binaryNode:
		return evalBinary(n, get)
	case callNode:
		return functions[n.name](n.args, get)
	}
	return errValue
}

func evalBinary(n binaryNode, get lookup) Value {
	l, r := eval(n.l, get), eval(n.r, get)
	if l.Kind == Error {
		return l
	}
	if r.Kind == Error {
		return r
	}
	a, b := asNum(l), asNum(r)
	switch n.op {
	case "+":
		return num(a + b)
	case "-":
		return num(a - b)
	case "*":
		return num(a * b)
	case "/":
		if b == 0 {
			return errValue
		}
		return num(a / b)
	case "^":
		return num(math.Pow(a, b))
	case "&":
		return Value{Kind: Label, Str: asStr(l) + asStr(r)}
	case "#AND#":
		return boolNum(a != 0 && b != 0)
	case "#OR#":
		return boolNum(a != 0 || b != 0)
	}
	cmp := compare(l, r)
	switch n.op {
	case "=":
		return boolNum(cmp == 0)
	case "<>":
		return boolNum(cmp != 0)
	case "<":
		return boolNum(cmp < 0)
	case ">":
		return boolNum(cmp > 0)
	case "<=":
		return boolNum(cmp <= 0)
	case ">=":
		return boolNum(cmp >= 0)
	}
	return errValue
}

func compare(l, r Value) int {
	if l.Kind == Label && r.Kind == Label {
		return strings.Compare(strings.ToUpper(l.Str), strings.ToUpper(r.Str))
	}
	a, b := asNum(l), asNum(r)
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

type function func(args []Node, get lookup) Value

var functions map[string]function

func init() {
	functions = map[string]function{
		"SUM":   aggregate(func(s stats) Value { return num(s.sum) }),
		"COUNT": aggregate(func(s stats) Value { return num(float64(s.count)) }),
		"AVG": aggregate(func(s stats) Value {
			if s.count == 0 {
				return errValue
			}
			return num(s.sum / float64(s.count))
		}),
		"MIN": aggregate(func(s stats) Value {
			if s.nums == 0 {
				return num(0)
			}
			return num(s.min)
		}),
		"MAX": aggregate(func(s stats) Value {
			if s.nums == 0 {
				return num(0)
			}
			return num(s.max)
		}),
		"ABS":  math1(math.Abs),
		"INT":  math1(math.Trunc),
		"SQRT": math1(math.Sqrt),
		"ROUND": math2(func(x, places float64) float64 {
			p := math.Pow(10, math.Trunc(places))
			return math.Round(x*p) / p
		}),
		"MOD": math2(func(x, y float64) float64 {
			if y == 0 {
				return math.NaN()
			}
			return math.Mod(x, y)
		}),
		"PI":    constant(num(math.Pi)),
		"TRUE":  constant(num(1)),
		"FALSE": constant(num(0)),
		"ERR":   constant(errValue),
		"NA":    constant(naValue),
		"IF": func(args []Node, get lookup) Value {
			if len(args) != 3 {
				return errValue
			}
			c := eval(args[0], get)
			if c.Kind == Error {
				return c
			}
			if asNum(c) != 0 {
				return eval(args[1], get)
			}
			return eval(args[2], get)
		},
	}
}

type stats struct {
	sum      float64
	count    int // non-blank cells, as 1-2-3's @COUNT
	nums     int
	min, max float64
}

// aggregate builds a list function such as @SUM. Ranges contribute every
// non-blank cell; labels count toward @COUNT but add 0, as in 1-2-3.
func aggregate(done func(stats) Value) function {
	return func(args []Node, get lookup) Value {
		s := stats{min: math.Inf(1), max: math.Inf(-1)}
		add := func(v Value) *Value {
			switch v.Kind {
			case Error:
				return &v
			case Empty:
				return nil
			case Number:
				s.nums++
				s.min, s.max = math.Min(s.min, v.Num), math.Max(s.max, v.Num)
			}
			s.count++
			s.sum += asNum(v)
			return nil
		}
		for _, arg := range args {
			if rn, ok := arg.(rangeNode); ok {
				for r := rn.r.From.Row; r <= rn.r.To.Row; r++ {
					for c := rn.r.From.Col; c <= rn.r.To.Col; c++ {
						if e := add(get(Addr{Col: c, Row: r})); e != nil {
							return *e
						}
					}
				}
				continue
			}
			if e := add(eval(arg, get)); e != nil {
				return *e
			}
		}
		return done(s)
	}
}

func math1(f func(float64) float64) function {
	return func(args []Node, get lookup) Value {
		if len(args) != 1 {
			return errValue
		}
		x := eval(args[0], get)
		if x.Kind == Error {
			return x
		}
		return num(f(asNum(x)))
	}
}

func math2(f func(float64, float64) float64) function {
	return func(args []Node, get lookup) Value {
		if len(args) != 2 {
			return errValue
		}
		x, y := eval(args[0], get), eval(args[1], get)
		if x.Kind == Error {
			return x
		}
		if y.Kind == Error {
			return y
		}
		return num(f(asNum(x), asNum(y)))
	}
}

func constant(v Value) function {
	return func(args []Node, _ lookup) Value {
		if len(args) != 0 {
			return errValue
		}
		return v
	}
}
