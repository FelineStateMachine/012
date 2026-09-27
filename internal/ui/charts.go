package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/chart"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Charts follow Sheets: Insert > Chart charts the selection (or the block
// of data around the active cell) and opens the chart editor, a bar on the
// context line for the type, series direction, headers, range and title.
// Charts float over the grid anchored at a cell, redraw as their data
// recalculates, and can be dragged, resized from the corner, and deleted
// with Del while selected. On terminals with kitty graphics the plot is an
// image (graphics.go); labels, legend and frame are always text.

// chartBoxID is the id of chart i's box in hit tests.
func chartBoxID(i int) string { return "chart" + strconv.Itoa(i) }

// Default size of a new chart, in cells; smaller screens get smaller
// charts.
const newChartW, newChartH = 48, 16

func init() {
	register(
		&command{id: "insert.chart", title: "Chart", desc: "Chart the selected data, or the table around the active cell", run: (*Model).insertChart},
		&command{id: "chart.edit", title: "Edit chart", desc: "Change a chart's type, data and labels",
			enabled: hasCharts, run: func(m *Model) tea.Cmd {
				m.openChartEditor(m.targetChart(), false, m.sheet.StateID())
				return nil
			}},
		&command{id: "chart.delete", title: "Delete chart", desc: "Remove a chart from the sheet",
			enabled: hasCharts, run: func(m *Model) tea.Cmd {
				m.deleteChart(m.targetChart())
				return nil
			}},
		&command{id: "chart.select", title: "Select chart", desc: "Select the next chart, to move, resize or delete it with the keyboard",
			enabled: hasCharts, run: func(m *Model) tea.Cmd {
				i := 0
				if s, ok := m.overlay.(*chartSel); ok {
					i = (s.i + 1) % len(m.sheet.Charts())
				}
				m.selectChart(i)
				m.showChart(i)
				return nil
			}},
	)
}

func hasCharts(m *Model) bool { return len(m.sheet.Charts()) > 0 }

// targetChart is the chart a chart command acts on: the selected one, the
// last one selected, one charting the active cell, or the newest.
func (m *Model) targetChart() int {
	charts := m.sheet.Charts()
	switch o := m.overlay.(type) {
	case *chartSel:
		return o.i
	case *chartEditor:
		return o.i
	}
	if m.lastChart >= 0 && m.lastChart < len(charts) {
		return m.lastChart
	}
	for i, c := range charts {
		if c.Data.Contains(m.cur) {
			return i
		}
	}
	return len(charts) - 1
}

// insertChart charts the selection and opens the editor.
func (m *Model) insertChart() tea.Cmd {
	r := m.selection()
	if !m.hasRange() {
		r = m.sheet.Region(m.cur)
	}
	if used, ok := m.sheet.UsedRange(); !ok || r.From.Col > used.To.Col || r.From.Row > used.To.Row {
		m.note = "Select the data to chart first"
		return nil
	}
	start := m.sheet.StateID()
	c := m.sheet.GuessChart(r)
	c.W = clamp(newChartW, sheet.MinChartW, max(m.width-rowHdrW-2, sheet.MinChartW))
	c.H = clamp(newChartH, sheet.MinChartH, max(m.visibleRows()-1, sheet.MinChartH))
	// Next to the data when it fits on screen, as Sheets does; otherwise
	// below it.
	c.At = sheet.Addr{Col: c.Data.To.Col + 1, Row: c.Data.From.Row}
	if m.colStart(c.At.Col)+c.W > m.width {
		c.At = sheet.Addr{Col: c.Data.From.Col, Row: c.Data.To.Row + 2}
	}
	i := m.sheet.AddChart(c)
	m.changed = true
	m.clearSelection()
	m.showChart(i)
	m.openChartEditor(i, true, start)
	return nil
}

// showChart scrolls so chart i is on screen, as far as it fits.
func (g *grid) showChart(i int) {
	charts := g.sheet.Charts()
	if i < 0 || i >= len(charts) {
		return
	}
	c := charts[i]
	rows := g.visibleRows()
	if c.At.Row < g.top || c.At.Row+c.H > g.top+rows {
		g.top = clamp(c.At.Row+c.H-rows, 0, c.At.Row)
	}
	if c.At.Col < g.left || g.colStart(c.At.Col)+c.W > g.width {
		g.left = c.At.Col
		for g.left > 0 && g.colStart(c.At.Col)+c.W+g.sheet.ColWidth(g.left-1) <= g.width {
			g.left--
		}
	}
}

func (m *Model) deleteChart(i int) {
	if i < 0 || i >= len(m.sheet.Charts()) {
		return
	}
	m.sheet.DeleteChart(i)
	m.changed = true
	m.lastChart = -1
	if _, ok := m.overlay.(*chartSel); ok {
		m.closeOverlay()
	}
	m.note = "Deleted the chart   " + m.th.KeyHints(shortcut("edit.undo"), "undo")
}

// displayCharts are the charts as drawn: with the one being dragged at
// its live position.
func (m *Model) displayCharts() []sheet.Chart {
	charts := m.sheet.Charts()
	if s, ok := m.overlay.(*chartSel); ok && s.preview != nil && s.i < len(charts) {
		charts[s.i] = *s.preview
	}
	return charts
}

// chartInner is the size of a chart's contents inside its frame and one
// column of padding each side.
func chartInner(c sheet.Chart) (w, h int) { return c.W - 4, c.H - 2 }

// chartScreen returns where chart c's top-left corner is on screen; it
// may be off screen.
func (g *grid) chartScreen(c sheet.Chart) (x, y int) {
	return g.colStart(c.At.Col), gridTop + c.At.Row - g.top
}

// chartAt returns the topmost chart drawn under x, y, or -1.
func (m *Model) chartAt(x, y int) int {
	if y < gridTop || y >= gridTop+m.visibleRows() || x < rowHdrW {
		return -1
	}
	charts := m.displayCharts()
	for i := len(charts) - 1; i >= 0; i-- {
		cx, cy := m.chartScreen(charts[i])
		if x >= cx && x < cx+charts[i].W && y >= cy && y < cy+charts[i].H {
			return i
		}
	}
	return -1
}

// selectedChart is the index of the selected chart, or -1.
func (m *Model) selectedChart() int {
	switch o := m.overlay.(type) {
	case *chartSel:
		return o.i
	case *chartEditor:
		return o.i
	}
	return -1
}

// chartBoxes draws the charts on screen, clipped to the grid, bottom
// first.
func (m *Model) chartBoxes() []box {
	var boxes []box
	sel := m.selectedChart()
	top, bottom := gridTop, gridTop+m.visibleRows()
	for i, c := range m.displayCharts() {
		x, y := m.chartScreen(c)
		if x >= m.width || y >= bottom || x+c.W <= rowHdrW || y+c.H <= top {
			continue
		}
		lines := m.drawChart(i, c, i == sel)
		// Clip to the grid: the headers and panel stay on top.
		from, to := max(rowHdrW-x, 0), min(c.W, m.width-x)
		b := box{id: chartBoxID(i), x: x + from, y: max(y, top)}
		for k, l := range lines {
			if y+k >= top && y+k < bottom {
				b.lines = append(b.lines, ansi.Cut(l, from, to))
			}
		}
		if len(b.lines) > 0 && to > from {
			boxes = append(boxes, b)
		}
	}
	return boxes
}

// drawChart renders chart i in its frame: the title in the top border,
// the data range in the bottom one, and when selected a highlighted
// border with a resize handle in the corner.
func (m *Model) drawChart(i int, c sheet.Chart, selected bool) []string {
	w, h := chartInner(c)
	o := m.chartOptions()
	if i >= maxImages {
		o.Image = false
	}
	g := chart.Draw(c.Type, m.sheet.ChartData(c), w, h, o)
	border := m.th.ChartFrame
	if selected {
		border = m.th.ChartSelected
	}
	title := c.Title
	if title == "" {
		title = c.Type.Title() + " chart"
	}
	title = ansi.Truncate(" "+title+" ", c.W-4, "…")
	footer := " " + c.Data.String() + " "
	lines := make([]string, 0, c.H)
	lines = append(lines, border.Render("┌─")+m.th.Title.Render(title)+
		border.Render(strings.Repeat("─", max(c.W-3-ansi.StringWidth(title), 0))+"┐"))
	for y := range h {
		lines = append(lines, border.Render("│")+" "+m.chartRow(g, y, firstImageID+i, o.Image)+" "+border.Render("│"))
	}
	corner := "┘"
	if selected {
		corner = "◢"
	}
	fill := c.W - 2 - ansi.StringWidth(footer) - 1
	if fill < 1 {
		lines = append(lines, border.Render("└"+strings.Repeat("─", c.W-2)+corner))
	} else {
		lines = append(lines, border.Render("└"+strings.Repeat("─", fill))+m.th.Muted.Render(footer)+border.Render("─"+corner))
	}
	return lines
}

// chartRow renders row y of a drawn chart with the theme; with images,
// the plot area is the image's placeholders.
func (m *Model) chartRow(g *chart.Grid, y, imageID int, image bool) string {
	var b, run strings.Builder
	var style lipgloss.Style
	styled := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if styled {
			b.WriteString(style.Render(run.String()))
		} else {
			b.WriteString(run.String())
		}
		run.Reset()
	}
	prev := -1
	for x := range g.W {
		c := g.At(x, y)
		key, st, isStyled := 0, lipgloss.Style{}, true
		text := c.Text
		switch {
		case image && y >= g.Plot.Min.Y && y < g.Plot.Max.Y && x >= g.Plot.Min.X && x < g.Plot.Max.X:
			key, st = 1, m.th.ImageID(imageID)
			text = chart.Placeholder(y-g.Plot.Min.Y, x-g.Plot.Min.X)
		case c.Fg == chart.None && c.Bg == chart.None:
			key, isStyled = 2, false
		default:
			key = 3 + int(c.Fg)*32 + int(c.Bg)
			st = m.chartRole(c.Fg)
			if c.Bg >= chart.Series {
				st = st.Inherit(m.th.SeriesBg[int(c.Bg-chart.Series)%chart.Colors])
			}
		}
		if key != prev {
			flush()
			prev, style, styled = key, st, isStyled
		}
		run.WriteString(text)
	}
	flush()
	return b.String()
}

func (m *Model) chartRole(r chart.Role) lipgloss.Style {
	switch {
	case r == chart.Axis:
		return m.th.ChartAxis
	case r == chart.Label:
		return m.th.ChartLabel
	case r == chart.Muted:
		return m.th.Muted
	case r >= chart.Series:
		return m.th.Series[int(r-chart.Series)%chart.Colors]
	}
	return m.th.Cell
}

// chartClick handles a press on the grid in READY: pressing a chart
// selects it and starts dragging it, and right-clicking opens its menu.
func (m *Model) chartClick(mouse tea.Mouse) (tea.Cmd, bool) {
	i := m.chartAt(mouse.X, mouse.Y)
	if i < 0 {
		return nil, false
	}
	m.selectChart(i)
	s := m.overlay.(*chartSel)
	if mouse.Button == tea.MouseRight {
		m.showContextMenu(chartMenu, mouse.X, mouse.Y+1)
		return nil, true
	}
	s.press(m, mouse.X, mouse.Y)
	return nil, true
}

var chartMenu = []menuItem{{cmd: "chart.edit"}, {cmd: "chart.delete"}}

// selectChart selects chart i.
func (m *Model) selectChart(i int) {
	m.clearSelection()
	m.lastChart = i
	m.openOverlay(&chartSel{i: i})
}

// setShape sets the terminal's pointer shape, if it changed.
func (m *Model) setShape(shape string) tea.Cmd {
	if shape == m.shape {
		return nil
	}
	m.shape = shape
	return tea.Raw(ansi.SetPointerShape(shape))
}

// chartSel is a selected chart. Arrows move it a cell at a time, Shift
// and arrows resize it, Enter edits it and Del deletes it; the mouse
// drags it or its corner. Other keys deselect it and act as usual.
type chartSel struct {
	i       int
	drag    int // chartDragNone, chartDragMove or chartDragResize
	grabX   int // where the chart was grabbed, relative to its corner
	grabY   int
	preview *sheet.Chart // the chart while being dragged
}

const (
	chartDragNone = iota
	chartDragMove
	chartDragResize
)

func (s *chartSel) indicator() string { return "CHART" }
func (s *chartSel) layout(*Model) []box {
	return nil // drawn with the other charts, see chartBoxes
}

func (s *chartSel) chart(m *Model) (sheet.Chart, bool) {
	charts := m.sheet.Charts()
	if s.i < 0 || s.i >= len(charts) {
		return sheet.Chart{}, false
	}
	return charts[s.i], true
}

func (s *chartSel) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	c, ok := s.chart(m)
	if !ok {
		m.closeOverlay()
		return nil
	}
	moved := c
	switch k.String() {
	case "esc":
		m.closeOverlay()
		return nil
	case "enter":
		m.openChartEditor(s.i, false, m.sheet.StateID())
		return nil
	case "delete", "backspace":
		m.deleteChart(s.i)
		return nil
	case "tab":
		m.selectChart((s.i + 1) % len(m.sheet.Charts()))
		return nil
	case "up":
		moved.At.Row--
	case "down":
		moved.At.Row++
	case "left":
		moved.At.Col--
	case "right":
		moved.At.Col++
	case "shift+up":
		moved.H--
	case "shift+down":
		moved.H++
	case "shift+left":
		moved.W -= 2
	case "shift+right":
		moved.W += 2
	default:
		m.closeOverlay()
		return m.handleKey(k)
	}
	label := "move chart"
	if moved.W != c.W || moved.H != c.H {
		label = "resize chart"
	}
	m.sheet.SetChart(s.i, moved, label)
	m.changed = true
	m.showChart(s.i)
	return nil
}

// press starts a drag at x, y: the corner resizes, anywhere else moves.
func (s *chartSel) press(m *Model, x, y int) {
	c, ok := s.chart(m)
	if !ok {
		return
	}
	cx, cy := m.chartScreen(c)
	s.grabX, s.grabY = x-cx, y-cy
	s.drag = chartDragMove
	if x >= cx+c.W-2 && y == cy+c.H-1 {
		s.drag = chartDragResize
	}
	s.preview = &c
}

func (s *chartSel) mouse(m *Model, e mouseEvent) tea.Cmd {
	switch e.kind {
	case mousePress:
		i := m.chartAt(e.x, e.y)
		switch {
		case i < 0:
			m.closeOverlay()
			return m.handlePress(tea.Mouse{X: e.x, Y: e.y, Button: e.button})
		case e.button == tea.MouseRight:
			m.selectChart(i)
			m.showContextMenu(chartMenu, e.x, e.y+1)
		case e.button == tea.MouseLeft:
			if i != s.i {
				m.selectChart(i)
				s = m.overlay.(*chartSel)
			}
			s.press(m, e.x, e.y)
		}
	case mouseMotion:
		if s.drag == chartDragNone || s.preview == nil {
			shape := "default"
			if i := m.chartAt(e.x, e.y); i >= 0 {
				shape = "move"
				if c := m.displayCharts()[i]; i == s.i {
					cx, cy := m.chartScreen(c)
					if e.x >= cx+c.W-2 && e.y == cy+c.H-1 {
						shape = "nwse-resize"
					}
				}
			}
			return m.setShape(shape)
		}
		p := *s.preview
		if s.drag == chartDragResize {
			cx, cy := m.chartScreen(p)
			p.W = clamp(e.x-cx+1, sheet.MinChartW, sheet.MaxChartW)
			p.H = clamp(e.y-cy+1, sheet.MinChartH, sheet.MaxChartH)
		} else {
			p.At = m.chartCellAt(e.x-s.grabX, e.y-s.grabY)
		}
		s.preview = &p
	case mouseRelease:
		if s.preview != nil && s.drag != chartDragNone {
			label := "move chart"
			if s.drag == chartDragResize {
				label = "resize chart"
			}
			before := m.sheet.StateID()
			m.sheet.SetChart(s.i, *s.preview, label)
			if m.sheet.StateID() != before {
				m.changed = true
			}
		}
		s.drag, s.preview = chartDragNone, nil
	case mouseWheel:
		m.handleWheel(tea.Mouse{X: e.x, Y: e.y, Button: e.button})
	}
	return nil
}

// chartCellAt returns the cell a chart's corner snaps to when dragged to
// screen position x, y, which may be past the grid's edges.
func (g *grid) chartCellAt(x, y int) sheet.Addr {
	a := sheet.Addr{Row: max(g.top+y-gridTop, 0), Col: g.left}
	switch {
	case x >= rowHdrW:
		if col, _, ok := g.colSpan(x); ok {
			a.Col = col
		} else {
			a.Col = g.left + g.visibleCols(g.left) - 1
		}
	default:
		for cx := rowHdrW; cx > x && a.Col > 0; {
			a.Col--
			cx -= g.sheet.ColWidth(a.Col)
		}
	}
	return clampAddr(a)
}

func (s *chartSel) status(m *Model) (string, string) {
	c, ok := s.chart(m)
	if !ok {
		return "", ""
	}
	desc := m.th.Muted.Render("Chart of " + c.Data.String())
	pairs := []string{"Enter", "edit", "Del", "delete", "Arrows", "move", "Shift+arrows", "resize", "Esc", "done"}
	for {
		keys := m.th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= m.width:
			return desc, keys
		case desc != "":
			desc = ""
		case len(pairs) > 4:
			pairs = append(pairs[:len(pairs)-4], pairs[len(pairs)-2:]...) // keep Esc
		default:
			return "", keys
		}
	}
}

// contextLine says what's selected and how to change it.
func (s *chartSel) contextLine(m *Model) (string, string) {
	c, ok := s.chart(m)
	if !ok {
		return "", ""
	}
	return m.th.Key.Render(c.Type.Title()+" chart") + m.th.Muted.Render(" of ") + c.Data.String() +
		m.th.Muted.Render("   drag to move, drag the corner to resize"), ""
}

// chartEditor is the chart editor: a bar on the context line, in the
// spirit of the find bar, with the type as a row of chips and the options
// as toggles. Changes show at once; Enter keeps them and Esc undoes them,
// removing a chart that was just inserted.
type chartEditor struct {
	i     int
	start int // the sheet's state before editing, to undo back to
	isNew bool
}

// openChartEditor edits chart i. start is the sheet's state to return to
// on Esc.
func (m *Model) openChartEditor(i int, isNew bool, start int) {
	if i < 0 || i >= len(m.sheet.Charts()) {
		return
	}
	m.lastChart = i
	m.openOverlay(&chartEditor{i: i, start: start, isNew: isNew})
}

func (e *chartEditor) indicator() string   { return "CHART" }
func (e *chartEditor) layout(*Model) []box { return nil }

func (e *chartEditor) chart(m *Model) sheet.Chart { return m.sheet.Charts()[e.i] }

// set applies a change to the chart as its own undo step.
func (e *chartEditor) set(m *Model, fn func(c *sheet.Chart)) {
	c := e.chart(m)
	fn(&c)
	m.sheet.SetChart(e.i, c, "edit chart")
	m.changed = m.sheet.StateID() != m.saved
}

// Editor keys, also the toggles' mouse targets.
const (
	editorSwitch = "s"
	editorHeader = "h"
	editorLabels = "l"
	editorRange  = "r"
	editorTitle  = "t"
)

func (e *chartEditor) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	if e.i >= len(m.sheet.Charts()) {
		m.closeOverlay()
		return nil
	}
	switch key := strings.ToLower(k.String()); key {
	case "enter":
		m.selectChart(e.i)
		if e.isNew {
			m.note = "Inserted a chart of " + e.chart(m).Data.String()
		}
	case "esc":
		for m.sheet.StateID() != e.start && m.sheet.CanUndo() {
			m.sheet.Undo()
		}
		m.changed = m.sheet.StateID() != m.saved
		m.closeOverlay()
		m.lastChart = -1
	case "left", "right", "shift+tab", "tab":
		d := 1
		if key == "left" || key == "shift+tab" {
			d = -1
		}
		e.set(m, func(c *sheet.Chart) {
			n := len(sheet.ChartTypes)
			c.Type = sheet.ChartTypes[((int(c.Type)+d)%n+n)%n]
		})
	case "1", "2", "3", "4":
		e.set(m, func(c *sheet.Chart) { c.Type = sheet.ChartTypes[key[0]-'1'] })
	case editorSwitch:
		e.set(m, func(c *sheet.Chart) { c.ByRow = !c.ByRow })
	case editorHeader:
		e.set(m, func(c *sheet.Chart) { c.Header = !c.Header })
	case editorLabels:
		e.set(m, func(c *sheet.Chart) { c.Labels = !c.Labels })
	case editorRange:
		e.editRange(m)
	case editorTitle:
		e.editTitle(m)
	}
	return nil
}

// reopen returns to the editor after a prompt.
func (e *chartEditor) reopen(m *Model) {
	m.openOverlay(e)
}

// editRange asks for the data range by pointing, starting from the
// current one.
func (e *chartEditor) editRange(m *Model) {
	d := e.chart(m).Data
	m.closeOverlay()
	m.cur, m.ext, m.selecting, m.whole = d.From, d.To, true, wholeNone
	m.openRange("Chart data:", func(m *Model, r sheet.Rect) tea.Cmd {
		m.clearSelection()
		e.set(m, func(c *sheet.Chart) { c.Data = r })
		e.reopen(m)
		return nil
	})
	m.prompt.onCancel = func(m *Model) {
		m.clearSelection()
		e.reopen(m)
	}
}

func (e *chartEditor) editTitle(m *Model) {
	title := e.chart(m).Title
	m.closeOverlay()
	m.openText("Chart title:", title, func(m *Model, text string) tea.Cmd {
		e.set(m, func(c *sheet.Chart) { c.Title = text })
		e.reopen(m)
		return nil
	})
	m.prompt.onCancel = e.reopen
}

// editorPart is a piece of the editor bar: a type chip or a toggle.
type editorPart struct {
	text string
	key  string // the key it stands for
}

func (e *chartEditor) parts(m *Model) []editorPart {
	c := e.chart(m)
	chip := func(on bool, name, key string) editorPart {
		style := m.th.Muted
		if on {
			style = m.th.MenuSelected
		}
		return editorPart{text: style.Render(" " + name + " "), key: key}
	}
	var parts []editorPart
	for i, t := range sheet.ChartTypes {
		parts = append(parts, chip(t == c.Type, t.Title(), strconv.Itoa(i+1)))
	}
	series := "Series in columns"
	if c.ByRow {
		series = "Series in rows"
	}
	header, labels := "Header row", "Labels in column"
	if c.ByRow {
		header, labels = "Header column", "Labels in row"
	}
	return append(parts,
		editorPart{text: m.th.Key.Render(c.Data.String()), key: editorRange},
		chip(true, series, editorSwitch),
		chip(c.Header, header, editorHeader),
		chip(c.Labels, labels, editorLabels))
}

// spans lays the parts out on the context line, dropping options from the
// right on narrow screens, and returns each part's x.
func (e *chartEditor) spans(m *Model) ([]editorPart, []int) {
	parts := e.parts(m)
	for {
		xs := make([]int, len(parts))
		x := 0
		for i, p := range parts {
			if i == len(sheet.ChartTypes) {
				x += 2 // a wider gap between the types and the options
			}
			xs[i] = x
			x += ansi.StringWidth(p.text) + 1
		}
		if x-1 <= m.width || len(parts) <= len(sheet.ChartTypes) {
			return parts, xs
		}
		parts = parts[:len(parts)-1]
	}
}

func (e *chartEditor) contextLine(m *Model) (string, string) {
	if e.i >= len(m.sheet.Charts()) {
		return "", ""
	}
	parts, xs := e.spans(m)
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(strings.Repeat(" ", xs[i]-ansi.StringWidth(b.String())))
		b.WriteString(p.text)
	}
	return b.String(), ""
}

func (e *chartEditor) mouse(m *Model, ev mouseEvent) tea.Cmd {
	if ev.kind != mousePress {
		return nil
	}
	if ev.y != contextLine {
		// A click elsewhere keeps the changes and acts as usual.
		m.closeOverlay()
		return m.handlePress(tea.Mouse{X: ev.x, Y: ev.y, Button: ev.button})
	}
	parts, xs := e.spans(m)
	for i, p := range parts {
		if ev.x >= xs[i] && ev.x < xs[i]+ansi.StringWidth(p.text) {
			return e.key(m, tea.KeyPressMsg{Code: rune(p.key[0]), Text: p.key})
		}
	}
	return nil
}

func (e *chartEditor) status(m *Model) (string, string) {
	pairs := []string{"←/→", "type", "S", "switch rows/columns", "H", "header", "L", "labels", "R", "range", "T", "title", "Enter", "done", "Esc", "cancel"}
	for {
		keys := m.th.KeyHints(pairs...)
		if ansi.StringWidth(keys) <= m.width || len(pairs) <= 4 {
			return "", keys
		}
		pairs = append(pairs[:len(pairs)-6], pairs[len(pairs)-4:]...) // keep Enter and Esc
	}
}

// contextLiner is an overlay drawn on the context line, like the chart
// editor.
type contextLiner interface {
	contextLine(m *Model) (left, right string)
}
