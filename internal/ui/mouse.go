package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"one23/internal/sheet"
)

// Mouse handling follows Google Sheets. The view asks Bubble Tea for every
// motion event (tea.MouseModeAllMotion), so headers react to hover and the
// terminal's pointer shape (OSC 22) changes over resize handles, cells and
// the formula bar. Clicks work in every mode: clicking a cell while typing
// accepts the entry, or inserts the cell's reference into a formula.

// hitKind is what's under the mouse.
type hitKind int

const (
	hitNone hitKind = iota
	hitCorner
	hitColHeader
	hitColBorder // the resize handle at the right edge of a column header
	hitRowHeader
	hitCell
	hitFormulaBar // the active cell's contents in READY mode
	hitEditLine   // the entry being typed
	hitPanel
	hitStatus
)

type hit struct {
	kind hitKind
	addr sheet.Addr // the cell; for headers, the column or row
	x    int        // x within the formula bar or edit line text
}

// formulaBarAt returns where the active cell's contents are drawn in READY
// mode: the screen line and the x of the first character.
func (m *Model) formulaBarAt() (line, x int) { return formulaLine, formulaBarTextX() }

// editLineAt returns where the entry being typed is drawn: in place, in the
// formula bar.
func (m *Model) editLineAt() (line, x int) { return formulaLine, formulaBarTextX() }

// hitTest maps a screen position to what's there.
func (m *Model) hitTest(x, y int) hit {
	rows := m.visibleRows()
	switch {
	case y < panelLines:
		if line, tx := m.editLineAt(); y == line && m.editing() {
			return hit{kind: hitEditLine, x: x - tx}
		}
		if line, tx := m.formulaBarAt(); y == line && x >= tx && m.mode == modeReady {
			return hit{kind: hitFormulaBar, x: x - tx}
		}
		return hit{kind: hitPanel}
	case y == headerLine && x < rowHdrW:
		return hit{kind: hitCorner}
	case y == headerLine:
		col, start, ok := m.colSpan(x)
		if !ok {
			return hit{}
		}
		kind := hitColHeader
		if x == start+m.sheet.ColWidth(col)-1 {
			kind = hitColBorder
		}
		return hit{kind: kind, addr: sheet.Addr{Col: col, Row: m.top}}
	case y >= gridTop && y < gridTop+rows:
		row := m.top + y - gridTop
		if x < rowHdrW {
			return hit{kind: hitRowHeader, addr: sheet.Addr{Col: m.left, Row: row}}
		}
		col, _, ok := m.colSpan(x)
		if !ok {
			return hit{}
		}
		return hit{kind: hitCell, addr: sheet.Addr{Col: col, Row: row}}
	case y == m.height-1:
		return hit{kind: hitStatus}
	}
	return hit{}
}

// colSpan returns the visible column under x and the x where it starts.
func (m *Model) colSpan(x int) (col, start int, ok bool) {
	cx := rowHdrW
	for c := m.left; c < sheet.MaxCols && cx < m.width; c++ {
		w := m.sheet.ColWidth(c)
		if x >= cx && x < cx+w {
			return c, cx, true
		}
		cx += w
	}
	return 0, 0, false
}

// colStart returns the screen x where column c starts, which may be off
// screen.
func (m *Model) colStart(c int) int {
	x := rowHdrW
	if c >= m.left {
		for k := m.left; k < c; k++ {
			x += m.sheet.ColWidth(k)
		}
		return x
	}
	for k := c; k < m.left; k++ {
		x -= m.sheet.ColWidth(k)
	}
	return x
}

// dragTarget maps a position during a drag to a cell, clamping to the
// visible grid, and reports which way to autoscroll when it's outside.
func (m *Model) dragTarget(x, y int) (a sheet.Addr, dc, dr int) {
	rows, cols := m.visibleRows(), m.visibleCols(m.left)
	switch {
	case y < gridTop:
		dr = -1
	case y >= gridTop+rows:
		dr = 1
	}
	right := m.colStart(m.left + cols)
	switch {
	case x < rowHdrW:
		dc = -1
	case x >= right:
		dc = 1
	}
	a.Row = clamp(m.top+y-gridTop, m.top, m.top+rows-1)
	if col, _, ok := m.colSpan(x); ok && dc == 0 {
		a.Col = col
	} else if dc < 0 {
		a.Col = m.left
	} else {
		a.Col = m.left + cols - 1
	}
	return clampAddr(a), dc, dr
}

// editing reports whether an entry is being typed.
func (m *Model) editing() bool {
	return m.mode == modeEnter || m.mode == modeEdit || m.mode == modePoint
}

func (m *Model) handlePress(mouse tea.Mouse) tea.Cmd {
	h := m.hitTest(mouse.X, mouse.Y)
	double := h.kind == m.lastHit.kind && h.addr == m.lastHit.addr && time.Since(m.lastClick) < doubleClick
	m.lastClick, m.lastHit = time.Now(), h
	// Right clicks open context menus (context.go) before reaching here.
	if mouse.Button == tea.MouseLeft {
		return m.leftPress(h, mouse, double)
	}
	return nil
}

func (m *Model) leftPress(h hit, mouse tea.Mouse, double bool) tea.Cmd {
	switch m.mode {
	case modeHelp, modeError:
		m.errMsg, m.mode = "", modeReady
		return nil
	case modeMenu:
		return nil // overlays take their clicks in shellMouse first
	case modePrompt:
		if m.pointing() && h.kind == hitCell {
			m.point = pointer{at: h.addr, anchor: h.addr}
			m.drag = dragPoint
		}
		return nil
	case modePoint:
		switch h.kind {
		case hitCell:
			m.point = pointer{at: h.addr, anchor: h.addr}
			m.drag = dragPoint
		case hitEditLine:
			m.resumeEntry(m.point.text())
			m.setCaret(h.x)
		}
		return nil
	case modeEnter, modeEdit:
		switch {
		case h.kind == hitEditLine:
			m.setCaret(h.x)
			return nil
		case h.kind == hitCell && m.isFormula() && m.canPoint():
			m.pointPrefix = string(m.buf[:m.bufPos])
			m.pointSuffix = string(m.buf[m.bufPos:])
			m.point = pointer{at: h.addr, anchor: h.addr}
			m.mode, m.drag = modePoint, dragPoint
			return nil
		case h.kind == hitNone, h.kind == hitPanel, h.kind == hitStatus:
			return nil
		}
		// Clicking elsewhere accepts the entry, then acts as in READY.
		if !m.commit() {
			return nil
		}
		m.tabbing = false
		double = false
	}
	return m.readyPress(h, mouse, double)
}

func (m *Model) readyPress(h hit, mouse tea.Mouse, double bool) tea.Cmd {
	shift := mouse.Mod.Contains(tea.ModShift)
	switch h.kind {
	case hitFormulaBar:
		m.startEdit()
		m.setCaret(h.x)
	case hitCorner:
		m.cur = sheet.Addr{Col: m.left, Row: m.top}
		m.selecting, m.whole, m.ext = true, wholeAll, m.cur
	case hitColBorder:
		if double {
			m.autofit(h.addr.Col)
			return nil
		}
		m.resizeCol, m.drag = h.addr.Col, dragResize
	case hitCell:
		switch {
		case shift:
			m.selecting, m.whole, m.ext = true, wholeNone, h.addr
		case double && h.addr == m.cur:
			m.clearSelection()
			return m.startEdit()
		default:
			m.cur = h.addr
			m.clearSelection()
		}
		m.drag = dragCells
	case hitColHeader, hitRowHeader:
		whole, drag := wholeCols, dragCols
		if h.kind == hitRowHeader {
			whole, drag = wholeRows, dragRows
		}
		if !shift || m.whole != whole {
			m.cur = h.addr
		}
		m.selecting, m.whole, m.ext = true, whole, h.addr
		m.drag = drag
	}
	return nil
}

// setCaret puts the edit caret at display column x of the entry.
func (m *Model) setCaret(x int) {
	w := 0
	for i, r := range m.buf {
		if w >= x {
			m.bufPos = i
			return
		}
		w += ansi.StringWidth(string(r))
	}
	m.bufPos = len(m.buf)
}

func (m *Model) handleMotion(mouse tea.Mouse) tea.Cmd {
	m.hover = m.hitTest(mouse.X, mouse.Y)
	m.mouseX, m.mouseY = mouse.X, mouse.Y
	var cmd tea.Cmd
	if m.drag != dragNone && mouse.Button == tea.MouseLeft {
		cmd = m.dragTo(mouse.X, mouse.Y)
	}
	return tea.Batch(cmd, m.pointerShape())
}

// dragTo extends the drag in progress to (x, y), starting autoscroll when
// the mouse is past the edge of the grid.
func (m *Model) dragTo(x, y int) tea.Cmd {
	if m.drag == dragResize {
		m.sheet.SetColWidth(m.resizeCol, clamp(x-m.colStart(m.resizeCol)+1, 1, 240))
		m.changed = true
		return nil
	}
	a, dc, dr := m.dragTarget(x, y)
	switch m.drag {
	case dragPoint:
		m.point.at, m.point.anchored = a, a != m.point.anchor || m.point.anchored
	case dragCells:
		m.selecting, m.ext = true, a
	case dragCols:
		m.ext.Col = a.Col
	case dragRows:
		m.ext.Row = a.Row
	}
	if (dc != 0 || dr != 0) && !m.autoscrolling {
		m.autoscrolling = true
		return autoscrollTick()
	}
	return nil
}

type autoscrollMsg struct{}

func autoscrollTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg { return autoscrollMsg{} })
}

// handleAutoscroll moves a drag that's past the edge of the grid one more
// step and keeps ticking until the mouse comes back or is released.
func (m *Model) handleAutoscroll() tea.Cmd {
	if m.drag == dragNone || m.drag == dragResize {
		m.autoscrolling = false
		return nil
	}
	_, dc, dr := m.dragTarget(m.mouseX, m.mouseY)
	if dc == 0 && dr == 0 {
		m.autoscrolling = false
		return nil
	}
	step := func(a *sheet.Addr) {
		*a = clampAddr(sheet.Addr{Col: a.Col + dc, Row: a.Row + dr})
	}
	switch m.drag {
	case dragPoint:
		step(&m.point.at)
		m.point.anchored = true
	case dragCells, dragCols, dragRows:
		m.selecting = true
		step(&m.ext)
	}
	m.scrollTo(*m.focus())
	return autoscrollTick()
}

func (m *Model) handleRelease() tea.Cmd {
	m.drag, m.autoscrolling = dragNone, false
	if m.selecting && m.whole == wholeNone && m.ext == m.cur {
		m.clearSelection()
	}
	return nil
}

// autofit sizes column c to its widest content, as double-clicking a
// header border does in Sheets. Selected columns fit together.
func (m *Model) autofit(c int) {
	cols := []int{c}
	if r := m.selection(); m.whole == wholeCols && c >= r.From.Col && c <= r.To.Col {
		cols = cols[:0]
		for k := r.From.Col; k <= r.To.Col; k++ {
			cols = append(cols, k)
		}
	}
	widest := map[int]int{}
	for _, a := range m.sheet.Addrs() {
		if cell := m.sheet.Cell(a); cell != nil {
			widest[a.Col] = max(widest[a.Col], ansi.StringWidth(cell.Value.String()))
		}
	}
	for _, k := range cols {
		// One column of padding each side, and never narrower than the
		// header letters.
		m.sheet.SetColWidth(k, clamp(widest[k]+2, len(sheet.ColName(k))+2, 240))
	}
	m.changed = true
}

// pointerShape asks the terminal for the pointer shape that fits what's
// under the mouse (OSC 22, supported by Ghostty, kitty, foot and xterm;
// others ignore it).
func (m *Model) pointerShape() tea.Cmd {
	shape := "default"
	switch {
	case m.drag == dragResize, m.hover.kind == hitColBorder:
		shape = "col-resize"
	case m.hover.kind == hitCell:
		shape = "cell"
	case m.hover.kind == hitFormulaBar, m.hover.kind == hitEditLine:
		shape = "text"
	case m.hover.kind == hitColHeader, m.hover.kind == hitRowHeader, m.hover.kind == hitCorner:
		shape = "pointer"
	}
	if shape == m.shape {
		return nil
	}
	m.shape = shape
	return tea.Raw(ansi.SetPointerShape(shape))
}

// Terminal modes set at startup and undone on exit.
const (
	// shiftEscapeOn asks the terminal to report Shift+click to the program
	// instead of starting its own text selection (XTSHIFTESCAPE), so
	// Shift+click can extend a selection. Ghostty and xterm honor it.
	shiftEscapeOn  = "\x1b[>1s"
	shiftEscapeOff = "\x1b[>0s"
)

// exit restores the pointer shape and Shift+click handling, then quits.
func exit() tea.Cmd {
	return tea.Sequence(tea.Raw(ansi.SetPointerShape("default")+shiftEscapeOff), tea.Quit)
}
