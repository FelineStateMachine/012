package sheet

import (
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/numfmt"
)

// Display renders v under format f for a cell width columns wide, as the
// grid shows it: the text without padding, and where it goes. Numbers go
// right, text left, booleans and errors center. A formatted number that
// doesn't fit in width-1 columns becomes a run of #, as in Sheets;
// Automatic first drops decimals and falls back to scientific notation.
// Accounting returns AlignFill: exactly width columns, $ at the left.
func Display(v Value, f Format, width int) (string, Align) {
	return DisplayIn(v, f, width, locale.Canonical)
}

// DisplayIn is Display as shown in loc: its separators, currency and
// date order (see Format.CodeIn).
func DisplayIn(v Value, f Format, width int, loc *locale.Locale) (string, Align) {
	inner := max(width-1, 0)
	switch v.Kind {
	case Empty:
		return "", AlignLeft
	case Text:
		return v.Str, AlignLeft
	case Bool, Error:
		return v.String(), AlignCenter
	}
	switch f.Kind {
	case FmtAuto:
		return numfmt.GeneralFitIn(v.Num, inner, loc), AlignRight
	case FmtText:
		return numfmt.GeneralIn(v.Num, loc), AlignLeft
	case FmtAccounting:
		if s, ok := numfmt.AccountingIn(v.Num, f.Decimals, width, loc); ok {
			return s, AlignFill
		}
		return strings.Repeat("#", inner), AlignRight
	}
	s := numfmt.FormatIn(v.Num, f.CodeIn(loc), loc)
	if utf8.RuneCountInString(s) > inner {
		s = strings.Repeat("#", inner)
	}
	return s, AlignRight
}

// FormatPattern renders v with a number format pattern, as TEXT() does.
func FormatPattern(v float64, pat string) string { return numfmt.Format(v, pat) }

// FormatText renders v under f with no width limit, e.g. for TEXT() or
// copying out of the grid.
func FormatText(v Value, f Format) string { return FormatTextIn(v, f, locale.Canonical) }

// FormatTextIn is FormatText as shown in loc, e.g. for a CSV file
// written in loc.
func FormatTextIn(v Value, f Format, loc *locale.Locale) string {
	if v.Kind != Number {
		return text(v)
	}
	switch f.Kind {
	case FmtAuto, FmtText:
		return numfmt.GeneralIn(v.Num, loc)
	}
	return numfmt.FormatIn(v.Num, f.CodeIn(loc), loc)
}

// FormatValue renders v in width columns with one column of padding, the
// way the grid shows it in Automatic format: numbers right-aligned,
// booleans and errors centered. Text is not handled here because it can
// overflow into neighboring cells.
func FormatValue(v Value, width int) string {
	switch v.Kind {
	case Number:
		s, _ := Display(v, Format{}, width)
		return padLeft(s, width-1) + " "
	case Bool, Error:
		return centerPad(v.String(), width)
	}
	return ""
}

// FormatNumber formats a number in the General format within width
// columns, for use outside the grid (e.g. the status line).
func FormatNumber(v float64, width int) string {
	return numfmt.GeneralFit(v, width)
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func centerPad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	pad := width - len(s)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}
