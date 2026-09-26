package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	v.MouseMode = tea.MouseModeAllMotion // hover feedback; see mouse.go
	v.WindowTitle = "one23 - " + m.displayName()
	if m.changed {
		v.WindowTitle += " (modified)"
	}
	if x, ok := m.cursorX(); ok {
		line, lx := m.editLineAt()
		v.Cursor = tea.NewCursor(lx+x, line)
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

// panelLine1 is the formula bar: the active cell's address and contents as
// typed, with the mode indicator on the right.
func (m *Model) panelLine1() string {
	left := m.th.header.Render(padRight(" "+m.cur.String(), rowHdrW-1)) + " "
	if c := m.sheet.Cell(m.cur); c != nil {
		left += c.Input
	}
	ind := m.mode.String()
	if m.mode == modePrompt {
		ind = m.prompt.indicator
	}
	ind = m.th.indicator.Render(" " + ind + " ")
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(ind)
	if gap < 1 {
		left = ansi.Truncate(left, m.width-ansi.StringWidth(ind)-2, "…")
		gap = m.width - ansi.StringWidth(left) - ansi.StringWidth(ind)
	}
	return left + strings.Repeat(" ", max(gap, 1)) + ind
}

// panelLine2 is the edit line, the menu, or a prompt.
func (m *Model) panelLine2() string {
	switch m.mode {
	case modeEnter, modeEdit:
		return string(m.buf)
	case modePoint:
		return m.pointPrefix + m.th.selection.Render(m.point.text()) + m.pointSuffix
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
			return m.promptPrefix() + m.th.selection.Render(m.point.text())
		}
		return m.promptPrefix() + string(m.buf)
	}
	return ""
}

func (m *Model) promptPrefix() string {
	return m.prompt.label + " "
}

// panelLine3 explains the current state: the highlighted menu item, a
// formula error, or which keys do what.
func (m *Model) panelLine3() string {
	switch {
	case m.drag == dragResize:
		return m.th.key.Render("Column "+sheet.ColName(m.resizeCol)) + m.th.muted.Render(" width ") +
			strconv.Itoa(m.sheet.ColWidth(m.resizeCol)) + m.th.muted.Render("   double-click the border to fit")
	case m.hint != "":
		return m.th.warning.Render(m.hint)
	case m.mode == modeMenu:
		lvl := m.menu[len(m.menu)-1]
		return lvl.items[lvl.sel].description(m)
	case m.mode == modePrompt && len(m.files) > 0:
		return m.th.muted.Render(strings.Join(m.files, "  "))
	case m.mode == modePrompt && m.prompt.kind == promptWidth:
		return m.keyHints("Left/Right", "adjust", "Enter", "apply", "Esc", "cancel")
	case m.pointing():
		return m.keyHints("Arrows", "move", "Shift+arrows", "extend", "Enter", "apply", "Esc", "cancel")
	case m.mode == modePrompt:
		return m.keyHints("Enter", "apply", "Esc", "cancel")
	case m.mode == modePoint:
		return m.keyHints("Arrows", "pick a cell", "Shift+arrows", "pick a range", "Enter", "accept", "Esc", "back")
	case m.mode == modeEnter && m.isFormula():
		return m.keyHints("Enter", "accept", "Tab", "accept and go right", "Arrows after an operator", "pick cells", "Esc", "cancel")
	case m.mode == modeEnter:
		return m.keyHints("Enter", "accept", "Tab", "accept and go right", "Arrows", "accept and move", "Esc", "cancel")
	case m.mode == modeEdit:
		return m.keyHints("Enter", "accept", "Left/Right", "move the caret", "Esc", "cancel")
	case m.mode == modeReady:
		return m.readyLine()
	}
	return ""
}

// keyHints renders key and description pairs, e.g. "Enter accept".
func (m *Model) keyHints(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, m.th.key.Render(pairs[i])+" "+m.th.muted.Render(pairs[i+1]))
	}
	return strings.Join(parts, m.th.muted.Render("   "))
}

// cursorX returns where the terminal cursor goes on the edit line.
func (m *Model) cursorX() (int, bool) {
	switch {
	case m.mode == modeEnter, m.mode == modeEdit:
		return ansi.StringWidth(string(m.buf[:m.bufPos])), true
	case m.mode == modePrompt && !m.pointing():
		return ansi.StringWidth(m.promptPrefix() + string(m.buf[:m.bufPos])), true
	}
	return 0, false
}

// active is the cell drawn as the cell pointer: the pointer while
// pointing, otherwise the active cell (which stays put while a selection is
// extended, as in Sheets).
func (m *Model) active() sheet.Addr {
	if m.mode == modePoint || m.pointing() {
		return m.point.at
	}
	return m.cur
}

// highlight is the range drawn as selected: the pointer's range while
// pointing, otherwise the selection.
func (m *Model) highlight() (sheet.Rect, bool) {
	if m.mode == modePoint || m.pointing() {
		return m.point.rect(), true
	}
	return m.selection(), m.hasRange()
}

func (m *Model) headerRow() string {
	var b strings.Builder
	b.WriteString(m.th.header.Render(strings.Repeat(" ", rowHdrW)))
	focus := m.active()
	sel, selecting := m.highlight()
	for i := range m.visibleCols(m.left) {
		c := m.left + i
		w := m.sheet.ColWidth(c)
		style := m.th.header
		switch {
		case c == focus.Col:
			style = m.th.headerActive
		case selecting && c >= sel.From.Col && c <= sel.To.Col:
			style = m.th.headerSel
		case m.hover.addr.Col == c && (m.hover.kind == hitColHeader || m.hover.kind == hitColBorder):
			style = m.th.headerHover
		}
		label := center(sheet.ColName(c), w)
		if m.showHandle(c) && w > 1 {
			// Draw the resize handle in the header's last cell.
			b.WriteString(style.Render(label[:len(label)-1]) + m.th.handle.Render("▐"))
			continue
		}
		b.WriteString(style.Render(label))
	}
	return b.String()
}

// showHandle reports whether column c's resize handle is visible: while
// hovering it or dragging it.
func (m *Model) showHandle(c int) bool {
	if m.drag == dragResize {
		return m.resizeCol == c
	}
	return m.hover.kind == hitColBorder && m.hover.addr.Col == c
}

func (m *Model) gridRow(row int) string {
	if row >= sheet.MaxRows {
		return ""
	}
	focus := m.active()
	sel, selecting := m.highlight()
	hdr := m.th.header
	switch {
	case row == focus.Row:
		hdr = m.th.headerActive
	case selecting && row >= sel.From.Row && row <= sel.To.Row:
		hdr = m.th.headerSel
	case m.hover.kind == hitRowHeader && m.hover.addr.Row == row:
		hdr = m.th.headerHover
	}
	var b strings.Builder
	b.WriteString(hdr.Render(padLeft(strconv.Itoa(row+1), rowHdrW-1) + " "))

	for i, text := range m.rowText(row) {
		a := sheet.Addr{Col: m.left + i, Row: row}
		if a == m.cur && (m.mode == modeEnter || m.mode == modeEdit) {
			text = m.inCellText(m.sheet.ColWidth(a.Col))
		}
		var style lipgloss.Style
		styled := true
		switch {
		case a == focus:
			style = m.th.pointer
		case selecting && sel.Contains(a):
			style = m.th.selection
		case m.sheet.Value(a).Kind == sheet.Error:
			style = m.th.errorCell
		default:
			styled = false
		}
		// The copy marker is layered on the cell's own colors.
		if m.copyMarked(a) {
			style, styled = style.Inherit(m.th.copied), true
		}
		if styled {
			text = style.Render(text)
		}
		b.WriteString(text)
	}
	return b.String()
}

// inCellText shows the entry being typed inside the cell, keeping the end
// of long entries visible, as Sheets does.
func (m *Model) inCellText(w int) string {
	text := " " + string(m.buf)
	if over := ansi.StringWidth(text) - w; over > 0 {
		text = ansi.TruncateLeft(text, over+1, "…")
	}
	return padRight(text, w)
}

// rowText returns the visible text of each column in row, each exactly the
// column's width. Text starts after one column of padding and overflows
// into blank cells to the right, including text that starts left of the
// viewport, as in Sheets.
func (m *Model) rowText(row int) []string {
	ncols := m.visibleCols(m.left)
	out := make([]string, ncols)

	var spill string
	for c := m.left - 1; c >= 0; c-- {
		cell := m.sheet.Cell(sheet.Addr{Col: c, Row: row})
		if cell == nil {
			continue
		}
		if cell.Value.Kind == sheet.Text {
			spill = " " + cell.Value.Str
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
		case cell != nil && cell.Value.Kind == sheet.Text:
			var head string
			head, spill = cut(" "+cell.Value.Str, w)
			out[i] = padRight(head, w)
		case cell != nil:
			spill = ""
			out[i] = padRight(sheet.FormatValue(cell.Value, w), w)
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

func (m *Model) statusLine() string {
	if m.mode == modeError {
		return m.th.error.Render(m.errMsg) + m.th.muted.Render("   press any key")
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
		right = m.keyHints("F1", "help", "F10", "menu", "Ctrl+Q", "quit")
	}
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// fmtStat formats a status line statistic with at most two decimals, as
// Sheets does.
func fmtStat(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func (m *Model) helpLines() []string {
	lines := []string{
		m.th.indicator.Render(" HELP ") + "  Keyboard shortcuts " + m.th.muted.Render("(press any key to return)"),
		"",
		"  Arrows              Move            Shift+arrows      Select",
		"  Ctrl+arrows         Jump to the edge of the data (add Shift to select)",
		"  Tab / Shift+Tab     Right / left    PgUp / PgDn       Screen up / down",
		"  Home / Ctrl+Home    Column A / A1   Ctrl+End          Last used cell",
		"  Mouse               Click, drag or Shift+click to select; click headers",
		"                      for whole columns or rows; double-click to edit",
		"",
	}
	// Shortcuts come from the keymap so help can't drift from behavior.
	var ids []string
	for id := range commands {
		if len(keysFor(id)) > 0 && id != "help" {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int { return strings.Compare(commands[a].title, commands[b].title) })
	entries := make([]string, len(ids))
	width := 0
	for i, id := range ids {
		keys := keysFor(id)
		for j, k := range keys {
			keys[j] = keyLabel(k)
		}
		entries[i] = fmt.Sprintf("%-22s%s", strings.Join(keys, " / "), commands[id].title)
		width = max(width, len(entries[i])+4)
	}
	lines = append(lines, columns(entries, width, m.width)...)
	names := make([]string, 0, len(sheet.Funcs()))
	for _, f := range sheet.Funcs() {
		names = append(names, f.Name)
	}
	lines = append(lines,
		"",
		"  Typing replaces the cell. Start with = for a formula, ' to force text.",
		"  While typing a formula, arrows after an operator pick cells.",
		"",
	)
	return append(lines, wrapWords("  Functions: ", names, m.width)...)
}

// columns lays out entries in two columns, top to bottom, when two of the
// given width fit on the screen, and in one column otherwise.
func columns(entries []string, width, screen int) []string {
	if 2+2*width > screen {
		out := make([]string, len(entries))
		for i, e := range entries {
			out[i] = "  " + e
		}
		return out
	}
	half := (len(entries) + 1) / 2
	out := make([]string, half)
	for i := range half {
		out[i] = "  " + entries[i]
		if j := i + half; j < len(entries) {
			out[i] = padRight(out[i], 2+width) + entries[j]
		}
	}
	return out
}

// wrapWords lays out words after prefix, wrapping to width and indenting
// continuation lines under the first word.
func wrapWords(prefix string, words []string, width int) []string {
	var lines []string
	cur := prefix
	indent := strings.Repeat(" ", len(prefix))
	for i, w := range words {
		if i > 0 && len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = indent + w
			continue
		}
		if i > 0 {
			cur += " "
		}
		cur += w
	}
	return append(lines, cur)
}

// keyLabel formats a key binding for display, e.g. "ctrl+s" -> "Ctrl+S".
func keyLabel(k string) string {
	switch k {
	case "delete":
		return "Del"
	case "backspace":
		return "Backspace"
	case "esc":
		return "Esc"
	}
	parts := strings.Split(k, "+")
	for i, p := range parts {
		switch {
		case len(p) == 1:
			parts[i] = strings.ToUpper(p)
		case p[0] == 'f' && len(p) <= 3 && p[1] >= '0' && p[1] <= '9':
			parts[i] = strings.ToUpper(p)
		default:
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
