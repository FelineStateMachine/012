package sheet

import (
	"cmp"
	"slices"
	"strings"
)

// Summarize is how a pivot value summarizes a group's cells, as Sheets'
// "Summarize by". Each is one entry of summaries.
type Summarize uint8

const (
	SumBy Summarize = iota
	CountABy
	CountBy
	CountUniqueBy
	AverageBy
	MaxBy
	MinBy
	// CountRowsBy counts rows, blank or not. Sheets has no such choice;
	// frequency tables use it so blank values are counted.
	CountRowsBy
	numSummarize
)

// summary is one way of summarizing: its name in files, its title (the
// function Sheets names it after), what it does, whether the result
// keeps the column's number format, and how it's computed.
type summary struct {
	name, title, desc string
	keepsFormat       bool
	result            func(a *accum) Value
}

var summaries = [numSummarize]summary{
	SumBy: {"sum", "SUM", "Add the numbers", true, func(a *accum) Value {
		return a.or(num(a.sum))
	}},
	CountABy: {"counta", "COUNTA", "Count the cells that aren't blank", false, func(a *accum) Value {
		return num(float64(a.counta))
	}},
	CountBy: {"count", "COUNT", "Count the numbers", false, func(a *accum) Value {
		return num(float64(a.n))
	}},
	CountUniqueBy: {"countunique", "COUNTUNIQUE", "Count the distinct values", false, func(a *accum) Value {
		return num(float64(len(a.uniq)))
	}},
	AverageBy: {"average", "AVERAGE", "Average the numbers", true, func(a *accum) Value {
		if a.n == 0 {
			return a.or(ErrDiv0)
		}
		return a.or(num(a.sum / float64(a.n)))
	}},
	MaxBy: {"max", "MAX", "The largest number", true, func(a *accum) Value {
		return a.or(num(a.hi))
	}},
	MinBy: {"min", "MIN", "The smallest number", true, func(a *accum) Value {
		return a.or(num(a.lo))
	}},
	CountRowsBy: {"rows", "ROWS", "Count the rows, blank or not", false, func(a *accum) Value {
		return num(float64(a.rows))
	}},
}

// Summaries lists the choices of "Summarize by", in Sheets' order.
func Summaries() []Summarize {
	return []Summarize{SumBy, CountABy, CountBy, CountUniqueBy, AverageBy, MaxBy, MinBy}
}

func (f Summarize) String() string { return summaries[f].name }
func (f Summarize) Title() string  { return summaries[f].title }
func (f Summarize) Desc() string   { return summaries[f].desc }

// ParseSummarize is the inverse of Summarize.String.
func ParseSummarize(name string) (Summarize, bool) {
	for i, s := range summaries {
		if s.name == name {
			return Summarize(i), true
		}
	}
	return SumBy, false
}

// accum gathers one value of one group: what every summary needs, so a
// value can be summarized any way without reading the source again.
type accum struct {
	rows, counta, n int
	sum, lo, hi     float64
	err             Value // the first error, which SUM and the rest show
	uniq            map[groupKey]struct{}
}

func (a *accum) add(v Value, unique bool) {
	a.rows++
	switch v.Kind {
	case Empty:
		return
	case Number:
		if a.n == 0 {
			a.lo, a.hi = v.Num, v.Num
		}
		a.lo, a.hi = min(a.lo, v.Num), max(a.hi, v.Num)
		a.n++
		a.sum += v.Num
	case Error:
		if a.err.Kind == Empty {
			a.err = v
		}
	}
	a.counta++
	if unique {
		if a.uniq == nil {
			a.uniq = map[groupKey]struct{}{}
		}
		a.uniq[keyOf(v, false)] = struct{}{}
	}
}

// or returns the first error gathered, if any, else v.
func (a *accum) or(v Value) Value {
	if a.err.Kind == Error {
		return a.err
	}
	return v
}

// groupKey identifies a value for grouping: numbers (dates too) by value,
// text ignoring case when folded, as Sheets groups "east" with "East".
// Numbers and text never share a key, so 1 and "1" are two groups. It is
// a comparable struct, so looking a group up allocates nothing.
type groupKey struct {
	kind Kind
	num  float64
	str  string
}

func keyOf(v Value, fold bool) groupKey {
	k := groupKey{kind: v.Kind, num: v.Num, str: v.Str}
	if fold && v.Kind == Text {
		k.str = strings.ToLower(v.Str)
	}
	return k
}

// pivotNode is a group of source rows: the root holds them all, its
// children the groups of the first row field, and so on. Each keeps its
// accumulators by column group (-1 for all of them), then by value.
type pivotNode struct {
	label  Value
	format Format
	kids   []*pivotNode
	byKey  map[groupKey]*pivotNode
	acc    map[int][]accum
}

// colGroup is one combination of column fields' values.
type colGroup struct {
	id      int
	labels  []Value
	formats []Format
}

// colNode finds column groups by each column field's value in turn.
type colNode struct {
	kids  map[groupKey]*colNode
	group *colGroup
}

// pivotCalc is a pivot being computed.
type pivotCalc struct {
	w       *Workbook
	p       *Pivot
	src     *Sheet
	root    *pivotNode
	cols    []*colGroup // sorted once gathered
	colRoot colNode
	unique  []bool   // by value: whether to gather distinct values
	formats []Format // by value: the format results show in
	records int      // source rows summarized
}

func newPivotCalc(w *Workbook, p *Pivot, src *Sheet) *pivotCalc {
	c := &pivotCalc{w: w, p: p, src: src, root: &pivotNode{}}
	for _, v := range p.Values {
		c.unique = append(c.unique, v.Summarize == CountUniqueBy)
		c.formats = append(c.formats, c.valueFormat(v))
	}
	return c
}

// valueFormat is the format a value's results show in: percent for
// shares, the column's own for sums, averages, minimums and maximums.
func (c *pivotCalc) valueFormat(v PivotValue) Format {
	if v.ShowAs != ShowValue {
		return Preset(FmtPercent)
	}
	if !summaries[v.Summarize].keepsFormat {
		return Format{}
	}
	for row := c.p.Range.From.Row + 1; row <= c.p.Range.To.Row; row++ {
		if a := (Addr{Col: v.Col, Row: row}); !c.src.cells.get(a).Blank() {
			return c.src.DisplayFormat(a)
		}
	}
	return Format{}
}

// gather reads the source rows the filters let through, skipping rows
// blank across the range, into groups.
func (c *pivotCalc) gather() {
	tests := pivotTests(c.p.Filters)
	r := c.p.Range
	for row := r.From.Row + 1; row <= r.To.Row; row++ {
		if c.src.rowBlank(row, r) || !c.src.rowPasses(row, tests) {
			continue
		}
		c.records++
		col := c.colGroupOf(row)
		n := c.root
		c.add(n, col, row)
		for _, g := range c.p.Rows {
			n = n.child(c.src, Addr{Col: g.Col, Row: row})
			c.add(n, col, row)
		}
	}
	c.sortRows(c.root, 0)
	c.sortCols()
}

// rowBlank reports whether row is blank across r's columns.
func (s *Sheet) rowBlank(row int, r Rect) bool {
	for col := r.From.Col; col <= r.To.Col; col++ {
		if !s.cells.get(Addr{Col: col, Row: row}).Blank() {
			return false
		}
	}
	return true
}

// pivotTests prepares a pivot's filters as a filter's column tests.
func pivotTests(fs []PivotFilter) []colTest {
	var tests []colTest
	for _, f := range fs {
		if f.Criteria.IsZero() {
			continue
		}
		t := colTest{col: f.Col, cond: f.Criteria.Cond.test()}
		if len(f.Criteria.Hidden) > 0 {
			t.hidden = make(map[string]bool, len(f.Criteria.Hidden))
			for _, h := range f.Criteria.Hidden {
				t.hidden[h] = true
			}
		}
		tests = append(tests, t)
	}
	return tests
}

// child returns the group of n for the value at a, adding it if new.
func (n *pivotNode) child(src *Sheet, a Addr) *pivotNode {
	v := src.Value(a)
	k := keyOf(v, true)
	if kid, ok := n.byKey[k]; ok {
		return kid
	}
	if n.byKey == nil {
		n.byKey = map[groupKey]*pivotNode{}
	}
	kid := &pivotNode{label: v, format: src.DisplayFormat(a)}
	n.byKey[k] = kid
	n.kids = append(n.kids, kid)
	return kid
}

// colGroupOf returns the id of row's column group, or -1 without column
// fields.
func (c *pivotCalc) colGroupOf(row int) int {
	if len(c.p.Columns) == 0 || len(c.p.Values) == 0 {
		return -1
	}
	n := &c.colRoot
	for _, g := range c.p.Columns {
		k := keyOf(c.src.Value(Addr{Col: g.Col, Row: row}), true)
		kid := n.kids[k]
		if kid == nil {
			if n.kids == nil {
				n.kids = map[groupKey]*colNode{}
			}
			kid = &colNode{}
			n.kids[k] = kid
		}
		n = kid
	}
	if n.group == nil {
		cg := &colGroup{id: len(c.cols)}
		for _, g := range c.p.Columns {
			a := Addr{Col: g.Col, Row: row}
			cg.labels = append(cg.labels, c.src.Value(a))
			cg.formats = append(cg.formats, c.src.DisplayFormat(a))
		}
		n.group = cg
		c.cols = append(c.cols, cg)
	}
	return n.group.id
}

// add gathers row's values into n, for its column group and for all.
func (c *pivotCalc) add(n *pivotNode, col, row int) {
	if len(c.p.Values) == 0 {
		return
	}
	if n.acc == nil {
		n.acc = map[int][]accum{}
	}
	for _, id := range [2]int{-1, col} {
		accs := n.acc[id]
		if accs == nil {
			accs = make([]accum, len(c.p.Values))
			n.acc[id] = accs
		}
		for i, v := range c.p.Values {
			accs[i].add(c.src.Value(Addr{Col: v.Col, Row: row}), c.unique[i])
		}
		if col == -1 {
			return
		}
	}
}

// result is value i of n in column group col, as gathered (not yet shown
// as a share), and false when no row of n is in col.
func (c *pivotCalc) result(n *pivotNode, col, i int) (Value, bool) {
	accs, ok := n.acc[col]
	if !ok {
		return Value{}, false
	}
	return summaries[c.p.Values[i].Summarize].result(&accs[i]), true
}

// sortRows orders n's children by the row field at depth, then theirs.
func (c *pivotCalc) sortRows(n *pivotNode, depth int) {
	if depth >= len(c.p.Rows) {
		return
	}
	g := c.p.Rows[depth]
	slices.SortStableFunc(n.kids, func(a, b *pivotNode) int {
		return c.order(g, []Value{a.label}, []Value{b.label}, func(i int) (Value, Value) {
			va, _ := c.result(a, -1, i)
			vb, _ := c.result(b, -1, i)
			return va, vb
		})
	})
	for _, kid := range n.kids {
		c.sortRows(kid, depth+1)
	}
}

// sortCols orders the column groups by each column field in turn.
func (c *pivotCalc) sortCols() {
	slices.SortStableFunc(c.cols, func(a, b *colGroup) int {
		for k, g := range c.p.Columns {
			d := c.order(g, a.labels[k:k+1], b.labels[k:k+1], func(i int) (Value, Value) {
				va, _ := c.result(c.root, a.id, i)
				vb, _ := c.result(c.root, b.id, i)
				return va, vb
			})
			if d != 0 {
				return d
			}
		}
		return 0
	})
}

// order compares two groups by a field's order: by label, as Sheets sorts
// (blanks last either way), or by a value's total, labels breaking ties.
func (c *pivotCalc) order(g PivotGroup, a, b []Value, totals func(i int) (Value, Value)) int {
	sign := 1
	if g.Desc {
		sign = -1
	}
	if i := g.SortBy - 1; i >= 0 && i < len(c.p.Values) {
		va, vb := totals(i)
		if d := compareTotals(va, vb); d != 0 {
			return sign * d
		}
		sign = 1
	}
	return labelOrder(a[0], b[0], sign)
}

// labelOrder compares group labels A to Z (sign 1) or Z to A (-1), blanks
// last either way.
func labelOrder(a, b Value, sign int) int {
	if (a.Kind == Empty) != (b.Kind == Empty) {
		if a.Kind == Empty {
			return 1
		}
		return -1
	}
	return sign * sortCompare(a, b)
}

// compareTotals orders totals by number, anything else (an error, a
// group with no rows) below every number.
func compareTotals(a, b Value) int {
	an, bn := a.Kind == Number, b.Kind == Number
	switch {
	case an && bn:
		return cmp.Compare(a.Num, b.Num)
	case an:
		return 1
	case bn:
		return -1
	}
	return 0
}

// shown is value i of n in column group col as the pivot shows it: as
// gathered, or as a share of its row's, column's or the grand total.
func (c *pivotCalc) shown(n *pivotNode, col, i int) (Value, bool) {
	v, ok := c.result(n, col, i)
	show := c.p.Values[i].ShowAs
	if !ok || show == ShowValue || v.Kind != Number {
		return v, ok
	}
	var total Value
	switch show {
	case ShowPctRow:
		total, _ = c.result(n, -1, i)
	case ShowPctColumn:
		total, _ = c.result(c.root, col, i)
	default:
		total, _ = c.result(c.root, -1, i)
	}
	switch {
	case total.Kind != Number:
		return total, true
	case total.Num == 0:
		return ErrDiv0, true
	}
	return num(v.Num / total.Num), true
}
