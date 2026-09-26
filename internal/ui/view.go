package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"one23/internal/sheet"
)

// Styles use the 16 ANSI colors so the sheet follows the user's terminal
// theme.
var (
	indicatorStyle = lipgloss.NewStyle().Background(lipgloss.Cyan).Foreground(lipgloss.Black).Bold(true)
	headerStyle    = lipgloss.NewStyle().Background(lipgloss.BrightBlack).Foreground(lipgloss.BrightWhite)
	headerCurStyle = lipgloss.NewStyle().Background(lipgloss.Cyan).Foreground(lipgloss.Black).Bold(true)
	pointerStyle   = lipgloss.NewStyle().Background(lipgloss.Cyan).Foreground(lipgloss.Black)
	rangeStyle     = lipgloss.NewStyle().Background(lipgloss.Blue).Foreground(lipgloss.BrightWhite)
	menuSelStyle   = lipgloss.NewStyle().Reverse(true)
	hintStyle      = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.BrightRed).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
)

// View implements tea.Model.
func (m *Model) View() tea.View {
	var lines []string
	if m.mode == modeHelp {
		lines = m.helpLines()
	} else {
		lines = append(lines, m.panelLine1(), m.panelLine2(), m.panelLine3(), m.headerRow())
		for i := range m.visibleRows() {
			lines = append(lines, m.gridRow(m.top+i))
		}
		lines = append(lines, m.statusLine())
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "")
	}

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "one23 - " + m.displayName()
	if x, ok := m.cursorX(); ok {
		v.Cursor = tea.NewCursor(x, 1)
	}
	return v
}

func (m *Model) displayName() string {
	if m.filename == "" {
		return "untitled"
	}
	return filepath.Base(m.filename)
}

// panelLine1 shows the current cell's address, width and contents, with the
// mode indicator on the right.
func (m *Model) panelLine1() string {
	left := m.cur.String() + ":"
	if w := m.sheet.ColWidth(m.cur.Col); w != sheet.DefaultWidth {
		left += fmt.Sprintf(" [W%d]", w)
	}
	if c := m.sheet.Cell(m.cur); c != nil {
		left += " " + c.Input
	}
	ind := m.mode.String()
	if m.mode == modePrompt {
		ind = m.prompt.indicator
	}
	ind = indicatorStyle.Render(" " + ind + " ")
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(ind)
	if gap < 1 {
		left = ansi.Truncate(left, m.width-ansi.StringWidth(ind)-1, "")
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + ind
}

// panelLine2 is the edit line, the menu, or a prompt.
func (m *Model) panelLine2() string {
	switch m.mode {
	case modeLabel, modeValue, modeEdit:
		return string(m.buf)
	case modePoint:
		return m.pointPrefix + m.point.text()
	case modeMenu:
		lvl := m.menu[len(m.menu)-1]
		parts := make([]string, len(lvl.items))
		for i, it := range lvl.items {
			parts[i] = it.name
			if i == lvl.sel {
				parts[i] = menuSelStyle.Render(it.name)
			}
		}
		return strings.Join(parts, "  ")
	case modePrompt:
		if m.pointing() {
			return m.prompt.label + " " + m.point.text()
		}
		return m.promptPrefix() + string(m.buf)
	}
	return ""
}

func (m *Model) promptPrefix() string {
	return m.prompt.label + " "
}

// panelLine3 describes the highlighted menu item, lists files, or shows a
// hint such as a formula error.
func (m *Model) panelLine3() string {
	switch {
	case m.mode == modeMenu:
		lvl := m.menu[len(m.menu)-1]
		return lvl.items[lvl.sel].description(m)
	case m.mode == modePrompt && len(m.files) > 0:
		return dimStyle.Render(strings.Join(m.files, "  "))
	case m.mode == modePrompt && m.prompt.kind == promptWidth:
		return dimStyle.Render("Type a width or use the left and right arrows")
	case m.pointing() || m.mode == modePoint:
		return dimStyle.Render("Arrows move, . anchors a range, Esc unanchors, Enter accepts")
	case m.hint != "":
		return hintStyle.Render(m.hint)
	}
	return ""
}

// cursorX returns where the terminal cursor goes on the edit line.
func (m *Model) cursorX() (int, bool) {
	switch {
	case m.mode == modeLabel, m.mode == modeValue, m.mode == modeEdit:
		return ansi.StringWidth(string(m.buf[:m.bufPos])), true
	case m.mode == modePrompt && !m.pointing():
		return ansi.StringWidth(m.promptPrefix() + string(m.buf[:m.bufPos])), true
	}
	return 0, false
}

func (m *Model) headerRow() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(strings.Repeat(" ", rowHdrW)))
	focus := *m.focus()
	for i := range m.visibleCols(m.left) {
		c := m.left + i
		style := headerStyle
		if c == focus.Col {
			style = headerCurStyle
		}
		b.WriteString(style.Render(center(sheet.ColName(c), m.sheet.ColWidth(c))))
	}
	return b.String()
}

func (m *Model) gridRow(row int) string {
	if row >= sheet.MaxRows {
		return ""
	}
	focus := *m.focus()
	hdr := headerStyle
	if row == focus.Row {
		hdr = headerCurStyle
	}
	var b strings.Builder
	b.WriteString(hdr.Render(padRight(strconv.Itoa(row+1), rowHdrW)))

	var sel sheet.Rect
	selecting := m.mode == modePoint || m.pointing()
	if selecting {
		sel = m.point.rect()
	}
	for i, text := range m.rowText(row) {
		a := sheet.Addr{Col: m.left + i, Row: row}
		switch {
		case a == focus:
			b.WriteString(pointerStyle.Render(text))
		case selecting && sel.Contains(a):
			b.WriteString(rangeStyle.Render(text))
		default:
			b.WriteString(text)
		}
	}
	return b.String()
}

// rowText returns the visible text of each column in row, each exactly the
// column's width. Left-aligned labels spill into blank cells to the right,
// including labels that start left of the viewport.
func (m *Model) rowText(row int) []string {
	ncols := m.visibleCols(m.left)
	out := make([]string, ncols)

	var spill string
	for c := m.left - 1; c >= 0; c-- {
		cell := m.sheet.Cell(sheet.Addr{Col: c, Row: row})
		if cell == nil {
			continue
		}
		if cell.Align() == '\'' {
			spill = cell.Value.Str
			for k := c; k < m.left && spill != ""; k++ {
				_, spill = cut(spill, m.sheet.ColWidth(k))
			}
		}
		break
	}

	for i := range ncols {
		c := m.left + i
		w := m.sheet.ColWidth(c)
		cell := m.sheet.Cell(sheet.Addr{Col: c, Row: row})
		switch {
		case cell != nil:
			spill = ""
			out[i] = cellText(cell, w, &spill)
		case spill != "":
			var head string
			head, spill = cut(spill, w)
			out[i] = padRight(head, w)
		default:
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// cellText formats a non-blank cell to width w. Left-aligned label text
// that doesn't fit is returned through spill.
func cellText(c *sheet.Cell, w int, spill *string) string {
	v := c.Value
	switch {
	case v.Kind == sheet.Number || v.Kind == sheet.Error:
		return sheet.FormatValue(v, w)
	case c.Align() == '"':
		return padLeft(ansi.Truncate(v.Str, w, ""), w)
	case c.Align() == '^':
		return center(ansi.Truncate(v.Str, w, ""), w)
	case c.Align() == '\'':
		var head string
		head, *spill = cut(v.Str, w)
		return padRight(head, w)
	}
	// String formula results are left-aligned and don't spill.
	return padRight(ansi.Truncate(v.Str, w, ""), w)
}

func (m *Model) statusLine() string {
	if m.mode == modeError {
		return errorStyle.Render(m.errMsg) + dimStyle.Render("  (press any key)")
	}
	left := m.displayName()
	if m.changed {
		left += " [modified]"
	}
	var ind []string
	if m.sheet.Circular {
		ind = append(ind, "CIRC")
	}
	ind = append(ind, "F1 Help  / Menu")
	right := dimStyle.Render(strings.Join(ind, "  "))
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) helpLines() []string {
	return []string{
		indicatorStyle.Render(" HELP ") + "  one23 keys (press any key to return)",
		"",
		"  Arrows            Move the cell pointer",
		"  PgUp / PgDn       Move one screen up or down",
		"  Tab / Shift+Tab   Move one screen right or left",
		"  Home              Go to A1",
		"  F5                Go to a cell address",
		"  F2                Edit the current cell",
		"  Del               Erase the current cell",
		"  / or <            Open the menu (type an item's first letter)",
		"  Ctrl+C            Quit immediately",
		"",
		"  Entries",
		"  Text              Label (prefix ' left, \" right, ^ center)",
		"  0-9 + - . ( @ #   Value or formula, e.g. +A1*2, @SUM(A1..A5)",
		"  Arrows after an operator enter POINT mode; . anchors a range",
		"",
		"  Functions: @SUM @AVG @COUNT @MIN @MAX @ABS @INT @SQRT @ROUND @MOD",
		"             @IF @PI @TRUE @FALSE @ERR @NA",
		"  Operators: + - * / ^ & = <> < > <= >= #AND# #OR# #NOT#",
	}
}

// cut splits s after w display columns.
func cut(s string, w int) (head, tail string) {
	head = ansi.Truncate(s, w, "")
	return head, s[len(head):]
}

func padRight(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

func padLeft(s string, w int) string {
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}

func center(s string, w int) string {
	pad := max(w-ansi.StringWidth(s), 0)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}
