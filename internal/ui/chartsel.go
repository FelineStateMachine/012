package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

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
		return m.runCommand("chart.edit")
	case "delete", "backspace":
		return m.runCommand("chart.delete")
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
		return s.pressAt(m, e)
	case mouseMotion:
		return s.motion(m, e)
	case mouseRelease:
		s.release(m)
	case mouseWheel:
		m.handleWheel(tea.Mouse{X: e.x, Y: e.y, Button: e.button})
	}
	return nil
}

// pressAt selects the chart pressed and starts dragging it, or opens its
// menu; pressing off the charts deselects and acts as usual.
func (s *chartSel) pressAt(m *Model, e mouseEvent) tea.Cmd {
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
	return nil
}

// motion moves or resizes the chart being dragged, or else sets the
// pointer shape for what's under the mouse.
func (s *chartSel) motion(m *Model, e mouseEvent) tea.Cmd {
	if s.drag == chartDragNone || s.preview == nil {
		return m.setShape(s.hoverShape(m, e.x, e.y))
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
	return nil
}

// hoverShape is the pointer shape at x, y: a move cursor over charts,
// and a resize one over the selected chart's corner.
func (s *chartSel) hoverShape(m *Model, x, y int) string {
	i := m.chartAt(x, y)
	if i < 0 {
		return "default"
	}
	if c := m.displayCharts()[i]; i == s.i {
		cx, cy := m.chartScreen(c)
		if x >= cx+c.W-2 && y == cy+c.H-1 {
			return "nwse-resize"
		}
	}
	return "move"
}

// release drops the chart being dragged where it is, as one undo step.
func (s *chartSel) release(m *Model) {
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
}

// chartCellAt returns the cell a chart's corner snaps to when dragged to
// screen position x, y, which may be past the grid's edges.
func (g *grid) chartCellAt(x, y int) sheet.Addr {
	a := sheet.Addr{Row: max(g.top+y-gridTop, 0), Col: g.left}
	switch {
	case x >= g.hdrW():
		if col, _, ok := g.colSpan(x); ok {
			a.Col = col
		} else {
			a.Col = g.left + g.visibleCols(g.left) - 1
		}
	default:
		for cx := g.hdrW(); cx > x && a.Col > 0; {
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

const (
	chartDragNone = iota
	chartDragMove
	chartDragResize
)
