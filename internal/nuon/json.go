package nuon

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// AppendJSON appends v to b as nushell's `to json` writes it: sizes as
// bytes, durations as nanoseconds, dates as RFC 3339 strings, binary as
// a list of byte values, and infinities and NaN as null.
func AppendJSON(b []byte, v Value) []byte {
	switch v.Kind {
	case Null:
		return append(b, "null"...)
	case Bool:
		return strconv.AppendBool(b, v.Bool)
	case Int, Filesize, Duration:
		return strconv.AppendInt(b, v.Int, 10)
	case Float:
		if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
			return append(b, "null"...)
		}
		return appendFloat(b, v.Float)
	case String:
		return appendJSONString(b, v.Str)
	case Date:
		return append(appendDate(append(b, '"'), v.Time), '"')
	case Binary:
		b = append(b, '[')
		for i, c := range v.Bytes {
			if i > 0 {
				b = append(b, ',')
			}
			b = strconv.AppendUint(b, uint64(c), 10)
		}
		return append(b, ']')
	case List:
		b = append(b, '[')
		for i, item := range v.List {
			if i > 0 {
				b = append(b, ',')
			}
			b = AppendJSON(b, item)
		}
		return append(b, ']')
	case Record:
		return AppendJSONRecord(b, v.Fields)
	}
	return b
}

// AppendJSONRecord appends fields as a JSON object, in order.
func AppendJSONRecord(b []byte, fs []Field) []byte {
	b = append(b, '{')
	for i, f := range fs {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendJSONString(b, f.Key)
		b = append(b, ':')
		b = AppendJSON(b, f.Value)
	}
	return append(b, '}')
}

// appendJSONString quotes s for JSON, escaping only what JSON needs.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	for _, r := range strings.ToValidUTF8(s, "�") {
		switch {
		case r == '"' || r == '\\':
			b = append(b, '\\', byte(r))
		case r == '\n':
			b = append(b, `\n`...)
		case r == '\r':
			b = append(b, `\r`...)
		case r == '\t':
			b = append(b, `\t`...)
		case r < 0x20:
			b = append(b, fmt.Sprintf(`\u%04x`, r)...)
		default:
			b = utf8.AppendRune(b, r)
		}
	}
	return append(b, '"')
}
