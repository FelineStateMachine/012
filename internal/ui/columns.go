package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Column widths: typed on the context line with a live preview, dragged
// by the header border (mouse.go), or fitted to the contents by
// double-clicking it, as in Sheets.

// openWidth asks for the width of the selected columns, previewing it
// live as the arrows change it. Esc restores the original widths.
func (m *Model) openWidth() tea.Cmd {
	r := m.selection()
	orig := map[int]int{}
	for c := r.From.Col; c <= r.To.Col; c++ {
		orig[c] = m.sheet.ColWidth(c)
	}
	restore := func(m *Model) {
		for c, w := range orig {
			m.sheet.SetColWidth(c, w)
		}
	}
	m.openPrompt(&prompt{
		kind:      promptWidth,
		label:     "Column width (1-240):",
		indicator: "WIDTH",
		fresh:     true,
		onText: func(m *Model, text string) tea.Cmd {
			w, err := strconv.Atoi(text)
			if err != nil || w < 1 || w > 240 {
				restore(m)
				m.fail("Column width must be between 1 and 240")
				return nil
			}
			m.setWidths(w)
			m.changed = true
			return nil
		},
		onCancel: restore,
	}, strconv.Itoa(m.sheet.ColWidth(m.cur.Col)))
	return nil
}

// setWidths sets the width of every selected column, as one step.
func (m *Model) setWidths(w int) {
	r := m.selection()
	m.sheet.Batch(sheet.Change{Label: "column width", Focus: r}, func() error {
		for c := r.From.Col; c <= r.To.Col; c++ {
			m.sheet.SetColWidth(c, w)
		}
		return nil
	})
}

// autofit sizes column c to its widest content, as double-clicking a
// header border does in Sheets. Selected columns fit together.
func (m *Model) autofit(c int) {
	cols := []int{c}
	if r := m.selection(); m.whole == wholeCols && c >= r.From.Col && c <= r.To.Col {
		cols = cols[:0]
		for k := r.From.Col; k <= r.To.Col; k++ {
			cols = append(cols, k)
		}
	}
	widest := map[int]int{}
	for _, a := range m.sheet.Addrs() {
		if cell := m.sheet.Cell(a); cell != nil {
			widest[a.Col] = max(widest[a.Col], ansi.StringWidth(cell.Value.String()))
		}
	}
	for _, k := range cols {
		// One column of padding each side, and never narrower than the
		// header letters.
		w := clamp(widest[k]+2, len(sheet.ColName(k))+2, 240)
		m.sheet.SetColWidth(k, w)
		m.record(widthAction(k, k, w))
	}
	m.changed = true
}

// widthAction records setting columns from..to to width w.
func widthAction(from, to, w int) macro.Action {
	cols := sheet.ColName(from)
	if to != from {
		cols += ":" + sheet.ColName(to)
	}
	return macro.Call("set_width", cols, w)
}
