package fileio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/FelineStateMachine/012/internal/sheet"
)

// TableInfo describes a table or view in a SQLite database, for the
// table picker.
type TableInfo struct {
	Name string
	View bool
	Rows int
	Cols []string
}

// openSQLite opens an existing database for reading only.
func openSQLite(ctx context.Context, name string) (*sql.DB, error) {
	if _, err := os.Stat(name); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", name)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA query_only = 1"); err != nil {
		db.Close()
		return nil, sqliteErr(err)
	}
	return db, nil
}

// sqliteErr shortens the driver's errors, e.g. "file is not a database
// (26)" for a file that isn't SQLite.
func sqliteErr(err error) error {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && strings.HasPrefix(msg, "SQL logic error") {
		msg = msg[i+2:]
	}
	msg = strings.TrimSuffix(msg, " (1)")
	return errors.New(msg)
}

// quoteIdent quotes a table or column name for SQL.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// Tables lists the tables and views of a SQLite database with their row
// counts and columns.
func Tables(ctx context.Context, name string) ([]TableInfo, error) {
	db, err := openSQLite(ctx, name)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return tables(ctx, db)
}

func tables(ctx context.Context, db *sql.DB) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, type FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, sqliteErr(err)
	}
	var out []TableInfo
	for rows.Next() {
		var t TableInfo
		var typ string
		if err := rows.Scan(&t.Name, &typ); err != nil {
			rows.Close()
			return nil, err
		}
		t.View = typ == "view"
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, sqliteErr(err)
	}
	for i := range out {
		t := &out[i]
		db.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdent(t.Name)).Scan(&t.Rows)
		if r, err := db.QueryContext(ctx, "SELECT * FROM "+quoteIdent(t.Name)+" LIMIT 0"); err == nil {
			t.Cols, _ = r.Columns()
			r.Close()
		}
	}
	return out, nil
}

func importSQLite(ctx context.Context, name string, opt Options) (*Result, error) {
	db, err := openSQLite(ctx, name)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query, total := opt.Query, 0
	if query == "" {
		table := opt.Table
		if table == "" {
			ts, err := tables(ctx, db)
			switch {
			case err != nil:
				return nil, err
			case len(ts) == 0:
				return nil, errors.New("the database has no tables")
			case len(ts) > 1:
				return nil, &ErrNeedTable{Tables: ts}
			}
			table = ts[0].Name
		}
		query = "SELECT * FROM " + quoteIdent(table)
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdent(table)).Scan(&total); err != nil {
			return nil, sqliteErr(err)
		}
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, sqliteErr(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, sqliteErr(err)
	}

	b := newBuilder()
	header := sheet.Style{Bold: true}
	for c, name := range cols {
		b.text(sheet.Addr{Col: c}, name, sheet.Format{}, header)
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	row, blobs := 1, 0
	for ; rows.Next(); row++ {
		if row%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			prog := opt.Progress
			prog.setRows(row)
			prog.setFrac(int64(row), int64(total))
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, sqliteErr(err)
		}
		if row >= sheet.MaxRows {
			b.fits(sheet.Addr{Row: row})
			continue
		}
		for c, v := range vals {
			if putValue(b, sheet.Addr{Col: c, Row: row}, v) {
				blobs++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, sqliteErr(err)
	}
	opt.Progress.setRows(row)
	var notes []string
	if blobs > 0 {
		notes = append(notes, count(blobs, "binary value", "binary values")+" shown as sizes")
	}
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: row, Notes: notes}, nil
}

// putValue stores a database value, reporting whether it was binary data
// that could only be described.
func putValue(b *builder, a sheet.Addr, v any) (blob bool) {
	switch v := v.(type) {
	case nil:
	case int64:
		b.number(a, float64(v), sheet.Format{}, sheet.Style{})
	case float64:
		b.number(a, v, sheet.Format{}, sheet.Style{})
	case bool:
		b.boolean(a, v, sheet.Style{})
	case time.Time:
		f := sheet.Format{Kind: sheet.FmtDateTime}
		if h, m, s := v.Clock(); h == 0 && m == 0 && s == 0 && v.Nanosecond() == 0 {
			f = sheet.Format{Kind: sheet.FmtDate}
		}
		b.number(a, serialOf(v), f, sheet.Style{})
	case string:
		textOrDate(b, a, v)
	case []byte:
		if utf8.Valid(v) {
			textOrDate(b, a, string(v))
			return false
		}
		b.text(a, fmt.Sprintf("(%s)", count(len(v), "byte", "bytes")), sheet.Format{}, sheet.Style{})
		return true
	default:
		b.text(a, fmt.Sprint(v), sheet.Format{}, sheet.Style{})
	}
	return false
}

// textOrDate stores database text as text, except dates and times such
// as 2026-09-26, which become dates as they would when typed. Numbers
// stored as text (zip codes, IDs) stay text.
func textOrDate(b *builder, a sheet.Addr, s string) {
	if _, f, ok := sheet.ParseValue(s); ok {
		switch f.Kind {
		case sheet.FmtDate, sheet.FmtTime, sheet.FmtDateTime:
			b.entry(a, s)
			return
		}
	}
	b.text(a, s, sheet.Format{}, sheet.Style{})
}

// epoch is day 0 of serial dates.
var epoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// serialOf is t's serial date, reading its wall clock.
func serialOf(t time.Time) float64 {
	wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
	return wall.Sub(epoch).Hours() / 24
}

// timeOf is the time of a serial date, to the second.
func timeOf(v float64) time.Time {
	days := math.Floor(v)
	secs := math.Round((v - days) * 86400)
	return epoch.AddDate(0, 0, int(days)).Add(time.Duration(secs) * time.Second)
}

// sqlColumn is a column of an exported table.
type sqlColumn struct {
	name string
	typ  string // INTEGER, REAL or TEXT
}

// exportSQLite writes the snapshot as a table, its first row naming the
// columns. An existing table of that name is replaced; the rest of the
// database is left alone.
func exportSQLite(ctx context.Context, name string, snap *Snapshot, table string) (*ExportResult, error) {
	if table == "" {
		table = TableName(snap.Name)
	}
	r := snap.Range
	cols := make([]sqlColumn, r.To.Col-r.From.Col+1)
	seen := map[string]bool{}
	for i := range cols {
		a := sheet.Addr{Col: r.From.Col + i, Row: r.From.Row}
		n := strings.TrimSpace(snap.Cells[a].Text())
		if n == "" {
			n = "column_" + sheet.ColName(a.Col)
		}
		base := n
		for k := 2; seen[strings.ToLower(n)]; k++ {
			n = fmt.Sprintf("%s_%d", base, k)
		}
		seen[strings.ToLower(n)] = true
		cols[i] = sqlColumn{name: n, typ: columnType(snap, a.Col)}
	}

	db, err := sql.Open("sqlite", name)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, sqliteErr(err)
	}
	defer tx.Rollback()
	defs := make([]string, len(cols))
	marks := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = quoteIdent(c.name) + " " + c.typ
		marks[i] = "?"
	}
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS " + quoteIdent(table),
		"CREATE TABLE " + quoteIdent(table) + " (" + strings.Join(defs, ", ") + ")",
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return nil, sqliteErr(err)
		}
	}
	ins, err := tx.PrepareContext(ctx, "INSERT INTO "+quoteIdent(table)+" VALUES ("+strings.Join(marks, ", ")+")")
	if err != nil {
		return nil, sqliteErr(err)
	}
	defer ins.Close()
	args := make([]any, len(cols))
	n := 0
	for row := r.From.Row + 1; row <= r.To.Row; row++ {
		empty := true
		for i, col := range cols {
			c, ok := snap.Cells[sheet.Addr{Col: r.From.Col + i, Row: row}]
			args[i] = nil
			if ok {
				args[i] = sqlValue(c, col.typ)
				empty = false
			}
		}
		if empty {
			continue
		}
		if _, err := ins.ExecContext(ctx, args...); err != nil {
			return nil, sqliteErr(err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return nil, sqliteErr(err)
	}
	return &ExportResult{Rows: n}, nil
}

// columnType picks a column's type from its cells below the header:
// INTEGER or REAL when every value is a number (or boolean), TEXT when
// any is text or a date.
func columnType(snap *Snapshot, col int) string {
	typ := ""
	for row := snap.Range.From.Row + 1; row <= snap.Range.To.Row; row++ {
		c, ok := snap.Cells[sheet.Addr{Col: col, Row: row}]
		if !ok || c.Value.Kind == sheet.Empty || c.Value.Kind == sheet.Error {
			continue
		}
		switch {
		case c.Value.Kind == sheet.Text, c.Format.Kind == sheet.FmtDate, c.Format.Kind == sheet.FmtTime,
			c.Format.Kind == sheet.FmtDateTime, c.Format.Kind == sheet.FmtText:
			return "TEXT"
		case c.Value.Kind == sheet.Number && c.Value.Num != math.Trunc(c.Value.Num):
			typ = "REAL"
		case typ == "":
			typ = "INTEGER"
		}
	}
	if typ == "" {
		return "TEXT"
	}
	return typ
}

// sqlValue is a cell's value for a column of type typ.
func sqlValue(c SnapCell, typ string) any {
	v := c.Value
	switch v.Kind {
	case sheet.Empty, sheet.Error:
		return nil
	case sheet.Bool:
		if typ == "TEXT" {
			return v.String()
		}
		return int64(v.Num)
	case sheet.Text:
		return v.Str
	}
	switch typ {
	case "INTEGER":
		return int64(v.Num)
	case "REAL":
		return v.Num
	}
	t := timeOf(v.Num)
	switch c.Format.Kind {
	case sheet.FmtDate:
		return t.Format(time.DateOnly)
	case sheet.FmtTime:
		return t.Format(time.TimeOnly)
	case sheet.FmtDateTime:
		return t.Format(time.DateTime)
	}
	return v.String()
}

// TableName makes a table name from a file or sheet name: letters,
// digits and underscores, not starting with a digit.
func TableName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "sheet"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "t_" + out
	}
	return out
}
