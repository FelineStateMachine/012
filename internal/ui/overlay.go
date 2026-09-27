package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// openOverlay shows o, replacing any open overlay.
func (m *Model) openOverlay(o overlay.Overlay) {
	m.overlay = o
	m.mode = modeMenu
}

// closeOverlay closes the open overlay and returns to READY.
func (m *Model) closeOverlay() {
	m.overlay = nil
	m.line.clear()
	if m.mode == modeMenu {
		m.mode = modeReady
	}
}

// runFromOverlay closes the overlay, then runs a command, which may open
// a prompt or another overlay of its own. Unavailable commands do nothing.
func (m *Model) runFromOverlay(id string) tea.Cmd {
	c, ok := commands[id]
	if !ok || !c.available(m) {
		return nil
	}
	m.closeOverlay()
	return m.runCommand(id)
}

// compositor stacks boxes over the screen in order, each on its own
// layer so hit tests find the topmost box.
func compositor(boxes []overlay.Box) *lipgloss.Compositor {
	layers := make([]*lipgloss.Layer, len(boxes))
	for i, b := range boxes {
		layers[i] = lipgloss.NewLayer(strings.Join(b.Lines, "\n")).X(b.X).Y(b.Y).Z(i + 1).ID(b.ID)
	}
	return lipgloss.NewCompositor(layers...)
}

// floating returns the boxes drawn over the screen: the open overlay's,
// or the formula suggestions while typing.
func (m *Model) floating() []overlay.Box {
	if m.overlay != nil {
		return m.overlay.Layout()
	}
	if b, ok := m.entry.assist.box(m); ok {
		return []overlay.Box{b}
	}
	return nil
}

// compose draws the charts floating over the grid, then boxes (the open
// overlay or formula suggestions), over the rendered screen.
func (m *Model) compose(screen string, boxes []overlay.Box) string {
	c := lipgloss.NewCanvas(m.width, m.height)
	c.Compose(lipgloss.NewLayer(screen))
	if charts := m.chartBoxes(); len(charts) > 0 {
		c.Compose(compositor(charts))
	}
	if len(boxes) > 0 {
		c.Compose(compositor(boxes))
	}
	return c.Render()
}

// shellMouse gives the open overlay, or else the menu bar, the first look
// at a mouse message. It reports false for messages the grid handles.
func (m *Model) shellMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.overlay != nil {
		return m.overlayMouse(msg), true
	}
	if m.entry.assist.mouse(m, msg) {
		return nil, true
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || m.mode != modeReady {
		return nil, false
	}
	mouse := click.Mouse()
	if cmd, ok := m.chartClick(mouse); ok {
		return cmd, true
	}
	switch {
	case mouse.Button == tea.MouseRight:
		m.rightClick(mouse.X, mouse.Y)
		return nil, true
	case mouse.Y == menuLine && mouse.Button == tea.MouseLeft:
		if i := barMenuAt(mouse.X); i >= 0 {
			m.showBarMenu(i)
			return nil, true
		}
	}
	return nil, false
}

// overlayMouse hands a mouse message to the open overlay, with the box and
// position under the mouse.
func (m *Model) overlayMouse(msg tea.MouseMsg) tea.Cmd {
	mouse := msg.Mouse()
	e := overlay.MouseEvent{Button: mouse.Button, X: mouse.X, Y: mouse.Y}
	switch msg.(type) {
	case tea.MouseClickMsg:
		e.Kind = overlay.MousePress
	case tea.MouseMotionMsg:
		e.Kind = overlay.MouseMotion
	case tea.MouseReleaseMsg:
		e.Kind = overlay.MouseRelease
	case tea.MouseWheelMsg:
		e.Kind = overlay.MouseWheel
	}
	if h := compositor(m.overlay.Layout()).Hit(e.X, e.Y); !h.Empty() {
		e.Box = h.ID()
		e.Col, e.Row = e.X-h.Bounds().Min.X, e.Y-h.Bounds().Min.Y
	}
	return m.overlay.Mouse(e)
}
