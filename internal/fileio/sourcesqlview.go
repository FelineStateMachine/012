package fileio

import (
	"context"
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// SQLite views push their sort and filter down to SQL: building one is
// one INSERT ... SELECT rowid ... WHERE ... ORDER BY into a table of the
// work database (sourcesqlwork.go), and a page reads its rowids there,
// by position, then the rows.
//
// The SQL follows a sheet's sort and filter, with these differences:
// SQLite folds case in ASCII only; text holding a date (2026-09-26),
// which the source shows as that date, is compared and sorted as text;
// and the text conditions (contains, starts with ...) test a number's
// text as SQLite writes it (1e+20), where a sheet tests it as shown.

// sqlStmt is a statement and its arguments.
type sqlStmt struct {
	q    string
	args []any
}

// View orders the rows as o says.
func (s *sqliteSource) View(ctx context.Context, o SourceOrder) (SourceView, error) {
	b, err := s.byNumber(ctx)
	if err != nil {
		return nil, err
	}
	if o.IsZero() {
		return sqlPlainView{s: s, b: b}, nil
	}
	where, args, order, err := viewSQL(o, b.cols)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	w, err := s.workDB()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("_012_v%d", w.n.Add(1))
	from := b.table
	if !b.work {
		from = "src." + b.table
	}
	n, err := w.build(ctx, s.spec.Path,
		sqlStmt{q: "CREATE TABLE " + name + " (r INTEGER)"},
		sqlStmt{q: "INSERT INTO " + name + " (r) SELECT rowid FROM " + from + where + " ORDER BY " + order, args: args})
	if err != nil {
		w.db.ExecContext(context.WithoutCancel(ctx), "DROP TABLE IF EXISTS "+name)
		return nil, err
	}
	return &sqlView{s: s, b: b, w: w, name: name, rows: n}, nil
}

// viewSQL is a view's WHERE clause (with its leading space), its
// arguments and its ORDER BY, over the columns cols.
func viewSQL(o SourceOrder, cols []string) (string, []any, string, error) {
	var conds []string
	var args []any
	for _, f := range o.Filter {
		if f.Col < 0 || f.Col >= len(cols) {
			return "", nil, "", fmt.Errorf("the source has no column %d", f.Col+1)
		}
		c, a := condSQL(cols[f.Col], f.Cond)
		conds, args = append(conds, c), append(args, a...)
	}
	var keys []string
	for _, k := range o.Sort {
		if k.Col < 0 || k.Col >= len(cols) {
			return "", nil, "", fmt.Errorf("the source has no column %d", k.Col+1)
		}
		keys = append(keys, orderSQL(cols[k.Col], k.Desc))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	return where, args, strings.Join(append(keys, "rowid"), ", "), nil
}

// orderSQL orders column x as a sheet sorts: numbers, text ignoring
// case, then the rest (booleans are numbers in SQLite, blobs last), and
// blanks last in either direction.
func orderSQL(x string, desc bool) string {
	dir := ""
	if desc {
		dir = " DESC"
	}
	v := "nullif(" + x + ", '')" // a blank, NULL or '', is NULL
	return "(" + v + " IS NULL), " +
		"CASE typeof(" + v + ") WHEN 'null' THEN 0 WHEN 'integer' THEN 0 WHEN 'real' THEN 0 WHEN 'text' THEN 1 ELSE 2 END" + dir + ", " +
		v + " COLLATE NOCASE" + dir
}

// condSQL is a filter's condition on column x as SQL, as
// sheet.Condition tests a cell: text conditions on the text a value
// shows, blank for NULL; comparisons with a number (or a date, $1,200,
// 12%) only of numbers, with text only of text ignoring case; neither
// matching a blank.
func condSQL(x string, c sheet.Condition) (string, []any) {
	shown := "lower(coalesce(CAST(" + x + " AS TEXT), ''))"
	arg := strings.ToLower(c.Arg)
	switch c.Op {
	case sheet.CondEmpty:
		return "(" + x + " IS NULL OR " + x + " = '')", nil
	case sheet.CondNotEmpty:
		return "(" + x + " IS NOT NULL AND " + x + " <> '')", nil
	case sheet.CondContains:
		return "instr(" + shown + ", ?) > 0", []any{arg}
	case sheet.CondNotContains:
		return "instr(" + shown + ", ?) = 0", []any{arg}
	case sheet.CondStartsWith:
		return "substr(" + shown + ", 1, length(?)) = ?", []any{arg, arg}
	case sheet.CondEndsWith:
		return "(length(?) = 0 OR substr(" + shown + ", -length(?)) = ?)", []any{arg, arg, arg}
	case sheet.CondExactly:
		return shown + " = ?", []any{arg}
	case sheet.CondGreater, sheet.CondGreaterEq, sheet.CondLess, sheet.CondLessEq, sheet.CondEqual:
		return compareSQL(x, c)
	case sheet.CondNotEqual:
		c.Op = sheet.CondEqual
		q, args := compareSQL(x, c)
		return "NOT " + q, args
	}
	return "1", nil
}

var compareOps = map[sheet.CondOp]string{
	sheet.CondGreater: ">", sheet.CondGreaterEq: ">=", sheet.CondLess: "<", sheet.CondLessEq: "<=", sheet.CondEqual: "=",
}

// compareSQL compares column x with a condition's value as the sheet's
// filter does: a number with numbers, text with text.
func compareSQL(x string, c sheet.Condition) (string, []any) {
	op := compareOps[c.Op]
	if n, _, ok := sheet.ParseValue(strings.TrimSpace(c.Arg)); ok {
		return "(typeof(" + x + ") IN ('integer', 'real') AND " + x + " " + op + " ?)", []any{n}
	}
	return "(typeof(" + x + ") = 'text' AND " + x + " <> '' AND upper(" + x + ") " + op + " upper(?))", []any{c.Arg}
}

// sqlView is a sorted or filtered view: its rows' rowids in a table of
// the work database.
type sqlView struct {
	s    *sqliteSource
	b    *sqlBase
	w    *sqlWork
	name string
	rows int64
}

func (v *sqlView) Rows() int64 { return v.rows }

func (v *sqlView) Page(ctx context.Context, from int64, n int) ([]int64, [][]sheet.LiveCell, error) {
	if from < 0 || from >= v.rows || n <= 0 {
		return nil, nil, nil
	}
	var nums []int64
	err := queryRows(ctx, v.w.db, "SELECT r FROM "+v.name+" WHERE rowid BETWEEN ? AND ? ORDER BY rowid",
		[]any{from + 1, from + int64(n)}, func(x []any) bool {
			r, _ := x[0].(int64)
			nums = append(nums, r-1)
			return true
		})
	if err != nil {
		return nil, nil, err
	}
	rows, err := v.b.fetch(ctx, nums, allCols(nil, len(v.b.cols)))
	if err != nil {
		return nil, nil, err
	}
	return nums, rows, nil
}

func (v *sqlView) Close() error {
	v.s.mu.Lock()
	defer v.s.mu.Unlock()
	if v.s.work != v.w {
		return nil // closed with the source
	}
	_, err := v.w.db.Exec("DROP TABLE IF EXISTS " + v.name)
	return sqliteErr(err)
}

// sqlPlainView is the rows in the source's own order.
type sqlPlainView struct {
	s *sqliteSource
	b *sqlBase
}

func (v sqlPlainView) Rows() int64 { return v.s.rows }

func (v sqlPlainView) Page(ctx context.Context, from int64, n int) ([]int64, [][]sheet.LiveCell, error) {
	return v.b.page(ctx, from, n, v.s.rows)
}

func (sqlPlainView) Close() error { return nil }
