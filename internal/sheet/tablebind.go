package sheet

import (
	"fmt"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// How formulas read tables (table.go): a structured reference,
// Sales[Amount], is kept as written and turned into the range it names
// each time the formula is evaluated, as named ranges are (names.go), so
// a table that grows, or whose columns move, is followed without
// rewriting a formula. A formula reading a table is one of the table's
// users, by its name's key, so a change to the table's cells or to the
// table itself recalculates it.
//
// A name alone that names no range but a table stands for the table's
// data rows, Sales as Sales[]. A region's table is found by the region's
// name the same way: app[status], app alone for its rows under the
// header, and nu.app, the whole table with its header row, as app[#All].

// tableView is a table as formulas read it: a table's, or a region's.
type tableView struct {
	s    *Sheet
	r    Rect     // header row included
	cols []string // the columns' names
	ok   bool     // false for a region with no table shown: #REF!
}

// findTable finds the table or region formulas name with the key k.
func (w *Workbook) findTable(k string) (tableView, bool) {
	for _, s := range w.sheets {
		if i := s.tableIndex(k); i >= 0 {
			t := s.view.tables[i]
			return tableView{s: s, r: t.Range, cols: t.Cols, ok: true}, true
		}
	}
	for _, s := range w.sheets {
		if i := s.regionIndex(k); i >= 0 {
			r, ok := s.RegionTable(s.regions.list[i].Name)
			if !ok {
				return tableView{s: s}, true
			}
			return tableView{s: s, r: r, cols: s.rowTexts(r), ok: true}, true
		}
	}
	return tableView{}, false
}

// rowTexts are the texts of r's first row, as shown.
func (s *Sheet) rowTexts(r Rect) []string {
	out := make([]string, 0, r.To.Col-r.From.Col+1)
	for c := r.From.Col; c <= r.To.Col; c++ {
		out = append(out, strings.TrimSpace(s.ShownText(Addr{Col: c, Row: r.From.Row})))
	}
	return out
}

// col is the index of the column named name, ignoring case, or -1.
func (v tableView) col(name string) int {
	for i, c := range v.cols {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return -1
}

// rows are the first and last row it names on the table's sheet, for a
// formula at a on the sheet same says whether it's the table's.
func (v tableView) rows(it formula.TableItems, a Addr, same bool) (int, int, bool) {
	h, last := v.r.From.Row, v.r.To.Row
	if it == formula.ItemThisRow {
		if !same || a.Row <= h || a.Row > last {
			return 0, 0, false
		}
		return a.Row, a.Row, true
	}
	lo, hi := -1, -1
	if it&formula.ItemHeaders != 0 {
		lo, hi = h, h
	}
	if it&formula.ItemData != 0 && last > h {
		if lo < 0 {
			lo = h + 1
		}
		hi = last
	}
	return lo, hi, lo >= 0
}

// colSpan is the first and last column t names on the sheet.
func (v tableView) colSpan(t formula.TableRef) (int, int, bool) {
	from, to := t.Cols()
	if from == "" {
		return v.r.From.Col, v.r.To.Col, true
	}
	i, j := v.col(from), v.col(to)
	if i < 0 || j < 0 {
		return 0, 0, false
	}
	return v.r.From.Col + min(i, j), v.r.From.Col + max(i, j), true
}

// bindTable is what the structured reference t in a formula at a on s
// stands for: a cell or a range, or #REF! when the table, a column or
// the row it names isn't there. thisRow is the absolute markers a
// reference to the formula's own row takes; every other is absolute.
func (s *Sheet) bindTable(a Addr, t formula.TableRef, thisRow formula.Abs) Node {
	v, found := s.wb.findTable(nameKey(t.Table))
	if !found || !v.ok {
		return formula.RefErr{}
	}
	lo, hi, okRows := v.rows(t.Rows(), a, v.s == s)
	c0, c1, okCols := v.colSpan(t)
	if !okRows || !okCols {
		return formula.RefErr{}
	}
	sheet := ""
	if v.s != s {
		sheet = v.s.name
	}
	abs := formula.AbsCol | formula.AbsRow
	if t.Items == formula.ItemThisRow {
		abs = thisRow
	}
	r := Rect{From: Addr{Col: c0, Row: lo}, To: Addr{Col: c1, Row: hi}}
	if r.From == r.To {
		return formula.Ref{Addr: r.From, Abs: abs, Sheet: sheet}
	}
	return formula.Range{Rect: r, Abs: [2]formula.Abs{abs, abs}, Sheet: sheet}
}

// bound returns the formula at a in c with its names and structured
// references replaced by the ranges they stand for: what is evaluated
// and what dependencies are traced through. Undefined names stay, and
// evaluate to #NAME?.
func (s *Sheet) bound(a Addr, c *Cell) Node {
	if len(c.names) == 0 {
		return c.expr
	}
	const fixed = formula.AbsCol | formula.AbsRow
	n, _ := formula.Rewrite(c.expr, formula.Rewriter{
		Name: func(nn formula.Name) Node {
			nm, ok := s.wb.names[nameKey(nn.Name)]
			if !ok {
				return s.boundTableName(a, nn)
			}
			if nm.Gone() {
				return formula.RefErr{}
			}
			sheet := ""
			if nm.Sheet != s {
				sheet = nm.Sheet.name
			}
			if nm.Range.From == nm.Range.To {
				return formula.Ref{Addr: nm.Range.From, Abs: fixed, Sheet: sheet}
			}
			return formula.Range{Rect: nm.Range, Abs: [2]formula.Abs{fixed, fixed}, Sheet: sheet}
		},
		Table: func(t formula.TableRef) Node { return s.bindTable(a, t, fixed) },
	})
	return n
}

// boundTableName is a name in a formula at a that isn't a named range:
// a table's data rows (Sales), a region's whole table (nu.app), or the
// name as written, which shows #NAME?.
func (s *Sheet) boundTableName(a Addr, nn formula.Name) Node {
	if _, found := s.wb.findTable(nameKey(nn.Name)); found {
		return s.bindTable(a, formula.TableRef{Table: nn.Name}, formula.AbsCol|formula.AbsRow)
	}
	return s.boundRegion(nn)
}

// tablesInUse are the tables and regions formulas name, by structured
// references or their names alone, with their cells, for finding what
// reads a changed cell. A region named as nu.app is among regionsInUse.
func (w *Workbook) tablesInUse() []namedUsers {
	var out []namedUsers
	for k, users := range w.nameUsers {
		if _, isName := w.names[k]; isName {
			continue
		}
		if v, ok := w.findTable(k); ok && v.ok {
			out = append(out, namedUsers{v.s, v.r, users})
		}
	}
	return out
}

// renameTableInFormulas rewrites the formulas reading the table with
// key from to name it to instead.
func (w *Workbook) renameTableInFormulas(from, to string) {
	w.rewriteUsers(from, formula.Rewriter{
		Name: func(n formula.Name) Node {
			if nameKey(n.Name) == from {
				return formula.Name{Name: to}
			}
			return n
		},
		Table: func(t formula.TableRef) Node {
			if nameKey(t.Table) == from {
				t.Table = to
			}
			return t
		},
	})
}

// renameColumnInFormulas rewrites the structured references to the
// column old of the table with key k to name the column name instead.
func (w *Workbook) renameColumnInFormulas(k, old, name string) {
	w.rewriteUsers(k, formula.Rewriter{Table: func(t formula.TableRef) Node {
		if nameKey(t.Table) != k {
			return t
		}
		if strings.EqualFold(t.From, old) {
			t.From = name
		}
		if strings.EqualFold(t.To, old) {
			t.To = name
		}
		return t
	}})
}

// unbindTable rewrites the formulas reading the table with key k to
// read the cells it stands for by address: its column in a row of its
// own relative to the formula (C5), the rest absolute ($C$2:$C$9).
func (w *Workbook) unbindTable(k string) {
	var users []loc
	for u := range w.nameUsers[k] {
		users = append(users, u)
	}
	for _, u := range users {
		c := u.s.cells.get(u.a)
		out := c.rewritten(formula.Rewriter{
			Name: func(n formula.Name) Node {
				if nameKey(n.Name) != k {
					return n
				}
				if _, isName := w.names[k]; isName {
					return n
				}
				return u.s.bindTable(u.a, formula.TableRef{Table: n.Name}, 0)
			},
			Table: func(t formula.TableRef) Node {
				if nameKey(t.Table) != k {
					return t
				}
				return u.s.bindTable(u.a, t, formula.AbsCol)
			},
		})
		if out != c {
			u.s.place(u.a, out)
		}
	}
}

// rewriteUsers rewrites the formulas using the name or table with key
// k with rw.
func (w *Workbook) rewriteUsers(k string, rw formula.Rewriter) {
	var users []loc
	for u := range w.nameUsers[k] {
		users = append(users, u)
	}
	for _, u := range users {
		c := u.s.cells.get(u.a)
		if out := c.rewritten(rw); out != c {
			u.s.place(u.a, out)
		}
	}
}

// TableInfo is a table as formulas find it: a table's, or a region's.
type TableInfo struct {
	Name  string
	Sheet *Sheet
	// Range is its cells, header row included, when Shown.
	Range Rect
	shown bool
	Cols  []string
	// Region is set on a region's table, which its source names and
	// shapes: the commands that change tables leave it alone.
	Region bool
}

// Shown reports whether the table has cells: a region's may have none
// yet.
func (t TableInfo) Shown() bool { return t.shown }

// TableInfos returns every table formulas can read, sheet by sheet:
// each sheet's tables in the order they were made, then its regions'.
func (w *Workbook) TableInfos() []TableInfo {
	var out []TableInfo
	for _, s := range w.sheets {
		for _, t := range s.view.tables {
			out = append(out, TableInfo{Name: t.Name, Sheet: s, Range: t.Range, shown: true, Cols: slices.Clone(t.Cols)})
		}
		for _, r := range s.regions.list {
			v, _ := w.findTable(nameKey(r.Name))
			out = append(out, TableInfo{Name: r.Name, Sheet: s, Range: v.r, shown: v.ok, Cols: v.cols, Region: true})
		}
	}
	return out
}

// LookupTable finds a table formulas can read by name, ignoring case.
func (w *Workbook) LookupTable(name string) (TableInfo, bool) {
	k := nameKey(name)
	for _, t := range w.TableInfos() {
		if nameKey(t.Name) == k {
			return t, true
		}
	}
	return TableInfo{}, false
}

// TableLook says how the cell at a draws as part of a table: in the
// header row's style, or on a band (every other data row), when its
// table has them.
func (s *Sheet) TableLook(a Addr) (header, band bool) {
	for i := range s.view.tables {
		t := &s.view.tables[i]
		if !t.Range.Contains(a) {
			continue
		}
		if a.Row == t.Range.From.Row {
			return t.Header, false
		}
		return false, t.Banded && (a.Row-t.Range.From.Row)%2 == 0
	}
	return false, false
}

// TableRange finds the cells a structured reference written alone
// stands for (Sales[Amount], Sales[#All]), as a formula outside the
// table reads them, or with a table's name alone its whole table,
// header row included. ok is false when ref is neither; err says why
// one names no cells.
func (w *Workbook) TableRange(ref string) (s *Sheet, r Rect, ok bool, err error) {
	n, perr := formula.Parse(ref, parserFuncs)
	if perr != nil {
		return nil, Rect{}, false, nil
	}
	var t formula.TableRef
	switch n := n.(type) {
	case formula.TableRef:
		t = n
	case formula.Name:
		t = formula.TableRef{Table: n.Name, Items: formula.ItemAll}
	default:
		return nil, Rect{}, false, nil
	}
	v, found := w.findTable(nameKey(t.Table))
	switch {
	case !found:
		if _, isName := n.(formula.Name); isName {
			return nil, Rect{}, false, nil
		}
		return nil, Rect{}, true, fmt.Errorf("there's no table named %s", t.Table)
	case !v.ok:
		return nil, Rect{}, true, fmt.Errorf("%s has no rows yet", t.Table)
	}
	lo, hi, okRows := v.rows(t.Rows(), Addr{}, false)
	c0, c1, okCols := v.colSpan(t)
	switch {
	case !okCols:
		return nil, Rect{}, true, fmt.Errorf("%s has no such column (columns: %s)", t.Table, strings.Join(v.cols, ", "))
	case !okRows:
		return nil, Rect{}, true, fmt.Errorf("%s names no rows of %s here: a formula's own row, or rows it hasn't", ref, t.Table)
	}
	return v.s, Rect{From: Addr{Col: c0, Row: lo}, To: Addr{Col: c1, Row: hi}}, true, nil
}
