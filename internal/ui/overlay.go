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
// cursor. The text is edited in m.buf, like any other entry.
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
	m.buf, m.bufPos = nil, 0
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

// compose draws the open overlay over the rendered screen.
func (m *Model) compose(screen string) string {
	c := lipgloss.NewCanvas(m.width, m.height)
	c.Compose(lipgloss.NewLayer(screen))
	c.Compose(compositor(m.overlay.layout(m)))
	return c.Render()
}

// shellMouse gives the open overlay, or else the menu bar, the first look
// at a mouse message. It reports false for messages the grid handles.
func (m *Model) shellMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.overlay != nil {
		return m.overlayMouse(msg), true
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || m.mode != modeReady {
		return nil, false
	}
	mouse := click.Mouse()
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

// A box is framed with light box-drawing lines, which keep the crisp
// character-grid look. Rows are exactly the inner width; separator rows
// become a line joined to the frame.

// sepRow marks a separator in the rows passed to frame.
const sepRow = "\x00"

// frame draws a border around rows. A title sits in the top border and a
// footer at the right of the bottom border.
func (m *Model) frame(inner int, title, footer string, rows []string) []string {
	inner = max(inner, 2) // screens smaller than the box get a clipped box
	b := m.th.border
	top := "┌" + strings.Repeat("─", inner) + "┐"
	if title != "" {
		t := ansi.Truncate(" "+title+" ", inner-1, "…")
		top = b.Render("┌─") + m.th.title.Render(t) + b.Render(strings.Repeat("─", inner-1-ansi.StringWidth(t))+"┐")
	} else {
		top = b.Render(top)
	}
	bottom := b.Render("└" + strings.Repeat("─", inner) + "┘")
	if footer != "" && ansi.StringWidth(footer)+4 <= inner {
		f := " " + footer + " "
		bottom = b.Render("└"+strings.Repeat("─", inner-1-ansi.StringWidth(f))) + m.th.muted.Render(f) + b.Render("─┘")
	}
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, top)
	for _, r := range rows {
		if r == sepRow {
			lines = append(lines, b.Render("├"+strings.Repeat("─", inner)+"┤"))
			continue
		}
		lines = append(lines, b.Render("│")+r+b.Render("│"))
	}
	return append(lines, bottom)
}

// cells pads or truncates s to exactly w columns, then styles it.
func cells(style lipgloss.Style, s string, w int) string {
	return style.Render(padRight(ansi.Truncate(s, w, "…"), w))
}

// clampBox keeps a w by h box at x, y on screen, shifting it left and up
// as needed.
func (m *Model) clampBox(x, y, w, h int) (int, int) {
	return max(min(x, m.width-w), 0), max(min(y, m.height-h), 0)
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
