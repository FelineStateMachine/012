package sheet

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// A filter hides the rows of a range whose values don't meet criteria set
// per column, as Sheets' Data > Create a filter. Rows are hidden, never
// deleted: formulas still see them (SUM over a filtered range includes
// hidden rows, as in Sheets). The range's first row holds the headers and
// is never hidden. Criteria are re-applied whenever values change.
type Filter struct {
	Range Rect
	Cols  map[int]Criteria // by column; a column without criteria hides nothing
}

// Criteria is what one column of a filter lets through: its displayed
// value must not be one of Hidden (Sheets' "Filter by values", unchecked
// values; "" stands for blanks) and must meet Cond ("Filter by condition").
type Criteria struct {
	Hidden []string
	Cond   Condition
}

// IsZero reports whether the criteria let every row through.
func (c Criteria) IsZero() bool { return len(c.Hidden) == 0 && c.Cond.Op == CondNone }

// Condition is a test on a cell, e.g. "greater than 100".
type Condition struct {
	Op  CondOp
	Arg string // the value compared with, as typed; unused by CondEmpty and CondNotEmpty
}

// CondOp is one of Sheets' filter conditions.
type CondOp uint8

const (
	CondNone CondOp = iota
	CondEmpty
	CondNotEmpty
	CondContains
	CondNotContains
	CondStartsWith
	CondEndsWith
	CondExactly
	CondGreater
	CondGreaterEq
	CondLess
	CondLessEq
	CondEqual
	CondNotEqual
	numCondOps
)

var condNames = [numCondOps]string{"", "empty", "not_empty", "contains", "not_contains",
	"starts_with", "ends_with", "exactly", "gt", "ge", "lt", "le", "eq", "ne"}

var condTitles = [numCondOps]string{"None", "Is empty", "Is not empty", "Text contains",
	"Text does not contain", "Text starts with", "Text ends with", "Text is exactly",
	"Greater than", "Greater than or equal to", "Less than", "Less than or equal to",
	"Is equal to", "Is not equal to"}

// CondOps lists the conditions in the order Sheets' menu shows them.
func CondOps() []CondOp {
	out := make([]CondOp, numCondOps)
	for i := range out {
		out[i] = CondOp(i)
	}
	return out
}

// String names the condition as stored in files.
func (op CondOp) String() string { return condNames[op] }

// Title names the condition for people, e.g. "Greater than".
func (op CondOp) Title() string { return condTitles[op] }

// TakesArg reports whether the condition compares with a value.
func (op CondOp) TakesArg() bool { return op != CondNone && op != CondEmpty && op != CondNotEmpty }

// ParseCondOp is the inverse of CondOp.String.
func ParseCondOp(s string) (CondOp, bool) {
	for i, n := range condNames {
		if n == s {
			return CondOp(i), true
		}
	}
	return CondNone, false
}

// clone returns a deep copy, so a stored filter is never changed in place.
func (f *Filter) clone() *Filter {
	if f == nil {
		return nil
	}
	cp := &Filter{Range: f.Range, Cols: make(map[int]Criteria, len(f.Cols))}
	for c, cr := range f.Cols {
		if cr.IsZero() {
			continue
		}
		cr.Hidden = slices.Clone(cr.Hidden)
		cp.Cols[c] = cr
	}
	return cp
}

// Filter returns a copy of the sheet's filter, or nil if it has none.
func (s *Sheet) Filter() *Filter { return s.view.filter.clone() }

// FilterRange returns the filter's range, and false if there is no
// filter.
func (s *Sheet) FilterRange() (Rect, bool) {
	if f := s.view.filter; f != nil {
		return f.Range, true
	}
	return Rect{}, false
}

// ColumnFiltered reports whether the filter has criteria for column col.
func (s *Sheet) ColumnFiltered(col int) bool {
	f := s.view.filter
	return f != nil && !f.Cols[col].IsZero()
}

// CreateFilter puts a filter on r, whose first row is the header row.
func (s *Sheet) CreateFilter(r Rect) {
	s.setFilter("create a filter on "+r.String(), &Filter{Range: r, Cols: map[int]Criteria{}})
}

// RemoveFilter removes the filter, showing every row again.
func (s *Sheet) RemoveFilter() {
	s.setFilter("remove the filter", nil)
}

// FilterColumn sets the criteria for column col of the filter.
func (s *Sheet) FilterColumn(col int, cr Criteria) {
	f := s.view.filter.clone()
	if f == nil {
		return
	}
	if cr.IsZero() {
		delete(f.Cols, col)
	} else {
		cr.Hidden = slices.Clone(cr.Hidden)
		slices.Sort(cr.Hidden)
		cr.Hidden = slices.Compact(cr.Hidden)
		f.Cols[col] = cr
	}
	s.setFilter("filter column "+ColName(col), f)
}

func (s *Sheet) setFilter(label string, f *Filter) {
	if reflect.DeepEqual(f.clone(), s.view.filter.clone()) {
		return
	}
	focus := Rect{}
	if f != nil {
		focus = f.Range
	} else if old := s.view.filter; old != nil {
		focus = old.Range
	}
	s.change(label, focus, func() {
		s.recordView()
		s.view.filter = f.clone()
		s.hidden.valid = false
	})
}

// hiddenCache remembers which rows the filter hides until values or the
// filter change, so navigation and drawing stay cheap.
type hiddenCache struct {
	valid bool
	rows  map[int]struct{}
}

// RowHidden reports whether the filter hides row r.
func (s *Sheet) RowHidden(r int) bool {
	if s.view.filter == nil {
		return false
	}
	s.updateHidden()
	_, hid := s.hidden.rows[r]
	return hid
}

// HiddenRows returns how many rows the filter hides.
func (s *Sheet) HiddenRows() int {
	if s.view.filter == nil {
		return 0
	}
	s.updateHidden()
	return len(s.hidden.rows)
}

func (s *Sheet) updateHidden() {
	if s.hidden.valid {
		return
	}
	s.hidden = hiddenCache{valid: true, rows: map[int]struct{}{}}
	f := s.view.filter
	if f == nil || len(f.Cols) == 0 {
		return
	}
	tests := s.criteriaTests(f, -1)
	for row := f.Range.From.Row + 1; row <= f.Range.To.Row; row++ {
		if !s.rowPasses(row, tests) {
			s.hidden.rows[row] = struct{}{}
		}
	}
}

// colTest is one column's criteria, ready to run.
type colTest struct {
	col    int
	hidden map[string]bool
	cond   func(Value, string) bool
}

// criteriaTests prepares the filter's criteria, leaving out column skip.
func (s *Sheet) criteriaTests(f *Filter, skip int) []colTest {
	var tests []colTest
	for _, c := range slices.Sorted(maps.Keys(f.Cols)) {
		cr := f.Cols[c]
		if c == skip || cr.IsZero() {
			continue
		}
		t := colTest{col: c, cond: cr.Cond.test()}
		if len(cr.Hidden) > 0 {
			t.hidden = make(map[string]bool, len(cr.Hidden))
			for _, h := range cr.Hidden {
				t.hidden[h] = true
			}
		}
		tests = append(tests, t)
	}
	return tests
}

func (s *Sheet) rowPasses(row int, tests []colTest) bool {
	for _, t := range tests {
		a := Addr{Col: t.col, Row: row}
		shown := s.ShownText(a)
		if t.hidden[shown] || !t.cond(s.Value(a), shown) {
			return false
		}
	}
	return true
}

// ShownText is the cell's value as its format displays it, with no width
// limit: what filters and the values list compare.
func (s *Sheet) ShownText(a Addr) string {
	c := s.cells[a]
	if c.Blank() {
		return ""
	}
	return FormatText(c.Value, s.DisplayFormat(a))
}

// test compiles the condition. Text conditions ignore case; comparisons
// need a number for a number (entered as $1,200 or 12% too) and text for
// text, and never match blanks.
func (c Condition) test() func(v Value, shown string) bool {
	arg := strings.ToLower(c.Arg)
	argVal := Value{Kind: Text, Str: c.Arg}
	if n, _, ok := ParseValue(strings.TrimSpace(c.Arg)); ok {
		argVal = Value{Kind: Number, Num: n}
	}
	lower := func(s string) string { return strings.ToLower(s) }
	cmpArg := func(v Value) (int, bool) {
		if v.Kind != argVal.Kind || (v.Kind != Number && v.Kind != Text) {
			return 0, false
		}
		return compare(v, argVal), true
	}
	ordered := func(ok func(int) bool) func(Value, string) bool {
		return func(v Value, _ string) bool {
			d, same := cmpArg(v)
			return same && ok(d)
		}
	}
	switch c.Op {
	case CondEmpty:
		return func(v Value, shown string) bool { return shown == "" }
	case CondNotEmpty:
		return func(v Value, shown string) bool { return shown != "" }
	case CondContains:
		return func(_ Value, shown string) bool { return strings.Contains(lower(shown), arg) }
	case CondNotContains:
		return func(_ Value, shown string) bool { return !strings.Contains(lower(shown), arg) }
	case CondStartsWith:
		return func(_ Value, shown string) bool { return strings.HasPrefix(lower(shown), arg) }
	case CondEndsWith:
		return func(_ Value, shown string) bool { return strings.HasSuffix(lower(shown), arg) }
	case CondExactly:
		return func(_ Value, shown string) bool { return lower(shown) == arg }
	case CondGreater:
		return ordered(func(d int) bool { return d > 0 })
	case CondGreaterEq:
		return ordered(func(d int) bool { return d >= 0 })
	case CondLess:
		return ordered(func(d int) bool { return d < 0 })
	case CondLessEq:
		return ordered(func(d int) bool { return d <= 0 })
	case CondEqual:
		return ordered(func(d int) bool { return d == 0 })
	case CondNotEqual:
		return func(v Value, shown string) bool {
			d, same := cmpArg(v)
			return !same || d != 0
		}
	}
	return func(Value, string) bool { return true }
}

// FilterValue is one entry of a filter's values list: a value as shown,
// how many rows have it, and whether it is checked (shown).
type FilterValue struct {
	Text  string // "" for blanks
	Count int
	Shown bool
}

// FilterValues lists the distinct values in column col of the filter's
// data rows, among the rows the other columns' criteria let through, as
// Sheets' "Filter by values" does. Numbers come first in numeric order,
// then text alphabetically, then blanks.
func (s *Sheet) FilterValues(col int) []FilterValue {
	f := s.view.filter
	if f == nil {
		return nil
	}
	tests := s.criteriaTests(f, col)
	hidden := map[string]bool{}
	for _, h := range f.Cols[col].Hidden {
		hidden[h] = true
	}
	counts := map[string]int{}
	values := map[string]Value{}
	for row := f.Range.From.Row + 1; row <= f.Range.To.Row; row++ {
		if !s.rowPasses(row, tests) {
			continue
		}
		a := Addr{Col: col, Row: row}
		t := s.ShownText(a)
		if _, seen := counts[t]; !seen {
			values[t] = s.Value(a)
		}
		counts[t]++
	}
	// Unchecked values stay listed even when no row has them any more, so
	// they can be checked again.
	for h := range hidden {
		if _, ok := counts[h]; !ok {
			counts[h] = 0
			values[h] = Value{Kind: Text, Str: h}
			if n, _, ok := ParseValue(h); ok {
				values[h] = Value{Kind: Number, Num: n}
			}
		}
	}
	out := make([]FilterValue, 0, len(counts))
	for t, n := range counts {
		out = append(out, FilterValue{Text: t, Count: n, Shown: !hidden[t]})
	}
	slices.SortFunc(out, func(a, b FilterValue) int {
		if (a.Text == "") != (b.Text == "") {
			if a.Text == "" {
				return 1
			}
			return -1
		}
		return cmp.Or(sortCompare(values[a.Text], values[b.Text]), cmp.Compare(a.Text, b.Text))
	})
	return out
}
