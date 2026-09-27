package functions

import (
	"math"

	"github.com/FelineStateMachine/012/internal/value"
)

// Finance functions follow the usual cash-flow sign convention: money
// paid out is negative. type (end_or_beginning) is 0 for payments at the
// end of each period and 1 for the beginning.

func init() {
	define(
		&FuncDef{Name: "PMT", Args: "rate, number_of_periods, present_value, [future_value], [end_or_beginning]", Desc: "Payment per period of a loan or investment", Min: 3, Max: 5,
			eval: finance(func(x []float64) Value {
				r, n, pv, fv, t := x[0], x[1], x[2], x[3], x[4]
				if n == 0 {
					return value.ErrNum
				}
				if r == 0 {
					return num(-(pv + fv) / n)
				}
				g := math.Pow(1+r, n)
				return num(-r * (fv + pv*g) / ((1 + r*t) * (g - 1)))
			})},
		&FuncDef{Name: "PV", Args: "rate, number_of_periods, payment_amount, [future_value], [end_or_beginning]", Desc: "Present value of a series of payments", Min: 3, Max: 5,
			eval: finance(func(x []float64) Value {
				r, n, pmt, fv, t := x[0], x[1], x[2], x[3], x[4]
				if r == 0 {
					return num(-(fv + pmt*n))
				}
				g := math.Pow(1+r, n)
				return num(-(fv + pmt*(1+r*t)*(g-1)/r) / g)
			})},
		&FuncDef{Name: "FV", Args: "rate, number_of_periods, payment_amount, [present_value], [end_or_beginning]", Desc: "Future value of a series of payments", Min: 3, Max: 5,
			eval: finance(func(x []float64) Value {
				r, n, pmt, pv, t := x[0], x[1], x[2], x[3], x[4]
				if r == 0 {
					return num(-(pv + pmt*n))
				}
				g := math.Pow(1+r, n)
				return num(-(pv*g + pmt*(1+r*t)*(g-1)/r))
			})},
		&FuncDef{Name: "NPER", Args: "rate, payment_amount, present_value, [future_value], [end_or_beginning]", Desc: "Number of periods to pay off a loan or reach a value", Min: 3, Max: 5,
			eval: finance(func(x []float64) Value {
				r, pmt, pv, fv, t := x[0], x[1], x[2], x[3], x[4]
				if r == 0 {
					if pmt == 0 {
						return value.ErrNum
					}
					return num(-(pv + fv) / pmt)
				}
				a := pmt * (1 + r*t)
				ratio := (a - fv*r) / (a + pv*r)
				if ratio <= 0 || r <= -1 {
					return value.ErrNum
				}
				return num(math.Log(ratio) / math.Log(1+r))
			})},
		&FuncDef{Name: "RATE", Args: "number_of_periods, payment_per_period, present_value, [future_value], [end_or_beginning], [rate_guess]", Desc: "Interest rate per period of an annuity", Min: 3, Max: 6,
			eval: rate, format: returns(value.Preset(value.FmtPercent))},
		&FuncDef{Name: "NPV", Args: "discount, cashflow1, [cashflow2, ...]", Desc: "Net present value of periodic cash flows", Min: 2, Max: -1,
			eval: func(args []Node, get lookup) Value {
				r, err := numArg(args[0], get)
				if err != nil {
					return *err
				}
				flows, err := nums(args[1:], get)
				if err != nil {
					return *err
				}
				if r == -1 {
					return value.ErrDiv0
				}
				return num(npv(r, flows, 1))
			}},
		&FuncDef{Name: "IRR", Args: "cashflow_amounts, [rate_guess]", Desc: "Internal rate of return of periodic cash flows", Min: 1, Max: 2,
			eval: irr, format: returns(value.Preset(value.FmtPercent))},
	)
}

// finance evaluates arguments as numbers, defaulting the optional
// future/present value and type to 0 (type is 1 if non-zero).
func finance(f func(x []float64) Value) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		x := make([]float64, 5)
		for i := range x {
			v, err := optNum(args, i, 0, get)
			if err != nil {
				return *err
			}
			x[i] = v
		}
		if x[4] != 0 {
			x[4] = 1
		}
		return f(x)
	}
}

// npv discounts flows, the first at period first.
func npv(r float64, flows []float64, first int) float64 {
	total := 0.0
	for i, v := range flows {
		total += v / math.Pow(1+r, float64(i+first))
	}
	return total
}

// rate solves the annuity equation for the rate with Newton's method, as
// Sheets does, starting from the guess (10% by default).
func rate(args []Node, get lookup) Value {
	x := make([]float64, 6)
	for i := range x {
		def := 0.0
		if i == 5 {
			def = 0.1
		}
		v, err := optNum(args, i, def, get)
		if err != nil {
			return *err
		}
		x[i] = v
	}
	n, pmt, pv, fv, t, r := x[0], x[1], x[2], x[3], x[4], x[5]
	if t != 0 {
		t = 1
	}
	if n <= 0 {
		return value.ErrNum
	}
	f := func(r float64) float64 {
		if math.Abs(r) < 1e-12 {
			return pv + pmt*n + fv
		}
		g := math.Pow(1+r, n)
		return pv*g + pmt*(1+r*t)*(g-1)/r + fv
	}
	for range 100 {
		y := f(r)
		h := 1e-7 * math.Max(1, math.Abs(r))
		d := (f(r+h) - f(r-h)) / (2 * h)
		if d == 0 || math.IsNaN(d) {
			return value.ErrNum
		}
		next := r - y/d
		if next <= -1 {
			next = (r - 1) / 2
		}
		if math.Abs(next-r) < 1e-12 {
			return num(next)
		}
		r = next
	}
	return value.ErrNum
}

// irr finds the rate where the cash flows' present value is zero: Newton
// from the guess, falling back to bisection.
func irr(args []Node, get lookup) Value {
	flows, err := nums(args[:1], get)
	if err != nil {
		return *err
	}
	guess, err := optNum(args, 1, 0.1, get)
	if err != nil {
		return *err
	}
	pos, neg := false, false
	for _, v := range flows {
		pos = pos || v > 0
		neg = neg || v < 0
	}
	if !pos || !neg {
		return value.ErrNum
	}
	f := func(r float64) float64 { return npv(r, flows, 0) }
	df := func(r float64) float64 {
		total := 0.0
		for i, v := range flows {
			total -= float64(i) * v / math.Pow(1+r, float64(i+1))
		}
		return total
	}
	r := guess
	for range 50 {
		d := df(r)
		if d == 0 || math.IsNaN(d) {
			break
		}
		next := r - f(r)/d
		if next <= -1 || math.IsNaN(next) || math.IsInf(next, 0) {
			break
		}
		if math.Abs(next-r) < 1e-12 {
			return num(next)
		}
		r = next
	}
	// Bisection over a bracket where the sign changes.
	lo, hi := -0.9999999, 1.0
	for f(hi)*f(lo) > 0 && hi < 1e6 {
		hi *= 2
	}
	if f(hi)*f(lo) > 0 {
		return value.ErrNum
	}
	for range 200 {
		mid := (lo + hi) / 2
		if f(lo)*f(mid) <= 0 {
			hi = mid
		} else {
			lo = mid
		}
	}
	return num((lo + hi) / 2)
}
