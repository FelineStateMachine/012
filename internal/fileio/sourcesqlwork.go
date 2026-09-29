package fileio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A SQLite source's work database: a file of its own in the temporary
// directory, never the source's, holding what reading rows by number
// needs: a copy of a source whose rows can't be found by number
// otherwise (_012_rows), and each view's rows in its order as a table of
// rowids (_012_v1, ...), paged by that table's own rowid, so a page
// deep in a view costs what a page at its top does. Tables are built
// in SQLite, the source attached read-only as src; SQLite sorts in
// temporary files, so memory stays bounded. The file goes when the
// source closes.

type sqlWork struct {
	path string
	db   *sql.DB
	n    atomic.Int64 // views made, naming their tables
}

func newSQLWork(dir string) (*sqlWork, error) {
	f, err := os.CreateTemp(dir, "012-source-*.db")
	if err != nil {
		return nil, err
	}
	f.Close()
	uri, err := sqliteURI(f.Name(), "_pragma=journal_mode(OFF)", "_pragma=synchronous(OFF)",
		"_pragma=busy_timeout(60000)", "_pragma=temp_store(1)")
	if err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	return &sqlWork{path: f.Name(), db: db}, nil
}

func (w *sqlWork) close() error {
	err := w.db.Close()
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if rerr := os.Remove(w.path + suffix); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
	}
	return err
}

// build runs statements on a connection of the work database with the
// source attached as src.
func (w *sqlWork) build(ctx context.Context, source string, stmts ...sqlStmt) (int64, error) {
	conn, err := w.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	uri, err := sqliteURI(source, "mode=ro")
	if err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS src", uri); err != nil {
		return 0, sqliteErr(err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), "DETACH DATABASE src")
	var n int64
	for _, q := range stmts {
		res, err := conn.ExecContext(ctx, q.q, q.args...)
		if err != nil {
			return 0, sqliteErr(err)
		}
		n, _ = res.RowsAffected()
	}
	return n, nil
}

// workDB is the source's work database, made when first needed. It is
// called with s.mu held.
func (s *sqliteSource) workDB() (*sqlWork, error) {
	if s.work == nil {
		w, err := newSQLWork(s.spec.TempDir)
		if err != nil {
			return nil, err
		}
		s.work = w
	}
	return s.work, nil
}

// sqlBase is a table whose rowids are its rows' numbers plus one: a
// dense source table, or the copy in the work database.
type sqlBase struct {
	db    *sql.DB
	table string   // quoted
	cols  []string // its columns, quoted
	work  bool     // in the work database
}

// byNumber is where rows are found by number, copying the source into
// the work database the first time when it must.
func (s *sqliteSource) byNumber(ctx context.Context) (*sqlBase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base != nil {
		return s.base, nil
	}
	if s.table != "" && s.dense {
		s.base = &sqlBase{db: s.db, table: s.table, cols: quoteAll(s.names)}
		return s.base, nil
	}
	w, err := s.workDB()
	if err != nil {
		return nil, err
	}
	if _, err := w.build(ctx, s.spec.Path, sqlStmt{q: "CREATE TABLE _012_rows AS " + s.query}); err != nil {
		w.db.ExecContext(context.WithoutCancel(ctx), "DROP TABLE IF EXISTS _012_rows")
		return nil, err
	}
	var names []string
	err = queryRows(ctx, w.db, "SELECT name FROM pragma_table_info('_012_rows') ORDER BY cid", nil, func(v []any) bool {
		names = append(names, fmt.Sprint(v[0]))
		return true
	})
	if err != nil {
		return nil, err
	}
	s.base = &sqlBase{db: w.db, table: "_012_rows", cols: quoteAll(names), work: true}
	return s.base, nil
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = quoteIdent(n)
	}
	return out
}

// list is the select list of cols after the rowid.
func (b *sqlBase) list(cols []int) string {
	parts := make([]string, 0, len(cols)+1)
	parts = append(parts, "rowid")
	for _, c := range cols {
		parts = append(parts, b.cols[c])
	}
	return strings.Join(parts, ", ")
}

// fetchChunk is how many rowids one query asks for.
const fetchChunk = 500

// fetch reads the rows numbered rows, in the order given.
func (b *sqlBase) fetch(ctx context.Context, rows []int64, cols []int) ([][]sheet.LiveCell, error) {
	got := make(map[int64][]sheet.LiveCell, len(rows))
	for i := 0; i < len(rows); i += fetchChunk {
		part := rows[i:min(i+fetchChunk, len(rows))]
		marks := strings.Repeat("?, ", len(part))
		args := make([]any, len(part))
		for j, r := range part {
			args[j] = r + 1
		}
		q := "SELECT " + b.list(cols) + " FROM " + b.table + " WHERE rowid IN (" + marks[:len(marks)-2] + ")"
		if err := b.collect(ctx, q, args, got); err != nil {
			return nil, err
		}
	}
	out := make([][]sheet.LiveCell, len(rows))
	for i, r := range rows {
		out[i] = got[r]
	}
	return out, nil
}

// page reads n rows from row from on.
func (b *sqlBase) page(ctx context.Context, from int64, n int, rows int64) ([]int64, [][]sheet.LiveCell, error) {
	if from < 0 || from >= rows || n <= 0 {
		return nil, nil, nil
	}
	got := map[int64][]sheet.LiveCell{}
	q := "SELECT " + b.list(allCols(nil, len(b.cols))) + " FROM " + b.table + " WHERE rowid BETWEEN ? AND ?"
	if err := b.collect(ctx, q, []any{from + 1, from + int64(n)}, got); err != nil {
		return nil, nil, err
	}
	var nums []int64
	var out [][]sheet.LiveCell
	for r := from; r < min(from+int64(n), rows); r++ {
		nums, out = append(nums, r), append(out, got[r])
	}
	return nums, out, nil
}

// collect runs a query of a rowid and values, keeping each row's
// values by its number.
func (b *sqlBase) collect(ctx context.Context, q string, args []any, got map[int64][]sheet.LiveCell) error {
	return queryRows(ctx, b.db, q, args, func(v []any) bool {
		cells := make([]sheet.LiveCell, len(v)-1)
		for i, x := range v[1:] {
			c, _ := sqliteCell(x)
			cells[i] = c.typed()
		}
		id, _ := v[0].(int64)
		got[id-1] = cells
		return true
	})
}
