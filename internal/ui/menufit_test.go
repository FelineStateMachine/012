package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Every menu of the menu bar shows whole, its last item and its bottom
// border, on a terminal 30 rows tall, above the status line.
func TestMenusFitThirtyRows(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for i, menu := range menuBar {
		m.showBarMenu(i)
		b := m.overlay.Layout()[0]
		if bottom := b.Y + len(b.Lines); bottom > m.height-1 {
			t.Errorf("%s reaches row %d of %d", menu.title, bottom, m.height)
		}
		last := menu.items[len(menu.items)-1].label()
		if !strings.Contains(screen(m), last) {
			t.Errorf("%s: %q isn't shown", menu.title, last)
		}
		m.closeOverlay()
	}
}
