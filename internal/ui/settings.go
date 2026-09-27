package ui

import tea "charm.land/bubbletea/v2"

// File > Settings: workbook settings saved in the file. Sheets keeps its
// calculation settings there too.

func init() {
	register(&command{id: "settings.decimal", title: "Decimal arithmetic",
		desc: "Compute money exactly: + - * /, SUM, AVERAGE and ROUND in decimal, so 0.1+0.2 is 0.3",
		run: func(m *Model) tea.Cmd {
			on := !m.sheet.Decimal()
			m.sheet.SetDecimal(on)
			m.changed = m.sheet.StateID() != m.saved
			m.note = "Decimal arithmetic off: binary floating point, as in Sheets"
			if on {
				m.note = "Decimal arithmetic on: + - * /, SUM, AVERAGE and ROUND are exact in decimal"
			}
			return nil
		},
		checked: func(m *Model) bool { return m.sheet.Decimal() },
	})
}
