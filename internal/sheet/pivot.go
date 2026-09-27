package sheet

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// A pivot table summarizes a range of another sheet, as Sheets' Insert >
// Pivot table: rows grouped by the values of some columns (Rows), spread
// across others (Columns), with values summarized per group (Values), and
// rows left out by Filters. It lives on a sheet of its own, starting at
// A1. The engine owns its results: they are derived cells, recomputed
// whenever the source's cells change, never saved (the file keeps the
// definition) and never edited by hand. A sheet has at most one pivot.
type Pivot struct {
	// Source names the sheet the data is on. Like a reference written
	// with a sheet name, it follows renames, and while no sheet has the
	// name the pivot shows #REF!.
	Source string
	// Range is the data on the source sheet; its first row holds the
	// headers, which name the fields.
	Range   Rect
	Rows    []PivotGroup
	Columns []PivotGroup
	Values  []PivotValue
	Filters []PivotFilter
	// RowTotals adds a Grand Total row, and a subtotal row after each
	// outer group when there are several row groups. ColumnTotals adds a
	// Grand Total column when there are column groups.
	RowTotals, ColumnTotals bool
	// Lost is set when the source range was deleted.
	Lost bool
}

// PivotGroup is a column of the source whose distinct values become rows
// or columns of the pivot.
type PivotGroup struct {
	Col  int  // column on the source sheet
	Desc bool // Z to A, or largest first
	// SortBy orders the groups by their label (0) or by the grand total
	// of Values[SortBy-1].
	SortBy int
}

// PivotValue is a column of the source summarized per group.
type PivotValue struct {
	Col       int
	Summarize Summarize
	ShowAs    ShowAs
	Name      string // the header; "" for Sheets' "SUM of Sales"
}

// PivotFilter leaves out the source rows whose value in Col doesn't meet
// Criteria, as a filter's column does.
type PivotFilter struct {
	Col      int
	Criteria Criteria
}

// ShowAs is how a summarized value shows: as itself, or as a share of a
// total, as Sheets' "Show as".
type ShowAs uint8

const (
	ShowValue ShowAs = iota
	ShowPctRow
	ShowPctColumn
	ShowPctTotal
	numShowAs
)

var showAsNames = [numShowAs]string{"", "percent_of_row", "percent_of_column", "percent_of_total"}
var showAsTitles = [numShowAs]string{"Default", "% of row", "% of column", "% of grand total"}

// ShowAsList lists the choices in Sheets' order.
func ShowAsList() []ShowAs { return []ShowAs{ShowValue, ShowPctRow, ShowPctColumn, ShowPctTotal} }

func (s ShowAs) String() string { return showAsNames[s] }
func (s ShowAs) Title() string  { return showAsTitles[s] }

// ParseShowAs is the inverse of ShowAs.String.
func ParseShowAs(name string) (ShowAs, bool) {
	i := slices.Index(showAsNames[:], name)
	return ShowAs(max(i, 0)), i >= 0
}

// Errors of pivot tables.
var (
	// ErrPivotEdit is what editing a pivot table's results says, as
	// Sheets refuses to.
	ErrPivotEdit = errors.New("Pivot table results can't be edited: change the pivot with Data > Edit pivot table")
	errPivotSelf = errors.New("A pivot table can't summarize its own sheet")
	errPivotLoop = errors.New("A pivot table can't summarize a pivot table that reads it")
)

// pivotState is what a sheet keeps about its pivot. def is the definition
// (in the undo history and the file, never modified in place); the rest
// is derived from it and the source.
type pivotState struct {
	def   *Pivot
	stale bool   // recompute at the end of the current change
	out   Rect   // what the last result covered, from A1
	err   string // why the pivot shows #REF!, or ""
}

// clone returns a deep copy, so a stored pivot never shares slices.
func (p *Pivot) clone() *Pivot {
	if p == nil {
		return nil
	}
	cp := *p
	cp.Rows, cp.Columns, cp.Values = slices.Clone(p.Rows), slices.Clone(p.Columns), slices.Clone(p.Values)
	cp.Filters = make([]PivotFilter, len(p.Filters))
	for i, f := range p.Filters {
		f.Criteria.Hidden = slices.Clone(f.Criteria.Hidden)
		cp.Filters[i] = f
	}
	if len(p.Filters) == 0 {
		cp.Filters = nil
	}
	return &cp
}

func samePivot(a, b *Pivot) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(a.clone(), b.clone())
}

// Pivot returns a copy of the sheet's pivot table, and false if it has
// none.
func (s *Sheet) Pivot() (Pivot, bool) {
	if s.pivot.def == nil {
		return Pivot{}, false
	}
	return *s.pivot.def.clone(), true
}

// PivotRange returns the cells the pivot's results cover, from A1 (at
// least A1, even while it shows nothing), and false without a pivot.
func (s *Sheet) PivotRange() (Rect, bool) {
	if s.pivot.def == nil {
		return Rect{}, false
	}
	return s.pivot.out, true
}

// InPivot reports whether r overlaps the pivot table's results, which
// can't be edited.
func (s *Sheet) InPivot(r Rect) bool {
	out, ok := s.PivotRange()
	return ok && out.From.Col <= r.To.Col && r.From.Col <= out.To.Col &&
		out.From.Row <= r.To.Row && r.From.Row <= out.To.Row
}

// PivotError says why the pivot shows #REF!, or "" when it doesn't.
func (s *Sheet) PivotError() string { return s.pivot.err }

// PivotSource returns the sheet the pivot reads, or nil when no sheet has
// its name.
func (s *Sheet) PivotSource() *Sheet {
	if s.pivot.def == nil {
		return nil
	}
	return s.wb.Lookup(s.pivot.def.Source)
}

// NextPivotName is the name Sheets gives a new pivot sheet: Pivot Table 1,
// then 2 and so on.
func (w *Workbook) NextPivotName() string {
	for n := 1; ; n++ {
		if name := fmt.Sprintf("Pivot Table %d", n); w.Lookup(name) == nil {
			return name
		}
	}
}

// CreatePivot adds a sheet named name ("" for the next Pivot Table N)
// right after src, holding p over r on src, as one undo step.
func (w *Workbook) CreatePivot(src *Sheet, r Rect, name string, p Pivot) (*Sheet, error) {
	if name == "" {
		name = w.NextPivotName()
	}
	name = w.freeName(name)
	if err := w.checkName(nil, name); err != nil {
		return nil, err
	}
	p.Source, p.Range = src.name, r
	s := w.newSheet(name)
	w.change(s, "create "+name, Rect{}, func() {
		w.recordSheets()
		w.insert(s, w.Index(src)+1)
		s.pivot.def = p.clone()
		s.pivot.stale = true
		s.pivot.out = Rect{}
	})
	return s, nil
}

// SetPivot replaces the sheet's pivot definition as one undo step,
// labelled label ("edit pivot table" when empty).
func (s *Sheet) SetPivot(p Pivot, label string) error {
	if err := s.checkPivot(p); err != nil {
		return err
	}
	if samePivot(&p, s.pivot.def) {
		return nil
	}
	if label == "" {
		label = "edit pivot table"
	}
	s.change(label, Rect{}, func() { s.putPivot(p.clone()) })
	return nil
}

// checkPivot refuses a pivot reading its own sheet, or a pivot whose
// results it reads.
func (s *Sheet) checkPivot(p Pivot) error {
	name := p.Source
	for range len(s.wb.sheets) + 1 {
		src := s.wb.Lookup(name)
		switch {
		case src == nil || src.pivot.def == nil:
			return nil
		case src == s && name == p.Source:
			return errPivotSelf
		case src == s:
			return errPivotLoop
		}
		name = src.pivot.def.Source
	}
	return errPivotLoop
}

// putPivot stores a pivot definition (nil for none), recording the old one
// for undo, and has it recomputed.
func (s *Sheet) putPivot(p *Pivot) {
	s.recordPivot()
	s.pivot.def = p
	s.pivot.stale = true
}

// recordPivot saves the sheet's pivot before its first change in the
// open step.
func (s *Sheet) recordPivot() {
	if st := s.wb.hist.open; st != nil {
		if _, seen := st.pivots[s]; !seen {
			st.pivots[s] = s.pivot.def
		}
	}
}

// pivotsOn returns the sheets whose pivot reads src.
func (w *Workbook) pivotsOn(src *Sheet) []*Sheet {
	var out []*Sheet
	for _, s := range w.sheets {
		if p := s.pivot.def; p != nil && w.Lookup(p.Source) == src {
			out = append(out, s)
		}
	}
	return out
}

// renamePivots points pivots reading the sheet with key old at name.
func (w *Workbook) renamePivots(old, name string) {
	for _, s := range w.sheets {
		if p := s.pivot.def; p != nil && formula.SheetKey(p.Source) == old {
			np := p.clone()
			np.Source = name
			s.putPivot(np)
		}
	}
}

// shiftPivots keeps the pivots reading s in step with rows or columns
// inserted or deleted on it: the range moves or resizes like a range
// reference, fields on deleted columns go, and a pivot whose whole range
// was deleted shows #REF!.
func (s *Sheet) shiftPivots(rows bool, sp formula.Span) {
	cell, rng := formula.AxisMaps(rows, sp)
	for _, t := range s.wb.pivotsOn(s) {
		p := t.pivot.def.clone()
		if r, ok := rng(p.Range); ok && !p.Lost {
			p.Range = r
		} else {
			p.Lost = true
		}
		if !rows {
			col := func(c int) (int, bool) {
				to, ok := cell(Addr{Col: c})
				return to.Col, ok
			}
			p.Rows, p.Columns = shiftGroups(p.Rows, col), shiftGroups(p.Columns, col)
			p.Values = shiftCols(p.Values, func(v *PivotValue) *int { return &v.Col }, col)
			p.Filters = shiftCols(p.Filters, func(f *PivotFilter) *int { return &f.Col }, col)
		}
		if !samePivot(p, t.pivot.def) {
			t.putPivot(p)
		}
	}
}

func shiftGroups(gs []PivotGroup, col func(int) (int, bool)) []PivotGroup {
	return shiftCols(gs, func(g *PivotGroup) *int { return &g.Col }, col)
}

// shiftCols moves the column of each item, dropping those on deleted
// columns.
func shiftCols[T any](items []T, at func(*T) *int, col func(int) (int, bool)) []T {
	var out []T
	for _, it := range items {
		c := at(&it)
		if to, ok := col(*c); ok {
			*c = to
			out = append(out, it)
		}
	}
	return out
}

// FieldName is the header of column col in the pivot's source, or
// "Column B" when the header is blank.
func (w *Workbook) FieldName(p Pivot, col int) string {
	if src := w.Lookup(p.Source); src != nil && !p.Lost {
		if h := src.displayText(Addr{Col: col, Row: p.Range.From.Row}); h != "" {
			return h
		}
	}
	return "Column " + ColName(col)
}

// ValueTitle is the header of a value: its name, or Sheets' "SUM of
// Sales".
func (w *Workbook) ValueTitle(p Pivot, v PivotValue) string {
	if v.Name != "" {
		return v.Name
	}
	return v.Summarize.Title() + " of " + w.FieldName(p, v.Col)
}

// DefaultSummarize is how a new value summarizes column col, as in
// Sheets: SUM when the column holds a number, COUNTA otherwise.
func (w *Workbook) DefaultSummarize(p Pivot, col int) Summarize {
	if src := w.Lookup(p.Source); src != nil {
		for row := p.Range.From.Row + 1; row <= p.Range.To.Row; row++ {
			if src.Value(Addr{Col: col, Row: row}).Kind == Number {
				return SumBy
			}
		}
	}
	return CountABy
}

// NewPivot starts a pivot over r on src with nothing chosen yet and both
// grand totals on, as Sheets' new pivot tables.
func NewPivot(src *Sheet, r Rect) Pivot {
	return Pivot{Source: src.name, Range: r, RowTotals: true, ColumnTotals: true}
}

// FrequencyPivot is a frequency table of column col of r: each distinct
// value with how many rows have it and their share, most frequent first,
// as VisiData's Shift+F. It is a pivot like any other; it counts rows
// rather than values, so blanks are counted too.
func FrequencyPivot(src *Sheet, r Rect, col int) Pivot {
	p := NewPivot(src, r)
	p.Rows = []PivotGroup{{Col: col, Desc: true, SortBy: 1}}
	p.Values = []PivotValue{
		{Col: col, Summarize: CountRowsBy, Name: "Count"},
		{Col: col, Summarize: CountRowsBy, ShowAs: ShowPctTotal, Name: "Percent"},
	}
	return p
}
