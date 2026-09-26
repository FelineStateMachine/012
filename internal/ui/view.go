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
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "one23 - " + m.displayName()
	if m.changed {
		v.WindowTitle += " (modified)"
	}
	if x, ok := m.cursorX(); ok {
		v.Cursor = tea.NewCursor(x, 1)
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
	case m.hint != "":
		return m.th.warning.Render(m.hint)
	case m.note != "" && m.mode == modeReady:
		return m.th.hint.Render(m.note)
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
		style := m.th.header
		switch {
		case c == focus.Col:
			style = m.th.headerActive
		case selecting && c >= sel.From.Col && c <= sel.To.Col:
			style = m.th.headerSel
		}
		b.WriteString(style.Render(center(sheet.ColName(c), m.sheet.ColWidth(c))))
	}
	return b.String()
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
	}
	var b strings.Builder
	b.WriteString(hdr.Render(padLeft(strconv.Itoa(row+1), rowHdrW-1) + " "))

	for i, sp := range m.rowText(row) {
		a := sheet.Addr{Col: m.left + i, Row: row}
		base, colored := m.th.cell, true
		switch {
		case a == focus:
			base = m.th.pointer
		case selecting && sel.Contains(a):
			base = m.th.selection
		case m.sheet.Value(a).Kind == sheet.Error:
			base = m.th.errorCell
		default:
			colored = false
		}
		if a == m.cur && (m.mode == modeEnter || m.mode == modeEdit) {
			sp = span{text: m.inCellText(m.sheet.ColWidth(a.Col))}
		}
		b.WriteString(m.renderSpan(sp, base, colored))
	}
	return b.String()
}

// span is what one grid column shows in a row: blank columns, the text,
// blank columns. Only the text takes the owning cell's text style, so an
// underline doesn't run into the padding.
type span struct {
	lead  int
	text  string
	trail int
	style sheet.Style
	owner int // column of the cell the text belongs to
}

// renderSpan draws a span on base, one of the cell roles. Plain cells
// with no text style are written without escape codes.
func (m *Model) renderSpan(sp span, base lipgloss.Style, colored bool) string {
	lead, trail := strings.Repeat(" ", sp.lead), strings.Repeat(" ", sp.trail)
	st := sp.style
	st.Align = sheet.AlignAuto
	switch {
	case st.IsZero() && !colored:
		return lead + sp.text + trail
	case st.IsZero():
		return base.Render(lead + sp.text + trail)
	case !colored:
		return lead + m.th.text(base, st).Render(sp.text) + trail
	}
	return base.Render(lead) + m.th.text(base, st).Render(sp.text) + base.Render(trail)
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

// rowText lays out the visible columns of row, each span exactly its
// column's width. Values are formatted with their cell's number format
// and aligned as Sheets does: numbers right, text left, booleans and
// errors centered, unless the cell sets an alignment. Text runs on into
// blank neighbors: to the right when left-aligned, to the left when
// right-aligned, both ways when centered. It can come from cells outside
// the viewport, so the scan starts at the nearest filled cell on each
// side.
func (m *Model) rowText(row int) []span {
	ncols := m.visibleCols(m.left)
	lo, hi := m.left, m.left+ncols-1
	out := make([]span, ncols)
	for i := range out {
		out[i] = span{trail: m.sheet.ColWidth(lo + i)}
	}
	content := func(c int) *sheet.Cell {
		if c := m.sheet.Cell(sheet.Addr{Col: c, Row: row}); !c.Blank() {
			return c
		}
		return nil
	}
	first, last := lo, hi
	for c := lo - 1; c >= 0; c-- {
		if content(c) != nil {
			first = c
			break
		}
	}
	for c := hi + 1; c < sheet.MaxCols; c++ {
		if content(c) != nil {
			last = c
			break
		}
	}
	// x[k] is where column first+k starts, relative to column first.
	x := make([]int, last-first+2)
	for c := first; c <= last; c++ {
		x[c-first+1] = x[c-first] + m.sheet.ColWidth(c)
	}
	col := func(c int) (int, int) { return x[c-first], x[c-first+1] }

	claimed := 0 // text of earlier cells reaches up to here
	for c := first; c <= last; c++ {
		cell := content(c)
		if cell == nil {
			continue
		}
		x0, x1 := col(c)
		f := m.sheet.DisplayFormat(sheet.Addr{Col: c, Row: row})
		text, align := sheet.Display(cell.Value, f, x1-x0)
		pad := 1
		// A number one character too wide (12/31/2026 in a default
		// column) may use the padding when nothing is to its right,
		// rather than turning into #s.
		if cell.Value.Kind == sheet.Number && strings.Trim(text, "#") == "" && content(c+1) == nil {
			if wider, _ := sheet.Display(cell.Value, f, x1-x0+1); strings.Trim(wider, "#") != "" {
				text, pad = wider, 0
			}
		}
		if a := cell.Style.Align; a != sheet.AlignAuto && align != sheet.AlignFill {
			align = a
		}
		tw := ansi.StringWidth(text)
		start := x0 + pad
		switch align {
		case sheet.AlignFill:
			start = x0
		case sheet.AlignRight:
			start = x1 - pad - tw
		case sheet.AlignCenter:
			start = x0 + (x1-x0-tw)/2
		}
		from, to := max(x0, claimed), x1 // where this cell's text may go
		if cell.Value.Kind == sheet.Text {
			for k := c + 1; k <= last && start+tw > to && content(k) == nil; k++ {
				_, to = col(k)
			}
			for k := c - 1; k >= first && start < from && content(k) == nil; k-- {
				if kx0, _ := col(k); kx0 >= claimed {
					from = kx0
				} else {
					from = claimed
					break
				}
			}
		}
		claimed = to
		for k := max(first, lo); k <= min(last, hi); k++ {
			kx0, kx1 := col(k)
			if kx1 <= from || kx0 >= to {
				continue
			}
			sp := span{trail: kx1 - kx0, style: cell.Style, owner: c}
			if seg0, seg1 := max(start, kx0, from), min(start+tw, kx1, to); seg1 > seg0 {
				sp.lead, sp.text, sp.trail = seg0-kx0, ansi.Cut(text, seg0-start, seg1-start), kx1-seg1
			}
			out[k-lo] = sp
		}
	}
	// Text that runs to the edge of its column would touch a neighbor
	// that starts at its own edge ("Groceries9/28/2026"); keep a gap.
	for i := 0; i+1 < len(out); i++ {
		l, r := &out[i], &out[i+1]
		if l.text != "" && l.trail == 0 && r.text != "" && r.lead == 0 && l.owner != r.owner {
			l.text = ansi.Truncate(l.text, ansi.StringWidth(l.text)-1, "")
			l.trail = 1
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
	for _, id := range ids {
		keys := keysFor(id)
		for i, k := range keys {
			keys[i] = keyLabel(k)
		}
		lines = append(lines, fmt.Sprintf("  %-20s%s", strings.Join(keys, " / "), commands[id].desc))
	}
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
