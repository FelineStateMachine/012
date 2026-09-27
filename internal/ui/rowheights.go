package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Row heights, in lines: typed on the context line with a live preview,
// dragged by the bottom-right corner of a row's number (mouse.go), or
// fitted to the contents again by double-clicking it, as Sheets' Resize
// row offers a height or Fit to data. A row without a height fits the
// text it wraps.

func init() {
	register(
		&command{id: "row.height", title: "Row height", desc: "Set the height of the selected rows, in lines", run: (*Model).openHeight},
		&command{id: "row.fit", title: "Fit rows to data", desc: "Size the selected rows to their contents again",
			run: func(m *Model) tea.Cmd {
				r := m.selection()
				m.fitRange(r.From.Row, r.To.Row)
				return nil
			}},
	)
}

// openHeight asks for the height of the selected rows, previewing it
// live as the arrows change it. Esc restores the original heights.
func (m *Model) openHeight() tea.Cmd {
	r := m.selection()
	orig := map[int]int{}
	for row, h := range m.sheet.Heights() {
		if row >= r.From.Row && row <= r.To.Row {
			orig[row] = h
		}
	}
	restore := func() {
		m.sheet.Batch(sheet.Change{Label: "row height", Focus: r}, func() error {
			m.sheet.SetRowHeight(r.From.Row, r.To.Row, 0)
			for row, h := range orig {
				m.sheet.SetRowHeight(row, row, h)
			}
			return nil
		})
	}
	cur := m.shape(m.cur.Row).Lines
	m.openPrompt(&prompt{
		kind:      promptWidth,
		label:     "Row height (1-" + strconv.Itoa(sheet.MaxRowHeight) + " lines):",
		indicator: "HEIGHT",
		fresh:     true,
		resize:    func(h int) { m.sheet.SetRowHeight(r.From.Row, r.To.Row, h) },
		maxSize:   sheet.MaxRowHeight,
		onText: func(text string) tea.Cmd {
			h, err := strconv.Atoi(text)
			if err != nil || h < 1 || h > sheet.MaxRowHeight {
				restore()
				m.fail("Row height must be between 1 and " + strconv.Itoa(sheet.MaxRowHeight) + " lines")
				return nil
			}
			m.sheet.SetRowHeight(r.From.Row, r.To.Row, h)
			m.changed = true
			return nil
		},
		onCancel: restore,
	}, strconv.Itoa(cur))
	return nil
}

// fitRows fits row r to its contents, as double-clicking the border of
// its number does in Sheets; selected whole rows fit together.
func (m *Model) fitRows(r int) {
	from, to := r, r
	if sel := m.selection(); m.whole == wholeRows && r >= sel.From.Row && r <= sel.To.Row {
		from, to = sel.From.Row, sel.To.Row
	}
	m.fitRange(from, to)
	m.record(heightAction(from, to, 0))
}

// fitRange clears the heights of rows from..to, only those that have
// one, so rows fit their contents.
func (m *Model) fitRange(from, to int) {
	m.sheet.Batch(sheet.Change{Label: "fit rows to data", Focus: sheet.Rect{From: sheet.Addr{Row: from}, To: sheet.Addr{Col: sheet.MaxCols - 1, Row: to}}}, func() error {
		for row := range m.sheet.Heights() {
			if row >= from && row <= to {
				m.sheet.SetRowHeight(row, row, 0)
			}
		}
		return nil
	})
	m.changed = true
	m.note = "Rows " + strconv.Itoa(from+1) + ":" + strconv.Itoa(to+1) + " fit their contents"
	if from == to {
		m.note = "Row " + strconv.Itoa(from+1) + " fits its contents"
	}
}

// heightAction records setting rows from..to to h lines, 0 to fit.
func heightAction(from, to, h int) macro.Action {
	rows := strconv.Itoa(from + 1)
	if to != from {
		rows += ":" + strconv.Itoa(to+1)
	}
	return macro.Call("set_height", rows, h)
}
