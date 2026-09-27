package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// File > Settings > Locale: how the workbook's entries are typed and its
// numbers, dates and formulas shown (see sheet/locale.go). Cells keep
// their entries in en-US's form, so the UI translates at its edges: what
// the formula bar and editing show (shownEntry), and what's typed or
// pasted before it's stored (storedEntry).

func init() {
	register(&command{id: "settings.locale", title: "Locale",
		desc: "Type and show numbers, dates, currency and formulas as a country does: 1.234,56 and =ROUND(A1; 2) in Germany",
		run:  func(m *Model) tea.Cmd { m.openLocalePicker(); return nil }})
}

// locale is the workbook's locale.
func (m *Model) locale() *locale.Locale { return m.sheet.Locale() }

// shownEntry is a cell's entry as typed in the workbook's locale.
func (m *Model) shownEntry(input string) string { return sheet.LocalEntry(input, m.locale()) }

// storedEntry is text typed in the workbook's locale as the cell stores
// it.
func (m *Model) storedEntry(typed string) string { return sheet.CanonicalEntry(typed, m.locale()) }

// storedFormula is a formula being typed, in the syntax it's parsed in:
// what the caret's function and argument are read from. It has the same
// runes at the same places.
func (m *Model) storedFormula(buf []rune) []rune {
	if formula.SameSyntax(m.locale()) {
		return buf
	}
	return []rune(formula.Delocalize(string(buf), m.locale()))
}

// canPoint reports whether the caret follows an operator a reference may
// come after; not a decimal comma.
func (m *Model) canPoint() bool {
	if !m.line.CanPoint() {
		return false
	}
	return m.line.Buf[m.line.Pos-1] != ',' || m.locale().Decimal != ','
}

// openLocalePicker lists the locales, the workbook's own highlighted;
// the first follows the config's locale.
func (m *Model) openLocalePicker() {
	cur := m.book().LocaleTag()
	def := sheet.DefaultLocale()
	items := []picker.Item{m.localeItem("Default: "+def.Name, "", def)}
	sel := 0
	for i, l := range locale.All() {
		if l.Tag == cur {
			sel = i + 1
		}
		items = append(items, m.localeItem(l.Name, l.Tag, l))
	}
	p := m.newPicker("Locale", "Type a language, country or tag", 72, items)
	p.Action = "set"
	p.Answers = true
	p.Sel = sel
	m.openOverlay(p)
}

// localeItem is the picker's row for l, which picking sets as tag.
func (m *Model) localeItem(title, tag string, l *locale.Locale) picker.Item {
	return picker.Item{
		Title: title, Name: len(title), Detail: l.Tag + "   " + localeSample(l), Desc: localeDesc(l),
		Pick: func() tea.Cmd {
			m.closeOverlay()
			m.setLocale(tag)
			return nil
		},
	}
}

// localeSample shows a number, a date and the argument separator in l.
func localeSample(l *locale.Locale) string {
	return numfmt.FormatIn(1234.56, "#,##0.00", l) + "   " + numfmt.FormatIn(sampleDate, l.Date, l) + "   " + string(l.ArgSep())
}

// localeDesc says how l writes currency, dates and formulas.
func localeDesc(l *locale.Locale) string {
	return "Numbers " + numfmt.FormatIn(1234.56, sheet.Preset(sheet.FmtCurrency).CodeIn(l), l) +
		", dates " + numfmt.FormatIn(sampleDate, l.Date, l) + ", formulas =ROUND(A1" + string(l.ArgSep()) + " 2)"
}

// sampleDate is the date locales are shown with.
var sampleDate = numfmt.DateSerial(2026, 9, 26)

// setLocale sets the workbook's locale, "" to follow the default.
func (m *Model) setLocale(tag string) {
	m.book().SetLocale(tag)
	m.changed = m.sheet.StateID() != m.saved
	l := m.locale()
	name := l.Name
	if tag == "" {
		name = "the default, " + l.Name
	}
	m.note = "Locale " + name + ". " + localeDesc(l)
}
