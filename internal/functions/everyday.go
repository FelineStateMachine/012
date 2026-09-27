package functions

import (
	"math"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
)

// FuncDef describes a spreadsheet function. The table drives parsing
// (arity checks), evaluation, autocomplete and help.
type FuncDef struct {
	Name string
	Args string // signature shown to users, e.g. "value1, [value2, ...]"
	Desc string
	Min  int
	Max  int // -1 for variadic
	// step > 0 means arguments after Min come in groups of step, like
	// SUMIFS' (range, criterion) pairs.
	step int
	// Volatile functions (TODAY, NOW, RAND) change without their inputs
	// changing, so every recalculation recomputes them.
	Volatile bool
	eval     func(args []Node, get lookup) Value
	// format infers the result's display format for Automatic cells; nil
	// means none.
	format func(args []Node, infer func(Node) Format) Format
	// remote builds the question a JEV function asks (functions_jev.go).
	remote func(args []Node, get lookup) (RemoteCall, error)
}

func (f *FuncDef) call(args []Node, get lookup) Value { return f.eval(args, get) }

// Signature is how the parser checks calls to f.
func (f *FuncDef) Signature() formula.Signature {
	return formula.Signature{Name: f.Name, Args: f.Args, Min: f.Min, Max: f.Max, Step: f.step}
}

// Remote reports whether f asks a remote question (JEV functions).
func (f *FuncDef) Remote() bool { return f.remote != nil }

// Question is the remote question a call of f asks with its current
// arguments, or an error when they don't make one.
func (f *FuncDef) Question(args []Node, get *Reader) (RemoteCall, error) {
	return f.remote(args, get)
}

// Of is the function a parsed call calls: always one of the table's, as
// long as the parser found functions through LookupFunc.
func Of(c formula.Call) *FuncDef { return c.Fn.(*FuncDef) }

func funcOf(c formula.Call) *FuncDef { return Of(c) }

var funcs = map[string]*FuncDef{}

// aliases maps alternative names (including 1-2-3's) to canonical ones.
var aliases = map[string]string{"AVG": "AVERAGE"}

func define(defs ...*FuncDef) {
	for _, d := range defs {
		funcs[d.Name] = d
	}
}

// LookupFunc finds a function by name or alias, case-insensitively
// (callers pass upper case).
func LookupFunc(name string) (*FuncDef, bool) {
	if canon, ok := aliases[name]; ok {
		name = canon
	}
	f, ok := funcs[name]
	return f, ok
}

// Funcs returns every function, sorted by name.
func Funcs() []*FuncDef {
	out := make([]*FuncDef, 0, len(funcs))
	for _, f := range funcs {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b *FuncDef) int {
		if a.Name < b.Name {
			return -1
		}
		return 1
	})
	return out
}

func init() {
	define(
		&FuncDef{Name: "SUM", Args: "value1, [value2, ...]", Desc: "Sum of numbers", Min: 1, Max: -1,
			eval: aggregate(func(s Agg) Value { return num(s.sum) }), format: inherit},
		&FuncDef{Name: "AVERAGE", Args: "value1, [value2, ...]", Desc: "Average of numbers, ignoring text", Min: 1, Max: -1,
			eval: aggregate(func(s Agg) Value {
				if s.nums == 0 {
					return value.ErrDiv0
				}
				return num(s.sum / float64(s.nums))
			}), format: inherit},
		&FuncDef{Name: "COUNT", Args: "value1, [value2, ...]", Desc: "Count of numeric values", Min: 1, Max: -1,
			eval: aggregate(func(s Agg) Value { return num(float64(s.nums)) })},
		&FuncDef{Name: "COUNTA", Args: "value1, [value2, ...]", Desc: "Count of non-empty values", Min: 1, Max: -1,
			eval: aggregate(func(s Agg) Value { return num(float64(s.count)) })},
		&FuncDef{Name: "MIN", Args: "value1, [value2, ...]", Desc: "Smallest number", Min: 1, Max: -1,
			eval: aggregate(func(s Agg) Value {
				if s.nums == 0 {
					return num(0)
				}
				return num(s.min)
			}), format: inherit},
		&FuncDef{Name: "MAX", Args: "value1, [value2, ...]", Desc: "Largest number", Min: 1, Max: -1,
			eval: aggregate(func(s Agg) Value {
				if s.nums == 0 {
					return num(0)
				}
				return num(s.max)
			}), format: inherit},
		&FuncDef{Name: "ABS", Args: "value", Desc: "Absolute value", Min: 1, Max: 1, eval: math1(math.Abs), format: inherit},
		&FuncDef{Name: "INT", Args: "value", Desc: "Round down to the nearest integer", Min: 1, Max: 1, eval: math1(math.Floor), format: inherit},
		&FuncDef{Name: "SQRT", Args: "value", Desc: "Square root", Min: 1, Max: 1, eval: math1(math.Sqrt)},
		&FuncDef{Name: "ROUND", Args: "value, [places]", Desc: "Round to a number of decimal places, halves away from zero", Min: 1, Max: 2,
			eval: rounder(numfmt.HalfUp), format: inheritFrom(0)},
		&FuncDef{Name: "MOD", Args: "dividend, divisor", Desc: "Remainder, with the sign of the divisor", Min: 2, Max: 2,
			eval: numeric(func(x []float64) Value {
				if x[1] == 0 {
					return value.ErrDiv0
				}
				return num(x[0] - x[1]*math.Floor(x[0]/x[1]))
			})},
		&FuncDef{Name: "PI", Desc: "The number pi", Max: 0, eval: constant(num(math.Pi))},
		&FuncDef{Name: "TRUE", Desc: "The logical value TRUE", Max: 0, eval: constant(boolean(true))},
		&FuncDef{Name: "FALSE", Desc: "The logical value FALSE", Max: 0, eval: constant(boolean(false))},
		&FuncDef{Name: "NA", Desc: "The #N/A error", Max: 0, eval: constant(value.ErrNA)},
		&FuncDef{Name: "IF", Args: "condition, value_if_true, [value_if_false]", Desc: "Choose a value by a condition", Min: 2, Max: 3,
			eval: func(args []Node, get lookup) Value {
				c := eval(args[0], get)
				if c.Kind == value.Error {
					return c
				}
				f, err := toNum(c)
				if err != nil {
					return *err
				}
				if f != 0 {
					return eval(args[1], get)
				}
				if len(args) < 3 {
					return boolean(false)
				}
				return eval(args[2], get)
			}, format: func(args []Node, infer func(Node) Format) Format { return inherit(args[1:], infer) }},
		&FuncDef{Name: "IFERROR", Args: "value, [value_if_error]", Desc: "A fallback when a value is an error", Min: 1, Max: 2,
			eval: func(args []Node, get lookup) Value {
				v := eval(args[0], get)
				if v.Kind != value.Error {
					return v
				}
				if len(args) < 2 {
					return Value{Kind: value.Text}
				}
				return eval(args[1], get)
			}},
		&FuncDef{Name: "AND", Args: "logical1, [logical2, ...]", Desc: "TRUE if all arguments are true", Min: 1, Max: -1,
			eval: logical(func(t, n int) bool { return t == n })},
		&FuncDef{Name: "OR", Args: "logical1, [logical2, ...]", Desc: "TRUE if any argument is true", Min: 1, Max: -1,
			eval: logical(func(t, _ int) bool { return t > 0 })},
		&FuncDef{Name: "NOT", Args: "logical", Desc: "The opposite of a logical value", Min: 1, Max: 1,
			eval: numeric(func(x []float64) Value { return boolean(x[0] == 0) })},
	)
}

// Agg is the running aggregate of SUM-like functions (SUM, AVERAGE,
// COUNT, COUNTA, MIN, MAX, PRODUCT): what they read of a range, added
// in order. The engine keeps one per range shared by a recalculation
// (Book.RangeAgg).
type Agg struct {
	sum      float64
	prod     float64
	count    int // non-empty values
	nums     int
	min, max float64
}

// each calls fn for every value in args: every cell of a range that
// holds something (blank cells, which every caller skips, aren't
// visited), or the value of any other argument. direct is false for cells
// of a range or a reference: as in Sheets, SUM(A1) ignores text in A1 but
// SUM("a") is #VALUE!.
func each(args []Node, get lookup, fn func(v Value, direct bool) *Value) *Value {
	for _, arg := range args {
		if e := eachOf(arg, get, fn); e != nil {
			return e
		}
	}
	return nil
}

func eachOf(arg Node, get lookup, fn func(v Value, direct bool) *Value) *Value {
	switch arg := arg.(type) {
	case formula.Ref:
		return fn(get.cell(arg.Sheet, arg.Addr), false)
	case formula.Empty:
		return nil
	case formula.Range:
		var e *Value
		get.cells(arg.Sheet, arg.Rect, func(_ Addr, v Value) bool {
			e = fn(v, false)
			return e == nil
		})
		return e
	}
	return fn(eval(arg, get), true)
}

// Add counts v into the aggregate, with SUM's rules: blanks are skipped,
// errors returned, and text counts only when given directly.
func (s *Agg) Add(v Value, direct bool) *Value {
	switch v.Kind {
	case value.Error:
		return errOf(v)
	case value.Empty:
		return nil
	}
	s.count++
	if v.Kind != value.Number && !direct {
		return nil
	}
	f, err := toNum(v)
	if err != nil {
		return err
	}
	s.nums++
	s.sum += f
	s.prod *= f
	s.min, s.max = math.Min(s.min, f), math.Max(s.max, f)
	return nil
}

// NewAgg is the aggregate of nothing.
func NewAgg() Agg { return Agg{prod: 1, min: math.Inf(1), max: math.Inf(-1)} }

// aggregate builds SUM-like functions with Sheets semantics: in ranges only
// numbers count and text is ignored; direct arguments are coerced, so
// SUM("a") is #VALUE!. A range read first is served from the running
// aggregates shared by the recalculation (Book.RangeAgg; the engine's
// rangememo.go), so a thousand SUM(A:A) or a column of running totals
// read each cell once; other ranges the engine adds up itself
// (Book.Fold), as it reads them.
func aggregate(done func(Agg) Value) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		s := NewAgg()
		for _, arg := range args {
			if rn, ok := arg.(formula.Range); ok {
				var e *Value
				if s, e = rangeAgg(rn, s, get); e != nil {
					return *e
				}
				continue
			}
			if e := eachOf(arg, get, s.Add); e != nil {
				return *e
			}
		}
		return done(s)
	}
}

// rangeAgg adds the range rn to s: from the running aggregates when s is
// empty and the range is shared, and otherwise as the engine reads it.
func rangeAgg(rn formula.Range, s Agg, get lookup) (Agg, *Value) {
	if s.count == 0 {
		if a, e, ok := get.book.RangeAgg(rn.Sheet, rn.Rect); ok {
			return a, e
		}
	}
	return get.book.Fold(rn.Sheet, rn.Rect, s)
}

func logical(test func(trues, n int) bool) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		trues, n := 0, 0
		e := each(args, get, func(v Value, direct bool) *Value {
			switch {
			case v.Kind == value.Error:
				return errOf(v)
			case v.Kind == value.Empty, v.Kind == value.Text && !direct:
				return nil
			}
			f, err := toNum(v)
			if err != nil {
				return err
			}
			n++
			if f != 0 {
				trues++
			}
			return nil
		})
		switch {
		case e != nil:
			return *e
		case n == 0:
			return value.ErrValue
		}
		return boolean(test(trues, n))
	}
}

// numeric evaluates every argument as a number, propagating errors.
func numeric(f func([]float64) Value) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		xs := make([]float64, len(args))
		for i, a := range args {
			v := eval(a, get)
			if v.Kind == value.Error {
				return v
			}
			x, err := toNum(v)
			if err != nil {
				return *err
			}
			xs[i] = x
		}
		return f(xs)
	}
}

func math1(f func(float64) float64) func([]Node, lookup) Value {
	return numeric(func(x []float64) Value { return num(f(x[0])) })
}

func constant(v Value) func([]Node, lookup) Value {
	return func([]Node, lookup) Value { return v }
}

// rounder builds ROUND, ROUNDUP, ROUNDDOWN and TRUNC: round on the 15 digits a
// spreadsheet shows, so ROUNDUP(2.3, 1) stays 2.3.
func rounder(mode numfmt.Rounding) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		x, err := numArg(args[0], get)
		if err != nil {
			return *err
		}
		places, err := intArg(args, 1, 0, get)
		if err != nil {
			return *err
		}
		return num(numfmt.Round(x, clampInt(places, -308, 308), mode))
	}
}
