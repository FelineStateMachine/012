package functions

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// IsVolatile reports whether n calls a function whose result changes
// without its inputs changing (TODAY, NOW, RAND).
func IsVolatile(n Node) bool {
	if c, ok := n.(formula.Call); ok && funcOf(c).Volatile {
		return true
	}
	volatile := false
	formula.EachChild(n, func(k Node) { volatile = volatile || IsVolatile(k) })
	return volatile
}

// InferFormat picks the format Sheets shows a formula's result in when
// the cell is Automatic: dates from date functions, and otherwise the
// format of the first formatted input, so =B2+B3 of currency shows
// currency and a date plus days shows a date.
func InferFormat(n Node, at func(string, Addr) Format) Format {
	switch n := n.(type) {
	case formula.Ref:
		return at(n.Sheet, n.Addr)
	case formula.Range:
		return at(n.Sheet, n.Rect.From)
	case formula.Unary:
		if n.Op == "-" || n.Op == "+" {
			return InferFormat(n.X, at)
		}
	case formula.Binary:
		l, r := InferFormat(n.L, at), InferFormat(n.R, at)
		switch n.Op {
		case "+", "-":
			if n.Op == "-" && l.Kind.IsTime() && r.Kind.IsTime() {
				return Format{} // days between two dates
			}
			return firstFormat(l, r)
		case "*", "/":
			if l.Kind.IsTime() || r.Kind.IsTime() {
				return Format{}
			}
			return firstFormat(l, r)
		}
	case formula.Call:
		if funcOf(n).format != nil {
			return funcOf(n).format(n.Args, func(n Node) Format { return InferFormat(n, at) })
		}
	case formula.Array:
		return InferFormat(n.Rows[0][0], at)
	}
	return Format{}
}

func firstFormat(fs ...Format) Format {
	for _, f := range fs {
		if !f.IsZero() && f.Kind != value.FmtText {
			return f
		}
	}
	return Format{}
}

// Result formats for the function table.

// returns makes a function's result show in format f.
func returns(f Format) func([]Node, func(Node) Format) Format {
	return func([]Node, func(Node) Format) Format { return f }
}

// inherit makes a function's result take the format of its first
// formatted argument (SUM, MIN, ROUND...).
func inherit(args []Node, infer func(Node) Format) Format {
	for _, a := range args {
		if f := infer(a); !f.IsZero() && f.Kind != value.FmtText {
			return f
		}
	}
	return Format{}
}

// inheritFrom takes the format of argument i only.
func inheritFrom(i int) func([]Node, func(Node) Format) Format {
	return func(args []Node, infer func(Node) Format) Format {
		if i < len(args) {
			return infer(args[i])
		}
		return Format{}
	}
}
