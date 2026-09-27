package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// keyEvents is the terminal's answer when it reports key releases.
var keyEvents = tea.KeyboardEnhancementsMsg{Flags: 1 | ansi.KittyReportEventTypes}

func space() tea.Key { return tea.Key{Code: tea.KeySpace, Text: " "} }

// The view asks for key releases; Shift+Enter accepts an entry and moves
// up, and Ctrl+I, which only the protocol tells from Tab, is italic.
func TestKeyboardProtocol(t *testing.T) {
	m := newModel()
	if !m.View().KeyboardEnhancements.ReportEventTypes {
		t.Error("the view doesn't ask for key releases")
	}
	press(t, m, "<down>", "<down>", "5", "<shift+enter>")
	if input(m, "A3") != "5" || m.cur != addr("A2") {
		t.Errorf("Shift+Enter: A3 %q at %v", input(m, "A3"), m.cur)
	}
	press(t, m, "<down>", "<ctrl+i>")
	if c := m.sheet.Cell(addr("A3")); m.cur != addr("A3") || c == nil || !c.Style.Italic {
		t.Errorf("Ctrl+I: at %v", m.cur)
	}
	press(t, m, "<tab>")
	if m.cur != addr("B3") {
		t.Errorf("Tab: at %v", m.cur)
	}
	// A release with nothing held does nothing.
	if _, cmd := m.Update(tea.KeyReleaseMsg{Code: 'x', Text: "x"}); cmd != nil || m.mode != modeReady {
		t.Errorf("release: mode %v", m.mode)
	}
}

// Holding Space on a selected chart shows it across the grid until
// Space is let go, on a terminal that reports releases.
func TestHoldSpaceEnlargesChart(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "<enter>")
	c := m.sheet.Charts()[0]
	if strings.Contains(line(m, contextLine), "hold") {
		t.Errorf("hint without releases: %q", line(m, contextLine))
	}
	send(m, keyEvents)
	if !strings.Contains(line(m, contextLine), "hold Space to zoom") {
		t.Errorf("context: %q", line(m, contextLine))
	}
	send(m, tea.KeyPressMsg(space()))
	z := m.displayCharts()[0]
	if z.W <= c.W || z.H <= c.H || z.At != (sheet.Addr{Col: m.left, Row: m.top}) {
		t.Fatalf("held: %+v", z)
	}
	if !strings.Contains(line(m, m.height-1), "let go") {
		t.Errorf("held status: %q", line(m, m.height-1))
	}
	repeat := space()
	repeat.IsRepeat = true
	send(m, tea.KeyPressMsg(repeat))
	send(m, tea.KeyReleaseMsg(space()))
	if got := m.displayCharts()[0]; got.W != c.W || got.At != c.At || m.sheet.Charts()[0].W != c.W {
		t.Errorf("after release: shown %+v, stored %+v", got, m.sheet.Charts()[0])
	}
	if _, ok := m.overlay.(*chartSel); !ok {
		t.Errorf("release left %T", m.overlay)
	}
	// Any other key lets go first.
	send(m, tea.KeyPressMsg(space()))
	press(t, m, "<right>")
	if got := m.sheet.Charts()[0]; got.W != c.W || got.At.Col != c.At.Col+1 {
		t.Errorf("Right while held: %+v", got)
	}
}

// Without releases, Space on a selected chart deselects it and starts
// an entry, as it always has.
func TestSpaceOnChartWithoutReleases(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "<enter>")
	send(m, tea.KeyPressMsg(space()))
	if _, ok := m.overlay.(*chartSel); ok || m.mode != modeEnter {
		t.Errorf("Space: %T mode %v", m.overlay, m.mode)
	}
}

// Holding Space in the theme picker hides the list, showing the sheet
// in the highlighted theme, until it's let go.
func TestHoldSpaceInThemePicker(t *testing.T) {
	m, _, _ := configured(t, "theme = Nord\n")
	send(m, keyEvents)
	press(t, m, "<alt+f>")
	openSettingsItem(t, m, "Theme")
	press(t, m, "<down>")
	name := m.th.Name
	if !strings.Contains(screen(m), "Type a name") || !strings.Contains(line(m, m.height-1), "hold to peek") {
		t.Fatalf("picker: %q\n%s", line(m, m.height-1), screen(m))
	}
	send(m, tea.KeyPressMsg(space()))
	if s := screen(m); strings.Contains(s, "Type a name") || m.th.Name != name {
		t.Fatalf("held: theme %q\n%s", m.th.Name, s)
	}
	send(m, tea.KeyReleaseMsg(space()))
	if s := screen(m); !strings.Contains(s, "Type a name") || m.overlay == nil {
		t.Fatalf("released:\n%s", s)
	}
	// With a search typed, Space is typed into it.
	press(t, m, "no")
	send(m, tea.KeyPressMsg(space()))
	if m.line.Text() != "no " {
		t.Errorf("search: %q", m.line.Text())
	}
}
