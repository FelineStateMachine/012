package fileio

import (
	"context"
	"errors"
	"io"
	"math"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// parquetColumn is how a leaf column's values become cells.
type parquetColumn struct {
	logical format.LogicalTypeValue
	kind    parquet.Kind
	// repeated is set on a list's column, whose rows hold several
	// values; sources read it a row at a time (sourceparquet.go).
	repeated bool
}

// importParquet reads every leaf column, named by its path, into a
// sheet with a header row. Repeated values in a row are joined with
// commas. Rows past the sheet's last are counted from the file's
// metadata, not read.
func importParquet(ctx context.Context, name string, opt Options) (*Result, error) {
	prog := opt.Progress
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer context.AfterFunc(ctx, func() { f.Close() })() // unblocks a read from a pipe
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		return nil, err
	}
	b := newBuilder(ctx, opt.MaxCells)
	r := &parquetReader{
		b: b, cols: parquetHeader(b, pf.Schema()), prog: prog,
		row: 1, total: pf.NumRows(), buf: make([]parquet.Row, 256),
	}
	for _, rg := range pf.RowGroups() {
		if r.row >= sheet.MaxRows || b.isFull() {
			break
		}
		if err := r.group(ctx, rg); err != nil {
			return nil, err
		}
	}
	// The rows left out, as far as the file says there are.
	r.row = max(r.row, int(r.total)+1)
	b.fits(sheet.Addr{Row: r.row - 1})
	prog.setRows(r.row)
	var notes []string
	if r.lists > 0 {
		notes = append(notes, "repeated values joined with commas")
	}
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: r.row, Notes: notes}, nil
}

// parquetHeader writes the columns' paths as a bold header row and
// returns how each column's values become cells.
func parquetHeader(b *builder, schema *parquet.Schema) []parquetColumn {
	paths := schema.Columns()
	cols := make([]parquetColumn, len(paths))
	header := sheet.Style{Bold: true}
	for i, path := range paths {
		b.text(sheet.Addr{Col: i}, strings.Join(path, "."), sheet.Format{}, header)
		if leaf, ok := schema.Lookup(path...); ok && leaf.Node != nil {
			t := leaf.Node.Type()
			cols[i].kind = t.Kind()
			if lt := t.LogicalType(); lt != nil {
				cols[i].logical = lt.Value
			}
		}
	}
	return cols
}

// parquetReader holds the state of one import.
type parquetReader struct {
	b     *builder
	cols  []parquetColumn
	prog  *Progress
	row   int   // the sheet row of the next file row
	total int64 // rows in the file
	lists int   // rows with repeated values
	buf   []parquet.Row
}

// group reads a row group, a batch at a time, until it ends or the
// sheet is full.
func (r *parquetReader) group(ctx context.Context, rg parquet.RowGroup) error {
	rows := rg.Rows()
	defer rows.Close()
	for r.row < sheet.MaxRows && !r.b.isFull() {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := rows.ReadRows(r.buf)
		for _, row := range r.buf[:n] {
			if putParquetRow(r.b, r.row, row, r.cols) {
				r.lists++
			}
			r.row++
		}
		r.prog.setRows(r.row)
		r.prog.setFrac(int64(r.row-1), r.total)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
	return nil
}

// putParquetRow stores one row, reporting whether a column repeated.
func putParquetRow(b *builder, row int, r parquet.Row, cols []parquetColumn) (repeated bool) {
	if row >= sheet.MaxRows {
		b.fits(sheet.Addr{Row: row})
		return false
	}
	eachParquetColumn(r, func(col int, vals []parquet.Value) {
		if col < 0 || col >= len(cols) {
			return
		}
		c, rep := parquetCells(vals, cols[col])
		c.put(b, sheet.Addr{Col: col, Row: row})
		repeated = repeated || rep
	})
	return repeated
}

// eachParquetColumn calls fn with each column's values in a row: they
// arrive grouped by column, and a repeated column has several.
func eachParquetColumn(r parquet.Row, fn func(col int, vals []parquet.Value)) {
	for i := 0; i < len(r); {
		col := r[i].Column()
		j := i + 1
		for j < len(r) && r[j].Column() == col {
			j++
		}
		fn(col, r[i:j])
		i = j
	}
}

// parquetText is a value as text, for byte arrays and joined lists.
func parquetText(v parquet.Value, c parquetColumn) string {
	switch c.kind {
	case parquet.ByteArray, parquet.FixedLenByteArray:
		if _, isDec := c.logical.(*format.DecimalType); !isDec {
			return string(v.ByteArray())
		}
	}
	return v.String()
}

func timeUnit(u format.TimeUnit) time.Duration {
	if u.Value == nil {
		return time.Millisecond
	}
	return u.Value.Duration()
}

func timestamp(n int64, u format.TimeUnit) time.Time {
	d := timeUnit(u)
	return time.Unix(0, 0).UTC().Add(time.Duration(n) * d)
}

func timeInt(v parquet.Value) int64 {
	if v.Kind() == parquet.Int32 {
		return int64(v.Int32())
	}
	return v.Int64()
}

// decimal reads a DECIMAL stored as an integer or as big-endian bytes.
func decimal(v parquet.Value, scale int32) (float64, bool) {
	var unscaled float64
	switch v.Kind() {
	case parquet.Int32:
		unscaled = float64(v.Int32())
	case parquet.Int64:
		unscaled = float64(v.Int64())
	case parquet.ByteArray, parquet.FixedLenByteArray:
		raw := v.ByteArray()
		if len(raw) == 0 {
			return 0, false
		}
		n := new(big.Int).SetBytes(raw)
		if raw[0]&0x80 != 0 { // two's complement
			n.Sub(n, new(big.Int).Lsh(big.NewInt(1), uint(8*len(raw))))
		}
		unscaled, _ = new(big.Float).SetInt(n).Float64()
	default:
		return 0, false
	}
	return unscaled / math.Pow10(int(scale)), true
}
