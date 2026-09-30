package headless

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Value is a cell's value as agents write and read it, in JSON (see
// docs/reference/json.md#values): a number, a string (text, kept as
// text however it looks), true or false, null for blank, or an object
// naming its type, so money, percentages, dates, times, durations and
// sizes keep their meaning both ways:
//
//	{"currency": 3.5}                  $3.50; "symbol": "€", "decimals": 0
//	{"percent": 0.12}                  12%
//	{"date": "2026-09-29"}             a date, or "2026-09-29T14:30:00" with a time
//	{"time": "14:30:00"}               a time of day
//	{"duration": "90min"}              elapsed time, as nushell writes it, or seconds
//	{"size": 1500}                     bytes, or "1.5kb" as nushell writes it
//	{"number": 1234.5, "decimals": 2}  a number with a format
//	{"text": "00123"}                  text
//
// Any object may add "format", a number format code ("#,##0.00",
// "yyyy-mm-dd") shown in its type's place. Written as NUON, sizes,
// durations and dates may be nushell's own values.
type Value json.RawMessage

// MarshalJSON writes v as it is, null when empty.
func (v Value) MarshalJSON() ([]byte, error) {
	if len(v) == 0 {
		return []byte("null"), nil
	}
	return v, nil
}

// UnmarshalJSON keeps the JSON as it is: null is a Value too, one that
// clears a cell.
func (v *Value) UnmarshalJSON(b []byte) error {
	*v = append((*v)[:0], b...)
	return nil
}

// typedTypes are the types a Value's object may name.
var typedTypes = []string{"currency", "percent", "date", "time", "duration", "size", "number", "text"}

// typed is what writing a Value types into a cell: the entry, and the
// format it sets when formatted.
type typed struct {
	input     string
	format    sheet.Format
	formatted bool
}

// parseValue reads a Value, as JSON or NUON.
func parseValue(v Value) (typed, error) {
	n, err := nuon.Parse(v)
	if err != nil {
		return typed{}, fmt.Errorf("%s isn't a value: %v", v, err)
	}
	return typedOf(n)
}

// typedOf is what writing n types.
func typedOf(n nuon.Value) (typed, error) {
	switch n.Kind {
	case nuon.Null:
		return typed{}, nil
	case nuon.Bool:
		return typed{input: strings.ToUpper(fmt.Sprint(n.Bool))}, nil
	case nuon.Int, nuon.Float:
		f, _ := number(n)
		return typed{input: fileio.NumInput(f)}, nil
	case nuon.String:
		return typed{input: fileio.TextInput(n.Str)}, nil
	case nuon.Filesize, nuon.Duration, nuon.Date:
		c := fileio.NUONCell(n)
		return typed{input: fileio.NumInput(c.V.Num), format: c.F, formatted: true}, nil
	case nuon.Record:
		return typedRecord(n.Fields)
	}
	return typed{}, fmt.Errorf("%s isn't a cell's value: write a number, text, true, false, null or an object naming its type (%s)", n, strings.Join(typedTypes, ", "))
}

// number is n as a float64, if it's a number.
func number(n nuon.Value) (float64, bool) {
	switch n.Kind {
	case nuon.Int:
		return float64(n.Int), true
	case nuon.Float:
		return n.Float, true
	}
	return 0, false
}

// record is a Value's object: its type, the value under it, and the
// other fields.
type record struct {
	kind   string
	val    nuon.Value
	symbol string
	dec    *int
	code   string
}

func readRecord(fs []nuon.Field) (record, error) {
	var r record
	for _, f := range fs {
		switch {
		case f.Key == "symbol" && f.Value.Kind == nuon.String:
			r.symbol = f.Value.Str
		case f.Key == "decimals" && f.Value.Kind == nuon.Int && f.Value.Int >= 0 && f.Value.Int <= sheet.MaxDecimals:
			d := int(f.Value.Int)
			r.dec = &d
		case f.Key == "format" && f.Value.Kind == nuon.String:
			r.code = f.Value.Str
		case r.kind == "" && oneOf(f.Key, typedTypes):
			r.kind, r.val = f.Key, f.Value
		default:
			return r, fmt.Errorf("%q isn't a field of a value: name one type (%s), with symbol, decimals or format", f.Key, strings.Join(typedTypes, ", "))
		}
	}
	if r.kind == "" {
		return r, fmt.Errorf("a value's object names its type: %s", strings.Join(typedTypes, ", "))
	}
	return r, nil
}

func oneOf(s string, list []string) bool {
	for _, x := range list {
		if s == x {
			return true
		}
	}
	return false
}

// typedRecord is what writing a Value's object types.
func typedRecord(fs []nuon.Field) (typed, error) {
	r, err := readRecord(fs)
	if err != nil {
		return typed{}, err
	}
	t, err := r.typed()
	if err != nil {
		return typed{}, fmt.Errorf("%s: %w", r.kind, err)
	}
	if r.dec != nil && t.format.Kind.HasDecimals() {
		t.format.Decimals = *r.dec
	}
	if r.code != "" {
		t.format, t.formatted = FormatOfCode(r.code), true
	}
	return t, nil
}

func (r record) typed() (typed, error) {
	if r.kind == "text" {
		if r.val.Kind != nuon.String {
			return typed{}, fmt.Errorf("text is a string")
		}
		return typed{input: fileio.TextInput(r.val.Str)}, nil
	}
	v, f, err := r.serial()
	if err != nil {
		return typed{}, err
	}
	return typed{input: fileio.NumInput(v), format: f, formatted: true}, nil
}

// serial is the number r stands for and the format it's shown in.
func (r record) serial() (float64, sheet.Format, error) {
	switch r.kind {
	case "date", "time":
		return r.clock()
	case "duration", "size":
		return r.unit()
	}
	n, ok := number(r.val)
	if !ok {
		return 0, sheet.Format{}, fmt.Errorf("%s is a number", r.val)
	}
	switch r.kind {
	case "currency":
		dec := 2
		if r.dec != nil {
			dec = *r.dec
		}
		return n, currencyFormat(r.symbol, dec), nil
	case "percent":
		return n, sheet.Format{Kind: sheet.FmtPercent, Decimals: percentDecimals(n)}, nil
	}
	if r.dec == nil {
		return n, sheet.Format{}, nil
	}
	return n, sheet.Format{Kind: sheet.FmtNumber}, nil
}

// currencyFormat shows an amount in symbol ($ when ""), with dec
// decimals: 012's Currency for dollars, a pattern for the others.
func currencyFormat(symbol string, dec int) sheet.Format {
	if symbol == "" || symbol == "$" {
		return sheet.Format{Kind: sheet.FmtCurrency, Decimals: dec}
	}
	return sheet.Format{Kind: sheet.FmtCustom, Pattern: currencyPattern(symbol, dec)}
}

func currencyPattern(symbol string, dec int) string {
	p := `"` + strings.ReplaceAll(symbol, `"`, "") + `"#,##0`
	if dec > 0 {
		p += "." + strings.Repeat("0", dec)
	}
	return p
}

// percentDecimals are the decimals typing n as a percentage shows: none
// for a whole percentage (12%), two otherwise (12.50%).
func percentDecimals(n float64) int {
	p := n * 100
	if math.Abs(p-math.Round(p)) < 1e-9 {
		return 0
	}
	return 2
}

// clock reads a date, a date and time, or a time of day.
func (r record) clock() (float64, sheet.Format, error) {
	if r.val.Kind == nuon.Date {
		c := fileio.NUONCell(r.val)
		return c.V.Num, c.F, nil
	}
	if r.val.Kind != nuon.String {
		return 0, sheet.Format{}, fmt.Errorf("%s is a string: 2026-09-29, 2026-09-29T14:30:00, 14:30", r.val)
	}
	s := strings.TrimSpace(r.val.Str)
	if r.kind == "time" {
		v, f, ok := sheet.ParseValue(s)
		if !ok || f.Kind != sheet.FmtTime {
			return 0, sheet.Format{}, fmt.Errorf("%q isn't a time of day: 14:30, 14:30:05, 2:30 PM", s)
		}
		return v, sheet.Preset(sheet.FmtTime), nil
	}
	t, withTime, ok := parseISO(s)
	switch {
	case !ok:
		return 0, sheet.Format{}, fmt.Errorf("%q isn't a date: 2026-09-29, 2026-09-29T14:30:00", s)
	case t.Location() != time.UTC:
		return fileio.NUONCell(nuon.DateValue(t)).V.Num, sheet.Preset(sheet.FmtDateTime), nil
	case withTime:
		return numfmt.SerialOf(t), sheet.Preset(sheet.FmtDateTime), nil
	}
	return numfmt.SerialOf(t), sheet.Preset(sheet.FmtDate), nil
}

// isoLayouts are the dates a Value's date takes, without an offset: the
// time on the sheet's clock.
var isoLayouts = []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04"}

// parseISO reads a date in ISO 8601: a day, or a day and a time, with
// an offset (in a location of its own, for the sheet's time zone to
// convert) or without one (in UTC, the time on the sheet's clock).
func parseISO(s string) (t time.Time, withTime, ok bool) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, false, true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		_, off := t.Zone()
		return t.In(time.FixedZone("", off)), true, true
	}
	for _, l := range isoLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true, true
		}
	}
	return time.Time{}, false, false
}

// unit reads a duration or a size: a number (seconds, bytes) or
// nushell's literal (90min, 1.5kb).
func (r record) unit() (float64, sheet.Format, error) {
	v := r.val
	if v.Kind == nuon.String {
		p, err := nuon.Parse([]byte(v.Str))
		if err != nil {
			return 0, sheet.Format{}, fmt.Errorf("%q isn't nushell's %s: %s", v.Str, r.kind, unitExample(r.kind))
		}
		v = p
	}
	if n, ok := number(v); ok {
		if r.kind == "duration" {
			v = nuon.DurationValue(time.Duration(math.Round(n * 1e9)))
		} else {
			v = nuon.FilesizeValue(int64(math.Round(n)))
		}
	}
	if want := map[string]nuon.Kind{"duration": nuon.Duration, "size": nuon.Filesize}[r.kind]; v.Kind != want {
		return 0, sheet.Format{}, fmt.Errorf("%s isn't a %s: %s", v, r.kind, unitExample(r.kind))
	}
	c := fileio.NUONCell(v)
	return c.V.Num, c.F, nil
}

func unitExample(kind string) string {
	if kind == "duration" {
		return "90min, 1.5hr, 2day, or a number of seconds"
	}
	return "1.5kb, 3MiB, or a number of bytes"
}

// FormatOfCode is the format a number format code stands for: one of
// 012's formats when it's that format's code ("$"#,##0.00 is Currency),
// a date or time with that pattern, or a custom format.
func FormatOfCode(code string) sheet.Format {
	if f, ok := presetCodes()[code]; ok {
		return f
	}
	if rest, ok := strings.CutPrefix(code, "$"); ok {
		if f, ok := presetCodes()[`"$"`+rest]; ok {
			return f // $#,##0.00, as people write Currency's code
		}
	}
	switch k := fileio.CodeKind(code); k {
	case sheet.FmtDate, sheet.FmtTime, sheet.FmtDateTime, sheet.FmtDuration:
		return sheet.Format{Kind: k, Pattern: code}
	}
	return sheet.Format{Kind: sheet.FmtCustom, Pattern: code}
}

// presetCodes are 012's formats by their codes.
var presetCodes = sync.OnceValue(func() map[string]sheet.Format {
	m := map[string]sheet.Format{}
	for k := sheet.FmtSize; k > sheet.FmtAuto; k-- {
		if !k.HasDecimals() {
			m[sheet.Format{Kind: k}.Code()] = sheet.Format{Kind: k}
			continue
		}
		for d := sheet.MaxDecimals; d >= 0; d-- {
			f := sheet.Format{Kind: k, Decimals: d}
			m[f.Code()] = f
		}
	}
	delete(m, "")
	return m
})
