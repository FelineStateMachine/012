package headless

import (
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Reading a cell as a Value: its type from its format, so what reads
// it back writes the same value and format. A number in a format its
// type doesn't show by itself carries decimals or the format's code.

// CellValue is the value of the cell at a, typed by the format it's
// shown in.
func CellValue(s *sheet.Sheet, a sheet.Addr) Value {
	return valueOf(s.Value(a), s.DisplayFormat(a))
}

// valueOf is v, shown in f, as a Value.
func valueOf(v sheet.Value, f sheet.Format) Value {
	switch v.Kind {
	case sheet.Empty:
		return Value("null")
	case sheet.Bool:
		return Value(strconv.FormatBool(v.Num != 0))
	case sheet.Text:
		return Value(nuon.AppendJSON(nil, nuon.StringValue(v.Str)))
	case sheet.Number:
		return numberOf(v.Num, f)
	}
	return Value(nuon.AppendJSON(nil, nuon.StringValue(v.String()))) // an error, as it shows
}

// plainJSON is n as a JSON number: an int when whole.
func plainJSON(n float64) nuon.Value {
	if n == math.Trunc(n) && math.Abs(n) < 1<<53 {
		return nuon.IntValue(int64(n))
	}
	return nuon.FloatValue(n)
}

// numberOf is n shown in f: a plain number in the Automatic format, an
// object naming its type otherwise.
func numberOf(n float64, f sheet.Format) Value {
	kind, val := typeOf(n, f)
	if kind == "" {
		return Value(nuon.AppendJSON(nil, plainJSON(n)))
	}
	fs := []nuon.Field{{Key: kind, Value: val}}
	if kind == "currency" {
		if sym := currencySymbol(f); sym != "$" {
			fs = append(fs, nuon.Field{Key: "symbol", Value: nuon.StringValue(sym)})
		}
	}
	fs = append(fs, formatFields(fs, f)...)
	return Value(nuon.AppendJSON(nil, nuon.RecordValue(fs...)))
}

// formatFields are the fields a Value of fields needs to be shown in f:
// none when its type shows it so, decimals when they're all it takes,
// and the format's code otherwise.
func formatFields(fs []nuon.Field, f sheet.Format) []nuon.Field {
	tries := [][]nuon.Field{nil}
	if f.Kind.HasDecimals() || currencySymbol(f) != "" {
		tries = append(tries, []nuon.Field{{Key: "decimals", Value: nuon.IntValue(int64(decimalsOf(f)))}})
	}
	for _, extra := range tries {
		if t, err := typedRecord(append(fs[:len(fs):len(fs)], extra...)); err == nil && t.format == f {
			return extra
		}
	}
	return []nuon.Field{{Key: "format", Value: nuon.StringValue(f.Code())}}
}

// decimalsOf are the decimals f shows: its own, or a currency
// pattern's.
func decimalsOf(f sheet.Format) int {
	if m := currencyPatternRE.FindStringSubmatch(f.Pattern); f.Kind == sheet.FmtCustom && m != nil {
		return max(len(m[3])-1, 0)
	}
	return f.Decimals
}

// typeOf is the type a number shown in f is written as, and its value:
// "" for a plain number.
func typeOf(n float64, f sheet.Format) (string, nuon.Value) {
	kind := f.Kind
	if kind == sheet.FmtCustom {
		if currencySymbol(f) != "" {
			return "currency", plainJSON(n)
		}
		kind = fileio.CodeKind(f.Pattern)
	}
	switch kind {
	case sheet.FmtAuto, sheet.FmtText:
		return "", nuon.Value{}
	case sheet.FmtCurrency, sheet.FmtAccounting:
		return "currency", plainJSON(n)
	case sheet.FmtPercent:
		return "percent", plainJSON(n)
	case sheet.FmtDate, sheet.FmtDateTime:
		return "date", nuon.StringValue(isoDate(n))
	case sheet.FmtTime:
		if n >= 0 && n < 1 {
			return "time", nuon.StringValue(timeOfDay(n))
		}
		return "date", nuon.StringValue(isoDate(n))
	case sheet.FmtDuration:
		return "duration", nuon.StringValue(durationLiteral(time.Duration(math.Round(n * 86400e9))))
	case sheet.FmtSize:
		return "size", plainJSON(n)
	}
	return "number", plainJSON(n)
}

// currencyPatternRE is a custom format showing a currency: a symbol,
// quoted as currencyPattern writes it or one of the common ones bare,
// before a grouped amount.
var currencyPatternRE = regexp.MustCompile(`^(?:"([^"]+)"|([€£¥₹]))#,##0(\.0+)?$`)

// currencySymbol is the symbol a currency format shows: $ for 012's
// Currency and Accounting, the quoted symbol of a pattern such as
// "€"#,##0.00, "" for other formats.
func currencySymbol(f sheet.Format) string {
	switch f.Kind {
	case sheet.FmtCurrency, sheet.FmtAccounting:
		return "$"
	case sheet.FmtCustom:
		if m := currencyPatternRE.FindStringSubmatch(f.Pattern); m != nil {
			return m[1] + m[2]
		}
	}
	return ""
}

// isoDate is a date serial in ISO 8601, on the sheet's clock: the day
// alone when it's whole, else with the time, to the microsecond.
func isoDate(n float64) string {
	days := math.Floor(n)
	us := math.Round((n - days) * 86400e6)
	if math.Abs(days) > 1e7 {
		days = math.Copysign(1e7, days)
	}
	t := time.Date(1899, 12, 30+int(days), 0, 0, 0, int(us)*1e3, time.UTC)
	if us == 0 && n == days {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02T15:04:05.999999")
}

// timeOfDay is a fraction of a day as a time on the clock: 14:30:00.
func timeOfDay(n float64) string {
	us := math.Round(n * 86400e6)
	return time.Date(2000, 1, 1, 0, 0, 0, int(us)*1e3, time.UTC).Format("15:04:05.999999")
}

// durationUnits are nushell's units of time, largest first.
var durationUnits = []struct {
	name string
	d    time.Duration
}{{"day", 24 * time.Hour}, {"hr", time.Hour}, {"min", time.Minute}, {"sec", time.Second}, {"ms", time.Millisecond}, {"us", time.Microsecond}}

// durationLiteral is d as nushell writes a duration literal, in the
// largest unit that holds it whole: 90min, 1day, 1500ms.
func durationLiteral(d time.Duration) string {
	for _, u := range durationUnits {
		if d%u.d == 0 {
			return strconv.FormatInt(int64(d/u.d), 10) + u.name
		}
	}
	return strconv.FormatInt(int64(d), 10) + "ns"
}
