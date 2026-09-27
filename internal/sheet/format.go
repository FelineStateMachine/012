package sheet

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
)

var kindNames = [...]string{"auto", "text", "number", "percent", "scientific", "accounting",
	"financial", "currency", "date", "time", "datetime", "duration", "custom"}

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

// label names the kind in undo labels: "format B3 as date time".
func (k FormatKind) label() string {
	switch k {
	case FmtAuto:
		return "automatic"
	case FmtText:
		return "plain text"
	case FmtDateTime:
		return "date time"
	case FmtCustom:
		return "custom"
	}
	return k.String()
}

// hasDecimals reports whether the kind takes a number of decimal places.
func (k FormatKind) hasDecimals() bool {
	switch k {
	case FmtNumber, FmtPercent, FmtScientific, FmtAccounting, FmtFinancial, FmtCurrency:
		return true
	}
	return false
}

// isTime reports whether the kind shows a date, time or duration.
func (k FormatKind) isTime() bool {
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
// kinds.
func Preset(k FormatKind) Format {
	if k.hasDecimals() {
		return Format{Kind: k, Decimals: 2}
	}
	return Format{Kind: k}
}

// IsZero reports whether f is Automatic.
func (f Format) IsZero() bool { return f == Format{} }

// pattern returns the Sheets-style pattern f renders with, or "" for
// Automatic and Plain text.
func (f Format) pattern() string {
	dec := ""
	if f.Decimals > 0 {
		dec = "." + strings.Repeat("0", min(f.Decimals, MaxDecimals))
	}
	if f.Pattern != "" && (f.Kind == FmtCustom || f.Kind.isTime()) {
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
	}
	return ""
}

// WithDecimals returns f showing delta more (or fewer) decimal places, as
// Sheets' Increase and Decrease decimal places do. v is the value shown,
// used when f is Automatic to start from the decimals currently visible.
// Formats without decimals (dates, plain text) are returned unchanged.
func (f Format) WithDecimals(delta int, v float64) Format {
	switch {
	case f.Kind.hasDecimals():
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

// Align is a cell's horizontal alignment.
type Align uint8

const (
	AlignAuto   Align = iota // numbers right, text left, booleans and errors centered
	AlignLeft                //
	AlignCenter              //
	AlignRight               //
	// AlignFill is only returned by Display: the text spans the cell
	// exactly and isn't padded (Accounting's $ at the left edge).
	AlignFill
)

var alignNames = [...]string{"", "left", "center", "right"}

// String returns the alignment's name as stored in files.
func (a Align) String() string {
	if int(a) < len(alignNames) {
		return alignNames[a]
	}
	return ""
}

// ParseAlign is the inverse of Align.String.
func ParseAlign(s string) (Align, bool) {
	for i, n := range alignNames {
		if n == s {
			return Align(i), true
		}
	}
	return AlignAuto, false
}

// Style is a cell's text style. It is a plain value so cells can be
// copied freely.
type Style struct {
	Bold, Italic, Underline, Strikethrough bool
	Align                                  Align
}

// IsZero reports whether s is the default style.
func (s Style) IsZero() bool { return s == Style{} }
