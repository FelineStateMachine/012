package nuon

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"time"
)

// Parse reads one NUON value from data; anything after it but spaces
// and comments is an error.
func Parse(data []byte) (Value, error) {
	s := newScanner(bytes.NewReader(data))
	if _, ok, err := s.space(false); err != nil || !ok {
		if err == nil {
			err = s.fail("no value")
		}
		return Value{}, err
	}
	v, err := s.value()
	if err != nil {
		return Value{}, err
	}
	if c, ok, err := s.space(false); err != nil || ok {
		if err == nil {
			err = s.fail("unexpected %q after the value", c)
		}
		return Value{}, err
	}
	return v, nil
}

// value reads the value at the next byte, spaces skipped.
func (s *scanner) value() (Value, error) {
	c, _, err := s.peek()
	if err != nil {
		return Value{}, err
	}
	switch {
	case c == '[':
		return s.list()
	case c == '{':
		return s.record()
	case c == '"' || c == '\'' || c == '`':
		str, err := s.str()
		return StringValue(str), err
	case s.isRaw():
		str, err := s.raw()
		return StringValue(str), err
	case c == '(' || c == '$':
		return Value{}, s.fail("expressions and variables aren't NUON")
	}
	word, err := s.bare(false)
	if err != nil {
		return Value{}, err
	}
	if (word == "0x" || word == "0b" || word == "0o") && s.peekAt(0) == '[' {
		s.notJSON = true
		return s.binary(word[1])
	}
	v, json := classify(word)
	if !json {
		s.notJSON = true
	}
	return v, nil
}

// nest counts a level of lists and records.
func (s *scanner) nest() error {
	if s.depth++; s.depth > maxDepth {
		return s.fail("nested more than %d deep", maxDepth)
	}
	return nil
}

// list reads [a, b] or the table form [[a, b]; [1, 2], [3, 4]], which
// is a list of records.
func (s *scanner) list() (Value, error) {
	if err := s.nest(); err != nil {
		return Value{}, err
	}
	defer func() { s.depth-- }()
	s.take() // [
	var items []Value
	for i := 0; ; i++ {
		c, ok, err := s.space(true)
		if err != nil {
			return Value{}, err
		}
		if !ok {
			return Value{}, s.fail("unterminated list")
		}
		if c == ']' {
			s.take()
			return ListValue(items...), nil
		}
		if c == ';' && i == 1 && items[0].Kind == List {
			s.take()
			return s.tableRows(items[0].List)
		}
		v, err := s.value()
		if err != nil {
			return Value{}, err
		}
		items = append(items, v)
	}
}

// tableRows reads the rows of the table form after the ;, up to the
// closing ], as records.
func (s *scanner) tableRows(header []Value) (Value, error) {
	s.notJSON = true
	cols, err := s.columns(header)
	if err != nil {
		return Value{}, err
	}
	var rows []Value
	for {
		c, ok, err := s.space(true)
		if err != nil {
			return Value{}, err
		}
		if !ok {
			return Value{}, s.fail("unterminated table")
		}
		if c == ']' {
			s.take()
			return ListValue(rows...), nil
		}
		row, err := s.tableRow(cols)
		if err != nil {
			return Value{}, err
		}
		rows = append(rows, RecordValue(row...))
	}
}

// columns are a table's column names, from the list before the ;.
func (s *scanner) columns(header []Value) ([]string, error) {
	cols := make([]string, len(header))
	for i, h := range header {
		switch h.Kind {
		case String:
			cols[i] = h.Str
		case Int, Float, Bool:
			cols[i] = string(Append(nil, h))
		default:
			return nil, s.fail("a column name must be a string, not a %s", h.Kind)
		}
	}
	return cols, nil
}

// tableRow reads one row of the table form, [1, 2], as fields named by
// cols.
func (s *scanner) tableRow(cols []string) ([]Field, error) {
	if c, _, _ := s.peek(); c != '[' {
		return nil, s.fail("expected a table row, [...]")
	}
	row, err := s.list()
	if err != nil {
		return nil, err
	}
	if len(row.List) != len(cols) {
		return nil, s.fail("a row of %d values in a table of %d columns", len(row.List), len(cols))
	}
	fields := make([]Field, len(cols))
	for i, v := range row.List {
		fields[i] = Field{Key: cols[i], Value: v}
	}
	return fields, nil
}

// record reads {a: 1, "b c": 2}.
func (s *scanner) record() (Value, error) {
	if err := s.nest(); err != nil {
		return Value{}, err
	}
	defer func() { s.depth-- }()
	s.take() // {
	fields := []Field{}
	for {
		c, ok, err := s.space(true)
		if err != nil {
			return Value{}, err
		}
		if !ok {
			return Value{}, s.fail("unterminated record")
		}
		if c == '}' {
			s.take()
			return RecordValue(fields...), nil
		}
		key, err := s.key(c)
		if err != nil {
			return Value{}, err
		}
		if _, _, err := s.space(false); err != nil {
			return Value{}, err
		}
		if err := s.expect(':'); err != nil {
			return Value{}, err
		}
		if _, ok, err := s.space(false); err != nil || !ok {
			if err == nil {
				err = s.fail("a key without a value")
			}
			return Value{}, err
		}
		v, err := s.value()
		if err != nil {
			return Value{}, err
		}
		fields = append(fields, Field{Key: key, Value: v})
	}
}

// key reads a record's key, quoted or bare, starting with c.
func (s *scanner) key(c byte) (string, error) {
	switch {
	case c == '"' || c == '\'' || c == '`':
		return s.str()
	case s.isRaw():
		return s.raw()
	}
	s.notJSON = true
	return s.bare(true)
}

// binary reads the bytes of 0x[...], 0b[...] or 0o[...] after the
// prefix: digits of the base, with spaces between groups allowed.
func (s *scanner) binary(base byte) (Value, error) {
	s.take() // [
	var digits strings.Builder
	for {
		c, err := s.take()
		if err != nil {
			return Value{}, err
		}
		switch {
		case c == ']':
			b, ok := decodeBinary(digits.String(), base)
			if !ok {
				return Value{}, s.fail("bad binary digits")
			}
			return BinaryValue(b), nil
		case isSpace(c) || c == ',' || c == '_':
		default:
			digits.WriteByte(c)
		}
	}
}

// decodeBinary turns the digits of a binary literal into bytes: two hex
// digits, eight bits or three octal digits a byte, padded on the left.
func decodeBinary(digits string, base byte) ([]byte, bool) {
	per, bits := 2, 16
	switch base {
	case 'b':
		per, bits = 8, 2
	case 'o':
		per, bits = 3, 8
	}
	if pad := len(digits) % per; pad != 0 {
		digits = strings.Repeat("0", per-pad) + digits
	}
	out := make([]byte, 0, len(digits)/per)
	for i := 0; i < len(digits); i += per {
		n, err := strconv.ParseUint(digits[i:i+per], bits, 16)
		if err != nil || n > 255 {
			return nil, false
		}
		out = append(out, byte(n))
	}
	return out, true
}

// classify says what a bare word is: a keyword, a number, a size, a
// duration, a date, or else a string. json reports whether JSON has it.
func classify(w string) (v Value, json bool) {
	switch w {
	case "true", "false":
		return BoolValue(w == "true"), true
	case "null":
		return NullValue(), true
	case "inf", "+inf", "infinity", "+infinity":
		return FloatValue(math.Inf(1)), false
	case "-inf", "-infinity":
		return FloatValue(math.Inf(-1)), false
	case "NaN", "-NaN":
		return FloatValue(math.NaN()), false
	}
	if v, ok := parseNumber(w); ok {
		return v, jsonNumber(w)
	}
	if v, ok := parseUnit(w); ok {
		return v, false
	}
	if t, ok := parseDate(w); ok {
		return DateValue(t), false
	}
	return StringValue(w), false
}

// parseNumber reads an int (42, -7, 1_000, 0x2a, 0o17, 0b101) or a
// float (1.5, .5, 5., 1e3).
func parseNumber(w string) (Value, bool) {
	if len(w) > 2 && w[0] == '0' {
		base := map[byte]int{'x': 16, 'o': 8, 'b': 2}[w[1]]
		if base != 0 {
			n, err := strconv.ParseInt(strings.ReplaceAll(w[2:], "_", ""), base, 64)
			return IntValue(n), err == nil
		}
	}
	if !numeric(w) {
		return Value{}, false
	}
	plain := strings.ReplaceAll(w, "_", "")
	if n, err := strconv.ParseInt(plain, 10, 64); err == nil {
		return IntValue(n), true
	}
	f, err := strconv.ParseFloat(plain, 64)
	if err != nil && !isRange(err) {
		return Value{}, false
	}
	return FloatValue(f), true
}

func isRange(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

// numeric reports whether w is written with a number's characters and
// has a digit: a sign, digits, underscores between them, a point and an
// exponent.
func numeric(w string) bool {
	digit := false
	for i := 0; i < len(w); i++ {
		c := w[i]
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case c == '_' && i > 0 && i < len(w)-1:
		case c == '.':
		case c == 'e' || c == 'E':
			if !digit {
				return false
			}
		case c == '+' || c == '-':
			if i > 0 && w[i-1] != 'e' && w[i-1] != 'E' {
				return false
			}
		default:
			return false
		}
	}
	return digit
}

// jsonNumber reports whether JSON writes a number this way.
func jsonNumber(w string) bool {
	w = strings.TrimPrefix(w, "-")
	if w == "" || w[0] < '0' || w[0] > '9' || strings.Contains(w, "_") {
		return false
	}
	if len(w) > 1 && w[0] == '0' && w[1] != '.' && w[1] != 'e' && w[1] != 'E' {
		return false
	}
	return !strings.HasSuffix(w, ".") && !strings.Contains(w, ".e") && !strings.Contains(w, ".E")
}

// sizeUnits are the file size units, matched ignoring case.
var sizeUnits = map[string]float64{
	"b": 1, "kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12, "pb": 1e15, "eb": 1e18,
	"kib": 1 << 10, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40, "pib": 1 << 50, "eib": 1 << 60,
}

// durationUnits are the duration units, in nanoseconds.
var durationUnits = map[string]float64{
	"ns": 1, "us": 1e3, "µs": 1e3, "ms": 1e6, "sec": 1e9, "min": 60e9, "hr": 3600e9, "day": 86400e9, "wk": 7 * 86400e9,
}

// parseUnit reads a file size (1.5kb) or a duration (90sec): a number
// and a unit. Fractions of a byte or a nanosecond are dropped, as
// nushell drops them.
func parseUnit(w string) (Value, bool) {
	i := len(w)
	for i > 0 && (w[i-1] >= 'a' && w[i-1] <= 'z' || w[i-1] >= 'A' && w[i-1] <= 'Z' || w[i-1] >= 0x80) {
		i--
	}
	num, unit := w[:i], w[i:]
	if num == "" || unit == "" || !numeric(num) || strings.ContainsAny(num, "eE") {
		return Value{}, false
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(num, "_", ""), 64)
	if err != nil {
		return Value{}, false
	}
	if m, ok := durationUnits[unit]; ok {
		n, ok := toInt(f * m)
		return Value{Kind: Duration, Int: n}, ok
	}
	if m, ok := sizeUnits[strings.ToLower(unit)]; ok {
		n, ok := toInt(f * m)
		return Value{Kind: Filesize, Int: n}, ok
	}
	return Value{}, false
}

// toInt truncates f, reporting false when it doesn't fit an int64.
func toInt(f float64) (int64, bool) {
	if math.IsNaN(f) || f >= math.MaxInt64 || f < math.MinInt64 {
		return 0, false
	}
	return int64(f), true
}

// dateLayouts are the forms of date nushell reads; those without an
// offset are in UTC, as nushell reads them.
var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04",
	"2006-01-02",
}

func parseDate(w string) (time.Time, bool) {
	if len(w) < 10 || w[4] != '-' || w[0] < '0' || w[0] > '9' {
		return time.Time{}, false
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, w); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
