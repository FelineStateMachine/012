package sheet

import (
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Argument helpers shared by the function library. Errors come back as a
// *Value to return as is, so functions propagate the first error they
// meet, as Sheets does.

func numArg(n Node, get lookup) (float64, *Value) {
	v := eval(n, get)
	if v.Kind == Error {
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
		return 0, &ErrNum
	}
	return int(f), nil
}

func textArg(n Node, get lookup) (string, *Value) {
	v := eval(n, get)
	if v.Kind == Error {
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
// treated as a 1x1 block.
type matrix struct {
	rows, cols int
	cell       func(r, c int) Value
	origin     Addr   // top-left cell when the block is a reference
	sheet      string // the reference's sheet, as written
	ref        bool
}

func (m matrix) at(i int) Value { return m.cell(i/m.cols, i%m.cols) }

func (m matrix) size() int { return m.rows * m.cols }

// vector reports whether m is a single row or column.
func (m matrix) vector() bool { return m.rows == 1 || m.cols == 1 }

func matrixArg(n Node, get lookup) matrix {
	switch n := n.(type) {
	case formula.Range:
		return rectMatrix(n.Sheet, n.Rect, get)
	case formula.Ref:
		return rectMatrix(n.Sheet, Rect{From: n.Addr, To: n.Addr}, get)
	}
	v := eval(n, get)
	return matrix{rows: 1, cols: 1, cell: func(int, int) Value { return v }}
}

func rectMatrix(sheet string, r Rect, get lookup) matrix {
	return matrix{
		rows: r.To.Row - r.From.Row + 1, cols: r.To.Col - r.From.Col + 1,
		cell: func(row, col int) Value {
			return get(sheet, Addr{Col: r.From.Col + col, Row: r.From.Row + row})
		},
		origin: r.From, sheet: sheet, ref: true,
	}
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
		case v.Kind == Error:
			return errOf(v)
		case v.Kind == Empty, v.Kind != Number && !direct:
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

// texts flattens args to strings for CONCATENATE and TEXTJOIN: every cell
// of a range, blanks as "".
func texts(args []Node, get lookup) ([]string, *Value) {
	var out []string
	e := each(args, get, func(v Value, _ bool) *Value {
		if v.Kind == Error {
			return errOf(v)
		}
		out = append(out, text(v))
		return nil
	})
	return out, e
}

func str(s string) Value { return Value{Kind: Text, Str: s} }

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
	op   string // =, <>, <, >, <=, >=
	kind Kind   // Number, Text, Bool or Empty (blank)
	num  float64
	text string
}

func newCriterion(v Value) criterion {
	c := criterion{op: "=", kind: v.Kind, num: v.Num}
	if v.Kind == Text {
		s := v.Str
		for _, op := range []string{"<=", ">=", "<>", "<", ">", "="} {
			if rest, ok := strings.CutPrefix(s, op); ok {
				c.op, s = op, rest
				break
			}
		}
		c.text = s
		switch n, _, isNum := ParseValue(s); {
		case s == "":
			c.kind = Empty
		case isNum:
			c.kind, c.num = Number, n
		case strings.EqualFold(s, "TRUE") || strings.EqualFold(s, "FALSE"):
			c.kind, c.num = Bool, 0
			if strings.EqualFold(s, "TRUE") {
				c.num = 1
			}
		}
	}
	return c
}

// test reports whether a cell value satisfies the criterion.
func (c criterion) test(v Value) bool {
	blank := v.Kind == Empty || v.Kind == Text && v.Str == ""
	if c.kind == Empty {
		switch c.op {
		case "=":
			return blank
		case "<>":
			return !blank
		}
		return false
	}
	if c.kind == Text {
		if v.Kind != Text {
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

// criteriaMask evaluates (range, criterion) pairs starting at args[i],
// returning which cells of the first range satisfy all of them. All
// ranges must be the same size.
func criteriaMask(args []Node, i int, get lookup) (rows, cols int, mask []bool, err *Value) {
	for ; i+1 < len(args); i += 2 {
		m := matrixArg(args[i], get)
		cv := eval(args[i+1], get)
		if cv.Kind == Error {
			return 0, 0, nil, &cv
		}
		c := newCriterion(cv)
		if mask == nil {
			rows, cols = m.rows, m.cols
			mask = make([]bool, m.size())
			for k := range mask {
				mask[k] = true
			}
		} else if m.rows != rows || m.cols != cols {
			return 0, 0, nil, &ErrValue
		}
		for k := range mask {
			if mask[k] && !c.test(m.at(k)) {
				mask[k] = false
			}
		}
	}
	return rows, cols, mask, nil
}
