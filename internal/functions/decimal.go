package functions

import (
	"sync"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
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
//   - SUM, AVERAGE, PRODUCT, SUMIF, SUMIFS and SUMPRODUCT
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
			return value.ErrDiv0, true
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

// decAcc accumulates a sum or a product in decimal; ok turns false when
// a step can't be represented, and the caller falls back to binary.
type decAcc struct {
	d, x apd.Decimal
	ok   bool
}

func newDecAcc(start int64) *decAcc {
	a := &decAcc{ok: true}
	a.d.SetInt64(start)
	return a
}

func (a *decAcc) add(f float64) { a.step(f, decCtx.Add) }
func (a *decAcc) mul(f float64) { a.step(f, decCtx.Mul) }

func (a *decAcc) step(f float64, op func(d, x, y *apd.Decimal) (apd.Condition, error)) {
	if !a.ok {
		return
	}
	if _, err := a.x.SetFloat64(f); err != nil {
		a.ok = false
	} else if _, err := op(&a.d, &a.d, &a.x); err != nil {
		a.ok = false
	}
}

// addDec adds a decimal to the sum.
func (a *decAcc) addDec(d *apd.Decimal) {
	if !a.ok {
		return
	}
	if _, err := decCtx.Add(&a.d, &a.d, d); err != nil {
		a.ok = false
	}
}

// result is the accumulated value, or false to fall back.
func (a *decAcc) result() (Value, bool) {
	if !a.ok {
		return Value{}, false
	}
	return fromDec(&a.d)
}

// decFold feeds the numbers SUM and PRODUCT take to step and counts
// them: in ranges only numbers, direct arguments coerced.
func decFold(args []Node, get lookup, step func(float64)) (n int, e *Value) {
	e = each(args, get, func(v Value, direct bool) *Value {
		switch v.Kind {
		case value.Error:
			return &v
		case value.Empty:
			return nil
		}
		if v.Kind != value.Number && !direct {
			return nil
		}
		f, err := toNum(v)
		if err != nil {
			return err
		}
		n++
		step(f)
		return nil
	})
	return n, e
}

// decTerms builds the decimal twin of a function that visits the terms
// it adds (SUMIF, SUMIFS).
func decTerms(terms func([]Node, lookup, func(float64)) *Value) func([]Node, lookup) (Value, bool) {
	return func(args []Node, get lookup) (Value, bool) {
		acc := newDecAcc(0)
		if e := terms(args, get, acc.add); e != nil {
			return *e, true
		}
		return acc.result()
	}
}

// decSumProduct is SUMPRODUCT with each entry's product and the sum in
// decimal.
func decSumProduct(args []Node, get lookup) (Value, bool) {
	acc := newDecAcc(0)
	e := sumProductTerms(args, get, func(fs []float64) {
		p := newDecAcc(1)
		for _, f := range fs {
			p.mul(f)
		}
		if !p.ok {
			acc.ok = false
		}
		acc.addDec(&p.d)
	})
	if e != nil {
		return *e, true
	}
	return acc.result()
}

// decEvals are the decimal versions of functions, by name. Each reports
// false to fall back to the binary version.
var decEvals = map[string]func(args []Node, get lookup) (Value, bool){
	"SUM": func(args []Node, get lookup) (Value, bool) {
		acc := newDecAcc(0)
		if _, e := decFold(args, get, acc.add); e != nil {
			return *e, true
		}
		return acc.result()
	},
	"AVERAGE": func(args []Node, get lookup) (Value, bool) {
		acc := newDecAcc(0)
		n, e := decFold(args, get, acc.add)
		switch {
		case e != nil:
			return *e, true
		case n == 0:
			return value.ErrDiv0, true
		case !acc.ok:
			return Value{}, false
		}
		var q apd.Decimal
		if _, err := decCtx.Quo(&q, &acc.d, apd.New(int64(n), 0)); err != nil {
			return Value{}, false
		}
		return fromDec(&q)
	},
	"PRODUCT": func(args []Node, get lookup) (Value, bool) {
		acc := newDecAcc(1)
		n, e := decFold(args, get, acc.mul)
		switch {
		case e != nil:
			return *e, true
		case n == 0:
			return num(0), true
		}
		return acc.result()
	},
	"SUMIF":      decTerms(sumIfTerms),
	"SUMIFS":     decTerms(sumIfsTerms),
	"SUMPRODUCT": decSumProduct,
	"ROUND":      decRounder(numfmt.HalfUp),
	"ROUNDUP":    decRounder(numfmt.Up),
	"ROUNDDOWN":  decRounder(numfmt.Down),
	"TRUNC":      decRounder(numfmt.Down),
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

// Decimalize returns a copy of n that evaluates in decimal: operators are
// marked, and functions with a decimal twin call it.
func Decimalize(n Node) Node {
	switch n := n.(type) {
	case formula.Unary:
		n.X = Decimalize(n.X)
		return decUnary(n)
	case formula.Binary:
		n.L, n.R = Decimalize(n.L), Decimalize(n.R)
		return decBinary(n)
	case formula.Call:
		args := make([]Node, len(n.Args))
		for i, a := range n.Args {
			args[i] = Decimalize(a)
		}
		n.Args = args
		if twin, ok := decFuncs()[funcOf(n).Name]; ok {
			n.Fn = twin
		}
		return n
	}
	return n
}
