package ui

import (
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/tabstrip"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The control panel is three lines: the menu bar with the mode indicator,
// the formula bar, and the context line. Below the grid, the status line
// shows the file and selection statistics.

// View implements tea.Model.
func (m *Model) View() tea.View {
	if telemetry.Enabled() {
		defer m.timeFrame(time.Now())
	}
	clear(m.painted) // the theme may have changed since the last frame
	lines := []string{m.menuBarLine(), m.formulaBar(), m.contextLineText(), m.headerRow()}
	for _, b := range m.bands() {
		lines = m.appendBand(lines, b)
	}
	for len(lines) < gridTop+m.visibleRows() {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusLine())
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "")
	}
	m.drawBars(lines)

	content := strings.Join(lines, "\n")
	if boxes := m.floating(); len(boxes) > 0 || hasCharts(m) {
		content = m.compose(content, boxes)
	}
	content = m.fillScreen(content)
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion // hover feedback; see mouse.go
	v.ReportFocus = true                 // notifications only when the window is in the background
	v.WindowTitle = "012 - " + m.displayName()
	if m.jev.busy() != "" {
		// Terminals that support it (OSC 9;4) show activity in the tab.
		v.ProgressBar = tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
	}
	if bar := m.xfer.ProgressBar(); bar != nil {
		v.ProgressBar = bar
	}
	if m.changed {
		v.WindowTitle += " (modified)"
	}
	if x, y, ok := m.cursorPos(); ok {
		v.Cursor = tea.NewCursor(x, y)
		v.Cursor.Shape = tea.CursorBar
	}
	return v
}

// drawBars draws the menu bar, formula bar, context line, column header
// row and status line as full-width bands in the theme's bar roles,
// before overlays are composited over them.
func (m *Model) drawBars(lines []string) {
	bands := []struct {
		line int
		role lipgloss.Style
	}{
		{menuLine, m.th.MenuBarRow}, {formulaLine, m.th.FormulaBarRow}, {contextLine, m.th.ContextRow},
		{headerLine, m.th.ColumnHeaderRow}, {len(lines) - 1, m.th.StatusBarRow},
	}
	for _, b := range bands {
		if b.line < len(lines) {
			lines[b.line] = theme.Fill(lines[b.line], m.width, b.role)
		}
	}
}

// fillScreen draws everything else on the theme's background, out to the
// edges, when the theme has one of its own.
func (m *Model) fillScreen(content string) string {
	if isEmpty(m.th.Screen) {
		return content
	}
	lines := strings.Split(content, "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = theme.Fill(l, m.width, m.th.Screen)
	}
	return strings.Join(lines, "\n")
}

func isEmpty(s lipgloss.Style) bool {
	_, noBg := s.GetBackground().(lipgloss.NoColor)
	_, noFg := s.GetForeground().(lipgloss.NoColor)
	return noBg && noFg
}

// timeFrame reports a frame begun at start to telemetry, with how long
// ago the key press it answers came in.
func (m *Model) timeFrame(start time.Time) {
	var key time.Duration
	if !m.keyAt.IsZero() {
		key, m.keyAt = time.Since(m.keyAt), time.Time{}
	}
	telemetry.Frame(time.Since(start), key)
}

func (m *Model) displayName() string {
	if m.filename == "" && m.xfer.Source != "" {
		return filepath.Base(m.xfer.Source)
	}
	if m.filename == "" {
		return "untitled"
	}
	return filepath.Base(m.filename)
}

// indicator is the mode shown at the top right.
func (m *Model) indicator() string {
	switch {
	case m.xfer.Busy():
		return "WAIT"
	case m.macros.run != nil:
		return "CMD" // 1-2-3's indicator while a macro runs
	case m.overlay != nil:
		return m.overlay.Indicator()
	case m.mode == modePrompt:
		return m.prompt.indicator
	case m.vimActive() && m.visual() != visualNone:
		return "VISUAL"
	case m.vimActive():
		return "NORMAL"
	}
	return m.mode.String()
}

// menuBarLine is the menu bar with the mode indicator on the right.
func (m *Model) menuBarLine() string {
	left := m.menuBarTitles()
	ind := m.th.Indicator.Render(" " + m.indicator() + " ")
	if m.rec != nil {
		ind = m.th.Recording.Render(" REC ") + " " + ind
	}
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
	if m.away() {
		name = sheet.Qualified(m.entry.home.Name(), sheet.Rect{From: m.cur, To: m.cur})
	}
	if m.hasRange() && (m.mode == modeReady || m.mode == modeMenu) {
		name = m.selection().String()
	}
	if n, ok := m.namedSelection(); ok && (m.mode == modeReady || m.mode == modeMenu) {
		name = ansi.Truncate(n, nameBoxW-1, "…")
	}
	box := m.th.Header.Render(theme.PadRight(" "+ansi.Truncate(name, nameBoxW-1, "…"), nameBoxW)) + " "
	switch m.mode {
	case modeEnter, modeEdit:
		return box + m.line.Text()
	case modePoint:
		return box + m.entry.prefix + m.th.Selection.Render(m.pointRef()) + m.entry.suffix
	}
	if anchor, ok := m.sheet.SpillAnchor(m.cur); ok && m.sheet.HasSpills() {
		// A spilled cell shows the formula it spills from, dimmed, as
		// Sheets does.
		return box + m.th.Muted.Render(m.shownEntry(m.sheet.Cell(anchor).Input))
	}
	if c := m.sheet.Cell(m.cur); c != nil {
		return box + m.shownEntry(c.Input)
	}
	return box
}

// contextLineText says what's going on: a prompt, a formula error, or the
// keys that apply in the current mode.
func (m *Model) contextLineText() string {
	var left, right string
	switch {
	case m.xfer.Busy():
		left = m.xfer.Line(&m.th)
	case m.mouse.drag == dragResize:
		left = m.th.Key.Render("Column "+sheet.ColName(m.mouse.resizeCol)) + m.th.Muted.Render(" width ") +
			strconv.Itoa(m.sheet.ColWidth(m.mouse.resizeCol)) + m.th.Muted.Render("   double-click the border to fit")
	case m.mouse.drag == dragRowResize:
		h := m.shape(m.mouse.resizeRow).Lines
		left = m.th.Key.Render("Row "+strconv.Itoa(m.mouse.resizeRow+1)) + m.th.Muted.Render(" height ") +
			strconv.Itoa(h) + m.th.Muted.Render(plural(h, " line", " lines")+"   double-click the corner to fit")
	case m.mouse.drag == dragFill:
		left = m.fillLine()
	case m.entry.hint != "":
		left = m.th.Warning.Render(m.entry.hint)
	case m.macros.run != nil:
		left, right = "Running "+m.macros.run.name+"…", m.th.KeyHints("Esc", "stop")
	case m.vimActive() && (m.vim.pending() != "" || m.visual() != visualNone):
		left = m.vimLine()
	case m.mode == modeReady && m.trace != nil:
		left, right = m.trace.line(&m.th, m.width, m.sheet)
	case m.mode == modeReady:
		if left = m.readyLine(); left == "" {
			left = m.jev.line(&m.th, m.sheet.RemoteCalls(m.cur))
		}
		if left == "" {
			left = m.errorLine()
		}
		if left == "" {
			left = m.validationLine() // looks.go
		}
		if left == "" {
			left = m.noteLine()
		}
		if left == "" {
			left = m.spillLine()
		}
		if left == "" {
			left = m.recordingLine()
		}
	case m.mode == modeMenu:
		if o, ok := m.overlay.(overlay.Liner); ok {
			left, right = o.ContextLine()
		}
	case m.mode == modePrompt:
		left, right = m.prompt.line(m)
	case m.mode == modePoint:
		prefix := []rune(m.entry.prefix)
		var ok bool
		if left, right, ok = signatureLine(&m.th, m.width, m.storedFormula(prefix), len(prefix), m.locale().ArgSep(), m.th.KeyHints("Shift+arrows", "range", "Esc", "back")); !ok {
			left = m.th.KeyHints("Arrows", "pick a cell", "Shift+arrows", "pick a range", "Enter", "accept", "Esc", "back")
		}
	case (m.mode == modeEnter || m.mode == modeEdit) && m.line.IsFormula() && m.inFunction():
		left, right, _ = signatureLine(&m.th, m.width, m.storedFormula(m.line.Buf), m.line.Pos, m.locale().ArgSep(), m.th.KeyHints("Enter", "accept", "Esc", "cancel"))
	case m.mode == modeEnter && m.line.IsFormula():
		left = m.th.KeyHints("Enter", "accept", "Tab", "accept and go right", "Arrows", "pick cells after an operator", "Esc", "cancel")
	case m.mode == modeEnter:
		left = m.th.KeyHints("Enter", "accept", "Tab", "accept and go right", "Arrows", "accept and move", "Esc", "cancel")
	case m.mode == modeEdit:
		left = m.th.KeyHints("Enter", "accept", "Left/Right", "move the caret", "Esc", "cancel")
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

// cursorPos returns where the terminal cursor goes: in the formula bar
// while typing an entry, on the context line in a text prompt, or in an
// overlay's search field.
func (m *Model) cursorPos() (x, y int, ok bool) {
	if o, isText := m.overlay.(overlay.Text); isText {
		x, y = o.Cursor()
		return x, y, x >= 0
	}
	switch {
	case m.mode == modeEnter, m.mode == modeEdit:
		return formulaBarTextX() + ansi.StringWidth(m.line.Head()), formulaLine, true
	case m.mode == modePrompt && !m.pointing():
		return ansi.StringWidth(m.prompt.prefix() + m.prompt.head(m)), contextLine, true
	}
	return 0, 0, false
}

func (m *Model) statusLine() string {
	line, _ := m.statusLayout()
	return line
}

// statusLayout is the status line and where its sheet tabs are. From the
// left: the tabs, a divider, the file name and its state, then selection
// statistics or the ways in to everything else on the right. While a
// menu, picker or suggestion list is open, it says what the highlighted
// item does instead.
func (m *Model) statusLayout() (string, []tabstrip.Span) {
	if m.mode == modeError {
		return m.th.Error.Render(m.errMsg) + m.th.Muted.Render("   press any key"), nil
	}
	if m.xfer.Busy() {
		return m.spread(m.xfer.Status(&m.th, m.width)), nil
	}
	if line, ok := m.floatingStatus(); ok {
		return line, nil
	}
	return m.fileStatus()
}

// floatingStatus is what the highlighted item of the open overlay or the
// formula suggestions does, and the keys that apply.
func (m *Model) floatingStatus() (string, bool) {
	desc, keys, floating := m.entry.assist.status(m)
	if m.overlay != nil {
		desc, keys = m.overlay.Status()
		floating = true
	}
	if !floating || desc == "" && keys == "" {
		return "", false
	}
	if room := m.width - ansi.StringWidth(keys) - 3; room >= 12 {
		desc = ansi.Truncate(desc, room, "…")
	}
	return m.spread(desc, keys), true
}

// fileStatus is the tabs, the file and the right side. When space runs
// out, the right side gives up detail first, then the file name, then
// tabs scroll: first every tab is tried, then half the line of them, then
// just the one shown.
func (m *Model) fileStatus() (string, []tabstrip.Span) {
	v := m.tabView()
	state := m.statusState()
	infos := []string{m.displayName() + state, strings.TrimPrefix(state, "  ")}
	rights := m.statusRights()
	for _, need := range []int{v.FullWidth(), min(v.FullWidth(), m.width/2), v.MinWidth()} {
		for _, info := range infos {
			if info != "" {
				info = m.th.FrozenLine.Render(" │ ") + info // like a tmux pane border
			}
			if line, spans, ok := m.statusFits(v, need, info, rights); ok {
				return line, spans
			}
		}
	}
	return "", nil // not reached: the last choice has no right side or info
}

// statusFits lays out the status line with info after the tabs and the
// most detailed of rights that leaves the tabs need columns.
func (m *Model) statusFits(v tabstrip.View, need int, info string, rights []string) (string, []tabstrip.Span, bool) {
	for _, right := range rights {
		room := m.width - ansi.StringWidth(info)
		if right != "" {
			room -= ansi.StringWidth(right) + 3
		}
		if room < need && (right != "" || info != "") {
			continue
		}
		tabs, spans := m.tabs.Layout(&m.th, v, room)
		left := tabs + info
		gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
		return left + strings.Repeat(" ", gap) + right, spans, true
	}
	return "", nil, false
}

// statusState is what the status line says about the file after its
// name: modified, decimal arithmetic, a circular reference, rows hidden
// by the filter, JEV at work. Each part starts with two spaces.
func (m *Model) statusState() string {
	var b strings.Builder
	if m.changed {
		b.WriteString(m.th.Muted.Render("  modified"))
	}
	if m.book().Decimal() {
		b.WriteString(m.th.Muted.Render("  decimal"))
	}
	if m.book().Circular {
		b.WriteString("  " + m.th.Warning.Render("Circular reference"))
	}
	if n := m.sheet.HiddenRows(); n > 0 {
		b.WriteString("  " + m.th.Hint.Render("Filter hides "+rowCount(n)))
	}
	if busy := m.jev.busy(); busy != "" {
		b.WriteString("  " + m.th.Hint.Render(busy))
	}
	return b.String()
}

// statusRights are the choices for the right of the status line, most
// detailed first, ending with nothing: statistics of the selection, or
// the keys that open the palette, the shortcuts and the menu.
func (m *Model) statusRights() []string {
	var out []string
	if m.hasRange() && m.mode == modeReady {
		r := m.selection()
		st := m.sheet.RangeStats(r)
		rng := m.th.Key.Render(r.String())
		sum, avg := "", ""
		if st.Nums > 0 {
			sum = m.th.Muted.Render("Sum ") + m.fmtStat(st.Sum)
			avg = m.th.Muted.Render("Avg ") + m.fmtStat(st.Sum/float64(st.Nums))
		}
		count := m.th.Muted.Render("Count ") + strconv.Itoa(st.Count)
		// As many stats as fit: Avg goes first, then Sum, then Count.
		for _, parts := range [][]string{{rng, sum, avg, count}, {rng, sum, count}, {rng, count}, {rng}} {
			out = append(out, strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), "   "))
		}
		return append(out, "")
	}
	pairs := []string{m.shortcut("palette"), "search", m.shortcut("help"), "shortcuts", m.shortcut("menu"), "menu"}
	if m.prefs.vim {
		pairs = append([]string{":", "command"}, pairs...)
	}
	for ; len(pairs) > 0; pairs = pairs[:len(pairs)-2] {
		out = append(out, m.th.KeyHints(pairs...))
	}
	return append(out, "")
}

// fmtStat formats a status line statistic with at most two decimals, as
// Sheets does.
func (m *Model) fmtStat(v float64) string {
	return numfmt.Localize(strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64), m.locale())
}
