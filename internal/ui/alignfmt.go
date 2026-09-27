package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// Format > Alignment's vertical alignments, and the color Format >
// Borders draws with, which it keeps as it keeps the line style.

var valignments = []struct {
	id, title, desc string
	v               sheet.VAlign
}{
	{"format.valign_top", "Align top", "Put text at the top of a tall row", sheet.VAlignTop},
	{"format.valign_middle", "Align middle", "Center text between the top and bottom of a tall row", sheet.VAlignMiddle},
	{"format.valign_bottom", "Align bottom", "Put text at the bottom of a tall row, as Sheets does unless told", sheet.VAlignBottom},
}

func init() {
	for _, va := range valignments {
		v, title := va.v, va.title
		register(&command{id: va.id, title: title, desc: va.desc, edits: (*Model).selection, keepsSpills: true,
			checked: func(m *Model) bool { return m.sheet.CellStyle(m.cur).VAlign == v },
			run: func(m *Model) tea.Cmd {
				r := m.selection()
				m.sheet.Batch(sheet.Change{Label: strings.ToLower(title) + " " + r.String(), Focus: r}, func() error {
					m.sheet.SetStyle(r, func(s *sheet.Style) { s.VAlign = v })
					return nil
				})
				m.formatted("Aligned to the " + strings.ToLower(strings.TrimPrefix(title, "Align ")))
				return nil
			}})
	}
	register(&command{id: "format.border_color", title: "Border color",
		desc: "Choose the color borders draw with: the text's, or one of the terminal's colors",
		run:  func(m *Model) tea.Cmd { m.openBorderColors(); return nil }})
}

// openBorderColors lists the colors borders may draw with, the current
// one highlighted.
func (m *Model) openBorderColors() {
	var items []picker.Item
	for _, c := range sheet.Colors() {
		title, desc := c.Title(), "Draw borders in "+c.String()
		if c == sheet.ColorNone {
			title, desc = "Automatic", "Draw borders in the text's color, as Sheets draws them black"
		}
		items = append(items, picker.Item{Title: title, Name: len(title), Desc: desc, Swatch: m.borderRole(c).Render("━━━"), Pick: func() tea.Cmd {
			m.closeOverlay()
			m.borderColor = c
			m.note = "Borders draw in " + strings.ToLower(title) + " now"
			if c == sheet.ColorNone {
				m.note = "Borders draw in the text's color now"
			}
			return nil
		}})
	}
	p := m.newPicker("Border color", "Type a color", 40, items)
	p.Action = "choose"
	p.Answers = true
	p.Sel = int(m.borderColor)
	m.openOverlay(p)
}
