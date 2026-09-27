package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// chartSel is a selected chart. Arrows move it a cell at a time, Shift
// and arrows resize it, Enter edits it and Del deletes it; the mouse
// drags it or its corner. Other keys deselect it and act as usual.
type chartSel struct {
	m       chartHost // the model, through what a selected chart needs of it
	i       int
	drag    int // chartDragNone, chartDragMove or chartDragResize
	grabX   int // where the chart was grabbed, relative to its corner
	grabY   int
	preview *sheet.Chart // the chart while being dragged
}

func (s *chartSel) Indicator() string { return "CHART" }

func (s *chartSel) Layout() []overlay.Box {
	return nil // drawn with the other charts, see chartBoxes
}

func (s *chartSel) chart() (sheet.Chart, bool) {
	charts := s.m.sheetShown().Charts()
	if s.i < 0 || s.i >= len(charts) {
		return sheet.Chart{}, false
	}
	return charts[s.i], true
}

// shown is the chart as drawn: where it's being dragged, if it is.
func (s *chartSel) shown() (sheet.Chart, bool) {
	if s.preview != nil {
		return *s.preview, true
	}
	return s.chart()
}

func (s *chartSel) Key(k tea.KeyPressMsg) tea.Cmd {
	m := s.m
	c, ok := s.chart()
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
		m.selectChart((s.i + 1) % len(m.sheetShown().Charts()))
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
		return m.passKey(k)
	}
	label := "move chart"
	if moved.W != c.W || moved.H != c.H {
		label = "resize chart"
	}
	s.place(c, moved, label)
	m.showChart(s.i)
	return nil
}

// place puts the chart at to, from where it was, as one undo step, and
// records it.
func (s *chartSel) place(from, to sheet.Chart, label string) {
	sh := s.m.sheetShown()
	sh.SetChart(s.i, to, label)
	s.m.syncChanged()
	if now, ok := s.chart(); ok {
		s.m.recordChart("chart.edit", s.i, from, now)
	}
}

// press starts a drag at x, y: the corner resizes, anywhere else moves.
func (s *chartSel) press(x, y int) {
	c, ok := s.chart()
	if !ok {
		return
	}
	cx, cy := s.m.chartScreen(c)
	s.grabX, s.grabY = x-cx, y-cy
	s.drag = chartDragMove
	if x >= cx+c.W-2 && y == cy+c.H-1 {
		s.drag = chartDragResize
	}
	s.preview = &c
}

func (s *chartSel) Mouse(e overlay.MouseEvent) tea.Cmd {
	switch e.Kind {
	case overlay.MousePress:
		return s.pressAt(e)
	case overlay.MouseMotion:
		return s.motion(e)
	case overlay.MouseRelease:
		s.release()
	case overlay.MouseWheel:
		return s.m.passMouse(e)
	}
	return nil
}

// pressAt selects the chart pressed and starts dragging it, or opens its
// menu; pressing off the charts deselects and acts as usual.
func (s *chartSel) pressAt(e overlay.MouseEvent) tea.Cmd {
	m := s.m
	i := m.chartAt(e.X, e.Y)
	switch {
	case i < 0:
		m.closeOverlay()
		return m.passMouse(e)
	case e.Button == tea.MouseRight:
		m.selectChart(i)
		m.showChartMenu(e.X, e.Y+1)
	case e.Button == tea.MouseLeft:
		if i != s.i {
			s = m.selectChart(i)
		}
		s.press(e.X, e.Y)
	}
	return nil
}

// motion moves or resizes the chart being dragged, or else sets the
// pointer shape for what's under the mouse.
func (s *chartSel) motion(e overlay.MouseEvent) tea.Cmd {
	m := s.m
	if s.drag == chartDragNone || s.preview == nil {
		return m.setShape(s.hoverShape(e.X, e.Y))
	}
	p := *s.preview
	if s.drag == chartDragResize {
		cx, cy := m.chartScreen(p)
		p.W = clamp(e.X-cx+1, sheet.MinChartW, sheet.MaxChartW)
		p.H = clamp(e.Y-cy+1, sheet.MinChartH, sheet.MaxChartH)
	} else {
		p.At = m.chartCellAt(e.X-s.grabX, e.Y-s.grabY)
	}
	s.preview = &p
	return nil
}

// hoverShape is the pointer shape at x, y: a move cursor over charts,
// and a resize one over the selected chart's corner.
func (s *chartSel) hoverShape(x, y int) string {
	i := s.m.chartAt(x, y)
	if i < 0 {
		return "default"
	}
	if c, ok := s.shown(); ok && i == s.i {
		cx, cy := s.m.chartScreen(c)
		if x >= cx+c.W-2 && y == cy+c.H-1 {
			return "nwse-resize"
		}
	}
	return "move"
}

// release drops the chart being dragged where it is, as one undo step.
func (s *chartSel) release() {
	c, ok := s.chart()
	if ok && s.preview != nil && s.drag != chartDragNone && *s.preview != c {
		label := "move chart"
		if s.drag == chartDragResize {
			label = "resize chart"
		}
		s.place(c, *s.preview, label)
	}
	s.drag, s.preview = chartDragNone, nil
}

// chartCellAt returns the cell a chart's corner snaps to when dragged to
// screen position x, y, which may be past the grid's edges.
func (g *grid) chartCellAt(x, y int) sheet.Addr {
	a := sheet.Addr{Row: g.rowAtLine(y), Col: g.left}
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

func (s *chartSel) Status() (string, string) {
	th := s.m.styles()
	width, _ := s.m.size()
	c, ok := s.chart()
	if !ok {
		return "", ""
	}
	desc := th.Muted.Render("Chart of " + c.Data.String())
	pairs := []string{"Enter", "edit", "Del", "delete", "Arrows", "move", "Shift+arrows", "resize", "Esc", "done"}
	for {
		keys := th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= width:
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

// ContextLine says what's selected and how to change it.
func (s *chartSel) ContextLine() (string, string) {
	th := s.m.styles()
	c, ok := s.chart()
	if !ok {
		return "", ""
	}
	return th.Key.Render(c.Type.Title()+" chart") + th.Muted.Render(" of ") + c.Data.String() +
		th.Muted.Render("   drag to move, drag the corner to resize"), ""
}

const (
	chartDragNone = iota
	chartDragMove
	chartDragResize
)
