package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Hidden sheets, as in Sheets: the tab menu's Hide sheet takes a sheet
// out of the tabs, and View > Hidden sheets lists them to show one
// again. Formulas still read a hidden sheet; moving between sheets, the
// tab strip, Go to sheet and Find in all sheets skip it, and going to a
// cell on it (Go to, a named range) says it's hidden rather than show it.

func init() {
	register(
		&command{id: "sheet.hide", title: "Hide sheet", desc: "Hide the sheet's tab; formulas still read it",
			enabled: func(m *Model) bool { return len(m.book().Visible()) > 1 }, run: (*Model).hideSheet},
		&command{id: "sheet.unhide", title: "Hidden sheets", desc: "List the hidden sheets and show one again",
			enabled: func(m *Model) bool { return len(m.book().HiddenSheets()) > 0 }, run: func(m *Model) tea.Cmd {
				m.openHiddenPicker()
				return nil
			}},
	)
}

// hideSheet hides the sheet shown and shows the next visible one.
func (m *Model) hideSheet() tea.Cmd {
	s := m.sheet
	i := m.book().Index(s)
	if err := m.book().HideSheet(s); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.afterSheetsChange(nil, i)
	m.note = "Hid " + s.Name() + "; View > Hidden sheets shows it again"
	return nil
}

// openHiddenPicker lists the hidden sheets; Enter shows the one picked
// again. A macro records the pick as the command's answer.
func (m *Model) openHiddenPicker() {
	var items []pickItem
	for _, s := range m.book().HiddenSheets() {
		detail := "empty"
		if used, ok := s.UsedRange(); ok {
			detail = used.String() + ", " + cellCount(s.Len())
		}
		items = append(items, pickItem{
			title: s.Name(), name: len(s.Name()), detail: detail, desc: "Show " + s.Name() + " again",
			pick: func(m *Model) tea.Cmd {
				m.closeOverlay()
				if err := m.book().UnhideSheet(s); err != nil {
					m.fail(err.Error())
					return nil
				}
				m.showSheet(s)
				m.note = "Unhid " + s.Name()
				return nil
			},
		})
	}
	p := newPicker(m, "Hidden sheets", "Type a sheet name", 60, items)
	p.action = "unhide"
	p.answers = true
	m.openOverlay(p)
}

// refuseHidden says so and reports true when s is a hidden sheet, which
// isn't shown until it's unhidden.
func (m *Model) refuseHidden(s *sheet.Sheet) bool {
	if s == nil || !s.Hidden() {
		return false
	}
	m.fail(hiddenMsg(s))
	return true
}

func hiddenMsg(s *sheet.Sheet) string {
	return s.Name() + " is hidden; View > Hidden sheets shows it again"
}

// visibleSheets are the sheets with tabs, and the sheet shown if it
// somehow isn't among them, so the tab strip always has it.
func (m *Model) visibleSheets() []*sheet.Sheet {
	if !m.sheet.Hidden() {
		return m.book().Visible()
	}
	var out []*sheet.Sheet
	for _, s := range m.book().Sheets() {
		if !s.Hidden() || s == m.sheet {
			out = append(out, s)
		}
	}
	return out
}

// nearVisible is the visible sheet at index i of the workbook or the
// nearest one after it, else before it.
func (m *Model) nearVisible(i int) *sheet.Sheet {
	book := m.book()
	for j := max(i, 0); j < book.Len(); j++ {
		if s := book.Sheet(j); !s.Hidden() {
			return s
		}
	}
	for j := min(i, book.Len()-1); j >= 0; j-- {
		if s := book.Sheet(j); !s.Hidden() {
			return s
		}
	}
	return book.Sheet(0)
}
