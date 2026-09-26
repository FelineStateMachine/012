package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"one23/internal/sheet"
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
	ind = m.th.indicator.Render(" " + ind + " ")
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
				parts[i] = m.th.menuSelected.Render(it.name)
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
		return m.th.muted.Render(strings.Join(m.files, "  "))
	case m.mode == modePrompt && m.prompt.kind == promptWidth:
		return m.th.muted.Render("Type a width or use the left and right arrows")
	case m.pointing() || m.mode == modePoint:
		return m.th.muted.Render("Arrows move, . anchors a range, Esc unanchors, Enter accepts")
	case m.hint != "":
		return m.th.warning.Render(m.hint)
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
	b.WriteString(m.th.header.Render(strings.Repeat(" ", rowHdrW)))
	focus := *m.focus()
	for i := range m.visibleCols(m.left) {
		c := m.left + i
		style := m.th.header
		if c == focus.Col {
			style = m.th.headerActive
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
	hdr := m.th.header
	if row == focus.Row {
		hdr = m.th.headerActive
	}
	var b strings.Builder
	b.WriteString(hdr.Render(padLeft(strconv.Itoa(row+1), rowHdrW-1) + " "))

	var sel sheet.Rect
	selecting := m.mode == modePoint || m.pointing()
	if selecting {
		sel = m.point.rect()
	}
	for i, text := range m.rowText(row) {
		a := sheet.Addr{Col: m.left + i, Row: row}
		switch {
		case a == focus:
			b.WriteString(m.th.pointer.Render(text))
		case selecting && sel.Contains(a):
			b.WriteString(m.th.selection.Render(text))
		case m.sheet.Value(a).Kind == sheet.Error:
			b.WriteString(m.th.errorCell.Render(text))
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
		return m.th.error.Render(m.errMsg) + m.th.muted.Render("  (press any key)")
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
	right := m.th.muted.Render(strings.Join(ind, "  "))
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) helpLines() []string {
	lines := []string{
		m.th.indicator.Render(" HELP ") + "  one23 (press any key to return)",
		"",
		"  Arrows              Move the cell pointer",
		"  PgUp / PgDn         Move one screen up or down",
		"  Tab / Shift+Tab     Move one screen right or left",
		"  Home                Go to A1",
		"  Ctrl+C              Quit immediately",
		"",
	}
	// Shortcuts come from the keymap so help can't drift from behavior.
	var ids []string
	for id := range commands {
		if len(keysFor(id)) > 0 {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(commands[a].title, commands[b].title) })
	for _, id := range ids {
		keys := keysFor(id)
		for i, k := range keys {
			keys[i] = keyLabel(k)
		}
		lines = append(lines, fmt.Sprintf("  %-20s%s", strings.Join(keys, " or "), commands[id].desc))
	}
	return append(lines,
		"",
		"  Entries",
		"  Text                Label (prefix ' left, \" right, ^ center)",
		"  0-9 + - . ( @ # $   Value or formula, e.g. +A1*2, @SUM(A1..A5)",
		"  Arrow after + ( ,   POINT mode: arrow to a cell, . anchors a range",
		"",
		"  Functions: @SUM @AVG @COUNT @MIN @MAX @ABS @INT @SQRT @ROUND @MOD",
		"             @IF @PI @TRUE @FALSE @ERR @NA",
		"  Operators: + - * / ^ & = <> < > <= >= #AND# #OR# #NOT#",
	)
}

// keyLabel formats a key binding for display, e.g. "delete" -> "Del".
func keyLabel(k string) string {
	switch k {
	case "delete":
		return "Del"
	case "enter":
		return "Enter"
	case "esc":
		return "Esc"
	}
	parts := strings.Split(k, "+")
	for i, p := range parts {
		if len(p) > 1 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "+")
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
