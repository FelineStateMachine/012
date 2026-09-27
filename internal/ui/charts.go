package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/chart"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
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
		&command{id: "chart.select", macro: macroView, title: "Select chart", desc: "Select the next chart, to move, resize or delete it with the keyboard",
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

// chartState is what chart commands remember between selections.
type chartState struct {
	last int // the chart last selected, for chart commands; -1 for none
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
	if m.charts.last >= 0 && m.charts.last < len(charts) {
		return m.charts.last
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
	c.W = clamp(newChartW, sheet.MinChartW, max(m.width-m.hdrW()-2, sheet.MinChartW))
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
	m.charts.last = -1
	if _, ok := m.overlay.(*chartSel); ok {
		m.closeOverlay()
	}
	m.note = "Deleted the chart   " + m.th.KeyHints(m.shortcut("edit.undo"), "undo")
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
	if y < gridTop || y >= gridTop+m.visibleRows() || x < m.hdrW() {
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
func (m *Model) chartBoxes() []overlay.Box {
	var boxes []overlay.Box
	sel := m.selectedChart()
	top, bottom := gridTop, gridTop+m.visibleRows()
	for i, c := range m.displayCharts() {
		x, y := m.chartScreen(c)
		if x >= m.width || y >= bottom || x+c.W <= m.hdrW() || y+c.H <= top {
			continue
		}
		lines := m.drawChart(i, c, i == sel)
		// Clip to the grid: the headers and panel stay on top.
		from, to := max(m.hdrW()-x, 0), min(c.W, m.width-x)
		b := overlay.Box{ID: chartBoxID(i), X: x + from, Y: max(y, top)}
		for k, l := range lines {
			if y+k >= top && y+k < bottom {
				b.Lines = append(b.Lines, ansi.Cut(l, from, to))
			}
		}
		if len(b.Lines) > 0 && to > from {
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
	o := m.term.chartOptions()
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
		lines = append(lines, border.Render("│")+" "+chartRow(&m.th, g, y, firstImageID+i, o.Image)+" "+border.Render("│"))
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
func chartRow(th *theme.Theme, g *chart.Grid, y, imageID int, image bool) string {
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
			key, st = 1, th.ImageID(imageID)
			text = chart.Placeholder(y-g.Plot.Min.Y, x-g.Plot.Min.X)
		case c.Fg == chart.None && c.Bg == chart.None:
			key, isStyled = 2, false
		default:
			key = 3 + int(c.Fg)*32 + int(c.Bg)
			st = chartRole(th, c.Fg)
			if c.Bg >= chart.Series {
				st = st.Inherit(th.SeriesBg[int(c.Bg-chart.Series)%chart.Colors])
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

func chartRole(th *theme.Theme, r chart.Role) lipgloss.Style {
	switch {
	case r == chart.Axis:
		return th.ChartAxis
	case r == chart.Label:
		return th.ChartLabel
	case r == chart.Muted:
		return th.Muted
	case r >= chart.Series:
		return th.Series[int(r-chart.Series)%chart.Colors]
	}
	return th.Cell
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
	m.charts.last = i
	m.openOverlay(&chartSel{m: m, i: i})
}

// setShape sets the terminal's pointer shape, if it changed.
func (m *Model) setShape(shape string) tea.Cmd {
	if shape == m.mouse.shape {
		return nil
	}
	m.mouse.shape = shape
	return tea.Raw(ansi.SetPointerShape(shape))
}
