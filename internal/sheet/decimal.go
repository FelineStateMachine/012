package sheet

import (
	"sync"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/cockroachdb/apd/v3"
)

// Decimal arithmetic is an opt-in, per-workbook calculation setting for
// money: with it, 0.1+0.2 is exactly 0.3, so =0.1+0.2=0.3 is TRUE and a
// column of cents sums to what an accountant expects. Sheets and Excel use
// binary floating point, so it is off by default.
//
// The boundary is deliberately small. Values stay float64 everywhere (in
// cells, the file, formats and every other function); only these steps
// compute in decimal:
//
//   - the operators + - * / and the postfix %
//   - SUM and AVERAGE
//   - ROUND, ROUNDUP, ROUNDDOWN and TRUNC
//
// Each step reads its float64 operands as the shortest decimal that
// round-trips (what the cell shows and what was typed: 0.1, not
// 0.1000000000000000055...), computes exactly or to 34 significant digits
// (IEEE decimal128) for division, and stores the float64 nearest to that
// decimal result. Comparisons, lookups and everything else then work on
// those float64s, so they see 0.3 rather than 0.30000000000000004.
// Exponents (^), statistics, finance and date functions stay binary. A
// step the decimal context can't represent (overflow past float64 range)
// falls back to the binary result, so the setting never adds errors.
//
// Like any decimal system, it doesn't make 1/3*3 equal 1: the quotient is
// rounded, and a float64 holds about 17 digits of it.

// decCtx is the context of every decimal step: 34 digits, as decimal128.
var decCtx = func() *apd.Context {
	c := apd.BaseContext.WithPrecision(34)
	c.Rounding = apd.RoundHalfEven
	return c
}()

// fromDec returns the value nearest to d, or false if it is out of
// float64 range.
func fromDec(d *apd.Decimal) (Value, bool) {
	f, err := d.Float64()
	if err != nil {
		return Value{}, false
	}
	return num(f), true
}

// decArith computes a op b in decimal, or reports false for operators
// that stay binary and for results it can't represent.
func decArith(op string, a, b float64) (Value, bool) {
	var x, y, z apd.Decimal
	if _, err := x.SetFloat64(a); err != nil {
		return Value{}, false
	}
	if _, err := y.SetFloat64(b); err != nil {
		return Value{}, false
	}
	var err error
	switch op {
	case "+":
		_, err = decCtx.Add(&z, &x, &y)
	case "-":
		_, err = decCtx.Sub(&z, &x, &y)
	case "*":
		_, err = decCtx.Mul(&z, &x, &y)
	case "/":
		if b == 0 {
			return ErrDiv0, true
		}
		_, err = decCtx.Quo(&z, &x, &y)
	default:
		return Value{}, false
	}
	if err != nil {
		return Value{}, false
	}
	return fromDec(&z)
}

// decRounding maps ROUND's modes to apd's: half away from zero, away
// from zero, toward zero.
var decRounding = [...]apd.Rounder{numfmt.HalfUp: apd.RoundHalfUp, numfmt.Up: apd.RoundUp, numfmt.Down: apd.RoundDown}

// decRound rounds x to places decimal places (negative places round to
// tens, hundreds...) in decimal.
func decRound(x float64, places int, mode numfmt.Rounding) (Value, bool) {
	var d, z apd.Decimal
	if _, err := d.SetFloat64(x); err != nil {
		return Value{}, false
	}
	c := *decCtx
	c.Rounding = decRounding[mode]
	if _, err := c.Quantize(&z, &d, int32(-places)); err != nil {
		return Value{}, false
	}
	return fromDec(&z)
}

// decSum adds the numbers SUM would, in decimal, and counts them.
func decSum(args []Node, get lookup) (sum apd.Decimal, n int, e *Value, ok bool) {
	ok = true
	var x apd.Decimal
	e = each(args, get, func(v Value, direct bool) *Value {
		switch v.Kind {
		case Error:
			return &v
		case Empty:
			return nil
		}
		if v.Kind != Number && !direct {
			return nil
		}
		f, err := toNum(v)
		if err != nil {
			return err
		}
		n++
		if _, err := x.SetFloat64(f); err != nil {
			ok = false
		} else if _, err := decCtx.Add(&sum, &sum, &x); err != nil {
			ok = false
		}
		return nil
	})
	return sum, n, e, ok
}

// decEvals are the decimal versions of functions, by name. Each reports
// false to fall back to the binary version.
var decEvals = map[string]func(args []Node, get lookup) (Value, bool){
	"SUM": func(args []Node, get lookup) (Value, bool) {
		sum, _, e, ok := decSum(args, get)
		if e != nil {
			return *e, true
		}
		if !ok {
			return Value{}, false
		}
		return fromDec(&sum)
	},
	"AVERAGE": func(args []Node, get lookup) (Value, bool) {
		sum, n, e, ok := decSum(args, get)
		switch {
		case e != nil:
			return *e, true
		case n == 0:
			return ErrDiv0, true
		case !ok:
			return Value{}, false
		}
		var q apd.Decimal
		if _, err := decCtx.Quo(&q, &sum, apd.New(int64(n), 0)); err != nil {
			return Value{}, false
		}
		return fromDec(&q)
	},
	"ROUND":     decRounder(numfmt.HalfUp),
	"ROUNDUP":   decRounder(numfmt.Up),
	"ROUNDDOWN": decRounder(numfmt.Down),
	"TRUNC":     decRounder(numfmt.Down),
}

func decRounder(mode numfmt.Rounding) func([]Node, lookup) (Value, bool) {
	return func(args []Node, get lookup) (Value, bool) {
		x, err := numArg(args[0], get)
		if err != nil {
			return *err, true
		}
		places, err := intArg(args, 1, 0, get)
		if err != nil {
			return *err, true
		}
		return decRound(x, clampInt(places, -308, 308), mode)
	}
}

// decFuncs returns the decimal twins of the functions in decEvals: the
// same definitions (name, help, format inference) with a decimal eval
// that falls back to the binary one. Built on first use, after every
// function is defined.
var decFuncs = sync.OnceValue(func() map[string]*FuncDef {
	out := map[string]*FuncDef{}
	for name, dec := range decEvals {
		bin := funcs[name]
		twin := *bin
		twin.eval = func(args []Node, get lookup) Value {
			if v, ok := dec(args, get); ok {
				return v
			}
			return bin.eval(args, get)
		}
		out[name] = &twin
	}
	return out
})

// Operators computed in decimal are marked by these types, which eval
// knows; parsed formulas never contain them.
type (
	decUnary  formula.Unary
	decBinary formula.Binary
)

// decimalize returns a copy of n that evaluates in decimal: operators are
// marked, and functions with a decimal twin call it.
func decimalize(n Node) Node {
	switch n := n.(type) {
	case formula.Unary:
		n.X = decimalize(n.X)
		return decUnary(n)
	case formula.Binary:
		n.L, n.R = decimalize(n.L), decimalize(n.R)
		return decBinary(n)
	case formula.Call:
		args := make([]Node, len(n.Args))
		for i, a := range n.Args {
			args[i] = decimalize(a)
		}
		n.Args = args
		if twin, ok := decFuncs()[funcOf(n).Name]; ok {
			n.Fn = twin
		}
		return n
	}
	return n
}

// arith returns the formula to evaluate under the workbook's arithmetic
// setting.
func (w *Workbook) arith(n Node) Node {
	if w.decimal {
		return decimalize(n)
	}
	return n
}

// Decimal reports whether the workbook computes in decimal.
func (w *Workbook) Decimal() bool { return w.decimal }

// SetDecimal turns decimal arithmetic on or off for every sheet, as one
// undo step, and recalculates every formula.
func (w *Workbook) SetDecimal(on bool) {
	if on == w.decimal {
		return
	}
	label := "turn off decimal arithmetic"
	if on {
		label = "turn on decimal arithmetic"
	}
	w.change(w.sheets[w.Active()], label, Rect{}, func() {
		w.recordDecimal()
		w.decimal = on
		w.structural = true // every formula computes differently
	})
}

// Decimal and SetDecimal on a sheet are its workbook's.
func (s *Sheet) Decimal() bool      { return s.wb.Decimal() }
func (s *Sheet) SetDecimal(on bool) { s.wb.SetDecimal(on) }
