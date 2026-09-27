package sheet

import (
	"strings"
	"sync/atomic"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
)

// A workbook's locale is how its entries are typed and its numbers,
// dates and formulas shown, as Sheets' File > Settings > Locale. Cells
// keep their entries in en-US's form whatever the locale (see
// value.Canonicalize and formula.Delocalize), so values never depend on
// it: switching the locale redraws the sheet and changes nothing else,
// and a file reads the same everywhere. Functions compute in en-US's
// form too (VALUE("1,5") is an error in every locale), so formulas give
// the same results everywhere.

// settings are the workbook's own settings, saved in its file: Sheets'
// File > Settings.
type settings struct {
	decimal bool           // decimal arithmetic, see decimal.go
	locale  *locale.Locale // nil follows the default
}

// defaultLocale is the locale of workbooks that don't name one: the
// config's locale, set with SetDefaultLocale.
var defaultLocale atomic.Pointer[locale.Locale]

// SetDefaultLocale sets the locale of workbooks that don't name one of
// their own; nil is en-US.
func SetDefaultLocale(l *locale.Locale) { defaultLocale.Store(l) }

// DefaultLocale is the locale of workbooks that don't name one.
func DefaultLocale() *locale.Locale { return defaultLocale.Load().Or() }

// Locale is the workbook's locale: its own, or the default.
func (w *Workbook) Locale() *locale.Locale {
	if w.locale != nil {
		return w.locale
	}
	return DefaultLocale()
}

// LocaleTag is the tag of the locale the workbook names, or "" when it
// follows the default.
func (w *Workbook) LocaleTag() string {
	if w.locale == nil {
		return ""
	}
	return w.locale.Tag
}

// SetLocale sets the workbook's locale by its tag, "" to follow the
// default, as one undo step. Values don't change, only how they're
// typed and shown. It reports false for a tag it doesn't know.
func (w *Workbook) SetLocale(tag string) bool {
	var l *locale.Locale
	if tag != "" {
		var ok bool
		if l, ok = locale.Lookup(tag); !ok {
			return false
		}
	}
	if l == w.locale {
		return true
	}
	label := "follow the default locale"
	if l != nil {
		label = "set the locale to " + l.Tag
	}
	w.change(w.sheets[w.Active()], label, Rect{}, func() {
		w.recordSettings()
		w.locale = l
	})
	return true
}

// Locale is the sheet's workbook's.
func (s *Sheet) Locale() *locale.Locale { return s.wb.Locale() }

// CanonicalEntry is an entry typed in loc as the cell stores it: numbers,
// dates and times in en-US's form, formulas with its separators, text as
// typed, behind a ' when it would read as something else stored (1.5,
// typed as text in fr-FR).
func CanonicalEntry(typed string, loc *locale.Locale) string {
	if loc.IsCanonical() || typed == "" || strings.HasPrefix(typed, "'") {
		return typed
	}
	if c, ok := value.Canonicalize(typed, loc); ok {
		return c
	}
	switch strings.ToUpper(typed) {
	case "TRUE", "FALSE":
		return typed
	}
	if strings.HasPrefix(typed, "=") {
		return formula.Delocalize(typed, loc)
	}
	if strings.HasPrefix(typed, "+") || strings.HasPrefix(typed, "-") {
		if f := formula.Delocalize(typed, loc); IsFormulaEntry(f) {
			return f
		}
	}
	if storedAsOther(typed) {
		return "'" + typed
	}
	return typed
}

// LocalEntry is the inverse of CanonicalEntry: a cell's entry as typed
// in loc, as the formula bar shows it and editing starts from.
func LocalEntry(input string, loc *locale.Locale) string {
	if loc.IsCanonical() || input == "" {
		return input
	}
	if rest, ok := strings.CutPrefix(input, "'"); ok {
		// The ' CanonicalEntry adds, when typing rest in loc is text.
		if storedAsOther(rest) && CanonicalEntry(rest, loc) == input {
			return rest
		}
		return input
	}
	if _, _, ok := ParseValue(input); ok {
		return value.Localize(input, loc)
	}
	if IsFormulaEntry(input) {
		return formula.Localize(input, loc)
	}
	return input
}

// CanonicalArg is the argument of a rule or filter condition typed in
// loc as it's stored: a number or date in en-US's form, a formula (after
// =) in its syntax, text as typed.
func CanonicalArg(arg string, loc *locale.Locale) string {
	if loc.IsCanonical() {
		return arg
	}
	if c, ok := value.Canonicalize(arg, loc); ok {
		return c
	}
	if strings.HasPrefix(arg, "=") {
		return formula.Delocalize(arg, loc)
	}
	return arg
}

// LocalArg is the inverse of CanonicalArg, for editing an argument again.
func LocalArg(arg string, loc *locale.Locale) string {
	if loc.IsCanonical() {
		return arg
	}
	if _, _, ok := ParseValue(arg); ok {
		return value.Localize(arg, loc)
	}
	if strings.HasPrefix(arg, "=") {
		return formula.Localize(arg, loc)
	}
	return arg
}

// LocalizeError writes a formula's parse error, alone or wrapped, as the
// formula was typed in loc: "Expected ; or ) in ROUND" in de-DE.
func LocalizeError(err error, loc *locale.Locale) error { return formula.LocalizeError(err, loc) }

// LocalText is the cell's value as its format displays it in the
// sheet's locale, with no width limit: what text conditions test.
func (s *Sheet) LocalText(a Addr) string {
	if !s.cells.filledAt(a) {
		return ""
	}
	return FormatTextIn(s.cells.value(a), s.DisplayFormat(a), s.Locale())
}

// localLabel is the cell at a, shown as shown in en-US, as its
// locale shows it.
func (s *Sheet) localLabel(a Addr, shown string) string {
	if shown == "" || s.Locale().IsCanonical() {
		return shown
	}
	return s.LocalText(a)
}

// CondArg is a condition's value typed in loc as it's kept: as typed
// for a condition on text, as CanonicalArg for the rest.
func CondArg(op CondOp, typed string, loc *locale.Locale) string {
	if op.OnText() {
		return typed
	}
	return CanonicalArg(typed, loc)
}

// LocalCondArg is the inverse of CondArg, for editing a value again.
func LocalCondArg(op CondOp, arg string, loc *locale.Locale) string {
	if op.OnText() {
		return arg
	}
	return LocalArg(arg, loc)
}

// textIn is v as General shows it in loc: 1,5 in de-DE.
func textIn(v Value, loc *locale.Locale) string {
	if v.Kind == Number {
		return numfmt.GeneralIn(v.Num, loc)
	}
	return v.String()
}

// storedAsOther reports whether an entry stored as s isn't text: a
// number, date, formula or boolean.
func storedAsOther(s string) bool {
	if _, _, ok := ParseValue(s); ok {
		return true
	}
	switch strings.ToUpper(s) {
	case "TRUE", "FALSE":
		return true
	}
	return IsFormulaEntry(s)
}
