package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/cmdline"
)

// The kitty keyboard protocol. Bubble Tea asks every terminal for its
// first level, which tells apart the keys legacy encodings merge:
// Shift+Enter from Enter (it accepts an entry and moves up, as in
// Sheets), Ctrl+I from Tab (italic), Ctrl+M from Enter, and Alt+letter
// from Esc followed by the letter. 012 also asks for key events
// (View.KeyboardEnhancements.ReportEventTypes), which say when a key is
// let go; holding a key then previews something until it is released:
// Space held on a selected chart shows it across the grid, and in the
// theme picker hides the list to show the sheet in the highlighted
// theme. A terminal without the protocol sends neither, and those keys
// do what they do there: Shift+Enter is Enter, Ctrl+I is Tab, and Space
// is typed.

// session is what lasts as long as the program, across files opened.
type session struct {
	history cmdline.History // lines run on the : line
	// releases is whether the terminal reports key releases, so a key
	// can be held.
	releases bool
}

// keyboard notes what the terminal said of the keyboard protocol.
func (m *Model) keyboard(msg tea.KeyboardEnhancementsMsg) {
	m.session.releases = msg.SupportsEventTypes()
}

// releaser is an overlay that acts when a key it holds is let go.
type releaser interface {
	Release(k tea.KeyReleaseMsg) tea.Cmd
}

// held reports whether an open overlay is waiting for a release.
func (m *Model) held() bool {
	_, ok := m.overlay.(releaser)
	return ok
}

// keyReleased passes a key's release to the overlay holding it.
func (m *Model) keyReleased(k tea.KeyReleaseMsg) tea.Cmd {
	if r, ok := m.overlay.(releaser); ok {
		return r.Release(k)
	}
	return nil
}

// holdKey starts holding Space on a selected chart, on a terminal that
// reports releases, and reports whether it took k. Repeats while it's
// held do nothing; any other key lets go first.
func (s *chartSel) holdKey(m *Model, c sheet.Chart, k tea.KeyPressMsg) bool {
	if k.String() != "space" || !m.session.releases {
		s.unzoom()
		return false
	}
	if !s.zoom {
		z := m.zoomed(c)
		s.preview, s.zoom = &z, true
	}
	return true
}

// Release ends a hold of Space.
func (s *chartSel) Release(k tea.KeyReleaseMsg) tea.Cmd {
	if k.String() == "space" {
		s.unzoom()
	}
	return nil
}

func (s *chartSel) unzoom() {
	if s.zoom {
		s.preview, s.zoom = nil, false
	}
}

// zoomed is chart c spread over the scrolling part of the grid.
func (m *Model) zoomed(c sheet.Chart) sheet.Chart {
	c.At = sheet.Addr{Col: m.left, Row: m.top}
	x, y := m.chartScreen(c)
	c.W = min(max(m.width-x, sheet.MinChartW), sheet.MaxChartW)
	c.H = min(max(gridTop+m.visibleRows()-y, sheet.MinChartH), sheet.MaxChartH)
	return c
}

// chartOrder is the order charts are drawn in, bottom first: as they
// are, but with a chart held open on top.
func (m *Model) chartOrder(n int) []int {
	order := make([]int, 0, n)
	z := -1
	if s, ok := m.overlay.(*chartSel); ok && s.zoom {
		z = s.i
	}
	for i := range n {
		if i != z {
			order = append(order, i)
		}
	}
	if z >= 0 && z < n {
		order = append(order, z)
	}
	return order
}
