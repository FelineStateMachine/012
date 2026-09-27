package functions

import (
	"math"
	"math/rand/v2"

	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
)

func init() {
	define(
		&FuncDef{Name: "SUMIF", Args: "range, criterion, [sum_range]", Desc: "Sum of the cells that meet a condition", Min: 2, Max: 3,
			eval: sumIf, format: inheritFrom(2)},
		&FuncDef{Name: "SUMIFS", Args: "sum_range, criteria_range1, criterion1, [criteria_range2, criterion2, ...]", Desc: "Sum of the cells that meet every condition", Min: 3, Max: -1, step: 2,
			eval: sumIfs, format: inheritFrom(0)},
		&FuncDef{Name: "SUMPRODUCT", Args: "array1, [array2, ...]", Desc: "Sum of the products of matching entries", Min: 1, Max: -1,
			eval: sumProduct},
		&FuncDef{Name: "PRODUCT", Args: "factor1, [factor2, ...]", Desc: "Product of numbers", Min: 1, Max: -1,
			eval: aggregate(func(s Agg) Value {
				if s.nums == 0 {
					return num(0)
				}
				return num(s.prod)
			})},
		&FuncDef{Name: "POWER", Args: "base, exponent", Desc: "A number raised to a power", Min: 2, Max: 2,
			eval: numeric(func(x []float64) Value { return powerOf(x[0], x[1]) })},
		&FuncDef{Name: "ROUNDUP", Args: "value, [places]", Desc: "Round away from zero", Min: 1, Max: 2,
			eval: rounder(numfmt.Up), format: inheritFrom(0)},
		&FuncDef{Name: "ROUNDDOWN", Args: "value, [places]", Desc: "Round toward zero", Min: 1, Max: 2,
			eval: rounder(numfmt.Down), format: inheritFrom(0)},
		&FuncDef{Name: "TRUNC", Args: "value, [places]", Desc: "Drop decimals past a number of places", Min: 1, Max: 2,
			eval: rounder(numfmt.Down), format: inheritFrom(0)},
		&FuncDef{Name: "CEILING", Args: "value, [factor]", Desc: "Round up to a multiple of factor", Min: 1, Max: 2,
			eval: toMultiple(math.Ceil), format: inheritFrom(0)},
		&FuncDef{Name: "FLOOR", Args: "value, [factor]", Desc: "Round down to a multiple of factor", Min: 1, Max: 2,
			eval: toMultiple(math.Floor), format: inheritFrom(0)},
		&FuncDef{Name: "SIGN", Args: "value", Desc: "1, 0 or -1 by the sign of a number", Min: 1, Max: 1,
			eval: numeric(func(x []float64) Value {
				switch {
				case x[0] > 0:
					return num(1)
				case x[0] < 0:
					return num(-1)
				}
				return num(0)
			})},
		&FuncDef{Name: "EXP", Args: "exponent", Desc: "e raised to a power", Min: 1, Max: 1, eval: math1(math.Exp)},
		&FuncDef{Name: "LN", Args: "value", Desc: "Natural logarithm", Min: 1, Max: 1,
			eval: numeric(func(x []float64) Value {
				if x[0] <= 0 {
					return value.ErrNum
				}
				return num(math.Log(x[0]))
			})},
		&FuncDef{Name: "LOG", Args: "value, [base]", Desc: "Logarithm, base 10 by default", Min: 1, Max: 2,
			eval: func(args []Node, get lookup) Value {
				x, err := numArg(args[0], get)
				if err != nil {
					return *err
				}
				base, err := optNum(args, 1, 10, get)
				if err != nil {
					return *err
				}
				switch {
				case x <= 0 || base <= 0:
					return value.ErrNum
				case base == 1:
					return value.ErrDiv0
				}
				return num(math.Log(x) / math.Log(base))
			}},
		&FuncDef{Name: "LOG10", Args: "value", Desc: "Base-10 logarithm", Min: 1, Max: 1,
			eval: numeric(func(x []float64) Value {
				if x[0] <= 0 {
					return value.ErrNum
				}
				return num(math.Log10(x[0]))
			})},
		&FuncDef{Name: "QUOTIENT", Args: "dividend, divisor", Desc: "Integer part of a division", Min: 2, Max: 2,
			eval: numeric(func(x []float64) Value {
				if x[1] == 0 {
					return value.ErrDiv0
				}
				return num(math.Trunc(x[0] / x[1]))
			})},
		&FuncDef{Name: "RAND", Desc: "A random number from 0 up to 1, new on every change", Max: 0, Volatile: true,
			eval: func([]Node, lookup) Value { return num(rand.Float64()) }},
		&FuncDef{Name: "RANDBETWEEN", Args: "low, high", Desc: "A random integer between two values, new on every change", Min: 2, Max: 2, Volatile: true,
			eval: numeric(func(x []float64) Value {
				lo, hi := math.Ceil(x[0]), math.Floor(x[1])
				if lo > hi {
					return value.ErrNum
				}
				return num(lo + math.Floor(rand.Float64()*(hi-lo+1)))
			})},
	)
}

func powerOf(b, e float64) Value {
	if b == 0 && e < 0 {
		return value.ErrDiv0
	}
	return num(math.Pow(b, e))
}

// toMultiple builds CEILING and FLOOR with Sheets semantics: the factor
// defaults to 1, a zero factor gives 0, and a positive value with a
// negative factor is #NUM!.
func toMultiple(round func(float64) float64) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		x, err := numArg(args[0], get)
		if err != nil {
			return *err
		}
		f, err := optNum(args, 1, 1, get)
		if err != nil {
			return *err
		}
		switch {
		case f == 0 || x == 0:
			return num(0)
		case x > 0 && f < 0:
			return value.ErrNum
		}
		q := x / f
		// Snap quotients like 2.0000000000000004 to the integer they mean.
		if r := math.Round(q); math.Abs(q-r) < 1e-12*math.Max(1, math.Abs(q)) {
			q = r
		}
		// With both negative the quotient is positive, so CEILING moves
		// the result away from zero and FLOOR toward it.
		return num(round(q) * f)
	}
}
