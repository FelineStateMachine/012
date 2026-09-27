package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// overlay is a floating box drawn over the grid: a dropdown or context
// menu, the command palette or a dialog. While one is open it takes every
// key and mouse event, and Esc closes one level. Overlays are composited
// on top of the finished screen, so the grid underneath never moves.
type overlay interface {
	// indicator is the mode shown at the top right, e.g. "MENU".
	indicator() string
	// layout places the overlay's boxes on the screen, bottom first.
	layout(m *Model) []box
	key(m *Model, k tea.KeyPressMsg) tea.Cmd
	mouse(m *Model, e mouseEvent) tea.Cmd
	// status is what the status line says while the overlay is open: what
	// the highlighted item does, and the keys that apply.
	status(m *Model) (desc, keys string)
}

// textOverlay is an overlay with a text input, which gets the terminal
// cursor. The text is edited in Model.line, like any other entry.
type textOverlay interface {
	overlay
	cursor(m *Model) (x, y int)
	changed(m *Model) // the text changed
}

// box is a rectangle of styled lines at a screen position. Every line has
// the same display width.
type box struct {
	id    string // identifies the box in mouse hit tests
	x, y  int
	lines []string
}

func (b box) width() int  { return ansi.StringWidth(b.lines[0]) }
func (b box) height() int { return len(b.lines) }

type mouseKind int

const (
	mousePress mouseKind = iota
	mouseMotion
	mouseRelease
	mouseWheel
)

// mouseEvent is a mouse event with what's under it already worked out.
type mouseEvent struct {
	kind     mouseKind
	button   tea.MouseButton
	x, y     int
	box      string // id of the topmost box under the mouse, "" for none
	col, row int    // position inside that box
}

// openOverlay shows o, replacing any open overlay.
func (m *Model) openOverlay(o overlay) {
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
func compositor(boxes []box) *lipgloss.Compositor {
	layers := make([]*lipgloss.Layer, len(boxes))
	for i, b := range boxes {
		layers[i] = lipgloss.NewLayer(strings.Join(b.lines, "\n")).X(b.x).Y(b.y).Z(i + 1).ID(b.id)
	}
	return lipgloss.NewCompositor(layers...)
}

// floating returns the boxes drawn over the screen: the open overlay's,
// or the formula suggestions while typing.
func (m *Model) floating() []box {
	if m.overlay != nil {
		return m.overlay.layout(m)
	}
	if b, ok := m.assistBox(); ok {
		return []box{b}
	}
	return nil
}

// compose draws the charts floating over the grid, then boxes (the open
// overlay or formula suggestions), over the rendered screen.
func (m *Model) compose(screen string, boxes []box) string {
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
	if m.assistMouse(msg) {
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
	e := mouseEvent{button: mouse.Button, x: mouse.X, y: mouse.Y}
	switch msg.(type) {
	case tea.MouseClickMsg:
		e.kind = mousePress
	case tea.MouseMotionMsg:
		e.kind = mouseMotion
	case tea.MouseReleaseMsg:
		e.kind = mouseRelease
	case tea.MouseWheelMsg:
		e.kind = mouseWheel
	}
	if h := compositor(m.overlay.layout(m)).Hit(e.x, e.y); !h.Empty() {
		e.box = h.ID()
		e.col, e.row = e.x-h.Bounds().Min.X, e.y-h.Bounds().Min.Y
	}
	return m.overlay.mouse(m, e)
}

// list is the highlighted row and scroll position of a list in a box.
type list struct {
	sel, top int
}

// move highlights the row d steps away, wrapping around the ends.
func (l *list) move(d, n int) {
	if n > 0 {
		l.sel = ((l.sel+d)%n + n) % n
	}
}

// show scrolls so the highlighted row is among the rows visible ones.
func (l *list) show(rows int) {
	l.top = clamp(l.top, l.sel-rows+1, l.sel)
	l.top = max(l.top, 0)
}
