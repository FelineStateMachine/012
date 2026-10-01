package fileio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// sqliteSource is a SQLite table, view or query read in place, through
// connections that can only read (mode=ro, query_only), so the database
// is never written. Its rows are in rowid order for a table, and as the
// query or view gives them otherwise.
//
// A row's number is found cheaply only in a table whose rowids are 1 to
// its row count (dense), the common case of a table only ever appended
// to: row n is rowid n+1. Any other source is copied, when something
// first asks for rows by number, into a table of a work database in
// the temporary directory (sourcesqlwork.go), where it is dense; views'
// row orders are tables there too.
type sqliteSource struct {
	spec  SourceSpec
	db    *sql.DB
	query string // the SELECT giving the source's rows, in order
	table string // the table, quoted, when the source is one with rowids
	names []string
	meta  []SourceColumn
	rows  int64
	dense bool

	mu   sync.Mutex
	work *sqlWork // made when first needed
	base *sqlBase // where rows are found by number, once known
}

// sqliteURI is the URI opening a database file to read only.
func sqliteURI(path string, params ...string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: strings.Join(params, "&")}
	return u.String(), nil
}

func openSQLiteSource(ctx context.Context, spec SourceSpec) (*sqliteSource, error) {
	if _, err := os.Stat(spec.Path); err != nil {
		return nil, err
	}
	uri, err := sqliteURI(spec.Path, "mode=ro", "_pragma=query_only(1)", "_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	s := &sqliteSource{spec: spec, db: db}
	if err := s.open(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// open finds what the source reads, its columns and its row count.
func (s *sqliteSource) open(ctx context.Context) error {
	s.query = s.spec.Query
	if s.query == "" {
		table := s.spec.Table
		if table == "" {
			ts, err := tables(ctx, s.db)
			switch {
			case err != nil:
				return err
			case len(ts) == 0:
				return errors.New("the database has no tables")
			case len(ts) > 1:
				return &ErrNeedTable{Tables: ts}
			}
			table = ts[0].Name
		}
		s.query = "SELECT * FROM " + quoteIdent(table)
		if s.hasRowid(ctx, table) {
			s.table = quoteIdent(table)
			s.query += " ORDER BY rowid"
		}
	}
	if err := s.columns(ctx); err != nil {
		return err
	}
	return s.count(ctx)
}

// hasRowid reports whether table is a table with rowids (not a view or
// a WITHOUT ROWID table).
func (s *sqliteSource) hasRowid(ctx context.Context, table string) bool {
	rows, err := s.db.QueryContext(ctx, "SELECT rowid FROM "+quoteIdent(table)+" LIMIT 0")
	if err != nil {
		return false
	}
	rows.Close()
	var typ string
	err = s.db.QueryRowContext(ctx, "SELECT type FROM sqlite_master WHERE name = ?", table).Scan(&typ)
	return err == nil && typ == "table"
}

// count counts the rows, and for a table whether its rowids are dense.
func (s *sqliteSource) count(ctx context.Context) error {
	if s.table == "" {
		err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM ("+s.query+")").Scan(&s.rows)
		return sqliteErr(err)
	}
	var lo, hi sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT count(*), min(rowid), max(rowid) FROM "+s.table).Scan(&s.rows, &lo, &hi)
	if err != nil {
		return sqliteErr(err)
	}
	s.dense = s.rows == 0 || lo.Int64 == 1 && hi.Int64 == s.rows
	return nil
}

// columns reads the columns' names and types: a declared type says
// whether one holds numbers or dates; an undeclared one (a query's
// expressions) is judged by its first rows.
func (s *sqliteSource) columns(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "SELECT * FROM ("+s.query+") LIMIT 0")
	if err != nil {
		return sqliteErr(err)
	}
	types, err := rows.ColumnTypes()
	rows.Close()
	if err != nil {
		return sqliteErr(err)
	}
	var untyped []int
	for i, t := range types {
		c := SourceColumn{Name: t.Name()}
		decl := strings.ToUpper(t.DatabaseTypeName())
		if decl == "" {
			untyped = append(untyped, i)
		} else {
			c.Format, c.Numeric = declaredFormat(decl)
		}
		s.names = append(s.names, t.Name())
		s.meta = append(s.meta, c)
	}
	if len(untyped) > 0 {
		return s.sample(ctx, untyped)
	}
	return nil
}

// declaredFormat is the format a declared type shows its column's
// values in, and whether it holds numbers: dates and times, the
// decimals of a DECIMAL(10,2) or NUMERIC(10,2), and money as currency.
func declaredFormat(decl string) (sheet.Format, bool) {
	switch {
	case strings.Contains(decl, "DATETIME"), strings.Contains(decl, "TIMESTAMP"):
		return sheet.Format{Kind: sheet.FmtDateTime}, true
	case strings.Contains(decl, "DATE"):
		return sheet.Format{Kind: sheet.FmtDate}, true
	case strings.Contains(decl, "TIME"):
		return sheet.Format{Kind: sheet.FmtTime}, true
	case strings.Contains(decl, "MONEY"), strings.Contains(decl, "CURRENCY"):
		return sheet.Format{Kind: sheet.FmtCurrency, Decimals: 2}, true
	case strings.HasPrefix(decl, "DECIMAL"), strings.HasPrefix(decl, "NUMERIC"):
		if scale, ok := declaredScale(decl); ok {
			return sheet.Format{Kind: sheet.FmtNumber, Decimals: scale}, true
		}
	}
	return sheet.Format{}, numericAffinity(decl)
}

// declaredScale reads the scale of a type declared as DECIMAL(10,2).
func declaredScale(decl string) (int, bool) {
	_, args, found := strings.Cut(decl, "(")
	args, _, closed := strings.Cut(args, ")")
	_, scale, two := strings.Cut(args, ",")
	if !found || !closed || !two {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(scale))
	if err != nil || n < 0 || n > sheet.MaxDecimals {
		return 0, false
	}
	return n, true
}

// numericAffinity reports whether a declared type gives a column
// SQLite's INTEGER, REAL or NUMERIC affinity.
func numericAffinity(decl string) bool {
	switch {
	case strings.Contains(decl, "CHAR"), strings.Contains(decl, "CLOB"), strings.Contains(decl, "TEXT"),
		strings.Contains(decl, "BLOB"):
		return false
	}
	return true
}

// sampleRows is how many rows judge an undeclared column's type.
const sampleRows = 100

// sample marks the columns cols numeric when their first rows hold
// numbers and nothing else.
func (s *sqliteSource) sample(ctx context.Context, cols []int) error {
	num := make([]bool, len(s.meta))
	seen := make([]bool, len(s.meta))
	for _, c := range cols {
		num[c] = true
	}
	err := s.scanSQL(ctx, "SELECT * FROM ("+s.query+") LIMIT "+strconv.Itoa(sampleRows), nil, func(vals []any) bool {
		for _, c := range cols {
			switch vals[c].(type) {
			case nil:
			case int64, float64:
				seen[c] = true
			default:
				num[c] = false
			}
		}
		return true
	})
	for _, c := range cols {
		s.meta[c].Numeric = num[c] && seen[c]
	}
	return err
}

func (s *sqliteSource) Columns() []SourceColumn { return slices.Clone(s.meta) }

func (s *sqliteSource) Rows() int64 { return s.rows }

// selectCols is the select list reading cols, and whether the columns
// come back as asked: a table's by name, anything else whole, picked
// from.
func (s *sqliteSource) selectCols(cols []int) (string, bool) {
	if s.table == "" {
		return "*", false
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quoteIdent(s.names[c])
	}
	return strings.Join(names, ", "), true
}

// Scan streams the rows from row from on.
func (s *sqliteSource) Scan(ctx context.Context, from int64, cols []int, fn func(int64, []sheet.LiveCell) bool) error {
	cols = allCols(cols, len(s.names))
	if err := checkCols(cols, len(s.names)); err != nil {
		return err
	}
	list, picked := s.selectCols(cols)
	var q string
	var args []any
	switch {
	case s.table != "" && s.dense:
		q, args = "SELECT "+list+" FROM "+s.table+" WHERE rowid > ? ORDER BY rowid", []any{from}
	case s.table != "":
		q, args = "SELECT "+list+" FROM "+s.table+" ORDER BY rowid LIMIT -1 OFFSET ?", []any{from}
	default:
		q, args = "SELECT * FROM ("+s.query+") LIMIT -1 OFFSET ?", []any{from}
	}
	out := make([]sheet.LiveCell, len(cols))
	row := max(from, 0)
	return s.scanSQL(ctx, q, args, func(vals []any) bool {
		for i, c := range cols {
			v := vals[i]
			if !picked {
				v = vals[c]
			}
			cell, _ := sqliteCell(v)
			out[i] = cell.typed()
		}
		row++
		return fn(row-1, out)
	})
}

// scanSQL runs a query on the source, calling fn with each row's values
// until it returns false.
func (s *sqliteSource) scanSQL(ctx context.Context, q string, args []any, fn func([]any) bool) error {
	return queryRows(ctx, s.db, q, args, fn)
}

// queryRows runs a query, calling fn with each row's values (reused)
// until it returns false.
func queryRows(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, q string, args []any, fn func([]any) bool) error {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return sqliteErr(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return sqliteErr(err)
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return sqliteErr(err)
		}
		if !fn(vals) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return sqliteErr(err)
	}
	return nil
}

// Fetch reads the rows numbered rows.
func (s *sqliteSource) Fetch(ctx context.Context, rows []int64, cols []int) ([][]sheet.LiveCell, error) {
	cols = allCols(cols, len(s.names))
	if err := checkCols(cols, len(s.names)); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r < 0 || r >= s.rows {
			return nil, fmt.Errorf("the source has no row %d", r+1)
		}
	}
	b, err := s.byNumber(ctx)
	if err != nil {
		return nil, err
	}
	return b.fetch(ctx, rows, cols)
}

func (s *sqliteSource) Close() error {
	s.mu.Lock()
	w := s.work
	s.work, s.base = nil, nil
	s.mu.Unlock()
	var err error
	if w != nil {
		err = w.close()
	}
	return errors.Join(err, s.db.Close())
}
