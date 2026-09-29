// Package nuon reads and writes NUON, nushell's object notation: the
// text `to nuon` writes and `from nuon` reads, which keeps nushell's
// types (file sizes, durations, dates, binary) where JSON would turn
// them into numbers and strings. JSON is NUON too, so the same reader
// takes JSON documents, and a run of top-level values one after another
// (NDJSON, or records nushell streams) reads as one table.
//
// Tables are read a row at a time: a Reader takes an io.Reader and
// yields each row as it's parsed, never the whole document, with the
// header known up front for nushell's table form ([[a, b]; [1, 2]]) and
// from the records themselves for a list of records. So a reader may sit
// on a pipe that's still being written and hand over rows as they come.
// Each row's own values (a nested record, a list) are parsed whole.
//
// Writing goes the other way: Append writes a value as `to nuon` would,
// and a TableWriter writes rows in the table form as they're given.
// The package knows nothing of sheets; see internal/fileio for how
// values become cells.
package nuon

import (
	"math"
	"time"
)

// Kind is a NUON value's type.
type Kind uint8

// The kinds of value, as nushell's describe names them.
const (
	Null     Kind = iota
	Bool          // true, false
	Int           // 42, 0x2a, 1_000
	Float         // 1.5, 1e3, inf, NaN
	String        // "quoted", 'single', `backtick`, r#'raw'#, or a bare word
	Filesize      // 1646b, 1.5kb, 1MiB: Int bytes
	Duration      // 90sec, 1500000000ns: Int nanoseconds
	Date          // 2026-09-27T11:27:31-06:00
	Binary        // 0x[DEAD]
	List          // [1, 2]
	Record        // {a: 1}
)

var kindNames = [...]string{"nothing", "bool", "int", "float", "string", "filesize", "duration", "datetime", "binary", "list", "record"}

// String is the kind's name in nushell, e.g. "filesize".
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "unknown"
}

// Value is one NUON value. Which fields hold it depends on Kind: Int for
// Int, Filesize (bytes) and Duration (nanoseconds); Float; Str for
// String; Time for Date; Bytes for Binary; List; Fields for Record.
type Value struct {
	Kind   Kind
	Bool   bool
	Int    int64
	Float  float64
	Str    string
	Time   time.Time
	Bytes  []byte
	List   []Value
	Fields []Field
}

// Field is one of a record's fields, in the record's order.
type Field struct {
	Key   string
	Value Value
}

// Row is one row of a table: its columns' names and values, in order.
type Row = []Field

// Constructors for each kind.

// NullValue is null.
func NullValue() Value { return Value{} }

// BoolValue is true or false.
func BoolValue(b bool) Value { return Value{Kind: Bool, Bool: b} }

// IntValue is an integer.
func IntValue(n int64) Value { return Value{Kind: Int, Int: n} }

// FloatValue is a float.
func FloatValue(f float64) Value { return Value{Kind: Float, Float: f} }

// StringValue is a string.
func StringValue(s string) Value { return Value{Kind: String, Str: s} }

// FilesizeValue is a file size in bytes.
func FilesizeValue(bytes int64) Value { return Value{Kind: Filesize, Int: bytes} }

// DurationValue is a duration.
func DurationValue(d time.Duration) Value { return Value{Kind: Duration, Int: int64(d)} }

// DateValue is a date and time, with its offset from UTC.
func DateValue(t time.Time) Value { return Value{Kind: Date, Time: t} }

// BinaryValue is bytes.
func BinaryValue(b []byte) Value { return Value{Kind: Binary, Bytes: b} }

// ListValue is a list.
func ListValue(vs ...Value) Value { return Value{Kind: List, List: vs} }

// RecordValue is a record.
func RecordValue(fs ...Field) Value { return Value{Kind: Record, Fields: fs} }

// Equal reports whether a and b are the same value: the same kind and
// contents, dates at the same instant with the same offset, and NaN
// equal to NaN.
func Equal(a, b Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Bool:
		return a.Bool == b.Bool
	case Int, Filesize, Duration:
		return a.Int == b.Int
	case Float:
		return a.Float == b.Float || math.IsNaN(a.Float) && math.IsNaN(b.Float)
	case String:
		return a.Str == b.Str
	case Date:
		_, ao := a.Time.Zone()
		_, bo := b.Time.Zone()
		return a.Time.Equal(b.Time) && ao == bo
	case Binary:
		return string(a.Bytes) == string(b.Bytes)
	case List:
		return equalLists(a.List, b.List)
	case Record:
		return equalFields(a.Fields, b.Fields)
	}
	return true
}

func equalLists(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalFields(a, b []Field) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || !Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}
