// Package locale is the table of spreadsheet locales, as Sheets' File >
// Settings > Locale offers them: how numbers, dates and currency are
// typed and shown, and how formulas separate their arguments. It holds
// only the conventions, with no dependencies (no CLDR data): the
// packages that parse and render use them at their edges, while cells,
// formulas and files stay in the canonical form of en-US.
package locale

import (
	"strings"
)

// DateOrder is the order of day, month and year in a date typed or
// shown with numbers only.
type DateOrder uint8

const (
	MDY DateOrder = iota // 9/26/2026
	DMY                  // 26.09.2026
	YMD                  // 2026-09-26
)

func (o DateOrder) String() string { return [...]string{"MDY", "DMY", "YMD"}[o] }

// Locale is one locale's conventions.
type Locale struct {
	Tag  string // BCP 47, as written in files and the config: "de-DE"
	Name string // as the picker lists it: "German (Germany)"

	Decimal byte   // the decimal separator: '.' or ','
	Group   string // the thousands separator shown: ",", ".", a no-break space, an apostrophe

	Order   DateOrder
	DateSep byte   // between the numbers of a date: '/', '.' or '-'
	Date    string // the Date format's pattern: "dd.mm.yyyy"
	Time    string // the Time format's pattern: "hh:mm:ss"

	Currency string // the currency symbol: "€"
	After    bool   // the symbol follows the number: "1.234,56 €"
	Space    bool   // a space between the symbol and the number: "R$ 1.234,56"
}

// Canonical is en-US: what cells, formulas and files store, whatever the
// locale shown.
var Canonical = &table[0]

// nbsp and nnbsp are the no-break spaces locales group digits with, so
// a number never breaks across lines.
const (
	nbsp  = " "
	nnbsp = " "
)

// table lists the locales, en-US first. Each sticks to what Sheets shows
// by default: Date, Time and Currency are the formats of Format > Number
// in that locale.
var table = []Locale{
	{Tag: "en-US", Name: "English (United States)", Decimal: '.', Group: ",", Order: MDY, DateSep: '/', Date: "m/d/yyyy", Time: "h:mm:ss am/pm", Currency: "$"},
	{Tag: "en-GB", Name: "English (United Kingdom)", Decimal: '.', Group: ",", Order: DMY, DateSep: '/', Date: "dd/mm/yyyy", Time: "hh:mm:ss", Currency: "£"},
	{Tag: "en-CA", Name: "English (Canada)", Decimal: '.', Group: ",", Order: YMD, DateSep: '-', Date: "yyyy-mm-dd", Time: "h:mm:ss am/pm", Currency: "$"},
	{Tag: "en-AU", Name: "English (Australia)", Decimal: '.', Group: ",", Order: DMY, DateSep: '/', Date: "d/mm/yyyy", Time: "h:mm:ss am/pm", Currency: "$"},
	{Tag: "de-DE", Name: "German (Germany)", Decimal: ',', Group: ".", Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "hh:mm:ss", Currency: "€", After: true, Space: true},
	{Tag: "de-CH", Name: "German (Switzerland)", Decimal: '.', Group: "’", Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "hh:mm:ss", Currency: "CHF", Space: true},
	{Tag: "fr-FR", Name: "French (France)", Decimal: ',', Group: nnbsp, Order: DMY, DateSep: '/', Date: "dd/mm/yyyy", Time: "hh:mm:ss", Currency: "€", After: true, Space: true},
	{Tag: "fr-CA", Name: "French (Canada)", Decimal: ',', Group: nbsp, Order: YMD, DateSep: '-', Date: "yyyy-mm-dd", Time: "hh:mm:ss", Currency: "$", After: true, Space: true},
	{Tag: "es-ES", Name: "Spanish (Spain)", Decimal: ',', Group: ".", Order: DMY, DateSep: '/', Date: "dd/mm/yyyy", Time: "h:mm:ss", Currency: "€", After: true, Space: true},
	{Tag: "es-MX", Name: "Spanish (Mexico)", Decimal: '.', Group: ",", Order: DMY, DateSep: '/', Date: "dd/mm/yyyy", Time: "hh:mm:ss", Currency: "$"},
	{Tag: "it-IT", Name: "Italian (Italy)", Decimal: ',', Group: ".", Order: DMY, DateSep: '/', Date: "dd/mm/yyyy", Time: "hh:mm:ss", Currency: "€", After: true, Space: true},
	{Tag: "pt-BR", Name: "Portuguese (Brazil)", Decimal: ',', Group: ".", Order: DMY, DateSep: '/', Date: "dd/mm/yyyy", Time: "hh:mm:ss", Currency: "R$", Space: true},
	{Tag: "pt-PT", Name: "Portuguese (Portugal)", Decimal: ',', Group: nbsp, Order: DMY, DateSep: '/', Date: "dd/mm/yyyy", Time: "hh:mm:ss", Currency: "€", After: true, Space: true},
	{Tag: "nl-NL", Name: "Dutch (Netherlands)", Decimal: ',', Group: ".", Order: DMY, DateSep: '-', Date: "d-m-yyyy", Time: "hh:mm:ss", Currency: "€", Space: true},
	{Tag: "sv-SE", Name: "Swedish (Sweden)", Decimal: ',', Group: nbsp, Order: YMD, DateSep: '-', Date: "yyyy-mm-dd", Time: "hh:mm:ss", Currency: "kr", After: true, Space: true},
	{Tag: "da-DK", Name: "Danish (Denmark)", Decimal: ',', Group: ".", Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "hh:mm:ss", Currency: "kr.", After: true, Space: true},
	{Tag: "nb-NO", Name: "Norwegian (Norway)", Decimal: ',', Group: nbsp, Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "hh:mm:ss", Currency: "kr", Space: true},
	{Tag: "fi-FI", Name: "Finnish (Finland)", Decimal: ',', Group: nbsp, Order: DMY, DateSep: '.', Date: "d.m.yyyy", Time: "h:mm:ss", Currency: "€", After: true, Space: true},
	{Tag: "pl-PL", Name: "Polish (Poland)", Decimal: ',', Group: nbsp, Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "hh:mm:ss", Currency: "zł", After: true, Space: true},
	{Tag: "cs-CZ", Name: "Czech (Czechia)", Decimal: ',', Group: nbsp, Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "h:mm:ss", Currency: "Kč", After: true, Space: true},
	{Tag: "ru-RU", Name: "Russian (Russia)", Decimal: ',', Group: nbsp, Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "h:mm:ss", Currency: "₽", After: true, Space: true},
	{Tag: "tr-TR", Name: "Turkish (Türkiye)", Decimal: ',', Group: ".", Order: DMY, DateSep: '.', Date: "dd.mm.yyyy", Time: "hh:mm:ss", Currency: "₺"},
	{Tag: "ja-JP", Name: "Japanese (Japan)", Decimal: '.', Group: ",", Order: YMD, DateSep: '/', Date: "yyyy/mm/dd", Time: "h:mm:ss", Currency: "¥"},
	{Tag: "zh-CN", Name: "Chinese (China)", Decimal: '.', Group: ",", Order: YMD, DateSep: '/', Date: "yyyy/m/d", Time: "h:mm:ss", Currency: "¥"},
}

// All returns every locale, en-US first, then as the table lists them.
func All() []*Locale {
	out := make([]*Locale, len(table))
	for i := range table {
		out[i] = &table[i]
	}
	return out
}

// Tags are the tags of every locale, in the table's order.
func Tags() []string {
	out := make([]string, len(table))
	for i := range table {
		out[i] = table[i].Tag
	}
	return out
}

// Lookup finds a locale by its tag, ignoring case and taking _ for -, so
// "de-DE", "de_de" and "DE-de" are the same. A language alone ("de")
// means its first locale in the table.
func Lookup(tag string) (*Locale, bool) {
	tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	if tag == "" {
		return nil, false
	}
	for i := range table {
		if strings.EqualFold(table[i].Tag, tag) {
			return &table[i], true
		}
	}
	if !strings.Contains(tag, "-") {
		for i := range table {
			if lang, _, _ := strings.Cut(table[i].Tag, "-"); strings.EqualFold(lang, tag) {
				return &table[i], true
			}
		}
	}
	return nil, false
}

// FromPOSIX reads a POSIX locale, as LANG and LC_ALL hold it
// ("de_DE.UTF-8", "fr_FR@euro"), returning the tag of the locale in the
// table it names. C, POSIX and locales outside the table give "".
func FromPOSIX(v string) string {
	v, _, _ = strings.Cut(v, ".")
	v, _, _ = strings.Cut(v, "@")
	if v == "C" || v == "POSIX" {
		return ""
	}
	if l, ok := Lookup(v); ok {
		return l.Tag
	}
	return ""
}

// IsCanonical reports whether l types and shows everything as stored:
// en-US, or nil.
func (l *Locale) IsCanonical() bool { return l == nil || l == Canonical }

// Or returns l, or the canonical locale when l is nil.
func (l *Locale) Or() *Locale {
	if l == nil {
		return Canonical
	}
	return l
}

// ArgSep separates a formula's arguments: ; where the decimal separator
// is a comma, as in Sheets, so 1,5 stays a number.
func (l *Locale) ArgSep() byte {
	if l != nil && l.Decimal == ',' {
		return ';'
	}
	return ','
}

// ColSep separates the values of a row in an array literal: \ where the
// argument separator is ;, which also separates an array's rows.
func (l *Locale) ColSep() byte {
	if l != nil && l.Decimal == ',' {
		return '\\'
	}
	return ','
}

// IsGroup reports whether r separates thousands in l as typed: its own
// separator, any space where it groups with a space, and ' for ’.
func (l *Locale) IsGroup(r rune) bool {
	if string(r) == l.Group {
		return true
	}
	switch l.Group {
	case nbsp, nnbsp:
		return r == ' ' || r == ' ' || r == ' '
	case "’":
		return r == '\''
	}
	return false
}

// DateTime is the Date time format's pattern: the date, then the time on
// the 24-hour clock, as Sheets shows it in every locale.
func (l *Locale) DateTime() string {
	t := strings.TrimSpace(strings.Replace(l.Time, "am/pm", "", 1))
	return l.Date + " " + t
}
