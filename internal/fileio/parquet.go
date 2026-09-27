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
}

// importParquet reads every leaf column, named by its path, into a
// sheet with a header row. Repeated values in a row are joined with
// commas.
func importParquet(ctx context.Context, name string, prog *Progress) (*Result, error) {
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
	schema := pf.Schema()
	paths := schema.Columns()
	cols := make([]parquetColumn, len(paths))
	b := newBuilder()
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

	total := pf.NumRows()
	row := 1
	buf := make([]parquet.Row, 256)
	lists := 0
	for _, rg := range pf.RowGroups() {
		rows := rg.Rows()
		for {
			if err := ctx.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			n, err := rows.ReadRows(buf)
			for _, r := range buf[:n] {
				if putParquetRow(b, row, r, cols) {
					lists++
				}
				row++
			}
			prog.setRows(row)
			prog.setFrac(int64(row-1), total)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				rows.Close()
				return nil, err
			}
			if n == 0 {
				break
			}
		}
		rows.Close()
	}
	var notes []string
	if lists > 0 {
		notes = append(notes, "repeated values joined with commas")
	}
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: row, Notes: notes}, nil
}

// putParquetRow stores one row, reporting whether a column repeated.
func putParquetRow(b *builder, row int, r parquet.Row, cols []parquetColumn) (repeated bool) {
	if row >= sheet.MaxRows {
		b.fits(sheet.Addr{Row: row})
		return false
	}
	// Values arrive grouped by column; a repeated column has several.
	for i := 0; i < len(r); {
		col := r[i].Column()
		j := i + 1
		for j < len(r) && r[j].Column() == col {
			j++
		}
		if col < 0 || col >= len(cols) {
			i = j
			continue
		}
		a := sheet.Addr{Col: col, Row: row}
		vals := r[i:j]
		if len(vals) > 1 {
			parts := make([]string, 0, len(vals))
			for _, v := range vals {
				if !v.IsNull() {
					parts = append(parts, parquetText(v, cols[col]))
				}
			}
			if len(parts) > 0 {
				b.text(a, strings.Join(parts, ", "), sheet.Format{}, sheet.Style{})
				repeated = true
			}
		} else if !vals[0].IsNull() {
			putParquetValue(b, a, vals[0], cols[col])
		}
		i = j
	}
	return repeated
}

func putParquetValue(b *builder, a sheet.Addr, v parquet.Value, c parquetColumn) {
	switch lt := c.logical.(type) {
	case *format.DateType:
		b.number(a, float64(v.Int32())+serialOf(time.Unix(0, 0).UTC()), sheet.Format{Kind: sheet.FmtDate}, sheet.Style{})
		return
	case *format.TimestampType:
		t := timestamp(v.Int64(), lt.Unit)
		b.number(a, serialOf(t), sheet.Format{Kind: sheet.FmtDateTime}, sheet.Style{})
		return
	case *format.TimeType:
		d := timeUnit(lt.Unit) * time.Duration(timeInt(v))
		b.number(a, d.Hours()/24, sheet.Format{Kind: sheet.FmtTime}, sheet.Style{})
		return
	case *format.DecimalType:
		if x, ok := decimal(v, lt.Scale); ok {
			b.number(a, x, sheet.Format{}, sheet.Style{})
			return
		}
	}
	switch c.kind {
	case parquet.Boolean:
		b.boolean(a, v.Boolean(), sheet.Style{})
	case parquet.Int32:
		b.number(a, float64(v.Int32()), sheet.Format{}, sheet.Style{})
	case parquet.Int64:
		b.number(a, float64(v.Int64()), sheet.Format{}, sheet.Style{})
	case parquet.Float:
		b.number(a, float64(v.Float()), sheet.Format{}, sheet.Style{})
	case parquet.Double:
		b.number(a, v.Double(), sheet.Format{}, sheet.Style{})
	case parquet.Int96:
		// Legacy timestamps: nanoseconds of the day, then the Julian day.
		i := v.Int96()
		nanos := int64(i[1])<<32 | int64(i[0])
		days := int64(i[2]) - 2440588 // the Julian day of 1970-01-01
		t := time.Unix(days*86400, nanos).UTC()
		b.number(a, serialOf(t), sheet.Format{Kind: sheet.FmtDateTime}, sheet.Style{})
	default:
		textOrDate(b, a, parquetText(v, c))
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
