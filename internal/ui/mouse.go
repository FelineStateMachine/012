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

// mouseState is what the mouse is doing: the drag in progress, what's
// under it, and the last click, to tell double clicks.
type mouseState struct {
	drag          dragKind
	lastClick     time.Time
	lastHit       hit
	hover         hit    // what's under the mouse, for hover styling
	x, y          int    // last position, for autoscroll
	autoscrolling bool   // an autoscroll tick is pending
	resizeCol     int    // the column being resized by its header border
	shape         string // the pointer shape last sent to the terminal

	// A fill handle drag, see fill.go: where it points, and the range it
	// would fill.
	fillAt sheet.Addr
	fillTo sheet.Rect
}

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
	if m.mouse.drag == dragPoint {
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
	double := h.kind == m.mouse.lastHit.kind && h.addr == m.mouse.lastHit.addr && time.Since(m.mouse.lastClick) < doubleClick
	m.mouse.lastClick, m.mouse.lastHit = time.Now(), h
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
			m.mouse.drag = dragPoint
		}
		return nil
	case modePoint:
		switch h.kind {
		case hitCell:
			m.point = pointer{at: h.addr, anchor: h.addr}
			m.mouse.drag = dragPoint
		case hitEditLine:
			m.resumeEntry(m.pointRef())
			m.line.setCaret(h.x)
		}
		return nil
	case modeEnter, modeEdit:
		switch {
		case h.kind == hitEditLine:
			m.line.setCaret(h.x)
			return nil
		case h.kind == hitCell && m.line.isFormula() && m.line.canPoint():
			m.entry.prefix = m.line.head()
			m.entry.suffix = m.line.tail()
			m.point = pointer{at: h.addr, anchor: h.addr}
			m.mode, m.mouse.drag = modePoint, dragPoint
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
		m.entry.tabbing = false
		double = false
	}
	return m.readyPress(h, mouse, double)
}

func (m *Model) readyPress(h hit, mouse tea.Mouse, double bool) tea.Cmd {
	shift := mouse.Mod.Contains(tea.ModShift)
	switch h.kind {
	case hitFormulaBar:
		m.startEdit()
		m.line.setCaret(h.x)
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
		m.mouse.resizeCol, m.mouse.drag = h.addr.Col, dragResize
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
		m.mouse.drag = dragCells
	case hitColHeader, hitRowHeader:
		whole, drag := wholeCols, dragCols
		if h.kind == hitRowHeader {
			whole, drag = wholeRows, dragRows
		}
		if !shift || m.whole != whole {
			m.cur = h.addr
		}
		m.selecting, m.whole, m.ext = true, whole, h.addr
		m.mouse.drag = drag
	}
	return nil
}

// isTabHit reports whether k is a part of the tab strip.
func isTabHit(k hitKind) bool {
	return k == hitTab || k == hitTabAdd || k == hitTabPrev || k == hitTabNext
}

func (m *Model) handleMotion(mouse tea.Mouse) tea.Cmd {
	m.mouse.hover = m.hitTest(mouse.X, mouse.Y)
	m.mouse.x, m.mouse.y = mouse.X, mouse.Y
	var cmd tea.Cmd
	if m.mouse.drag != dragNone && mouse.Button == tea.MouseLeft {
		cmd = m.dragTo(mouse.X, mouse.Y)
	}
	return tea.Batch(cmd, m.pointerShape())
}

// dragTo extends the drag in progress to (x, y), starting autoscroll when
// the mouse is past the edge of the grid.
func (m *Model) dragTo(x, y int) tea.Cmd {
	switch m.mouse.drag {
	case dragResize:
		m.sheet.SetColWidth(m.mouse.resizeCol, clamp(x-m.colStart(m.mouse.resizeCol)+1, 1, 240))
		m.changed = true
		return nil
	case dragTab:
		return nil // handleMotion tracks the tab under the mouse
	}
	a, dc, dr := m.dragTarget(x, y)
	switch m.mouse.drag {
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
	if (dc != 0 || dr != 0) && !m.mouse.autoscrolling {
		m.mouse.autoscrolling = true
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
	if m.mouse.drag == dragNone || m.mouse.drag == dragResize || m.mouse.drag == dragTab {
		m.mouse.autoscrolling = false
		return nil
	}
	_, dc, dr := m.dragTarget(m.mouse.x, m.mouse.y)
	if dc == 0 && dr == 0 {
		m.mouse.autoscrolling = false
		return nil
	}
	step := func(a *sheet.Addr) {
		*a = clampAddr(sheet.Addr{Col: a.Col + dc, Row: m.stepRow(a.Row, dr)})
	}
	switch m.mouse.drag {
	case dragPoint:
		step(&m.point.at)
		m.point.anchored = true
	case dragCells, dragCols, dragRows:
		m.selecting = true
		step(&m.ext)
	case dragFill:
		step(&m.mouse.fillAt)
		m.dragFillTo(m.mouse.fillAt)
	}
	m.scrollTo(*m.focus())
	return autoscrollTick()
}

func (m *Model) handleRelease() tea.Cmd {
	switch m.mouse.drag {
	case dragFill:
		m.finishFill()
	case dragTab:
		m.dropTab()
	}
	m.mouse.drag, m.mouse.autoscrolling = dragNone, false
	if m.selecting && m.whole == wholeNone && m.ext == m.cur {
		m.clearSelection()
	}
	return nil
}

// pointerShape asks the terminal for the pointer shape that fits what's
// under the mouse (OSC 22, supported by Ghostty, kitty, foot and xterm;
// others ignore it).
func (m *Model) pointerShape() tea.Cmd {
	shape := "default"
	switch {
	case m.mouse.drag == dragResize, m.mouse.hover.kind == hitColBorder:
		shape = "col-resize"
	case m.mouse.drag == dragFill, m.mouse.hover.kind == hitFillHandle:
		shape = "crosshair"
	case m.mode == modeReady && m.chartAt(m.mouse.x, m.mouse.y) >= 0:
		shape = "move"
	case m.mouse.hover.kind == hitCell:
		shape = "cell"
	case m.mouse.hover.kind == hitFormulaBar, m.mouse.hover.kind == hitEditLine:
		shape = "text"
	case m.mouse.hover.kind == hitColHeader, m.mouse.hover.kind == hitRowHeader, m.mouse.hover.kind == hitCorner, m.mouse.hover.kind == hitFilterButton,
		isTabHit(m.mouse.hover.kind):
		shape = "pointer"
	}
	if shape == m.mouse.shape {
		return nil
	}
	m.mouse.shape = shape
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
