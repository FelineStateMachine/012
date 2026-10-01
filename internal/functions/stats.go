package functions

import (
	"math"

	"github.com/FelineStateMachine/012/internal/value"
)

func init() {
	define(
		&FuncDef{Name: "AVERAGEIF", Args: "criteria_range, criterion, [average_range]", Desc: "Average of the cells that meet a condition", Min: 2, Max: 3,
			eval: averageIf, format: inheritFrom(2)},
		&FuncDef{Name: "AVERAGEIFS", Args: "average_range, criteria_range1, criterion1, [criteria_range2, criterion2, ...]", Desc: "Average of the cells that meet every condition", Min: 3, Max: -1, step: 2,
			eval: averageIfs, format: inheritFrom(0)},
		&FuncDef{Name: "COUNTIF", Args: "range, criterion", Desc: "Count of the cells that meet a condition", Min: 2, Max: 2,
			eval: countIfs},
		&FuncDef{Name: "COUNTIFS", Args: "criteria_range1, criterion1, [criteria_range2, criterion2, ...]", Desc: "Count of the cells that meet every condition", Min: 2, Max: -1, step: 2,
			eval: countIfs},
		&FuncDef{Name: "COUNTBLANK", Args: "range", Desc: "Count of empty cells, including empty text", Min: 1, Max: 1,
			eval: countBlank},
		&FuncDef{Name: "MEDIAN", Args: "value1, [value2, ...]", Desc: "Middle value of numbers", Min: 1, Max: -1,
			eval: withSorted(true, medianOf), format: inherit},
		&FuncDef{Name: "MODE", Args: "value1, [value2, ...]", Desc: "Most common number (the first, on a tie)", Min: 1, Max: -1,
			eval: withSorted(true, modeOf), format: inherit},
		&FuncDef{Name: "STDEV", Args: "value1, [value2, ...]", Desc: "Standard deviation of a sample", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 1, true) })},
		&FuncDef{Name: "STDEVP", Args: "value1, [value2, ...]", Desc: "Standard deviation of a whole population", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 0, true) })},
		&FuncDef{Name: "VAR", Args: "value1, [value2, ...]", Desc: "Variance of a sample", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 1, false) })},
		&FuncDef{Name: "VARP", Args: "value1, [value2, ...]", Desc: "Variance of a whole population", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 0, false) })},
		&FuncDef{Name: "LARGE", Args: "data, n", Desc: "The nth largest number", Min: 2, Max: 2,
			eval: withSorted(false, nthOf(true)), format: inheritFrom(0)},
		&FuncDef{Name: "SMALL", Args: "data, n", Desc: "The nth smallest number", Min: 2, Max: 2,
			eval: withSorted(false, nthOf(false)), format: inheritFrom(0)},
		&FuncDef{Name: "RANK", Args: "value, data, [is_ascending]", Desc: "Rank of a number among others, largest first by default", Min: 2, Max: 3,
			eval: rank},
		&FuncDef{Name: "RANK.EQ", Args: "value, data, [is_ascending]", Desc: "Rank of a number among others, largest first by default (RANK)", Min: 2, Max: 3,
			eval: rank},
		&FuncDef{Name: "MODE.SNGL", Args: "value1, [value2, ...]", Desc: "Most common number, the first on a tie (MODE)", Min: 1, Max: -1,
			eval: withSorted(true, modeOf), format: inherit},
		&FuncDef{Name: "PERCENTILE", Args: "data, percentile", Desc: "The value at a percentile from 0 to 1, interpolated", Min: 2, Max: 2,
			eval: withSorted(false, percentileOf(false, false)), format: inheritFrom(0)},
		&FuncDef{Name: "PERCENTILE.INC", Args: "data, percentile", Desc: "The value at a percentile from 0 to 1, interpolated (PERCENTILE)", Min: 2, Max: 2,
			eval: withSorted(false, percentileOf(false, false)), format: inheritFrom(0)},
		&FuncDef{Name: "PERCENTILE.EXC", Args: "data, percentile", Desc: "The value at a percentile strictly between 0 and 1, interpolated", Min: 2, Max: 2,
			eval: withSorted(false, percentileOf(true, false)), format: inheritFrom(0)},
		&FuncDef{Name: "QUARTILE", Args: "data, quartile", Desc: "The minimum (0), a quartile (1 to 3) or the maximum (4)", Min: 2, Max: 2,
			eval: withSorted(false, percentileOf(false, true)), format: inheritFrom(0)},
		&FuncDef{Name: "QUARTILE.INC", Args: "data, quartile", Desc: "The minimum (0), a quartile (1 to 3) or the maximum (4) (QUARTILE)", Min: 2, Max: 2,
			eval: withSorted(false, percentileOf(false, true)), format: inheritFrom(0)},
		&FuncDef{Name: "QUARTILE.EXC", Args: "data, quartile", Desc: "A quartile from 1 to 3, between the numbers rather than at them", Min: 2, Max: 2,
			eval: withSorted(false, percentileOf(true, true)), format: inheritFrom(0)},
	)
}

// Evaluators for the table above, in its order.

// rank counts the numbers of data above and below the value as it reads
// them, holding none, so a linked source's column of any size streams.
func rank(args []Node, get lookup) Value {
	v, err := numArg(args[0], get)
	if err != nil {
		return *err
	}
	above, below, found := 0, 0, false
	err = each(args[1:2], get, func(d Value, direct bool) *Value {
		switch {
		case d.Kind == value.Error:
			return errOf(d)
		case d.Kind == value.Empty, d.Kind != value.Number && !direct:
			return nil
		}
		f, err := toNum(d)
		switch {
		case err != nil:
			return err
		case f == v:
			found = true
		case f > v:
			above++
		default:
			below++
		}
		return nil
	})
	if err != nil {
		return *err
	}
	asc, err := boolArg(args, 2, false, get)
	if err != nil {
		return *err
	}
	if !found {
		return value.ErrNA
	}
	if asc {
		return num(float64(below + 1))
	}
	return num(float64(above + 1))
}

// withNums passes the numbers of all arguments, with aggregate
// semantics, to f.
func withNums(f func([]float64) Value) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		x, err := nums(args, get)
		if err != nil {
			return *err
		}
		return f(x)
	}
}

// spread computes variance (or its root) with ddof 1 for samples and 0
// for populations.
func spread(x []float64, ddof int, root bool) Value {
	n := len(x)
	if n-ddof <= 0 {
		return value.ErrDiv0
	}
	mean := 0.0
	for _, v := range x {
		mean += v
	}
	mean /= float64(n)
	ss := 0.0
	for _, v := range x {
		ss += (v - mean) * (v - mean)
	}
	v := ss / float64(n-ddof)
	if root {
		v = math.Sqrt(v)
	}
	return num(v)
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
