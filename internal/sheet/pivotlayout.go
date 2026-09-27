package sheet

import (
	"fmt"
	"slices"
	"time"
)

// A pivot's results are laid out as Sheets lays them out, from A1: header
// rows (one per column field, naming the field and its values, then one
// naming the row fields and the values), then a row per group of the row
// fields, their labels shown where they change, with a subtotal row after
// each outer group and a Grand Total row when row totals are on. Columns
// are the row labels, then each column group's values, then the Grand
// Total column's.

// pivotCell is one cell of a pivot's results.
type pivotCell struct {
	a    Addr
	v    Value
	f    Format
	bold bool
}

// pivotLayout is a pivot's results being laid out.
type pivotLayout struct {
	c     *pivotCalc
	cells []pivotCell
	cols  []*colGroup // the column groups shown, nil for the Grand Total
	lc    int         // label columns
	y     int         // the next row
	over  bool        // something fell off the sheet
}

// blankLabel stands for a group of blank cells, as in Excel.
const blankLabel = "(blank)"

func (c *pivotCalc) layout() *pivotLayout {
	l := &pivotLayout{c: c, lc: max(len(c.p.Rows), 1)}
	if len(c.p.Values) > 0 {
		if len(c.p.Columns) == 0 {
			l.cols = []*colGroup{nil}
		} else {
			l.cols = slices.Clone(c.cols)
			if c.p.ColumnTotals {
				l.cols = append(l.cols, nil)
			}
		}
	}
	l.header()
	l.body()
	return l
}

func (l *pivotLayout) put(col, row int, v Value, f Format, bold bool) {
	if col >= MaxCols || row >= MaxRows {
		l.over = true
		return
	}
	l.cells = append(l.cells, pivotCell{Addr{Col: col, Row: row}, v, f, bold})
}

func (l *pivotLayout) text(col, row int, s string, bold bool) {
	l.put(col, row, Value{Kind: Text, Str: s}, Format{}, bold)
}

// label shows a group's value in its own format, or (blank).
func (l *pivotLayout) label(col, row int, v Value, f Format, bold bool) {
	if v.Kind == Empty {
		l.text(col, row, blankLabel, bold)
		return
	}
	l.put(col, row, v, f, bold)
}

func (l *pivotLayout) header() {
	c := l.c
	nv := len(c.p.Values)
	if nv > 0 {
		for k, g := range c.p.Columns {
			l.text(l.lc-1, l.y, c.w.FieldName(*c.p, g.Col), true)
			var prev *colGroup
			for j, cg := range l.cols {
				x := l.lc + j*nv
				switch {
				case cg == nil && k == 0:
					l.text(x, l.y, "Grand Total", true)
				case cg == nil:
				case prev == nil || !sameLabels(prev.labels[:k+1], cg.labels[:k+1]):
					l.label(x, l.y, cg.labels[k], cg.formats[k], true)
				}
				prev = cg
			}
			l.y++
		}
	}
	for d, g := range c.p.Rows {
		l.text(d, l.y, c.w.FieldName(*c.p, g.Col), true)
	}
	for j := range l.cols {
		for i, v := range c.p.Values {
			l.text(l.lc+j*nv+i, l.y, c.w.ValueTitle(*c.p, v), true)
		}
	}
	l.y++
}

func sameLabels(a, b []Value) bool {
	for i := range a {
		if keyOf(a[i], true) != keyOf(b[i], true) {
			return false
		}
	}
	return true
}

func (l *pivotLayout) body() {
	c := l.c
	if len(c.p.Rows) == 0 {
		if len(c.p.Values) > 0 {
			l.total(c.root, "Grand Total", 0)
		}
		return
	}
	l.rows(c.root, 0)
	if l.totals() {
		l.total(c.root, "Grand Total", 0)
	}
}

// rows lays out n's groups at depth: a label where each starts, then its
// groups or, at the last depth, its values.
func (l *pivotLayout) rows(n *pivotNode, depth int) {
	last := depth == len(l.c.p.Rows)-1
	for _, kid := range n.kids {
		l.label(depth, l.y, kid.label, kid.format, false)
		if last {
			l.data(kid, false)
			l.y++
			continue
		}
		l.rows(kid, depth+1)
		if l.totals() {
			name := blankLabel
			if kid.label.Kind != Empty {
				name = FormatText(kid.label, kid.format)
			}
			l.total(kid, name+" Total", depth)
		}
	}
}

// totals reports whether total rows show: with row totals on, and values
// to total.
func (l *pivotLayout) totals() bool { return l.c.p.RowTotals && len(l.c.p.Values) > 0 }

// total is a bold row of n's values labelled name in column col.
func (l *pivotLayout) total(n *pivotNode, name string, col int) {
	l.text(col, l.y, name, true)
	l.data(n, true)
	l.y++
}

// data lays out n's values in the current row.
func (l *pivotLayout) data(n *pivotNode, bold bool) {
	nv := len(l.c.p.Values)
	for j, cg := range l.cols {
		id := -1
		if cg != nil {
			id = cg.id
		}
		for i := range nv {
			if v, ok := l.c.shown(n, id, i); ok {
				l.put(l.lc+j*nv+i, l.y, v, l.c.formats[i], bold)
			}
		}
	}
}

// PivotInfo describes one pivot recomputation, for telemetry. It holds
// counts only, never contents.
type PivotInfo struct {
	Records  int // source rows summarized
	Groups   int // groups of the row fields
	Cells    int // cells of the results
	Failed   bool
	Duration time.Duration
}

// OnPivot, when set, is called after every pivot recomputation, like
// OnRecalc.
var OnPivot func(PivotInfo)

// maxPivotChain bounds pivots reading pivots that are refreshed in turn
// after one change.
const maxPivotChain = 16

// stalePivots returns the sheets whose pivot needs recomputing: its
// definition changed, a cell in its results was overwritten, or a cell
// of its source range is being recalculated. It reads the sheets' calc
// marks, so it runs between affected and evaluate.
func (w *Workbook) stalePivots() []*Sheet {
	var out []*Sheet
	for _, s := range w.sheets {
		if s.pivot.stale {
			out = append(out, s)
			continue
		}
		p := s.pivot.def
		if p == nil {
			continue
		}
		if src := w.Lookup(p.Source); src != nil && src != s && touched(src.calc, p.Range) {
			out = append(out, s)
		}
	}
	return out
}

func touched(calc map[Addr]int, r Rect) bool {
	for a := range calc {
		if r.Contains(a) {
			return true
		}
	}
	return false
}

// allPivots returns every live sheet that has a pivot or was told to
// recompute one.
func (w *Workbook) allPivots() []*Sheet {
	var out []*Sheet
	for _, s := range w.sheets {
		if s.pivot.def != nil || s.pivot.stale {
			out = append(out, s)
		}
	}
	return out
}

// refreshPivots recomputes the pivots of sheets, then recalculates what
// reads their results, which may be other pivots.
func (w *Workbook) refreshPivots(sheets []*Sheet) {
	var changed []loc
	for _, s := range sheets {
		changed = append(changed, w.refreshPivot(s)...)
	}
	if len(changed) == 0 {
		return
	}
	if w.pivotDepth >= maxPivotChain {
		w.Circular = true
		return
	}
	circular := w.Circular
	w.pivotDepth++
	w.recalc(changed)
	w.pivotDepth--
	w.Circular = w.Circular || circular
}

// refreshPivot recomputes s's pivot and writes its results, returning the
// cells that changed.
func (w *Workbook) refreshPivot(s *Sheet) []loc {
	var start time.Time
	if OnPivot != nil {
		start = time.Now()
	}
	s.pivot.stale = false
	want, calc, err := w.computePivot(s)
	s.pivot.err, s.pivot.blocked = "", Rect{}
	if err != nil {
		// Where the results would go, so clearing a cell in their way
		// has the pivot try again.
		s.pivot.blocked = extent(want)
		s.pivot.err = err.Error()
		want = []pivotCell{{v: ErrRef, bold: true}}
		if c := s.cells.get(Addr{}); c != nil && !c.derived && !c.Blank() {
			want = nil // never write over what the user typed
		}
	}
	s.pivot.out = extent(want)
	changed := s.writeDerived(want)
	if s.pivot.fit {
		s.fitPivot(want)
	}
	if OnPivot != nil {
		info := PivotInfo{Cells: len(want), Failed: err != nil, Duration: time.Since(start)}
		if calc != nil {
			info.Records, info.Groups = calc.records, calc.groups()
		}
		OnPivot(info)
	}
	return changed
}

// extent is the range from A1 covering cells.
func extent(cells []pivotCell) Rect {
	r := Rect{}
	for _, pc := range cells {
		r = union(r, Rect{From: pc.a, To: pc.a})
	}
	return r
}

// groups counts the groups of the row fields, at every depth.
func (c *pivotCalc) groups() int {
	n := 0
	var walk func(p *pivotNode)
	walk = func(p *pivotNode) {
		n += len(p.kids)
		for _, k := range p.kids {
			walk(k)
		}
	}
	walk(c.root)
	return n
}

// computePivot works out the cells of s's pivot, or why it can't.
func (w *Workbook) computePivot(s *Sheet) ([]pivotCell, *pivotCalc, error) {
	p := s.pivot.def
	if p == nil || !s.live {
		return nil, nil, nil
	}
	src := w.Lookup(p.Source)
	switch {
	case src == nil:
		return nil, nil, fmt.Errorf("The pivot table's source sheet %s doesn't exist", p.Source)
	case p.Lost:
		return nil, nil, fmt.Errorf("The pivot table's source range was deleted")
	case src == s:
		return nil, nil, errPivotSelf
	}
	for _, col := range p.fieldCols() {
		if col < p.Range.From.Col || col > p.Range.To.Col {
			return nil, nil, fmt.Errorf("Column %s is outside the pivot table's source range %s", ColName(col), p.Range)
		}
	}
	c := newPivotCalc(w, p, src)
	c.gather()
	l := c.layout()
	if l.over {
		return nil, c, fmt.Errorf("The pivot table doesn't fit on a sheet")
	}
	for _, pc := range l.cells {
		if old := s.cells.get(pc.a); old != nil && !old.derived && !old.Blank() {
			return l.cells, c, fmt.Errorf("The pivot table's results would overwrite data in %s", pc.a)
		}
	}
	return l.cells, c, nil
}

// fieldCols lists the source columns the pivot's fields read.
func (p *Pivot) fieldCols() []int {
	var cols []int
	for _, g := range slices.Concat(p.Rows, p.Columns) {
		cols = append(cols, g.Col)
	}
	for _, v := range p.Values {
		cols = append(cols, v.Col)
	}
	for _, f := range p.Filters {
		cols = append(cols, f.Col)
	}
	return cols
}

// writeDerived makes the sheet's derived cells exactly want, touching only
// cells that differ, outside the undo history (undo restores the source
// and the definition, and the pivot is recomputed from them). It returns
// the cells that changed.
func (s *Sheet) writeDerived(want []pivotCell) []loc {
	next := make(map[Addr]*Cell, len(want))
	for _, pc := range want {
		c := &Cell{Input: derivedInput(pc.v), Value: pc.v, Format: pc.f, derived: true}
		c.Style.Bold = pc.bold
		next[pc.a] = c
	}
	var changed []loc
	for a, c := range s.cells.all() {
		if c.derived && next[a] == nil {
			s.setDerived(a, nil)
			changed = append(changed, loc{s, a})
		}
	}
	for a, c := range next {
		if old := s.cells.get(a); old != nil && old.derived && old.Input == c.Input && old.Value == c.Value &&
			old.Format == c.Format && old.Style == c.Style {
			continue
		}
		s.setDerived(a, c)
		changed = append(changed, loc{s, a})
	}
	return changed
}

// setDerived stores or removes a derived cell without recording it for
// undo. Derived cells have no formula, so there are no references to
// index.
func (s *Sheet) setDerived(a Addr, c *Cell) {
	s.version++
	s.unlink(a)
	if c != nil {
		s.cells.set(a, c)
	}
}

// maxPivotWidth caps how wide fitting makes a pivot's column.
const maxPivotWidth = 30

// fitPivot widens the result's columns to their widest text, never below
// the default width. Numbers in the General format count at most
// maxGeneralFit characters: General shows fewer digits in a narrower
// column, as an average's 15 digits needn't all show.
func (s *Sheet) fitPivot(cells []pivotCell) {
	const maxGeneralFit = 10
	widest := map[int]int{}
	for _, pc := range cells {
		n := len([]rune(s.ShownText(pc.a)))
		if pc.v.Kind == Number && pc.f.IsZero() {
			n = min(n, maxGeneralFit)
		}
		widest[pc.a.Col] = max(widest[pc.a.Col], n)
	}
	for col, w := range widest {
		s.setWidth(col, clampInt(w+2, DefaultWidth, maxPivotWidth))
	}
}

// derivedInput is what the formula bar shows for a result: the value.
func derivedInput(v Value) string {
	if v.Kind == Text {
		return v.Str
	}
	return valueInput(v)
}

// plain returns a copy of c as an ordinary cell: a pivot's result becomes
// the constant it shows, so copying results pastes values.
func (c *Cell) plain() *Cell {
	if c == nil || !c.derived {
		return c.clone()
	}
	p, err := newCell(valueInput(c.Value), c.Format, c.Style, false)
	if err != nil || p == nil {
		return &Cell{Input: "'" + c.Value.String(), Value: c.Value, Format: c.Format, Style: c.Style}
	}
	return p
}
