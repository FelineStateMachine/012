// Package fileio moves data between sheets and other file formats: CSV
// and TSV, Excel workbooks, SQLite databases, Parquet files and Lotus
// 1-2-3 worksheets. Everything is pure Go. The sheet package knows none
// of these formats: importers build a sheet through its public API
// (Load, SetColWidth, RecalcAll) and exporters read a Snapshot of one.
package fileio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Kind is an external file format.
type Kind int

const (
	CSV Kind = iota + 1
	TSV
	XLSX
	SQLite
	Parquet
	WK1
)

var kinds = []struct {
	kind   Kind
	name   string   // short name, e.g. in "Export as CSV"
	label  string   // what the file is, for the import picker
	exts   []string // recognized extensions, the first one written
	export bool
}{
	{CSV, "CSV", "Comma-separated values", []string{".csv"}, true},
	{TSV, "TSV", "Tab-separated values", []string{".tsv", ".tab"}, true},
	{XLSX, "XLSX", "Excel workbook", []string{".xlsx", ".xlsm"}, true},
	{SQLite, "SQLite", "SQLite database", []string{".sqlite", ".sqlite3", ".db"}, true},
	{Parquet, "Parquet", "Parquet file", []string{".parquet"}, false},
	{WK1, "WK1", "Lotus 1-2-3 worksheet", []string{".wk1", ".wks"}, false},
}

// Kinds lists every format, in menu order.
func Kinds() []Kind {
	out := make([]Kind, len(kinds))
	for i, k := range kinds {
		out[i] = k.kind
	}
	return out
}

// String is the format's short name, e.g. "XLSX".
func (k Kind) String() string {
	if k < 1 || int(k) > len(kinds) {
		return "unknown"
	}
	return kinds[k-1].name
}

// Label says what a file of this kind is, e.g. "Excel workbook".
func (k Kind) Label() string {
	if k < 1 || int(k) > len(kinds) {
		return ""
	}
	return kinds[k-1].label
}

// Ext is the extension written for this kind, e.g. ".xlsx".
func (k Kind) Ext() string {
	if k < 1 || int(k) > len(kinds) {
		return ""
	}
	return kinds[k-1].exts[0]
}

// CanExport reports whether sheets can be written in this format.
func (k Kind) CanExport() bool {
	return k >= 1 && int(k) <= len(kinds) && kinds[k-1].export
}

// KindOf recognizes a file's format by its extension.
func KindOf(name string) (Kind, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	for _, k := range kinds {
		for _, e := range k.exts {
			if e == ext {
				return k.kind, true
			}
		}
	}
	return 0, false
}

// Progress reports how far an import has got. The importer updates it
// from its goroutine; the UI reads it on a timer.
type Progress struct {
	rows     atomic.Int64
	permille atomic.Int64 // 0-1000, or -1 while the total isn't known
}

// NewProgress returns a progress with an unknown total.
func NewProgress() *Progress {
	p := &Progress{}
	p.permille.Store(-1)
	return p
}

// Get returns the rows read so far and the fraction done, which is
// negative while unknown.
func (p *Progress) Get() (rows int, frac float64) {
	if p == nil {
		return 0, -1
	}
	pm := p.permille.Load()
	if pm < 0 {
		return int(p.rows.Load()), -1
	}
	return int(p.rows.Load()), float64(pm) / 1000
}

// Report sets the rows read and, with a positive total, the fraction
// done; importers call it, and tests of progress displays.
func (p *Progress) Report(rows int, done, total int64) {
	p.setRows(rows)
	p.setFrac(done, total)
}

func (p *Progress) setRows(n int) {
	if p != nil {
		p.rows.Store(int64(n))
	}
}

// setFrac records done out of total; a non-positive total is unknown.
func (p *Progress) setFrac(done, total int64) {
	if p == nil || total <= 0 {
		return
	}
	p.permille.Store(min(done*1000/total, 1000))
}

// Options tune an import.
type Options struct {
	// Table is the SQLite table to import, or Query a SELECT to run.
	// With neither, a database with one table imports it.
	Table, Query string
	Progress     *Progress
}

// Result is an imported sheet and what the import had to leave out or
// change, in sentences for the context line.
type Result struct {
	Sheet *sheet.Sheet
	Kind  Kind
	Rows  int // rows of the file that were read, including a header
	Notes []string
}

// ErrNeedTable is returned for a SQLite database with several tables
// when Options names neither a table nor a query; Tables lists them.
type ErrNeedTable struct{ Tables []TableInfo }

func (e *ErrNeedTable) Error() string {
	return fmt.Sprintf("the database has %d tables; pick one", len(e.Tables))
}

// ErrUnsupported is returned for a file whose extension isn't known.
var ErrUnsupported = errors.New("not a format 012 can import")

// Import reads the file name into a new sheet. It checks ctx between
// rows, so a long import can be cancelled.
func Import(ctx context.Context, name string, opt Options) (*Result, error) {
	k, ok := KindOf(name)
	if !ok {
		return nil, ErrUnsupported
	}
	span := telemetry.Start("import", slog.String("format", k.String()))
	r, err := importKind(ctx, name, k, opt)
	if err != nil {
		span.Fail(err)
		return nil, err
	}
	if telemetry.Enabled() {
		span.End(slog.Int64("bytes", fileSize(name)), slog.Int("rows", r.Rows), slog.Int("cells", r.Sheet.Len()), slog.Int("notes", len(r.Notes)))
	}
	return r, nil
}

func importKind(ctx context.Context, name string, k Kind, opt Options) (*Result, error) {
	var (
		r   *Result
		err error
	)
	switch k {
	case CSV, TSV:
		r, err = importDelimited(ctx, name, k, opt.Progress)
	case XLSX:
		r, err = importXLSX(ctx, name, opt.Progress)
	case SQLite:
		r, err = importSQLite(ctx, name, opt)
	case Parquet:
		r, err = importParquet(ctx, name, opt.Progress)
	case WK1:
		r, err = importWK1(ctx, name, opt.Progress)
	}
	if err != nil {
		return nil, err
	}
	// A file of one table becomes one sheet named after it, as in Sheets.
	if book := r.Sheet.Book(); k != XLSX && book.Len() == 1 {
		base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		if opt.Table != "" {
			base = opt.Table
		}
		if book.RenameSheet(r.Sheet, sheetName(base)) == nil {
			book.ClearHistory()
		}
	}
	r.Kind = k
	if p := opt.Progress; p != nil && p.permille.Load() >= 0 {
		p.permille.Store(1000)
	}
	return r, nil
}

// ExportOptions tune an export.
type ExportOptions struct {
	Table string // SQLite: the table to write; replaced if it exists
}

// ExportResult says what an export wrote and what it couldn't keep.
type ExportResult struct {
	Rows  int
	Notes []string
}

// Export writes a snapshot of a sheet to name in format k.
func Export(ctx context.Context, name string, k Kind, snap *Snapshot, opt ExportOptions) (*ExportResult, error) {
	span := telemetry.Start("export", slog.String("format", k.String()), slog.Int("cells", len(snap.Cells)))
	res, err := exportKind(ctx, name, k, snap, opt)
	if err != nil {
		span.Fail(err)
		return nil, err
	}
	if telemetry.Enabled() {
		span.End(slog.Int("rows", res.Rows), slog.Int64("bytes", fileSize(name)))
	}
	return res, nil
}

func exportKind(ctx context.Context, name string, k Kind, snap *Snapshot, opt ExportOptions) (*ExportResult, error) {
	switch k {
	case CSV, TSV:
		return exportDelimited(name, k, snap)
	case XLSX:
		return exportXLSX(name, snap)
	case SQLite:
		return exportSQLite(ctx, name, snap, opt.Table)
	}
	return nil, fmt.Errorf("can't export %s files", k)
}

// fileSize is the size of the file name, for telemetry; -1 if unknown.
func fileSize(name string) int64 {
	st, err := os.Stat(name)
	if err != nil {
		return -1
	}
	return st.Size()
}
