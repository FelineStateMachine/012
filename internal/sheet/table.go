package sheet

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Tables, as Sheets' Format > Convert to table and Excel's tables: a
// named range whose first row names its columns, so formulas read it by
// column name (Sales[Amount], see tablebind.go). A table grows as rows
// are typed or pasted just below it, and its columns' names follow its
// header row: renaming a header renames the column in every formula
// that reads it (tablesync.go). Tables belong to the sheet's view state,
// so each change to one is an undo step with the rest of the view.
//
// A region's table (a notebook cell's output sent to a sheet, or a
// linked file) is a table too, by the region's name: app[status] reads
// its status column. Its rows and columns are its source's, so the
// commands that rename, resize, style or remove a table leave it alone.

// Table is a table on a sheet.
type Table struct {
	Name  string // as the user spelled it; formulas match it in any case
	Range Rect   // header row included
	// Cols are the columns' names, left to right: the header row's text
	// as last settled, never blank and never the same twice.
	Cols []string
	// Banded shades every other data row; Header styles the header row.
	Banded, Header bool
}

// equal reports whether two tables are the same.
func (t Table) equal(u Table) bool {
	return t.Name == u.Name && t.Range == u.Range && slices.Equal(t.Cols, u.Cols) && t.Banded == u.Banded && t.Header == u.Header
}

// clone is a copy of t sharing nothing with it.
func (t Table) clone() Table {
	t.Cols = slices.Clone(t.Cols)
	return t
}

// Data is the table's data rows, below the header row.
func (t Table) Data() Rect {
	d := t.Range
	d.From.Row++
	return d
}

// Col returns the index of the column named name, ignoring case, or -1.
func (t Table) Col(name string) int {
	return slices.IndexFunc(t.Cols, func(c string) bool { return strings.EqualFold(c, name) })
}

// Errors of tables.
var (
	ErrNoTable      = errors.New("There's no table by that name")
	ErrTableOverlap = errors.New("A table can't overlap another table, a region or a pivot table")
	ErrTableMerged  = errors.New("A table can't hold merged cells: unmerge them first")
)

// tablePrefix is reserved for regions' names in formulas (nu.sales).
const tablePrefix = regionPrefix

// ValidTableName checks that name can name a table: a named range's
// rules, except that only a name Excel reads as a cell is refused, so
// Table1 is one; and not starting with nu., which regions' names use.
func ValidTableName(name string) error {
	if err := checkName(name, formula.LooksLikeCell); err != nil {
		return err
	}
	if strings.HasPrefix(nameKey(name), nameKey(tablePrefix)) {
		return errors.New("A table's name can't start with " + tablePrefix)
	}
	return nil
}

// checkTableName reports why name can't name a table, besides the one
// with key self (when renaming one).
func (w *Workbook) checkTableName(name, self string) error {
	if err := ValidTableName(name); err != nil {
		return err
	}
	k := nameKey(name)
	if n, ok := w.LookupName(name); ok {
		return fmt.Errorf("%s already names %s", n.Name, n.Ref())
	}
	if _, t, ok := w.Table(name); ok && k != self {
		return fmt.Errorf("There's already a table named %s", t.Name)
	}
	if _, r, ok := w.Region(name); ok {
		return fmt.Errorf("%s names a notebook output or a linked file", r.Name)
	}
	return nil
}

// Tables returns the sheet's tables, in the order they were made.
func (s *Sheet) Tables() []Table {
	out := make([]Table, len(s.view.tables))
	for i, t := range s.view.tables {
		out[i] = t.clone()
	}
	return out
}

// HasTables reports whether the sheet has tables, so what draws it can
// skip looking for them.
func (s *Sheet) HasTables() bool { return len(s.view.tables) > 0 }

// Table finds a table by name, ignoring case, on any sheet. Regions'
// tables aren't among them: see TableInfo.
func (w *Workbook) Table(name string) (*Sheet, Table, bool) {
	k := nameKey(name)
	for _, s := range w.sheets {
		if i := s.tableIndex(k); i >= 0 {
			return s, s.view.tables[i].clone(), true
		}
	}
	return nil, Table{}, false
}

func (s *Sheet) tableIndex(k string) int {
	return slices.IndexFunc(s.view.tables, func(t Table) bool { return nameKey(t.Name) == k })
}

// TableAt returns the table holding the cell at a.
func (s *Sheet) TableAt(a Addr) (Table, bool) {
	for _, t := range s.view.tables {
		if t.Range.Contains(a) {
			return t.clone(), true
		}
	}
	return Table{}, false
}

// NextTableName is a name no table has yet: Table1, Table2 ...
func (w *Workbook) NextTableName() string {
	for i := 1; ; i++ {
		name := "Table" + strconv.Itoa(i)
		if w.checkTableName(name, "") == nil {
			return name
		}
	}
}

// copiedTables are ts as a copied sheet holds them, each renamed to a
// name no table has: Sales_2, Sales_3 ...
func (w *Workbook) copiedTables(ts []Table) []Table {
	out := make([]Table, len(ts))
	taken := map[string]bool{}
	for i, t := range ts {
		t = t.clone()
		for n := 2; ; n++ {
			name := t.Name + "_" + strconv.Itoa(n)
			if !taken[nameKey(name)] && w.checkTableName(name, "") == nil {
				t.Name, taken[nameKey(name)] = name, true
				break
			}
		}
		out[i] = t
	}
	return out
}

// checkTableRange reports why r can't hold the table with key self:
// it overlaps another table, a region or a pivot table, or holds
// merged cells.
func (s *Sheet) checkTableRange(r Rect, self string) error {
	for _, t := range s.view.tables {
		if nameKey(t.Name) != self && overlaps(t.Range, r) {
			return ErrTableOverlap
		}
	}
	if _, _, ok := s.InRegion(r); ok || s.InPivot(r) {
		return ErrTableOverlap
	}
	if len(s.MergesIn(r)) > 0 {
		return ErrTableMerged
	}
	return nil
}

// tableRange is r as a table's range: a header row and at least one
// row of data.
func tableRange(r Rect) Rect {
	if r.To.Row == r.From.Row && r.To.Row < MaxRows-1 {
		r.To.Row++
	}
	return r
}

// CreateTable makes a table named name of r, whose first row names its
// columns, with its header styled and no bands, as one undo step. Blank or repeated names in the header row
// are replaced with ones of their own (Column2, Amount2), in the cells
// too; a range of one row gains an empty row of data.
func (s *Sheet) CreateTable(name string, r Rect) error {
	if err := s.wb.checkTableName(name, ""); err != nil {
		return err
	}
	r = tableRange(r)
	if r.To.Row == r.From.Row {
		return errors.New("A table needs a row below its header row")
	}
	if err := s.checkTableRange(r, ""); err != nil {
		return err
	}
	s.change("make table "+name, r, func() {
		t := Table{Name: name, Range: r, Header: true}
		t.Cols = s.headerNames(t)
		s.putTables(append(s.Tables(), t))
	})
	return nil
}

// RenameTable renames a table, as one undo step, and every formula
// that reads it with it.
func (w *Workbook) RenameTable(old, name string) error {
	s, t, ok := w.Table(old)
	if !ok {
		return ErrNoTable
	}
	if err := w.checkTableName(name, nameKey(old)); err != nil {
		return err
	}
	if t.Name == name {
		return nil
	}
	s.change("rename table "+t.Name, t.Range, func() {
		w.renameTableInFormulas(nameKey(old), name)
		s.editTable(nameKey(old), func(t *Table) { t.Name = name })
	})
	return nil
}

// ResizeTable points a table at r, as one undo step. Its columns take
// their names from r's first row.
func (w *Workbook) ResizeTable(name string, r Rect) error {
	s, t, ok := w.Table(name)
	if !ok {
		return ErrNoTable
	}
	r = tableRange(r)
	if err := s.checkTableRange(r, nameKey(name)); err != nil {
		return err
	}
	s.change("resize table "+t.Name, r, func() {
		s.editTable(nameKey(name), func(t *Table) {
			t.Range = r
			t.Cols = s.headerNames(*t)
		})
	})
	return nil
}

// SetTableStyle sets whether a table's data rows are banded and its
// header row styled, as one undo step.
func (w *Workbook) SetTableStyle(name string, banded, header bool) error {
	s, t, ok := w.Table(name)
	if !ok {
		return ErrNoTable
	}
	s.change("style table "+t.Name, t.Range, func() {
		s.editTable(nameKey(name), func(t *Table) { t.Banded, t.Header = banded, header })
	})
	return nil
}

// RemoveTable removes a table, keeping its cells, as one undo step.
// Formulas that read it read the same cells by address instead, as
// Excel's Convert to range leaves them.
func (w *Workbook) RemoveTable(name string) error {
	s, t, ok := w.Table(name)
	if !ok {
		return ErrNoTable
	}
	s.change("remove table "+t.Name, t.Range, func() {
		w.unbindTable(nameKey(name))
		st := s.Tables()
		st = slices.DeleteFunc(st, func(u Table) bool { return nameKey(u.Name) == nameKey(name) })
		s.putTables(st)
	})
	return nil
}

// editTable changes the table with key k with fn, recording the change.
func (s *Sheet) editTable(k string, fn func(*Table)) {
	st := s.Tables()
	if i := slices.IndexFunc(st, func(t Table) bool { return nameKey(t.Name) == k }); i >= 0 {
		fn(&st[i])
		s.putTables(st)
	}
}

// putTables replaces the sheet's tables, recording the view for undo,
// and has the formulas that read the tables changed recalculated. A
// filter on a table's range follows the table as it grows or shrinks.
// Every change to tables goes through here.
func (s *Sheet) putTables(st []Table) {
	if slices.EqualFunc(st, s.view.tables, Table.equal) {
		return
	}
	s.recordView()
	s.followFilter(s.view.tables, st)
	w := s.wb
	if w.hist.open != nil {
		w.hist.dirty = append(w.hist.dirty, w.tableUsers(s.view.tables, st)...)
	}
	s.view.tables = st
}

// followFilter moves the filter along with the table whose range it
// has, when the table's range changes.
func (s *Sheet) followFilter(before, after []Table) {
	f := s.view.filter
	if f == nil {
		return
	}
	for _, t := range before {
		i := slices.IndexFunc(after, func(u Table) bool { return nameKey(u.Name) == nameKey(t.Name) })
		if t.Range != f.Range || i < 0 || after[i].Range == t.Range {
			continue
		}
		nf := f.clone()
		nf.Range = after[i].Range
		for c := range nf.Cols {
			if c > nf.Range.To.Col {
				delete(nf.Cols, c)
			}
		}
		s.view.filter = nf
		s.hidden.valid = false
		return
	}
}

// tableUsers are the formulas reading any table in either list whose
// range, columns or name differ between them.
func (w *Workbook) tableUsers(before, after []Table) []loc {
	var out []loc
	add := func(k string) {
		for u := range w.nameUsers[k] {
			out = append(out, u)
		}
	}
	for _, t := range before {
		i := slices.IndexFunc(after, func(u Table) bool { return nameKey(u.Name) == nameKey(t.Name) })
		if i < 0 || !after[i].equal(t) {
			add(nameKey(t.Name))
		}
	}
	for _, t := range after {
		if !slices.ContainsFunc(before, func(u Table) bool { return nameKey(u.Name) == nameKey(t.Name) }) {
			add(nameKey(t.Name))
		}
	}
	return out
}
