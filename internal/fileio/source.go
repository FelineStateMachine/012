package fileio

import (
	"context"
	"errors"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Sources: a Parquet file, or a SQLite table or query, read in place a
// window of rows at a time rather than imported, for tables too big for
// any grid. A Source streams its rows in its own order for what reads
// them all (aggregates, pivot tables) and fetches the rows a screen
// shows; a SourceView is its rows as a sort and a filter order them,
// pushed down to SQL for SQLite and streamed for Parquet. Values come
// out typed as an import types them.

// SourceSpec names what a source reads.
type SourceSpec struct {
	// Path is the file.
	Path string
	// Format is "Parquet" or "SQLite", or "" to tell by the extension.
	Format string
	// Table is a SQLite table or view to read, or Query a query to run;
	// both empty reads a database's only table.
	Table, Query string
	// TempDir is where views keep what they build (sorted row orders,
	// SQLite's temporary tables); "" is the system's.
	TempDir string
}

// SourceColumn is one of a source's columns.
type SourceColumn struct {
	Name string
	// Format is what its values show in when the column's type says (a
	// Parquet DATE column's FmtDate); a value's own may differ.
	Format sheet.Format
	// Numeric is set when its type holds numbers, dates and times
	// included.
	Numeric bool
}

// SourceSort is a column a view is sorted by.
type SourceSort struct {
	Col  int
	Desc bool
}

// SourceFilter is a condition a view's rows meet on a column.
type SourceFilter struct {
	Col  int
	Cond sheet.Condition
}

// SourceOrder is how a view sorts and filters a source's rows.
type SourceOrder struct {
	Sort   []SourceSort
	Filter []SourceFilter
}

// IsZero reports whether the order is the source's own, every row.
func (o SourceOrder) IsZero() bool { return len(o.Sort) == 0 && len(o.Filter) == 0 }

// Source is a table read in place. It is safe for use by several
// goroutines at once.
type Source interface {
	// Columns are the source's columns, left to right.
	Columns() []SourceColumn
	// Rows counts its rows, the header not among them.
	Rows() int64
	// Scan streams the rows from row from on (counting from 0) in the
	// source's order, calling fn with each row's number and the values
	// of cols (every column when cols is nil, in that order; the slice
	// is reused) until fn returns false, the rows end or ctx is done.
	Scan(ctx context.Context, from int64, cols []int, fn func(row int64, vals []sheet.LiveCell) bool) error
	// Fetch reads the rows numbered rows, in the order given, with the
	// values of cols (every column when nil).
	Fetch(ctx context.Context, rows []int64, cols []int) ([][]sheet.LiveCell, error)
	// View orders the rows as o says. Building it may read the whole
	// source once, which the caller does in the background.
	View(ctx context.Context, o SourceOrder) (SourceView, error)
	// Close lets the file go, and what its views built.
	Close() error
}

// SourceView is a source's rows as a sort and a filter order them.
type SourceView interface {
	// Rows counts the rows the view shows.
	Rows() int64
	// Page reads n rows from position from of the view: each one's
	// number in the source and its values.
	Page(ctx context.Context, from int64, n int) ([]int64, [][]sheet.LiveCell, error)
	// Close lets go of what the view built.
	Close() error
}

// ErrNotSource is what OpenSource says of a file that is neither
// Parquet nor SQLite.
var ErrNotSource = errors.New("only Parquet files and SQLite tables or queries can be linked as sources")

// OpenSource opens what spec names.
func OpenSource(ctx context.Context, spec SourceSpec) (Source, error) {
	return nil, ErrNotSource
}
