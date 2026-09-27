package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
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
	hitFilterButton // the filter mark in a column header
	hitFillHandle   // the corner of the selection that drags out a fill
	hitTab          // a sheet's tab on the status line; addr.Col is its index
	hitTabAdd       // the + after the tabs
	hitTabPrev      // the ‹ before tabs scrolled off to the left
	hitTabNext      // the › after tabs scrolled off to the right
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
		switch {
		case x == start+m.sheet.ColWidth(col)-1:
			kind = hitColBorder
		case x == m.filterButtonX(col):
			kind = hitFilterButton
		}
		return hit{kind: kind, addr: sheet.Addr{Col: col, Row: m.top}}
	case y >= gridTop && y < gridTop+m.visibleRows():
		row, ok := m.rowAt(y)
		if !ok {
			return hit{}
		}
		if x < rowHdrW {
			return hit{kind: hitRowHeader, addr: sheet.Addr{Col: m.left, Row: row}}
		}
		col, start, ok := m.colSpan(x)
		if !ok {
			return hit{}
		}
		a := sheet.Addr{Col: col, Row: row}
		if m.mode == modeReady && a == m.fillCorner() && x == start+m.sheet.ColWidth(col)-1 {
			return hit{kind: hitFillHandle, addr: a}
		}
		return hit{kind: hitCell, addr: a}
	case y == m.height-1:
		if sp, ok := m.tabAt(x); ok {
			return hit{kind: sp.kind, addr: sheet.Addr{Col: sp.index}}
		}
		return hit{kind: hitStatus}
	}
	return hit{}
}

// colSpan returns the visible column under x and the x where it starts:
// a frozen column or a scrolling one, but not the divider between them.
func (m *Model) colSpan(x int) (col, start int, ok bool) {
	_, fc := m.frozen()
	cx := rowHdrW
	for c := 0; c < fc; c++ {
		w := m.sheet.ColWidth(c)
		if x >= cx && x < cx+w {
			return c, cx, true
		}
		cx += w
	}
	cx = m.scrollX()
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
// screen (or under the frozen columns, for a scrolling column left of
// m.left).
func (m *Model) colStart(c int) int {
	_, fc := m.frozen()
	if c < fc {
		x := rowHdrW
		for k := range c {
			x += m.sheet.ColWidth(k)
		}
		return x
	}
	x := m.scrollX()
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
// Dragging from the scrolling area into the frozen panes scrolls back
// toward them, as in Sheets, until the two meet.
func (m *Model) dragTarget(x, y int) (a sheet.Addr, dc, dr int) {
	rows := m.screenRows()
	fr, fc := m.frozen()
	anchor := m.dragAnchor()
	firstRow := m.visibleRow(fr)
	scrolledDown := m.top > firstRow
	var last int
	for _, r := range rows {
		if r != divider {
			last = r
		}
	}
	switch row, ok := m.rowAt(y); {
	case ok && (row >= fr || !scrolledDown || anchor.Row < fr):
		a.Row = row
	case y >= gridTop+len(rows):
		a.Row, dr = last, 1
	case y >= gridTop && !scrolledDown: // the divider, with nothing scrolled
		a.Row = m.top
	case scrolledDown && anchor.Row >= fr:
		a.Row, dr = m.top, -1
	default: // above the grid
		a.Row = rows[0]
		if scrolledDown {
			dr = -1
		}
	}
	scrolledRight := m.left > fc
	cols := m.visibleCols(m.left)
	right := m.colStart(m.left + cols)
	switch col, _, ok := m.colSpan(x); {
	case x >= right:
		a.Col, dc = m.left+cols-1, 1
	case ok && (col >= fc || !scrolledRight || anchor.Col < fc):
		a.Col = col
	case x >= rowHdrW && !scrolledRight:
		a.Col = m.left
	case scrolledRight && anchor.Col >= fc:
		a.Col, dc = m.left, -1
	default: // over the row numbers
		a.Col = 0
		if fc == 0 {
			a.Col = m.left
		}
		if scrolledRight || fc == 0 && m.left > 0 {
			dc = -1
		}
	}
	return clampAddr(a), dc, dr
}

// dragAnchor is where the drag in progress started.
func (m *Model) dragAnchor() sheet.Addr {
	if m.drag == dragPoint {
		return m.point.anchor
	}
	return m.cur
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
	if isTabHit(h.kind) && m.mode != modeMenu && m.mode != modePrompt {
		if m.mode == modeError {
			m.errMsg, m.mode = "", modeReady
		}
		return m.tabPress(h, double)
	}
	switch m.mode {
	case modeError:
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
			m.resumeEntry(m.pointRef())
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
		// Clicking elsewhere accepts the entry, then acts as in READY:
		// on the entry's sheet, unless the click was on another.
		away := m.away()
		if !m.commit() || away {
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
	case hitFillHandle:
		m.startFill()
	case hitFilterButton:
		m.openFilterPicker(h.addr.Col)
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

// isTabHit reports whether k is a part of the tab strip.
func isTabHit(k hitKind) bool {
	return k == hitTab || k == hitTabAdd || k == hitTabPrev || k == hitTabNext
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
	switch m.drag {
	case dragResize:
		m.sheet.SetColWidth(m.resizeCol, clamp(x-m.colStart(m.resizeCol)+1, 1, 240))
		m.changed = true
		return nil
	case dragTab:
		return nil // handleMotion tracks the tab under the mouse
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
	case dragFill:
		m.dragFillTo(a)
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
	if m.drag == dragNone || m.drag == dragResize || m.drag == dragTab {
		m.autoscrolling = false
		return nil
	}
	_, dc, dr := m.dragTarget(m.mouseX, m.mouseY)
	if dc == 0 && dr == 0 {
		m.autoscrolling = false
		return nil
	}
	step := func(a *sheet.Addr) {
		*a = clampAddr(sheet.Addr{Col: a.Col + dc, Row: m.stepRow(a.Row, dr)})
	}
	switch m.drag {
	case dragPoint:
		step(&m.point.at)
		m.point.anchored = true
	case dragCells, dragCols, dragRows:
		m.selecting = true
		step(&m.ext)
	case dragFill:
		step(&m.fillAt)
		m.dragFillTo(m.fillAt)
	}
	m.scrollTo(*m.focus())
	return autoscrollTick()
}

func (m *Model) handleRelease() tea.Cmd {
	switch m.drag {
	case dragFill:
		m.finishFill()
	case dragTab:
		m.dropTab()
	}
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
	case m.drag == dragFill, m.hover.kind == hitFillHandle:
		shape = "crosshair"
	case m.mode == modeReady && m.chartAt(m.mouseX, m.mouseY) >= 0:
		shape = "move"
	case m.hover.kind == hitCell:
		shape = "cell"
	case m.hover.kind == hitFormulaBar, m.hover.kind == hitEditLine:
		shape = "text"
	case m.hover.kind == hitColHeader, m.hover.kind == hitRowHeader, m.hover.kind == hitCorner, m.hover.kind == hitFilterButton,
		isTabHit(m.hover.kind):
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

// exit restores the pointer shape, Shift+click handling and the other
// terminal modes 012 changed, frees chart images, then quits.
func (m *Model) exit() tea.Cmd {
	return tea.Sequence(tea.Raw(ansi.SetPointerShape("default")+shiftEscapeOff+m.releaseTerminal()), tea.Quit)
}
