package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// Working in an output's grid (nbgrid.go). Enter on a table or record
// output, or a click in it, gives its grid the keys: the window shows
// the grid's active cell and selection, and keys, the mouse and the
// commands that act on a sheet's cells go to the grid's own model, so
// arrows, Shift+arrows, Ctrl+C, sorting and filtering from the menus,
// Ctrl+F and resizing columns work as on a sheet. What belongs to the
// notebook and the program (menus, the palette, help, saving, other
// tabs) stays theirs. Enter shows the grid full-screen, and Esc goes
// back a level: to the window, then to the notebook.

// gridIn is the grid entered, while the notebook shown still shows it.
func (m *Model) gridIn() *outGrid {
	g := m.nb.out.in
	if g == nil {
		return nil
	}
	if v := m.nbView(); v == nil || !v.Shows(g) {
		m.nb.out.in = nil
		return nil
	}
	return g
}

// enterGrid gives grid g the keys, reporting whether it could.
func (m *Model) enterGrid(g *outGrid) bool {
	if !g.ready(m.nbGridWidth(), nbview.Window) {
		m.note = "The output isn't a table: " + g.err
		return false
	}
	m.nb.out.in = g
	g.child.note, g.child.warn = "", ""
	return true
}

// gridBack goes back a level from grid g: from full-screen to the
// window, and from the window to the notebook.
func (m *Model) gridBack() {
	if v := m.nbView(); v != nil && v.FullOpen() {
		v.CloseFull()
		v.FollowOutput()
		return
	}
	m.nb.out.in = nil
}

// gridFull shows the grid entered full-screen.
func (m *Model) gridFull() {
	if v := m.nbView(); v != nil && !v.FullOpen() {
		v.OpenFull()
	}
}

// gridKey gives a key to grid g, after the keys the notebook and the
// program keep while the grid is at rest.
func (m *Model) gridKey(g *outGrid, k tea.KeyPressMsg) tea.Cmd {
	c := g.child
	if c.mode == modeReady && c.overlay == nil && c.mouse.drag == dragNone {
		if cmd, ok := m.gridOwnKey(g, k.String()); ok {
			return cmd
		}
	}
	return g.forward(k)
}

// gridOwnKey takes the keys the grid leaves to the notebook: Esc when
// nothing is selected, the menus, and the program's own commands.
func (m *Model) gridOwnKey(g *outGrid, key string) (tea.Cmd, bool) {
	if key == "esc" && !g.child.hasRange() {
		m.gridBack()
		return nil, true
	}
	if i := barMenuFor(key); i >= 0 {
		m.showBarMenu(i)
		return nil, true
	}
	if id, ok := keymap[canonicalKey(key)]; ok && notebookSafe(id) && !gridTakes(id) {
		return m.runCommand(id), true
	}
	return m.runShortcut(key)
}

// gridTakes reports whether a command the notebook would run acts on
// the grid entered instead: undo and redo, of its sorts and filters.
func gridTakes(id string) bool { return id == "edit.undo" || id == "edit.redo" }

// forward hands the grid a message, then keeps its active cell on the
// table, passes what it copied to the program's clipboard, and keeps
// its window on screen.
func (g *outGrid) forward(msg tea.Msg) tea.Cmd {
	c := g.child
	clip, focus := c.copied.clip, *c.focus()
	var cmd tea.Cmd
	g.roomy(func() { _, cmd = c.Update(msg) })
	if *c.focus() != focus {
		c.scrollTo(*c.focus()) // in the window, which is smaller
	}
	g.keep()
	if c.copied.clip != clip && c.copied.clip != nil {
		g.parent.copied = clipboard{clip: c.copied.clip, sheet: c.sheet}
		if c.note == "" {
			c.note = "Copied " + countCells(c.copied.clip.Range) + ": Ctrl+V pastes them on a sheet"
		}
	}
	if v := g.parent.nbView(); v != nil {
		v.FollowOutput()
	}
	return cmd
}

// run runs a command on the grid.
func (g *outGrid) run(id string) tea.Cmd {
	cmd := g.child.runCommand(id)
	g.keep()
	return cmd
}

// keep keeps the active cell and the selection's moving corner on the
// table's rows and columns, under its header.
func (g *outGrid) keep() {
	c := g.child
	in := func(a sheet.Addr) sheet.Addr {
		return sheet.Addr{Col: clamp(a.Col, 0, max(g.cols-1, 0)), Row: clamp(a.Row, 1, max(g.rows, 1))}
	}
	if c.whole != wholeNone {
		return
	}
	cur, ext := in(c.cur), in(c.ext)
	if cur != c.cur || ext != c.ext {
		c.cur, c.ext = cur, ext
		c.scrollTo(*c.focus())
	}
}

// gridMouse takes a mouse message over an output's grid, or any while
// the grid entered drags or has a menu open, reporting whether it did.
func (m *Model) gridMouse(v *nbview.View, msg tea.MouseMsg) (tea.Cmd, bool) {
	mouse := msg.Mouse()
	in := m.gridIn()
	if in != nil && (in.child.overlay != nil || in.child.mouse.drag != dragNone || in.child.mode != modeReady) {
		return in.forward(in.toChild(v, msg)), true
	}
	found, _, _, ok := v.GridAt(mouse.X, mouse.Y-nbBody)
	g, _ := found.(*outGrid)
	if in != nil && g != in {
		in.child.mouse.hover = hit{}
	}
	if !ok || g == nil {
		return nil, false
	}
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		if g != in || !sideways(tea.Mouse(msg)) {
			return nil, false // the notebook scrolls the window
		}
	case tea.MouseMotionMsg:
		if g != in {
			return nil, true
		}
	case tea.MouseClickMsg:
		if g != in {
			v.SelectGrid(g)
			if !m.enterGrid(g) {
				return nil, true
			}
		}
	}
	return g.forward(g.toChild(v, msg)), true
}

// sideways reports whether a wheel turn scrolls sideways.
func sideways(mouse tea.Mouse) bool {
	return mouse.Button == tea.MouseWheelLeft || mouse.Button == tea.MouseWheelRight || mouse.Mod.Contains(tea.ModShift)
}

// roomy runs fn with the child as large as the program's screen leaves
// it from the window down, so the boxes it opens (a column's filter, a
// menu) are laid out with the room the screen has rather than the
// window's.
func (g *outGrid) roomy(fn func()) {
	c, v := g.child, g.parent.nbView()
	if v == nil {
		fn()
		return
	}
	ox, oy := g.origin(v)
	w, h := c.width, c.height
	c.width, c.height = max(g.parent.width-ox, w), max(g.parent.height-oy+gridTop, h)
	defer func() { c.width, c.height = w, h }()
	fn()
}

// origin is where the grid's column header is on the program's screen.
func (g *outGrid) origin(v *nbview.View) (x, y int) {
	x, y, _ = v.GridOrigin(g)
	return x, y + nbBody
}

// The child's screen has its column header on headerLine, then the
// frozen header row and the divider under it, which the window leaves
// out, then its rows: childY and parentY map lines between the two.

// childY is the child's line for line y of the window, 0 its header.
func childY(y int) int {
	if y <= 0 {
		return headerLine + y
	}
	return gridTop + 1 + y
}

// windowY is the window's line for the child's line y.
func windowY(y int) int {
	if y <= headerLine {
		return y - headerLine
	}
	return max(y-gridTop-1, 1)
}

// toChild is a mouse message with its position on the child's screen:
// in one of the child's boxes, where in the box; elsewhere, where in the
// window.
func (g *outGrid) toChild(v *nbview.View, msg tea.MouseMsg) tea.MouseMsg {
	ox, oy := g.origin(v)
	mouse := msg.Mouse()
	x, y := mouse.X-ox, childY(mouse.Y-oy)
	for _, b := range g.boxes() {
		bx, by := b.X+ox, windowY(b.Y)+oy
		if mouse.X >= bx && mouse.Y >= by && len(b.Lines) > 0 && mouse.Y < by+len(b.Lines) && mouse.X < bx+boxWidth(b) {
			x, y = b.X+mouse.X-bx, b.Y+mouse.Y-by
		}
	}
	at := tea.Mouse{X: x, Y: y, Button: mouse.Button, Mod: mouse.Mod}
	switch msg.(type) {
	case tea.MouseClickMsg:
		return tea.MouseClickMsg(at)
	case tea.MouseReleaseMsg:
		return tea.MouseReleaseMsg(at)
	case tea.MouseWheelMsg:
		return tea.MouseWheelMsg(at)
	}
	return tea.MouseMotionMsg(at)
}

// boxes are the child's boxes, laid out with the screen's room.
func (g *outGrid) boxes() []overlay.Box {
	var out []overlay.Box
	g.roomy(func() { out = g.child.floating() })
	return out
}

// gridBoxes are the child's boxes (a column's filter, a menu) placed on the
// program's screen over the window.
func (m *Model) gridBoxes() []overlay.Box {
	g, v := m.gridIn(), m.nbView()
	if g == nil || v == nil {
		return nil
	}
	ox, oy := g.origin(v)
	boxes := g.boxes()
	out := make([]overlay.Box, 0, len(boxes))
	for _, b := range boxes {
		b.X, b.Y = max(b.X+ox, 0), max(windowY(b.Y)+oy, 0)
		out = append(out, b)
	}
	return out
}
