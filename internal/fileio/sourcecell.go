package fileio

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// How Parquet and SQLite values become cells, shared by imports (which
// store them through the builder) and sources (which hand them out as
// they are). Text a file holds as a value comes out raw: an import
// stores it as typed in its locale (textOrDate) and a source reads the
// dates in it (typed), so both type a value the same way. Text 012
// makes (joined lists, a blob's size) stays text.

// fileCell is a value from a file, raw when it is text to be read as a
// date where it is one.
type fileCell struct {
	sheet.LiveCell
	raw bool
}

// liveNum is a number in format f; NaN and infinities, which no cell
// holds, are #NUM!.
func liveNum(v float64, f sheet.Format) sheet.LiveCell {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return sheet.LiveCell{V: sheet.ErrNum}
	}
	return sheet.LiveCell{V: sheet.Value{Kind: sheet.Number, Num: v}, F: f}
}

func strCell(s string) sheet.LiveCell {
	return sheet.LiveCell{V: sheet.Value{Kind: sheet.Text, Str: s}}
}

func liveBool(b bool) sheet.LiveCell {
	v := sheet.Value{Kind: sheet.Bool}
	if b {
		v.Num = 1
	}
	return sheet.LiveCell{V: v}
}

// put stores a cell as an import does: raw text through textOrDate,
// which reads dates in the import's locale.
func (c fileCell) put(b *builder, a sheet.Addr) {
	switch c.V.Kind {
	case sheet.Empty:
	case sheet.Number:
		b.number(a, c.V.Num, c.F, sheet.Style{})
	case sheet.Bool:
		b.boolean(a, c.V.Num != 0, sheet.Style{})
	case sheet.Text:
		if c.raw {
			textOrDate(b, a, c.V.Str)
		} else {
			b.text(a, c.V.Str, c.F, sheet.Style{})
		}
	default:
		b.number(a, math.NaN(), c.F, sheet.Style{})
	}
}

// typed is the cell as a source hands it out: raw text that is a date
// or time (2026-09-26) as that date, as an import would store it, and
// other text, numbers stored as text included, as text.
func (c fileCell) typed() sheet.LiveCell {
	if !c.raw {
		return c.LiveCell
	}
	s := flatten(c.V.Str)
	if s == "" {
		return sheet.LiveCell{} // blank, as an import leaves it
	}
	if n, f, ok := sheet.ParseValue(s); ok {
		switch f.Kind {
		case sheet.FmtDate, sheet.FmtTime, sheet.FmtDateTime:
			return sheet.LiveCell{V: sheet.Value{Kind: sheet.Number, Num: n}, F: f}
		}
	}
	return strCell(s)
}

// parquetCells is a row's values of one column as a cell: empty for
// null, several joined with commas (reporting that they were).
func parquetCells(vals []parquet.Value, c parquetColumn) (cell fileCell, repeated bool) {
	if len(vals) == 1 {
		if vals[0].IsNull() {
			return fileCell{}, false
		}
		return parquetCell(vals[0], c), false
	}
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		if !v.IsNull() {
			parts = append(parts, parquetText(v, c))
		}
	}
	if len(parts) == 0 {
		return fileCell{}, false
	}
	return fileCell{LiveCell: strCell(strings.Join(parts, ", "))}, true
}

// parquetCell is one value that isn't null.
func parquetCell(v parquet.Value, c parquetColumn) fileCell {
	lc, ok := parquetValue(v, c)
	if !ok {
		return fileCell{LiveCell: strCell(parquetText(v, c)), raw: true}
	}
	return fileCell{LiveCell: lc}
}

// parquetValue is a value that isn't text, and false for text.
func parquetValue(v parquet.Value, c parquetColumn) (sheet.LiveCell, bool) {
	switch lt := c.logical.(type) {
	case *format.DateType:
		return liveNum(float64(v.Int32())+serialOf(time.Unix(0, 0).UTC()), sheet.Format{Kind: sheet.FmtDate}), true
	case *format.TimestampType:
		return liveNum(serialOf(timestamp(v.Int64(), lt.Unit)), sheet.Format{Kind: sheet.FmtDateTime}), true
	case *format.TimeType:
		d := timeUnit(lt.Unit) * time.Duration(timeInt(v))
		return liveNum(d.Hours()/24, sheet.Format{Kind: sheet.FmtTime}), true
	case *format.DecimalType:
		if x, ok := decimal(v, lt.Scale); ok {
			return liveNum(x, decimalFormat(lt)), true
		}
	}
	if c.interval {
		if d, ok := interval(v); ok {
			return liveNum(d, sheet.Format{Kind: sheet.FmtDuration}), true
		}
	}
	switch c.kind {
	case parquet.Boolean:
		return liveBool(v.Boolean()), true
	case parquet.Int32:
		return liveNum(float64(v.Int32()), sheet.Format{}), true
	case parquet.Int64:
		return liveNum(float64(v.Int64()), sheet.Format{}), true
	case parquet.Float:
		return liveNum(float64(v.Float()), sheet.Format{}), true
	case parquet.Double:
		return liveNum(v.Double(), sheet.Format{}), true
	case parquet.Int96:
		// Legacy timestamps: nanoseconds of the day, then the Julian day.
		i := v.Int96()
		nanos := int64(i[1])<<32 | int64(i[0])
		days := int64(i[2]) - 2440588 // the Julian day of 1970-01-01
		return liveNum(serialOf(time.Unix(days*86400, nanos).UTC()), sheet.Format{Kind: sheet.FmtDateTime}), true
	}
	return sheet.LiveCell{}, false
}

// parquetFormat is the format a column's type shows its values in, and
// whether it holds numbers.
func parquetFormat(c parquetColumn) (sheet.Format, bool) {
	switch c.logical.(type) {
	case *format.DateType:
		return sheet.Format{Kind: sheet.FmtDate}, true
	case *format.TimestampType:
		return sheet.Format{Kind: sheet.FmtDateTime}, true
	case *format.TimeType:
		return sheet.Format{Kind: sheet.FmtTime}, true
	case *format.DecimalType:
		return decimalFormat(c.logical.(*format.DecimalType)), true
	}
	if c.interval {
		return sheet.Format{Kind: sheet.FmtDuration}, true
	}
	switch c.kind {
	case parquet.Int32, parquet.Int64, parquet.Float, parquet.Double:
		return sheet.Format{}, true
	case parquet.Int96:
		return sheet.Format{Kind: sheet.FmtDateTime}, true
	}
	return sheet.Format{}, false
}

// decimalFormat is a DECIMAL's: a number with its scale's decimals.
func decimalFormat(t *format.DecimalType) sheet.Format {
	return sheet.Format{Kind: sheet.FmtNumber, Decimals: min(max(int(t.Scale), 0), sheet.MaxDecimals)}
}

// interval is an INTERVAL's length in days: its months as 30 days each,
// its days, and its milliseconds.
func interval(v parquet.Value) (float64, bool) {
	b := v.ByteArray()
	if len(b) != 12 {
		return 0, false
	}
	months, days, ms := binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:]), binary.LittleEndian.Uint32(b[8:])
	return 30*float64(months) + float64(days) + float64(ms)/86_400_000, true
}

// sqliteCell is a database value as a cell, reporting whether it was
// binary data that could only be described.
func sqliteCell(v any) (cell fileCell, blob bool) {
	switch v := v.(type) {
	case nil:
		return fileCell{}, false
	case int64:
		return fileCell{LiveCell: liveNum(float64(v), sheet.Format{})}, false
	case float64:
		return fileCell{LiveCell: liveNum(v, sheet.Format{})}, false
	case bool:
		return fileCell{LiveCell: liveBool(v)}, false
	case time.Time:
		f := sheet.Format{Kind: sheet.FmtDateTime}
		if h, m, s := v.Clock(); h == 0 && m == 0 && s == 0 && v.Nanosecond() == 0 {
			f = sheet.Format{Kind: sheet.FmtDate}
		}
		return fileCell{LiveCell: liveNum(serialOf(v), f)}, false
	case string:
		return fileCell{LiveCell: strCell(v), raw: true}, false
	case []byte:
		if utf8.Valid(v) {
			return fileCell{LiveCell: strCell(string(v)), raw: true}, false
		}
		return fileCell{LiveCell: strCell(fmt.Sprintf("(%s)", count(len(v), "byte", "bytes")))}, true
	}
	return fileCell{LiveCell: strCell(fmt.Sprint(v))}, false
}
