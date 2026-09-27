package sheet

import (
	"math"
	"slices"
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
			eval: withNums(median), format: inherit},
		&FuncDef{Name: "MODE", Args: "value1, [value2, ...]", Desc: "Most common number (the first, on a tie)", Min: 1, Max: -1,
			eval: withNums(mode), format: inherit},
		&FuncDef{Name: "STDEV", Args: "value1, [value2, ...]", Desc: "Standard deviation of a sample", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 1, true) })},
		&FuncDef{Name: "STDEVP", Args: "value1, [value2, ...]", Desc: "Standard deviation of a whole population", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 0, true) })},
		&FuncDef{Name: "VAR", Args: "value1, [value2, ...]", Desc: "Variance of a sample", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 1, false) })},
		&FuncDef{Name: "VARP", Args: "value1, [value2, ...]", Desc: "Variance of a whole population", Min: 1, Max: -1,
			eval: withNums(func(x []float64) Value { return spread(x, 0, false) })},
		&FuncDef{Name: "LARGE", Args: "data, n", Desc: "The nth largest number", Min: 2, Max: 2,
			eval: nth(func(a, b float64) int { return cmpFloat(b, a) }), format: inheritFrom(0)},
		&FuncDef{Name: "SMALL", Args: "data, n", Desc: "The nth smallest number", Min: 2, Max: 2,
			eval: nth(cmpFloat), format: inheritFrom(0)},
		&FuncDef{Name: "RANK", Args: "value, data, [is_ascending]", Desc: "Rank of a number among others, largest first by default", Min: 2, Max: 3,
			eval: rank},
	)
}

// Evaluators for the table above, in its order.

func averageIf(args []Node, get lookup) Value {
	rng := matrixArg(args[0], get)
	cv := eval(args[1], get)
	if cv.Kind == Error {
		return cv
	}
	c := newCriterion(cv)
	avg := rng
	if len(args) > 2 {
		avg = matrixArg(args[2], get).resized(rng.rows, rng.cols, get)
	}
	mask := make([]bool, rng.size())
	for i := range mask {
		mask[i] = c.test(rng.at(i))
	}
	return averageMasked(avg, mask)
}

func averageIfs(args []Node, get lookup) Value {
	avg := matrixArg(args[0], get)
	rows, cols, mask, err := criteriaMask(args, 1, get)
	switch {
	case err != nil:
		return *err
	case rows != avg.rows || cols != avg.cols:
		return ErrValue
	}
	return averageMasked(avg, mask)
}

func countBlank(args []Node, get lookup) Value {
	m := matrixArg(args[0], get)
	n := 0
	for i := range m.size() {
		if v := m.at(i); v.Kind == Empty || v.Kind == Text && v.Str == "" {
			n++
		}
	}
	return num(float64(n))
}

func median(x []float64) Value {
	if len(x) == 0 {
		return ErrNum
	}
	slices.Sort(x)
	n := len(x)
	if n%2 == 1 {
		return num(x[n/2])
	}
	return num((x[n/2-1] + x[n/2]) / 2)
}

func mode(x []float64) Value {
	best, bestN := 0.0, 1
	for i, v := range x {
		n := 0
		for _, w := range x[i:] {
			if w == v {
				n++
			}
		}
		if n > bestN {
			best, bestN = v, n
		}
	}
	if bestN < 2 {
		return ErrNA
	}
	return num(best)
}

func rank(args []Node, get lookup) Value {
	v, err := numArg(args[0], get)
	if err != nil {
		return *err
	}
	data, err := nums(args[1:2], get)
	if err != nil {
		return *err
	}
	asc, err := boolArg(args, 2, false, get)
	if err != nil {
		return *err
	}
	rank, found := 1, false
	for _, d := range data {
		switch {
		case d == v:
			found = true
		case asc && d < v, !asc && d > v:
			rank++
		}
	}
	if !found {
		return ErrNA
	}
	return num(float64(rank))
}

func countIfs(args []Node, get lookup) Value {
	_, _, mask, err := criteriaMask(args, 0, get)
	if err != nil {
		return *err
	}
	n := 0
	for _, ok := range mask {
		if ok {
			n++
		}
	}
	return num(float64(n))
}

// averageMasked averages the numbers of m where mask is set.
func averageMasked(m matrix, mask []bool) Value {
	sum, n := 0.0, 0
	for i, ok := range mask {
		if !ok {
			continue
		}
		v := m.at(i)
		switch v.Kind {
		case Error:
			return v
		case Number:
			sum += v.Num
			n++
		}
	}
	if n == 0 {
		return ErrDiv0
	}
	return num(sum / float64(n))
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
		return ErrDiv0
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

// nth builds LARGE and SMALL: the nth number of data in cmp order.
func nth(cmp func(a, b float64) int) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		x, err := nums(args[:1], get)
		if err != nil {
			return *err
		}
		n, err := numArg(args[1], get)
		if err != nil {
			return *err
		}
		k := int(math.Ceil(n))
		if k < 1 || k > len(x) {
			return ErrNum
		}
		slices.SortFunc(x, cmp)
		return num(x[k-1])
	}
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
