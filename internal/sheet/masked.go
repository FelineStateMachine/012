package sheet

import (
	"cmp"
	"slices"
)

// Functions over aligned ranges (SUMIF, COUNTIFS, SUMPRODUCT...) visit
// only the cells their ranges hold; the rest are blank in all of them,
// so they are accounted for at once. SUMIF(A:A, "", B:B) over ten filled
// rows looks at ten cells, however tall the sheet, and ten cells spread
// over a million rows are still ten.

// pos is a cell of same-sized matrices, by row and column from their
// top-left corner.
type pos struct{ r, c int }

// storedPos returns the cells of m that hold something, row by row, and
// false when every cell of its data area must be read instead (as tests
// ask with denseReads).
func (m matrix) storedPos(get lookup) ([]pos, bool) {
	if !m.ref {
		return []pos{{}}, true
	}
	t := get.sheet(m.sheet)
	switch {
	case denseReads:
		return nil, false
	case t == nil:
		return nil, true
	}
	r := Rect{From: m.origin, To: Addr{Col: m.origin.Col + m.cols - 1, Row: m.origin.Row + m.rows - 1}}
	var out []pos
	for a := range t.cells.inRange(r) {
		out = append(out, pos{a.Row - r.From.Row, a.Col - r.From.Col})
	}
	return out, true
}

// cellsOf returns the cells to visit of same-sized matrices: those any of
// them holds, in row-major order. Every other cell is blank in all of
// them.
func cellsOf(get lookup, ms ...matrix) []pos {
	var all []pos
	for _, m := range ms {
		p, ok := m.storedPos(get)
		if !ok {
			return denseArea(ms...)
		}
		all = append(all, p...)
	}
	if len(ms) > 1 {
		slices.SortFunc(all, func(a, b pos) int { return cmp.Or(cmp.Compare(a.r, b.r), cmp.Compare(a.c, b.c)) })
		all = slices.Compact(all)
	}
	return all
}

// denseArea is every cell of the union of the matrices' data areas.
func denseArea(ms ...matrix) []pos {
	rows, cols := 0, 0
	for _, m := range ms {
		rows = max(rows, min(m.dataRows, m.rows))
		cols = max(cols, min(m.dataCols, m.cols))
	}
	out := make([]pos, 0, rows*cols)
	for r := range rows {
		for c := range cols {
			out = append(out, pos{r, c})
		}
	}
	return out
}

// masked is which cells of aligned ranges meet criteria: those visited
// that do, and whether the rest, all blank, do.
type masked struct {
	pass     []pos
	tail     int  // cells not visited
	tailPass bool // whether they meet the criteria
}

// applyCriteria tests each criterion on its range, over the cells the
// ranges and also (aligned with them) hold.
func applyCriteria(get lookup, ms []matrix, cs []criterion, also ...matrix) masked {
	cells := cellsOf(get, append(also, ms...)...)
	k := masked{tail: ms[0].size() - len(cells), tailPass: true}
	for _, p := range cells {
		if passes(ms, cs, p) {
			k.pass = append(k.pass, p)
		}
	}
	for j, m := range ms {
		k.tailPass = k.tailPass && cs[j].test(m.blank)
	}
	return k
}

// passes reports whether the cell at p meets every criterion.
func passes(ms []matrix, cs []criterion, p pos) bool {
	for j, m := range ms {
		if !cs[j].test(m.cell(p.r, p.c)) {
			return false
		}
	}
	return true
}

// sumMasked calls add with the numbers of m where the criteria pass, or
// returns the first error there.
func sumMasked(m matrix, k masked, add func(float64)) *Value {
	for _, p := range k.pass {
		switch v := m.cell(p.r, p.c); v.Kind {
		case Error:
			return &v
		case Number:
			add(v.Num)
		}
	}
	if k.tail > 0 && k.tailPass && m.blank.Kind == Error {
		return &m.blank
	}
	return nil
}

// averageMasked averages the numbers of m where the criteria pass.
func averageMasked(m matrix, k masked) Value {
	sum, n := 0.0, 0
	for _, p := range k.pass {
		switch v := m.cell(p.r, p.c); v.Kind {
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

// sumIf, sumIfs and sumProduct add in binary; their terms functions
// visit the same terms for decimal arithmetic too.
func sumIf(args []Node, get lookup) Value {
	total := 0.0
	if e := sumIfTerms(args, get, func(f float64) { total += f }); e != nil {
		return *e
	}
	return num(total)
}

func sumIfs(args []Node, get lookup) Value {
	total := 0.0
	if e := sumIfsTerms(args, get, func(f float64) { total += f }); e != nil {
		return *e
	}
	return num(total)
}

func sumProduct(args []Node, get lookup) Value {
	total := 0.0
	e := sumProductTerms(args, get, func(fs []float64) {
		p := 1.0
		for _, f := range fs {
			p *= f
		}
		total += p
	})
	if e != nil {
		return *e
	}
	return num(total)
}

// sumIfTerms calls add with each number SUMIF sums, or returns the error
// it gives.
func sumIfTerms(args []Node, get lookup, add func(float64)) *Value {
	rng := matrixArg(args[0], get)
	cv := eval(args[1], get)
	if cv.Kind == Error {
		return &cv
	}
	sum := rng
	if len(args) > 2 {
		sum = matrixArg(args[2], get).resized(rng.rows, rng.cols, get)
		if sum.rows != rng.rows || sum.cols != rng.cols {
			return &ErrValue
		}
	}
	return sumMasked(sum, applyCriteria(get, []matrix{rng}, []criterion{newCriterion(cv)}, sum), add)
}

// sumIfsTerms calls add with each number SUMIFS sums, or returns the
// error it gives.
func sumIfsTerms(args []Node, get lookup, add func(float64)) *Value {
	sum := matrixArg(args[0], get)
	ms, cs, err := criteriaArgs(args, 1, get)
	switch {
	case err != nil:
		return err
	case len(ms) == 0 || ms[0].rows != sum.rows || ms[0].cols != sum.cols:
		return &ErrValue
	}
	return sumMasked(sum, applyCriteria(get, ms, cs, sum), add)
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
	return averageMasked(avg, applyCriteria(get, []matrix{rng}, []criterion{newCriterion(cv)}, avg))
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
	return averageMasked(avg, applyCriteria(get, ms, cs, avg))
}

func countIfs(args []Node, get lookup) Value {
	ms, cs, err := criteriaArgs(args, 0, get)
	if err != nil {
		return *err
	}
	k := applyCriteria(get, ms, cs)
	n := len(k.pass)
	if k.tailPass {
		n += k.tail
	}
	return num(float64(n))
}

func countBlank(args []Node, get lookup) Value {
	m := matrixArg(args[0], get)
	isBlank := func(v Value) bool { return v.Kind == Empty || v.Kind == Text && v.Str == "" }
	cells := cellsOf(get, m)
	n := 0
	for _, p := range cells {
		if isBlank(m.cell(p.r, p.c)) {
			n++
		}
	}
	if isBlank(m.blank) {
		n += m.size() - len(cells)
	}
	return num(float64(n))
}

// sumProductTerms calls term with the factors of each entry of
// same-sized arrays that any of them holds (text and blanks count as 0;
// entries blank in all of them are 0 and left out), or returns the
// error SUMPRODUCT gives. The slice is reused between calls.
func sumProductTerms(args []Node, get lookup, term func(fs []float64)) *Value {
	ms := make([]matrix, len(args))
	for i, a := range args {
		ms[i] = matrixArg(a, get)
		if ms[i].rows != ms[0].rows || ms[i].cols != ms[0].cols {
			return &ErrValue
		}
	}
	cells := cellsOf(get, ms...)
	fs := make([]float64, len(ms))
	for _, pc := range cells {
		for i, m := range ms {
			switch v := m.cell(pc.r, pc.c); v.Kind {
			case Error:
				return &v
			case Number:
				fs[i] = v.Num
			default:
				fs[i] = 0
			}
		}
		term(fs)
	}
	if ms[0].size() > len(cells) {
		for _, m := range ms {
			if m.blank.Kind == Error {
				return &m.blank
			}
		}
	}
	return nil
}
