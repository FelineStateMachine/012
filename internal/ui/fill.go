package ui

import (
	"log/slog"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// The fill handle, as in Sheets: the bottom-right corner of the selection
// shows a small handle (▟) when the mouse is over that cell. Dragging it
// down, up, right or left fills the cells it passes over, continuing a
// series (1, 2, 3; Jan, Feb; Item 1, Item 2) or copying. While dragging,
// the range that will be filled is highlighted and the context line says
// what releasing does; Esc cancels.

// fillCorner is the cell with the fill handle: the bottom-right corner of
// the selection. Whole rows and columns have none.
func (m *Model) fillCorner() sheet.Addr {
	if m.whole != wholeNone {
		return sheet.Addr{Col: -1, Row: -1}
	}
	return m.selection().To
}

// showFillHandle reports whether the fill handle is drawn in cell a:
// while the mouse is over the corner cell, or dragging the handle.
func (m *Model) showFillHandle(a sheet.Addr) bool {
	if m.mode != modeReady || a != m.fillCorner() {
		return false
	}
	switch {
	case m.drag == dragFill:
		return true
	case m.drag != dragNone:
		return false
	}
	return (m.hover.kind == hitCell || m.hover.kind == hitFillHandle) && m.hover.addr == a
}

// startFill starts dragging the fill handle.
func (m *Model) startFill() {
	m.drag = dragFill
	m.fillAt, m.fillTo = m.fillCorner(), m.selection()
}

// dragFillTo points the fill at cell a: the selection grows along the
// axis a is further out on, as in Sheets.
func (m *Model) dragFillTo(a sheet.Addr) {
	m.fillAt = a
	src := m.selection()
	down, up := a.Row-src.To.Row, src.From.Row-a.Row
	right, left := a.Col-src.To.Col, src.From.Col-a.Col
	r := src
	switch {
	case max(down, up) <= 0 && max(right, left) <= 0:
	case max(down, up) >= max(right, left):
		if down > 0 {
			r.To.Row = a.Row
		} else {
			r.From.Row = a.Row
		}
	case right > 0:
		r.To.Col = a.Col
	default:
		r.From.Col = a.Col
	}
	m.fillTo = r
}

// finishFill fills the range the handle was dragged over and selects it.
func (m *Model) finishFill() {
	m.drag = dragNone
	src, dst := m.selection(), m.fillTo
	if dst == src {
		return
	}
	span := telemetry.Start("fill", slog.Int("cells", (dst.To.Row-dst.From.Row+1)*(dst.To.Col-dst.From.Col+1)))
	got, err := m.sheet.FillSeries(src, dst)
	span.Fail(err)
	if err != nil {
		m.fail(err.Error())
		return
	}
	m.changed = true
	m.selectRect(got)
}

// cancelFill stops a fill handle drag without filling.
func (m *Model) cancelFill() {
	m.drag, m.autoscrolling = dragNone, false
	m.fillTo = m.selection()
}

// fillLine is the context line while dragging the fill handle.
func (m *Model) fillLine() string {
	if m.fillTo == m.selection() {
		return m.th.Key.Render("Fill") + m.th.Muted.Render("   drag down, up, right or left to fill a series or copy   ") + m.th.KeyHints("Esc", "cancel")
	}
	return m.th.Key.Render("Fill "+m.fillTo.String()) + m.th.Muted.Render("   release to fill   ") + m.th.KeyHints("Esc", "cancel")
}
