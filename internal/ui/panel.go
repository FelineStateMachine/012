package ui

import (
	"math"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"one23/internal/sheet"
)

// The control panel is three lines: the menu bar with the mode indicator,
// the formula bar, and the context line. Below the grid, the status line
// shows the file and selection statistics.

// View implements tea.Model.
func (m *Model) View() tea.View {
	var lines []string
	if m.mode == modeHelp {
		lines = m.helpLines()
	} else {
		lines = append(lines, m.menuBarLine(), m.formulaBar(), m.contextLineText(), m.headerRow())
		for i := range m.visibleRows() {
			lines = append(lines, m.gridRow(m.top+i))
		}
		lines = append(lines, m.statusLine())
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "")
	}

	content := strings.Join(lines, "\n")
	if m.overlay != nil {
		content = m.compose(content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion // hover feedback; see mouse.go
	v.WindowTitle = "one23 - " + m.displayName()
	if m.changed {
		v.WindowTitle += " (modified)"
	}
	if x, y, ok := m.cursorPos(); ok {
		v.Cursor = tea.NewCursor(x, y)
		v.Cursor.Shape = tea.CursorBar
	}
	return v
}

func (m *Model) displayName() string {
	if m.filename == "" {
		return "untitled"
	}
	return filepath.Base(m.filename)
}

// indicator is the mode shown at the top right.
func (m *Model) indicator() string {
	switch {
	case m.overlay != nil:
		return m.overlay.indicator()
	case m.mode == modePrompt:
		return m.prompt.indicator
	}
	return m.mode.String()
}

// menuBarLine is the menu bar with the mode indicator on the right.
func (m *Model) menuBarLine() string {
	left := m.menuBarTitles()
	ind := m.th.indicator.Render(" " + m.indicator() + " ")
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(ind)
	return left + strings.Repeat(" ", max(gap, 1)) + ind
}

// formulaBarTextX is the screen column where the formula bar's text (the
// cell's contents or the entry being typed) starts, after the name box.
func formulaBarTextX() int { return nameBoxW + 1 }

// formulaBar shows the name box (the active cell, or the selected range)
// and the active cell's contents as typed. While typing, the entry is
// edited here with the terminal cursor, and shown in the cell too.
func (m *Model) formulaBar() string {
	name := m.cur.String()
	if m.hasRange() && m.mode == modeReady {
		name = m.selection().String()
	}
	box := m.th.header.Render(padRight(" "+name, nameBoxW)) + " "
	switch m.mode {
	case modeEnter, modeEdit:
		return box + string(m.buf)
	case modePoint:
		return box + m.pointPrefix + m.th.selection.Render(m.point.text()) + m.pointSuffix
	}
	if c := m.sheet.Cell(m.cur); c != nil {
		return box + c.Input
	}
	return box
}

// contextLineText says what's going on: a prompt, a formula error, or the
// keys that apply in the current mode.
func (m *Model) contextLineText() string {
	var left, right string
	switch {
	case m.drag == dragResize:
		left = m.th.key.Render("Column "+sheet.ColName(m.resizeCol)) + m.th.muted.Render(" width ") +
			strconv.Itoa(m.sheet.ColWidth(m.resizeCol)) + m.th.muted.Render("   double-click the border to fit")
	case m.hint != "":
		left = m.th.warning.Render(m.hint)
	case m.mode == modeReady:
		left = m.readyLine()
	case m.mode == modeMenu:
		if c, ok := m.overlay.(*choiceBar); ok {
			left = c.line(m)
		}
	case m.mode == modePrompt:
		left, right = m.promptLine()
	case m.mode == modePoint:
		left = m.keyHints("Arrows", "pick a cell", "Shift+arrows", "pick a range", "Enter", "accept", "Esc", "back")
	case m.mode == modeEnter && m.isFormula():
		left = m.keyHints("Enter", "accept", "Tab", "accept and go right", "Arrows", "pick cells after an operator", "Esc", "cancel")
	case m.mode == modeEnter:
		left = m.keyHints("Enter", "accept", "Tab", "accept and go right", "Arrows", "accept and move", "Esc", "cancel")
	case m.mode == modeEdit:
		left = m.keyHints("Enter", "accept", "Left/Right", "move the caret", "Esc", "cancel")
	}
	return m.spread(left, right)
}

// spread puts right at the right edge after left, dropping it if there
// isn't room for both.
func (m *Model) spread(left, right string) string {
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if right == "" || gap < 3 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// promptLine is an open prompt, e.g. "Save as: budget.o23", and the keys
// or choices that go with it.
func (m *Model) promptLine() (left, right string) {
	switch {
	case m.pointing():
		return m.promptPrefix() + m.th.selection.Render(m.point.text()),
			m.keyHints("Arrows", "move", "Shift+arrows", "extend", "Enter", "apply", "Esc", "cancel")
	case len(m.files) > 0:
		return m.promptPrefix() + string(m.buf), m.th.muted.Render(strings.Join(m.files, "  "))
	case m.prompt.kind == promptWidth:
		return m.promptPrefix() + string(m.buf), m.keyHints("Left/Right", "adjust", "Enter", "apply", "Esc", "cancel")
	}
	return m.promptPrefix() + string(m.buf), m.keyHints("Enter", "apply", "Esc", "cancel")
}

func (m *Model) promptPrefix() string {
	return m.prompt.label + " "
}

// keyHints renders key and description pairs, e.g. "Enter accept", each
// key as a chip.
func (m *Model) keyHints(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, m.chip(pairs[i])+" "+m.th.muted.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

// chip draws a key name as a key cap, e.g. "Ctrl+S".
func (m *Model) chip(label string) string {
	return m.th.keyChip.Render(" " + label + " ")
}

// cursorPos returns where the terminal cursor goes: in the formula bar
// while typing an entry, on the context line in a text prompt, or in an
// overlay's search field.
func (m *Model) cursorPos() (x, y int, ok bool) {
	if o, isText := m.overlay.(textOverlay); isText {
		x, y = o.cursor(m)
		return x, y, true
	}
	switch {
	case m.mode == modeEnter, m.mode == modeEdit:
		return formulaBarTextX() + ansi.StringWidth(string(m.buf[:m.bufPos])), formulaLine, true
	case m.mode == modePrompt && !m.pointing():
		return ansi.StringWidth(m.promptPrefix() + string(m.buf[:m.bufPos])), contextLine, true
	}
	return 0, 0, false
}

func (m *Model) statusLine() string {
	if m.mode == modeError {
		return m.th.error.Render(m.errMsg) + m.th.muted.Render("   press any key")
	}
	if m.overlay != nil {
		// What the highlighted item does and the keys that apply.
		if desc, keys := m.overlay.status(m); desc != "" || keys != "" {
			return m.spread(desc, keys)
		}
	}
	left := m.displayName()
	if m.changed {
		left += m.th.muted.Render("  modified")
	}
	if m.sheet.Circular {
		left += "  " + m.th.warning.Render("Circular reference")
	}
	var right string
	if m.hasRange() && m.mode == modeReady {
		r := m.selection()
		st := m.sheet.RangeStats(r)
		parts := []string{m.th.key.Render(r.String())}
		if st.Nums > 0 {
			parts = append(parts,
				m.th.muted.Render("Sum ")+fmtStat(st.Sum),
				m.th.muted.Render("Avg ")+fmtStat(st.Sum/float64(st.Nums)))
		}
		parts = append(parts, m.th.muted.Render("Count ")+strconv.Itoa(st.Count))
		right = strings.Join(parts, "   ")
	} else {
		right = m.keyHints(shortcut("help"), "shortcuts", shortcut("menu"), "menu", shortcut("quit"), "quit")
	}
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// fmtStat formats a status line statistic with at most two decimals, as
// Sheets does.
func fmtStat(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}
