package value

import (
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/numfmt"
)

// FormatKind is a number format from Sheets' Format > Number menu.
type FormatKind uint8

const (
	FmtAuto       FormatKind = iota // General; formulas may infer a format
	FmtText                         // Plain text: entries stay as typed
	FmtNumber                       // 1,000.12
	FmtPercent                      // 10.12%
	FmtScientific                   // 1.01E+03
	FmtAccounting                   // $ (1,000.12)
	FmtFinancial                    // (1,000.12)
	FmtCurrency                     // $1,000.12
	FmtDate                         // 9/26/2026
	FmtTime                         // 3:59:00 PM
	FmtDateTime                     // 9/26/2026 15:59:00
	FmtDuration                     // 24:01:00
	FmtCustom                       // Pattern, e.g. from Increase decimal places on Automatic
	FmtSize                         // 1.6 kB: a count of bytes, as nushell shows file sizes
)

var kindNames = [...]string{"auto", "text", "number", "percent", "scientific", "accounting",
	"financial", "currency", "date", "time", "datetime", "duration", "custom", "size"}

// String returns the kind's name as stored in files.
func (k FormatKind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "auto"
}

// ParseFormatKind is the inverse of FormatKind.String.
func ParseFormatKind(s string) (FormatKind, bool) {
	for i, n := range kindNames {
		if n == s {
			return FormatKind(i), true
		}
	}
	return FmtAuto, false
}

// HasDecimals reports whether the kind takes a number of decimal places.
func (k FormatKind) HasDecimals() bool {
	switch k {
	case FmtNumber, FmtPercent, FmtScientific, FmtAccounting, FmtFinancial, FmtCurrency, FmtSize:
		return true
	}
	return false
}

// IsTime reports whether the kind shows a date, time or duration.
func (k FormatKind) IsTime() bool {
	return k >= FmtDate && k <= FmtDuration
}

// Format is a cell's number format. It is a plain value so cells can be
// copied freely.
type Format struct {
	Kind     FormatKind
	Decimals int // for Number, Percent, Scientific, Accounting, Financial, Currency
	// Pattern is the pattern of a FmtCustom, or overrides the default
	// pattern of a date or time kind (a date typed as 2026-09-26 keeps
	// that style, as in Sheets).
	Pattern string
}

// MaxDecimals caps Increase decimal places.
const MaxDecimals = 15

// Preset returns kind with Sheets' default decimals: two for the number
// kinds, and one for sizes, as nushell shows them.
func Preset(k FormatKind) Format {
	if k == FmtSize {
		return Format{Kind: k, Decimals: 1}
	}
	if k.HasDecimals() {
		return Format{Kind: k, Decimals: 2}
	}
	return Format{Kind: k}
}

// IsZero reports whether f is Automatic.
func (f Format) IsZero() bool { return f == Format{} }

// Code returns the Sheets-style format pattern f renders with, or "" for
// Automatic and Plain text.
func (f Format) Code() string {
	dec := ""
	if f.Decimals > 0 {
		dec = "." + strings.Repeat("0", min(f.Decimals, MaxDecimals))
	}
	if f.Pattern != "" && (f.Kind == FmtCustom || f.Kind.IsTime()) {
		return f.Pattern
	}
	switch f.Kind {
	case FmtNumber:
		return "#,##0" + dec
	case FmtPercent:
		return "0" + dec + "%"
	case FmtScientific:
		return "0" + dec + "E+00"
	case FmtAccounting:
		return `_("$"* #,##0` + dec + `_);_("$"* \(#,##0` + dec + `\);_("$"* "-"??_);_(@_)`
	case FmtFinancial:
		return "#,##0" + dec + ";(#,##0" + dec + ")"
	case FmtCurrency:
		return `"$"#,##0` + dec
	case FmtDate:
		return "m/d/yyyy"
	case FmtTime:
		return "h:mm:ss am/pm"
	case FmtDateTime:
		return "m/d/yyyy h:mm:ss"
	case FmtDuration:
		return "[h]:mm:ss"
	case FmtSize:
		return SizeCode(f.Decimals)
	}
	return ""
}

// SizeCode is the custom number format closest to the Size format with
// dec decimals, written with conditions as Sheets and Excel take them:
// bytes below 1000, then kB and MB, e.g.
// [<1000]0" B";[<1000000]0.0," kB";0.0,," MB". Size itself goes on to
// GB, TB, PB and EB; files keep this code for other programs.
func SizeCode(dec int) string {
	d := ""
	if dec > 0 {
		d = "." + strings.Repeat("0", min(dec, MaxDecimals))
	}
	return `[<1000]0" B";[<1000000]0` + d + `," kB";0` + d + `,," MB"`
}

// WithDecimals returns f showing delta more (or fewer) decimal places, as
// Sheets' Increase and Decrease decimal places do. v is the value shown,
// used when f is Automatic to start from the decimals currently visible.
// Formats without decimals (dates, plain text) are returned unchanged.
func (f Format) WithDecimals(delta int, v float64) Format {
	switch {
	case f.Kind.HasDecimals():
		f.Decimals = clampInt(f.Decimals+delta, 0, MaxDecimals)
	case f.Kind == FmtAuto:
		d := clampInt(visibleDecimals(v)+delta, 0, MaxDecimals)
		f = Format{Kind: FmtCustom, Pattern: "0"}
		if d > 0 {
			f.Pattern += "." + strings.Repeat("0", d)
		}
	case f.Kind == FmtCustom:
		f.Pattern = numfmt.AdjustDecimals(f.Pattern, delta)
	}
	return f
}

// visibleDecimals is how many decimals Automatic shows for v, up to ten.
func visibleDecimals(v float64) int {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	_, frac, ok := strings.Cut(s, ".")
	if !ok {
		return 0
	}
	return min(len(frac), 10)
}

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }
