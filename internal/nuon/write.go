package nuon

import (
	"bufio"
	"encoding/hex"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// String is v as NUON text.
func (v Value) String() string { return string(Append(nil, v)) }

// Append appends v to b as `to nuon` writes it: ints, floats with a
// point (1.0), sizes in bytes (1646b), durations in nanoseconds
// (90000000000ns), dates in RFC 3339 with their offset, binary as
// 0x[...], strings bare where nushell reads them back as the same
// string and quoted otherwise, and a list of records that share their
// columns in the table form.
func Append(b []byte, v Value) []byte {
	switch v.Kind {
	case Null:
		return append(b, "null"...)
	case Bool:
		return strconv.AppendBool(b, v.Bool)
	case Int:
		return strconv.AppendInt(b, v.Int, 10)
	case Float:
		return appendFloat(b, v.Float)
	case String:
		return appendString(b, v.Str)
	case Filesize:
		return append(strconv.AppendInt(b, v.Int, 10), 'b')
	case Duration:
		return append(strconv.AppendInt(b, v.Int, 10), "ns"...)
	case Date:
		return appendDate(b, v.Time)
	case Binary:
		return append(append(append(b, "0x["...), strings.ToUpper(hex.EncodeToString(v.Bytes))...), ']')
	case List:
		return appendList(b, v.List)
	case Record:
		return appendRecord(b, v.Fields)
	}
	return b
}

func appendFloat(b []byte, f float64) []byte {
	switch {
	case math.IsNaN(f):
		return append(b, "NaN"...)
	case math.IsInf(f, 1):
		return append(b, "inf"...)
	case math.IsInf(f, -1):
		return append(b, "-inf"...)
	}
	start := len(b)
	b = strconv.AppendFloat(b, f, 'f', -1, 64)
	if !strings.ContainsRune(string(b[start:]), '.') {
		b = append(b, ".0"...)
	}
	return b
}

// appendDate writes t as nushell does: RFC 3339 with its offset, and
// the fraction of a second in 3, 6 or 9 digits when there is one.
func appendDate(b []byte, t time.Time) []byte {
	layout := "2006-01-02T15:04:05"
	switch ns := t.Nanosecond(); {
	case ns == 0:
	case ns%1e6 == 0:
		layout += ".000"
	case ns%1e3 == 0:
		layout += ".000000"
	default:
		layout += ".000000000"
	}
	return t.AppendFormat(b, layout+"-07:00")
}

func appendList(b []byte, vs []Value) []byte {
	if cols, ok := tableColumns(vs); ok {
		return appendTable(b, cols, vs)
	}
	b = append(b, '[')
	for i, v := range vs {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = Append(b, v)
	}
	return append(b, ']')
}

func appendRecord(b []byte, fs []Field) []byte {
	b = append(b, '{')
	for i, f := range fs {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = appendString(b, f.Key)
		b = append(b, ": "...)
		b = Append(b, f.Value)
	}
	return append(b, '}')
}

// tableColumns reports whether vs is a list of records with the same
// keys in the same order, with the keys, which the table form writes
// once.
func tableColumns(vs []Value) ([]string, bool) {
	if len(vs) == 0 || vs[0].Kind != Record || len(vs[0].Fields) == 0 {
		return nil, false
	}
	first := vs[0].Fields
	for _, v := range vs[1:] {
		if v.Kind != Record || len(v.Fields) != len(first) {
			return nil, false
		}
		for i, f := range v.Fields {
			if f.Key != first[i].Key {
				return nil, false
			}
		}
	}
	cols := make([]string, len(first))
	for i, f := range first {
		cols[i] = f.Key
	}
	return cols, true
}

func appendTable(b []byte, cols []string, vs []Value) []byte {
	b = appendHeader(b, cols)
	for i, v := range vs {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = append(b, '[')
		for j, f := range v.Fields {
			if j > 0 {
				b = append(b, ", "...)
			}
			b = Append(b, f.Value)
		}
		b = append(b, ']')
	}
	return append(b, ']')
}

// appendHeader writes the table form's start: [[a, b]; .
func appendHeader(b []byte, cols []string) []byte {
	b = append(b, "[["...)
	for i, c := range cols {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = appendString(b, c)
	}
	return append(b, "]; "...)
}

// appendString writes s bare when nushell reads the bare word back as
// this string, and otherwise in double quotes.
func appendString(b []byte, s string) []byte {
	if bareSafe(s) {
		return append(b, s...)
	}
	b = append(b, '"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b = append(b, '\\', byte(r))
		case r == '\n':
			b = append(b, `\n`...)
		case r == '\r':
			b = append(b, `\r`...)
		case r == '\t':
			b = append(b, `\t`...)
		case r < 0x20 || r == 0x7f:
			b = append(b, `\u{`...)
			b = strconv.AppendInt(b, int64(r), 16)
			b = append(b, '}')
		default:
			b = utf8.AppendRune(b, r)
		}
	}
	return append(b, '"')
}

// bareSafe reports whether s can be written without quotes: letters,
// digits and _ - . only, starting with a letter or _, and not a word
// that reads as something else (true, inf, 1kb, 2026-01-01).
func bareSafe(s string) bool {
	if s == "" || !utf8.ValidString(s) {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case i > 0 && (r == '-' || r == '.' || unicode.IsDigit(r)):
		default:
			return false
		}
	}
	switch strings.ToLower(s) {
	case "true", "false", "null", "inf", "infinity", "nan":
		return false
	}
	v, _ := classify(s)
	return v.Kind == String
}

// TableWriter writes rows as a table in the table form, a row at a time.
// With no rows it writes an empty list, [], as nushell writes an empty
// table.
type TableWriter struct {
	w    *bufio.Writer
	cols []string
	rows int
	buf  []byte
}

// NewTableWriter writes a table of columns cols to w.
func NewTableWriter(w io.Writer, cols []string) *TableWriter {
	return &TableWriter{w: bufio.NewWriter(w), cols: cols}
}

// Write writes a row: a value for each column, in order.
func (t *TableWriter) Write(row []Value) error {
	b := t.buf[:0]
	if t.rows == 0 {
		b = appendHeader(b, t.cols)
	} else {
		b = append(b, ",\n"...)
	}
	t.rows++
	b = append(b, '[')
	for i, v := range row {
		if i > 0 {
			b = append(b, ", "...)
		}
		b = Append(b, v)
	}
	b = append(b, ']')
	t.buf = b
	_, err := t.w.Write(b)
	return err
}

// Close ends the table and flushes it.
func (t *TableWriter) Close() error {
	end := "]\n"
	if t.rows == 0 {
		end = "[]\n"
	}
	if _, err := t.w.WriteString(end); err != nil {
		return err
	}
	return t.w.Flush()
}
