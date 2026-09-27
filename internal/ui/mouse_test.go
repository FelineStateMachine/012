package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

func leftAt(x, y int) tea.Mouse { return tea.Mouse{X: x, Y: y, Button: tea.MouseLeft} }

// barText is the text in the formula bar after the name box.
func barText(m *Model) string {
	l := line(m, formulaLine)
	if len(l) <= formulaBarTextX() {
		return ""
	}
	return strings.TrimSpace(l[formulaBarTextX():])
}

func TestClickWhileTypingAccepts(t *testing.T) {
	m := newModel()
	press(t, m, "hello")
	click(m, cellX(2), gridTop+3, 0)
	if input(m, "A1") != "hello" || m.cur != addr("C4") || m.mode != modeReady {
		t.Errorf("A1 %q cur %v mode %v", input(m, "A1"), m.cur, m.mode)
	}
}

func TestClickInsertsReferenceIntoFormula(t *testing.T) {
	m := newModel()
	press(t, m, "5", "<enter>", "7", "<enter>", "=")
	click(m, cellX(0), gridTop, 0)
	if m.mode != modePoint || barText(m) != "=A1" {
		t.Fatalf("mode %v formula bar %q", m.mode, barText(m))
	}
	// Dragging makes it a range.
	send(m, tea.MouseClickMsg(leftAt(cellX(0), gridTop)))
	send(m, tea.MouseMotionMsg(leftAt(cellX(0), gridTop+1)))
	send(m, tea.MouseReleaseMsg(leftAt(cellX(0), gridTop+1)))
	if barText(m) != "=A1:A2" {
		t.Fatalf("formula bar %q", barText(m))
	}
	press(t, m, "*2", "<enter>")
	if got := m.sheet.Cell(addr("A3")); got == nil || got.Input != "=A1:A2*2" {
		t.Errorf("A3 = %+v", got)
	}
}

func TestClickFormulaBarEdits(t *testing.T) {
	m := newModel()
	press(t, m, "=1+2", "<enter>", "<up>")
	line0, x0 := formulaBarAt()
	click(m, x0+2, line0, 0)
	if m.mode != modeEdit || m.line.pos != 2 {
		t.Fatalf("mode %v caret %d", m.mode, m.line.pos)
	}
	// Clicking in the edit line moves the caret.
	line1, x1 := editLineAt()
	click(m, x1+4, line1, 0)
	if m.line.pos != 4 {
		t.Errorf("caret %d", m.line.pos)
	}
}

func TestResizeColumnByDragging(t *testing.T) {
	m := newModel()
	border := minRowHdrW + sheet.DefaultWidth - 1
	send(m, tea.MouseMotionMsg{X: border, Y: headerLine})
	if m.mouse.hover.kind != hitColBorder || !strings.Contains(line(m, headerLine), "▐") {
		t.Fatalf("hover %v header %q", m.mouse.hover.kind, line(m, headerLine))
	}
	send(m, tea.MouseClickMsg(leftAt(border, headerLine)))
	send(m, tea.MouseMotionMsg(leftAt(border+5, headerLine)))
	if w := m.sheet.ColWidth(0); w != sheet.DefaultWidth+5 {
		t.Errorf("width while dragging %d", w)
	}
	if !strings.Contains(line(m, 2), "Column A width 15") {
		t.Errorf("context line %q", line(m, 2))
	}
	send(m, tea.MouseReleaseMsg(leftAt(border+5, headerLine)))
	if m.mouse.drag != dragNone || !m.changed {
		t.Error("resize did not finish")
	}
}

func TestDoubleClickBorderAutofits(t *testing.T) {
	m := newModel()
	press(t, m, "A much longer label", "<enter>")
	border := minRowHdrW + sheet.DefaultWidth - 1
	click(m, border, headerLine, 0)
	click(m, border, headerLine, 0)
	if w := m.sheet.ColWidth(0); w != len("A much longer label")+2 {
		t.Errorf("autofit width %d", w)
	}
}

func TestDragAutoscrolls(t *testing.T) {
	m := newModel()
	send(m, tea.MouseClickMsg(leftAt(cellX(0), gridTop)))
	_, cmd := m.Update(tea.MouseMotionMsg(leftAt(cellX(0), m.height-1)))
	if cmd == nil || !m.mouse.autoscrolling {
		t.Fatal("dragging below the grid did not start autoscroll")
	}
	bottom := m.ext.Row
	for range 5 {
		send(m, autoscrollMsg{})
	}
	if m.ext.Row != bottom+5 || m.top == 0 {
		t.Errorf("ext %v top %d", m.ext, m.top)
	}
	send(m, tea.MouseReleaseMsg(leftAt(cellX(0), m.height-1)))
	send(m, autoscrollMsg{})
	if m.mouse.autoscrolling {
		t.Error("autoscroll kept going after release")
	}
}

func TestPointerShapes(t *testing.T) {
	m := newModel()
	for _, tt := range []struct {
		x, y int
		want string
	}{
		{cellX(1), gridTop + 1, "cell"},
		{minRowHdrW + sheet.DefaultWidth - 1, headerLine, "col-resize"},
		{cellX(1), headerLine, "pointer"},
		{formulaBarTextX() + 1, formulaLine, "text"},
		{m.width - 1, m.height - 1, "default"},
		{1, m.height - 1, "pointer"}, // the sheet tab
	} {
		send(m, tea.MouseMotionMsg{X: tt.x, Y: tt.y})
		if m.mouse.shape != tt.want {
			t.Errorf("at %d,%d shape %q, want %q", tt.x, tt.y, m.mouse.shape, tt.want)
		}
	}
}

func TestClickDismissesHelp(t *testing.T) {
	m := newModel()
	press(t, m, "<f1>")
	click(m, 1, m.height-1, 0) // outside the shortcuts overlay
	if m.mode != modeReady || m.overlay != nil {
		t.Errorf("mode %v overlay %T", m.mode, m.overlay)
	}
}
