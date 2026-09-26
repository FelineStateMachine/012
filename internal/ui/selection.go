package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"one23/internal/sheet"
)

// Selection follows Google Sheets: the active cell (m.cur) stays put while
// an extension corner (m.ext) moves with Shift+arrows, Shift+click or a
// mouse drag. Clicking a column or row header selects whole columns or
// rows, and the corner selects everything.

type wholeKind int

const (
	wholeNone wholeKind = iota
	wholeCols
	wholeRows
	wholeAll
)

type dragKind int

const (
	dragNone dragKind = iota
	dragCells
	dragCols
	dragRows
	dragPoint  // dragging out a range in POINT mode or a range prompt
	dragResize // dragging a column header border
)

// selection returns the selected range; just the active cell when nothing
// is selected.
func (m *Model) selection() sheet.Rect {
	if !m.selecting {
		return sheet.Rect{From: m.cur, To: m.cur}
	}
	r := sheet.NewRect(m.cur, m.ext)
	switch m.whole {
	case wholeCols:
		r.From.Row, r.To.Row = 0, sheet.MaxRows-1
	case wholeRows:
		r.From.Col, r.To.Col = 0, sheet.MaxCols-1
	case wholeAll:
		r = sheet.Rect{To: sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}}
	}
	return r
}

// hasRange reports whether more than one cell is selected.
func (m *Model) hasRange() bool {
	r := m.selection()
	return r.From != r.To
}

func (m *Model) clearSelection() {
	m.selecting, m.whole = false, wholeNone
	m.ext = m.cur
}

// extendKey reports whether key is a Shift-modified movement key and
// returns the movement without Shift. Shift+Tab is its own movement.
func extendKey(key string) (string, bool) {
	if key == "shift+tab" || !strings.Contains(key, "shift+") {
		return "", false
	}
	base := strings.Replace(key, "shift+", "", 1)
	return base, isMoveKey(base)
}

// moveKey handles a movement key in READY mode: plain keys move the active
// cell and drop the selection, Shift+keys extend it.
func (m *Model) moveKey(key string) bool {
	if base, ok := extendKey(key); ok {
		if !m.selecting {
			m.selecting, m.ext = true, m.cur
		}
		m.whole = wholeNone
		return m.navigate(base, &m.ext)
	}
	if m.navigate(key, &m.cur) {
		m.clearSelection()
		return true
	}
	return false
}

// pointMoveKey handles movement in POINT mode and range prompts: arrows
// move the pointer and Shift+arrows stretch it into a range, as when
// selecting a range while typing a formula in Sheets.
func (m *Model) pointMoveKey(key string) bool {
	if key == "tab" || key == "shift+tab" {
		return false
	}
	if base, ok := extendKey(key); ok {
		if !m.point.anchored {
			m.point.anchor, m.point.anchored = m.point.at, true
		}
		return m.navigate(base, &m.point.at)
	}
	if m.navigate(key, &m.point.at) {
		m.point.anchored = false
		return true
	}
	return false
}

// selectColumns selects the columns spanned by the selection (Ctrl+Space).
func (m *Model) selectColumns() tea.Cmd {
	if !m.selecting {
		m.ext = m.cur
	}
	m.selecting, m.whole = true, wholeCols
	return nil
}

// selectRows selects the rows spanned by the selection (Shift+Space).
func (m *Model) selectRows() tea.Cmd {
	if !m.selecting {
		m.ext = m.cur
	}
	m.selecting, m.whole = true, wholeRows
	return nil
}

func (m *Model) selectAll() tea.Cmd {
	used, ok := m.sheet.UsedRange()
	if ok && !(m.selecting && m.whole == wholeNone && m.selection() == used) {
		m.cur, m.ext = used.From, used.To
		m.selecting, m.whole = true, wholeNone
		return nil
	}
	m.cur = sheet.Addr{Col: m.left, Row: m.top}
	m.selecting, m.whole, m.ext = true, wholeAll, m.cur
	return nil
}
