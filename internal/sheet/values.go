package sheet

import (
	"github.com/FelineStateMachine/012/internal/functions"
	"github.com/FelineStateMachine/012/internal/value"
)

// Values, formats and the parsing of typed entries live in
// internal/value, below the function library and the engine; functions
// in internal/functions. The engine's API keeps their names, as it does
// the formula package's, so callers see one package.

type (
	// Kind is the type of a computed cell value.
	Kind = value.Kind
	// Value is the computed contents of a cell.
	Value = value.Value
	// FormatKind is a number format from Sheets' Format > Number menu.
	FormatKind = value.FormatKind
	// Format is a cell's number format.
	Format = value.Format
	// FuncDef describes a spreadsheet function.
	FuncDef = functions.FuncDef
)

// Kinds of values.
const (
	Empty  = value.Empty
	Number = value.Number
	Text   = value.Text
	Bool   = value.Bool
	Error  = value.Error
)

// Error values, using Google Sheets codes.
var (
	ErrDiv0  = value.ErrDiv0
	ErrValue = value.ErrValue
	ErrName  = value.ErrName
	ErrNA    = value.ErrNA
	ErrNum   = value.ErrNum
	ErrRef   = value.ErrRef // also circular references
)

// Number formats.
const (
	FmtAuto       = value.FmtAuto
	FmtText       = value.FmtText
	FmtNumber     = value.FmtNumber
	FmtPercent    = value.FmtPercent
	FmtScientific = value.FmtScientific
	FmtAccounting = value.FmtAccounting
	FmtFinancial  = value.FmtFinancial
	FmtCurrency   = value.FmtCurrency
	FmtDate       = value.FmtDate
	FmtTime       = value.FmtTime
	FmtDateTime   = value.FmtDateTime
	FmtDuration   = value.FmtDuration
	FmtCustom     = value.FmtCustom

	// MaxDecimals caps Increase decimal places.
	MaxDecimals = value.MaxDecimals
)

// Preset returns kind with Sheets' default decimals: two for the number
// kinds.
func Preset(k FormatKind) Format { return value.Preset(k) }

// ParseFormatKind is the inverse of FormatKind.String.
func ParseFormatKind(s string) (FormatKind, bool) { return value.ParseFormatKind(s) }

// ParseNumber recognizes numbers the way Google Sheets does on entry.
func ParseNumber(s string) (float64, bool) { return value.ParseNumber(s) }

// ParseValue recognizes everything Sheets turns into a number on entry:
// numbers, currency, percentages, dates and times, with the format
// Sheets applies.
func ParseValue(s string) (float64, Format, bool) { return value.ParseValue(s) }

// LookupFunc finds a function by name or alias; callers pass upper case.
func LookupFunc(name string) (*FuncDef, bool) { return functions.LookupFunc(name) }

// Funcs returns every function, sorted by name.
func Funcs() []*FuncDef { return functions.Funcs() }

func num(v float64) Value    { return value.Num(v) }
func boolean(b bool) Value   { return value.Boolean(b) }
func text(v Value) string    { return value.AsText(v) }
func compare(l, r Value) int { return value.Compare(l, r) }

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }
