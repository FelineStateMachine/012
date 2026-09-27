package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Format commands follow Sheets' Format menu and shortcuts. They apply to
// the whole selection through Sheet methods, each one undo step with a
// label such as "bold B2:B5", and confirm what changed on the third panel
// line.

// numberFormats are Format > Number, with Sheets' examples as
// descriptions.
var numberFormats = []struct {
	id, title, desc string
	f               sheet.Format
}{
	{"format.automatic", "Automatic", "Show numbers as typed, or as formulas infer", sheet.Format{}},
	{"format.plain_text", "Plain text", "Keep entries exactly as typed, even numbers and formulas", sheet.Preset(sheet.FmtText)},
	{"format.number", "Number", "Number with thousands separators: 1,000.12", sheet.Preset(sheet.FmtNumber)},
	{"format.percent", "Percent", "Percent: 10.12%", sheet.Preset(sheet.FmtPercent)},
	{"format.scientific", "Scientific", "Scientific notation: 1.01E+03", sheet.Preset(sheet.FmtScientific)},
	{"format.accounting", "Accounting", "Accounting, $ at the left and negatives in parentheses: $ (1,000.12)", sheet.Preset(sheet.FmtAccounting)},
	{"format.financial", "Financial", "Financial, negatives in parentheses: (1,000.12)", sheet.Preset(sheet.FmtFinancial)},
	{"format.currency", "Currency", "Currency: $1,000.12", sheet.Preset(sheet.FmtCurrency)},
	{"format.currency_rounded", "Currency rounded", "Currency without cents: $1,000", sheet.Format{Kind: sheet.FmtCurrency}},
	{"format.date", "Date", "Date: 9/26/2026", sheet.Preset(sheet.FmtDate)},
	{"format.time", "Time", "Time: 3:59:00 PM", sheet.Preset(sheet.FmtTime)},
	{"format.datetime", "Date time", "Date and time: 9/26/2026 15:59:00", sheet.Preset(sheet.FmtDateTime)},
	{"format.duration", "Duration", "Elapsed hours, minutes and seconds: 24:01:00", sheet.Preset(sheet.FmtDuration)},
}

// textStyles toggle like Sheets: on for the whole selection unless the
// active cell already has it.
var textStyles = []struct {
	id, title, desc string
	get             func(*sheet.Style) *bool
}{
	{"format.bold", "Bold", "Make text bold, or plain again", func(s *sheet.Style) *bool { return &s.Bold }},
	{"format.italic", "Italic", "Make text italic, or upright again", func(s *sheet.Style) *bool { return &s.Italic }},
	{"format.underline", "Underline", "Underline text, or remove the underline", func(s *sheet.Style) *bool { return &s.Underline }},
	{"format.strikethrough", "Strikethrough", "Strike through text, or remove the line", func(s *sheet.Style) *bool { return &s.Strikethrough }},
}

var alignments = []struct {
	id, title, desc string
	a               sheet.Align
}{
	{"format.align_left", "Left", "Align to the left", sheet.AlignLeft},
	{"format.align_center", "Center", "Center horizontally", sheet.AlignCenter},
	{"format.align_right", "Right", "Align to the right", sheet.AlignRight},
}

func init() {
	for _, nf := range numberFormats {
		f, title := nf.f, nf.title
		register(&command{id: nf.id, title: title, desc: nf.desc, edits: (*Model).selection, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			m.sheet.SetFormat(r, f)
			m.formatted(title + " format")
			return nil
		}})
	}
	for _, ts := range textStyles {
		get, title := ts.get, ts.title
		register(&command{id: ts.id, title: title, desc: ts.desc, edits: (*Model).selection, keepsSpills: true, run: func(m *Model) tea.Cmd {
			cur := m.sheet.CellStyle(m.cur)
			on := !*get(&cur)
			r := m.selection()
			label := strings.ToLower(title) + " " + r.String()
			if !on {
				label = "remove " + strings.ToLower(title) + " from " + r.String()
			}
			m.sheet.Batch(sheet.Change{Label: label, Focus: r}, func() error {
				m.sheet.SetStyle(r, func(s *sheet.Style) { *get(s) = on })
				return nil
			})
			m.formatted(title + onOff(on))
			return nil
		}})
	}
	for _, al := range alignments {
		a, title := al.a, al.title
		register(&command{id: al.id, title: title, desc: al.desc, edits: (*Model).selection, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			m.sheet.Batch(sheet.Change{Label: "align " + r.String() + " " + strings.ToLower(title), Focus: r}, func() error {
				m.sheet.SetStyle(r, func(s *sheet.Style) { s.Align = a })
				return nil
			})
			m.formatted("Aligned " + strings.ToLower(title))
			return nil
		}})
	}
	register(
		&command{id: "format.decimals_more", title: "Increase decimal places", desc: "Show one more decimal place", edits: (*Model).selection, keepsSpills: true, run: func(m *Model) tea.Cmd {
			m.sheet.AdjustDecimals(m.selection(), 1)
			m.formatted("One more decimal place")
			return nil
		}},
		&command{id: "format.decimals_less", title: "Decrease decimal places", desc: "Show one less decimal place", edits: (*Model).selection, keepsSpills: true, run: func(m *Model) tea.Cmd {
			m.sheet.AdjustDecimals(m.selection(), -1)
			m.formatted("One less decimal place")
			return nil
		}},
		&command{id: "format.clear", title: "Clear formatting", desc: "Reset number formats and text styles, keeping contents", edits: (*Model).selection, keepsSpills: true, run: func(m *Model) tea.Cmd {
			m.sheet.ClearFormatting(m.selection())
			m.formatted("Formatting cleared")
			return nil
		}},
	)

	// Sheets' shortcuts. Ctrl+Shift+digit needs a terminal that reports
	// it (the kitty keyboard protocol, as in Ghostty, kitty or WezTerm).
	for key, id := range map[string]string{
		"ctrl+shift+1": "format.number",
		"ctrl+shift+2": "format.time",
		"ctrl+shift+3": "format.date",
		"ctrl+shift+4": "format.currency",
		"ctrl+shift+5": "format.percent",
		"ctrl+shift+6": "format.scientific",
		"ctrl+b":       "format.bold",
		"ctrl+i":       "format.italic",
		"ctrl+u":       "format.underline",
		"alt+shift+5":  "format.strikethrough",
		"ctrl+shift+l": "format.align_left",
		"ctrl+shift+e": "format.align_center",
		"ctrl+shift+r": "format.align_right",
		"ctrl+\\":      "format.clear",
	} {
		keymap[key] = id
	}
}

// keyAliases maps other ways terminals report a shortcut to the form in
// keymap: without the kitty protocol, or with a shifted character,
// Ctrl+Shift+1 arrives as ctrl+! and Alt+Shift+5 as alt+%.
var keyAliases = func() map[string]string {
	a := map[string]string{"alt+%": "alt+shift+5", "alt+shift+%": "alt+shift+5"}
	for i, sym := range "!@#$^&" { // Sheets' border keys, Alt+Shift+1 to 4, 6 and 7
		d := string("123467"[i])
		a["alt+"+string(sym)] = "alt+shift+" + d
		a["alt+shift+"+string(sym)] = "alt+shift+" + d
	}
	for i, sym := range "!@#$%^" {
		d := string(rune('1' + i))
		a["ctrl+"+string(sym)] = "ctrl+shift+" + d
		a["ctrl+shift+"+string(sym)] = "ctrl+shift+" + d
	}
	for _, l := range "ler" {
		a["ctrl+"+strings.ToUpper(string(l))] = "ctrl+shift+" + string(l)
	}
	return a
}()

// canonicalKey resolves keyAliases.
func canonicalKey(k string) string {
	if c, ok := keyAliases[k]; ok {
		return c
	}
	return k
}

// formatted marks the sheet changed and says what happened, e.g. "Bold
// on for B2:B5", on the third panel line.
func (m *Model) formatted(what string) {
	m.changed = true
	m.note = what + " for " + m.selection().String()
}

func onOff(on bool) string {
	if on {
		return " on"
	}
	return " off"
}
