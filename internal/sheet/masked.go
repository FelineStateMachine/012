package sheet

// Functions over aligned ranges (SUMIF, COUNTIFS, SUMPRODUCT...) walk
// only the part of their ranges that may hold data, the union of the
// ranges' data areas; the cells past it are all blank, so they are
// accounted for at once. SUMIF(A:A, "", B:B) over ten filled rows looks at
// ten rows, however tall the sheet.

// dataArea is the union of the data areas of same-sized matrices.
func dataArea(ms ...matrix) (rows, cols int) {
	for _, m := range ms {
		rows = max(rows, min(m.dataRows, m.rows))
		cols = max(cols, min(m.dataCols, m.cols))
	}
	return rows, cols
}

// masked is which cells of aligned ranges meet criteria: over the data
// area (rows x cols, row-major), and for the cells past it, which are
// the same everywhere.
type masked struct {
	rows, cols int
	pass       []bool
	tail       int  // cells past the data area
	tailPass   bool // whether they meet the criteria
}

// applyCriteria tests each criterion on its range, over the data area of
// the ranges and of also, which is aligned with them.
func applyCriteria(ms []matrix, cs []criterion, also ...matrix) masked {
	rows, cols := dataArea(append(also, ms...)...)
	k := masked{rows: rows, cols: cols, pass: make([]bool, rows*cols), tailPass: true}
	for i := range k.pass {
		k.pass[i] = true
	}
	if len(ms) > 0 {
		k.tail = ms[0].size() - rows*cols
	}
	for j, m := range ms {
		for i, ok := range k.pass {
			if ok && !cs[j].test(m.cell(i/cols, i%cols)) {
				k.pass[i] = false
			}
		}
		k.tailPass = k.tailPass && cs[j].test(m.blank)
	}
	return k
}

// sumMasked adds the numbers of m where the criteria pass.
func sumMasked(m matrix, k masked) Value {
	total := 0.0
	for i, ok := range k.pass {
		if !ok {
			continue
		}
		switch v := m.cell(i/k.cols, i%k.cols); v.Kind {
		case Error:
			return v
		case Number:
			total += v.Num
		}
	}
	if k.tail > 0 && k.tailPass && m.blank.Kind == Error {
		return m.blank
	}
	return num(total)
}

// averageMasked averages the numbers of m where the criteria pass.
func averageMasked(m matrix, k masked) Value {
	sum, n := 0.0, 0
	for i, ok := range k.pass {
		if !ok {
			continue
		}
		switch v := m.cell(i/k.cols, i%k.cols); v.Kind {
		case Error:
			return v
		case Number:
			sum += v.Num
			n++
		}
	}
	switch {
	case k.tail > 0 && k.tailPass && m.blank.Kind == Error:
		return m.blank
	case n == 0:
		return ErrDiv0
	}
	return num(sum / float64(n))
}

func sumIf(args []Node, get lookup) Value {
	rng := matrixArg(args[0], get)
	cv := eval(args[1], get)
	if cv.Kind == Error {
		return cv
	}
	sum := rng
	if len(args) > 2 {
		sum = matrixArg(args[2], get).resized(rng.rows, rng.cols, get)
		if sum.rows != rng.rows || sum.cols != rng.cols {
			return ErrValue
		}
	}
	return sumMasked(sum, applyCriteria([]matrix{rng}, []criterion{newCriterion(cv)}, sum))
}

func sumIfs(args []Node, get lookup) Value {
	sum := matrixArg(args[0], get)
	ms, cs, err := criteriaArgs(args, 1, get)
	switch {
	case err != nil:
		return *err
	case len(ms) == 0 || ms[0].rows != sum.rows || ms[0].cols != sum.cols:
		return ErrValue
	}
	return sumMasked(sum, applyCriteria(ms, cs, sum))
}

func averageIf(args []Node, get lookup) Value {
	rng := matrixArg(args[0], get)
	cv := eval(args[1], get)
	if cv.Kind == Error {
		return cv
	}
	avg := rng
	if len(args) > 2 {
		avg = matrixArg(args[2], get).resized(rng.rows, rng.cols, get)
	}
	return averageMasked(avg, applyCriteria([]matrix{rng}, []criterion{newCriterion(cv)}, avg))
}

func averageIfs(args []Node, get lookup) Value {
	avg := matrixArg(args[0], get)
	ms, cs, err := criteriaArgs(args, 1, get)
	switch {
	case err != nil:
		return *err
	case len(ms) == 0 || ms[0].rows != avg.rows || ms[0].cols != avg.cols:
		return ErrValue
	}
	return averageMasked(avg, applyCriteria(ms, cs, avg))
}

func countIfs(args []Node, get lookup) Value {
	ms, cs, err := criteriaArgs(args, 0, get)
	if err != nil {
		return *err
	}
	k := applyCriteria(ms, cs)
	n := 0
	for _, ok := range k.pass {
		if ok {
			n++
		}
	}
	if k.tailPass {
		n += k.tail
	}
	return num(float64(n))
}

func countBlank(args []Node, get lookup) Value {
	m := matrixArg(args[0], get)
	isBlank := func(v Value) bool { return v.Kind == Empty || v.Kind == Text && v.Str == "" }
	rows, cols := dataArea(m)
	n := 0
	for r := range rows {
		for c := range cols {
			if isBlank(m.cell(r, c)) {
				n++
			}
		}
	}
	if isBlank(m.blank) {
		n += m.size() - rows*cols
	}
	return num(float64(n))
}

// sumProduct multiplies same-sized arrays entry by entry and sums; text
// and blanks count as 0.
func sumProduct(args []Node, get lookup) Value {
	ms := make([]matrix, len(args))
	for i, a := range args {
		ms[i] = matrixArg(a, get)
		if ms[i].rows != ms[0].rows || ms[i].cols != ms[0].cols {
			return ErrValue
		}
	}
	rows, cols := dataArea(ms...)
	total := 0.0
	for k := range rows * cols {
		p := 1.0
		for _, m := range ms {
			switch v := m.cell(k/cols, k%cols); v.Kind {
			case Error:
				return v
			case Number:
				p *= v.Num
			default:
				p = 0
			}
		}
		total += p
	}
	if ms[0].size() > rows*cols {
		for _, m := range ms {
			if m.blank.Kind == Error {
				return m.blank
			}
		}
	}
	return num(total)
}
