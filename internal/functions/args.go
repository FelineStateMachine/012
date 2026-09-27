package functions

import (
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Argument helpers shared by the function library. Errors come back as a
// *Value to return as is, so functions propagate the first error they
// meet, as Sheets does.

func numArg(n Node, get lookup) (float64, *Value) {
	v := eval1(n, get)
	if v.Kind == value.Error {
		return 0, errOf(v)
	}
	return toNum(v)
}

// given reports whether argument i was passed and not left empty.
func given(args []Node, i int) bool {
	if i >= len(args) {
		return false
	}
	_, empty := args[i].(formula.Empty)
	return !empty
}

// optNum is argument i as a number, or def when it wasn't given.
func optNum(args []Node, i int, def float64, get lookup) (float64, *Value) {
	if !given(args, i) {
		return def, nil
	}
	return numArg(args[i], get)
}

// intArg is a number argument truncated toward zero, as Sheets does for
// counts and positions.
func intArg(args []Node, i int, def float64, get lookup) (int, *Value) {
	f, err := optNum(args, i, def, get)
	if err != nil {
		return 0, err
	}
	if f > 1e9 || f < -1e9 {
		return 0, &value.ErrNum
	}
	return int(f), nil
}

func textArg(n Node, get lookup) (string, *Value) {
	v := eval1(n, get)
	if v.Kind == value.Error {
		return "", errOf(v)
	}
	return text(v), nil
}

func boolArg(args []Node, i int, def bool, get lookup) (bool, *Value) {
	if !given(args, i) {
		return def, nil
	}
	f, err := numArg(args[i], get)
	return f != 0, err
}

// matrix is a rectangular block of values: a range, or a single value
// treated as a 1x1 block. Only its top-left dataRows x dataCols may hold
// data: every cell past them is blank, so functions that walk a range
// (SUMIF(A:A, ...), MATCH, COUNTBLANK) visit what the sheet holds rather
// than the million rows of a whole column, while ROWS, INDEX and the
// positions they return keep the range's full size.
type matrix struct {
	rows, cols int
	cell       func(r, c int) Value
	origin     Addr   // top-left cell when the block is a reference
	sheet      string // the reference's sheet, as written
	ref        bool

	dataRows, dataCols int
	blank              Value // every cell outside the data: blank, or #REF! for a missing sheet
}

func (m matrix) at(i int) Value { return m.cell(i/m.cols, i%m.cols) }

func (m matrix) size() int { return m.rows * m.cols }

// vector reports whether m is a single row or column.
func (m matrix) vector() bool { return m.rows == 1 || m.cols == 1 }

// dataLen is how many of a vector's leading entries may hold data.
func (m matrix) dataLen() int {
	if m.cols == 1 {
		return m.dataRows
	}
	return m.dataCols
}

func matrixArg(n Node, get lookup) matrix {
	switch n := get.refOf(n).(type) {
	case formula.Range:
		return rectMatrix(n.Sheet, n.Rect, get)
	case formula.Ref:
		return rectMatrix(n.Sheet, Rect{From: n.Addr, To: n.Addr}, get)
	}
	v := wholeArg(n, get)
	if a := get.arrayOf(v); a != nil {
		return arrayMatrix(a)
	}
	if v.Kind == value.Array {
		v = value.ErrValue // a LAMBDA
	}
	return matrix{rows: 1, cols: 1, cell: func(int, int) Value { return v }, dataRows: 1, dataCols: 1, blank: v}
}

// arrayMatrix is an array as a block of values.
func arrayMatrix(a *Array) matrix {
	return matrix{rows: a.Rows, cols: a.Cols, cell: a.At, dataRows: a.DRows, dataCols: a.DCols, blank: a.Fill}
}

// wholeArg evaluates an argument that takes ranges or arrays, in an
// array context: the whole of an argument standing in for one being
// mapped over (liftArg), not its entry.
func wholeArg(n Node, get lookup) Value {
	if a, ok := n.(liftArg); ok {
		return a.value(get)
	}
	return evalArr(n, get)
}

func rectMatrix(sheet string, r Rect, get lookup) matrix {
	m := matrix{
		rows: r.To.Row - r.From.Row + 1, cols: r.To.Col - r.From.Col + 1,
		origin: r.From, sheet: sheet, ref: true,
	}
	switch b, any, exists := get.book.Bounds(sheet, r); {
	case !exists:
		m.blank = value.ErrRef
	case get.dense:
		m.dataRows, m.dataCols = m.rows, m.cols
	case any:
		m.dataRows, m.dataCols = b.To.Row-r.From.Row+1, b.To.Col-r.From.Col+1
	}
	rows, cols, blank := m.dataRows, m.dataCols, m.blank
	m.cell = func(row, col int) Value {
		if row >= rows || col >= cols {
			return blank
		}
		return get.cell(sheet, Addr{Col: r.From.Col + col, Row: r.From.Row + row})
	}
	return m
}

// part is the top-left rows x cols of a block that isn't a range.
func (m matrix) part(rows, cols int) matrix {
	m.rows, m.cols = rows, cols
	m.dataRows, m.dataCols = min(m.dataRows, rows), min(m.dataCols, cols)
	return m
}

// resized returns a block of rows x cols from the same top-left cell, as
// SUMIF does with a sum range of a different size.
func (m matrix) resized(rows, cols int, get lookup) matrix {
	if !m.ref {
		return m
	}
	to := Addr{Col: m.origin.Col + cols - 1, Row: m.origin.Row + rows - 1}
	return rectMatrix(m.sheet, Rect{From: m.origin, To: to}, get)
}

// nums collects the numbers in args with aggregate semantics: in ranges
// only numbers count; direct arguments are coerced.
func nums(args []Node, get lookup) ([]float64, *Value) {
	var out []float64
	e := each(args, get, func(v Value, direct bool) *Value {
		switch {
		case v.Kind == value.Error:
			return errOf(v)
		case v.Kind == value.Empty, v.Kind != value.Number && !direct:
			return nil
		}
		f, err := toNum(v)
		if err != nil {
			return err
		}
		out = append(out, f)
		return nil
	})
	return out, e
}

// texts flattens args to strings for CONCATENATE and TEXTJOIN: the cells
// of ranges that hold something, and any other argument, blank or not.
func texts(args []Node, get lookup) ([]string, *Value) {
	var out []string
	e := each(args, get, func(v Value, _ bool) *Value {
		if v.Kind == value.Error {
			return errOf(v)
		}
		out = append(out, text(v))
		return nil
	})
	return out, e
}

// textsWithBlanks is texts with every blank cell of a range as "", as
// TEXTJOIN keeping empty entries needs, but at most limit of them in all:
// past that many the joined text is too long anyway.
func textsWithBlanks(args []Node, get lookup, limit int) ([]string, *Value) {
	var out []string
	blanks := func(n int) {
		for ; n > 0 && len(out) < limit; n-- {
			out = append(out, "")
		}
	}
	for _, arg := range args {
		rn, ok := get.refOf(arg).(formula.Range)
		if !ok {
			if e := eachOf(arg, get, func(v Value, _ bool) *Value {
				if v.Kind == value.Error {
					return errOf(v)
				}
				out = append(out, text(v))
				return nil
			}); e != nil {
				return nil, e
			}
			continue
		}
		r, last := rn.Rect, -1
		width := r.To.Col - r.From.Col + 1
		var e *Value
		get.cells(rn.Sheet, r, func(a Addr, v Value) bool {
			if v.Kind == value.Error {
				e = errOf(v)
				return false
			}
			p := (a.Row-r.From.Row)*width + a.Col - r.From.Col
			blanks(p - last - 1)
			out, last = append(out, text(v)), p
			return true
		})
		if e != nil {
			return nil, e
		}
		blanks(width*(r.To.Row-r.From.Row+1) - last - 1)
	}
	return out, nil
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// wildMatch matches s against a Sheets wildcard pattern, case-insensitively:
// * is any run, ? any one character, ~ escapes the next.
func wildMatch(pattern, s string) bool {
	p := []rune(strings.ToLower(pattern))
	t := []rune(strings.ToLower(s))
	// Classic backtracking over the last *.
	pi, ti, star, mark := 0, 0, -1, 0
	for ti < len(t) {
		switch {
		case pi < len(p) && p[pi] == '~' && pi+1 < len(p) && p[pi+1] == t[ti]:
			pi += 2
			ti++
		case pi < len(p) && p[pi] != '~' && (p[pi] == '?' || p[pi] == t[ti]) && p[pi] != '*':
			pi++
			ti++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, ti
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			ti = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

func hasWildcards(s string) bool { return strings.ContainsAny(s, "*?~") }

// criterion is a condition of SUMIF, COUNTIFS and friends: 5, ">5",
// "<>done", "a*", "" (blank) or TRUE.
type criterion struct {
	op   string     // =, <>, <, >, <=, >=
	kind value.Kind // Number, Text, Bool or Empty (blank)
	num  float64
	text string
}

func newCriterion(v Value) criterion {
	c := criterion{op: "=", kind: v.Kind, num: v.Num}
	if v.Kind == value.Text {
		s := v.Str
		for _, op := range []string{"<=", ">=", "<>", "<", ">", "="} {
			if rest, ok := strings.CutPrefix(s, op); ok {
				c.op, s = op, rest
				break
			}
		}
		c.text = s
		switch n, _, isNum := value.ParseValue(s); {
		case s == "":
			c.kind = value.Empty
		case isNum:
			c.kind, c.num = value.Number, n
		case strings.EqualFold(s, "TRUE") || strings.EqualFold(s, "FALSE"):
			c.kind, c.num = value.Bool, 0
			if strings.EqualFold(s, "TRUE") {
				c.num = 1
			}
		}
	}
	return c
}

// test reports whether a cell value satisfies the criterion.
func (c criterion) test(v Value) bool {
	blank := v.Kind == value.Empty || v.Kind == value.Text && v.Str == ""
	if c.kind == value.Empty {
		switch c.op {
		case "=":
			return blank
		case "<>":
			return !blank
		}
		return false
	}
	if c.kind == value.Text {
		if v.Kind != value.Text {
			return c.op == "<>"
		}
		switch c.op {
		case "=":
			return wildMatch(c.text, v.Str)
		case "<>":
			return !wildMatch(c.text, v.Str)
		}
		return cmpResult(c.op, strings.Compare(strings.ToLower(v.Str), strings.ToLower(c.text)))
	}
	// Numbers and booleans only match their own kind.
	if v.Kind != c.kind {
		return c.op == "<>"
	}
	d := 0
	switch {
	case v.Num < c.num:
		d = -1
	case v.Num > c.num:
		d = 1
	}
	return cmpResult(c.op, d)
}

func cmpResult(op string, d int) bool {
	switch op {
	case "=":
		return d == 0
	case "<>":
		return d != 0
	case "<":
		return d < 0
	case ">":
		return d > 0
	case "<=":
		return d <= 0
	}
	return d >= 0
}

// criteriaArgs reads the (range, criterion) pairs starting at args[i].
// All ranges must be the same size.
func criteriaArgs(args []Node, i int, get lookup) ([]matrix, []criterion, *Value) {
	var ms []matrix
	var cs []criterion
	for ; i+1 < len(args); i += 2 {
		m := matrixArg(args[i], get)
		cv := eval(args[i+1], get)
		if cv.Kind == value.Error {
			return nil, nil, &cv
		}
		if len(ms) > 0 && (m.rows != ms[0].rows || m.cols != ms[0].cols) {
			return nil, nil, &value.ErrValue
		}
		ms, cs = append(ms, m), append(cs, newCriterion(cv))
	}
	return ms, cs, nil
}
