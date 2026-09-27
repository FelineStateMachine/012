package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"012/internal/sheet"
)

func rightClick(m *Model, x, y int) {
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseRight})
	mouseAt(m, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseRight})
}

func menuLabels(t *testing.T, m *Model) string {
	t.Helper()
	var got []string
	for _, it := range openMenu(t, m).top().items {
		if it.sep {
			got = append(got, "-")
		} else {
			got = append(got, it.label())
		}
	}
	return strings.Join(got, ",")
}

// tallModel has room for full-height context menus below the click.
func tallModel() *Model {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	return m
}

func TestRightClickCellMenu(t *testing.T) {
	copied := fakeCommand(t, "edit.copy", "Copy", nil)
	m := tallModel()
	press(t, m, "<shift+down>", "<shift+right>") // A1:B2
	// Inside the selection, the selection stays.
	rightClick(m, cellX(1), gridTop+1)
	if m.selection().String() != "A1:B2" {
		t.Errorf("right-click inside moved the selection to %s", m.selection())
	}
	if got := menuLabels(t, m); got != "Cut,Copy,Paste,Paste values only,-,Insert row above,Insert row below,Insert column left,Insert column right,-,Delete row,Delete column,-,Clear" {
		t.Errorf("cell menu %s", got)
	}
	o := openMenu(t, m)
	if b := o.layout(m)[0]; b.x != cellX(1) || b.y != gridTop+2 {
		t.Errorf("menu at %d,%d", b.x, b.y)
	}
	press(t, m, "<down>", "<enter>")
	if *copied != 1 || m.overlay != nil {
		t.Errorf("copy ran %d times", *copied)
	}
	// Outside the selection, the active cell moves there first.
	rightClick(m, cellX(3), gridTop+5)
	if m.cur != addr("D6") || m.hasRange() {
		t.Errorf("right-click outside: cur %v range %v", m.cur, m.hasRange())
	}
	// Right-clicking elsewhere with a menu open moves the menu.
	rightClick(m, cellX(0), gridTop)
	if m.cur != addr("A1") || m.overlay == nil {
		t.Errorf("second right-click: cur %v overlay %T", m.cur, m.overlay)
	}
	press(t, m, "<esc>")
	if m.overlay != nil || m.mode != modeReady {
		t.Error("Esc did not close the context menu")
	}
}

func TestRightClickHeaders(t *testing.T) {
	m := tallModel()
	rightClick(m, cellX(2), headerLine)
	if r := m.selection(); r.From != addr("C1") || r.To.Row != sheet.MaxRows-1 || r.To.Col != 2 {
		t.Errorf("column header selected %s", r)
	}
	if got := menuLabels(t, m); got != "Cut,Copy,Paste,-,Insert column left,Insert column right,-,Delete column,Clear,-,Resize column,Reset column width,-,Sort sheet A to Z,Sort sheet Z to A,-,Create a filter,Remove filter" {
		t.Errorf("column menu %s", got)
	}
	press(t, m, "r")
	if highlighted(t, m) != "Resize column" {
		t.Errorf("r highlighted %q", highlighted(t, m))
	}
	press(t, m, "<enter>")
	if m.mode != modePrompt || m.prompt.kind != promptWidth {
		t.Errorf("Resize column: mode %v", m.mode)
	}
	press(t, m, "<esc>")
	rightClick(m, 1, gridTop+3)
	if r := m.selection(); r.From != addr("A4") || r.To.Col != sheet.MaxCols-1 {
		t.Errorf("row header selected %s", r)
	}
	if got := menuLabels(t, m); got != "Cut,Copy,Paste,-,Insert row above,Insert row below,-,Delete row,Clear" {
		t.Errorf("row menu %s", got)
	}
}

func TestShiftF10OpensCellMenu(t *testing.T) {
	m := tallModel()
	press(t, m, "<right>", "<down>", "<shift+f10>")
	b := openMenu(t, m).layout(m)[0]
	if b.x != cellX(1)-2 || b.y != gridTop+2 {
		t.Errorf("menu at %d,%d", b.x, b.y)
	}
}
