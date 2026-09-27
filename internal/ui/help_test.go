package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

func TestShortcutsListEveryBoundCommand(t *testing.T) {
	rows := helpRows()
	for key, id := range keymap {
		c := commands[id]
		if c == nil {
			continue
		}
		found := false
		for _, r := range rows {
			if r.action == c.title && strings.Contains(strings.Join(r.keys, " "), theme.KeyLabel(key)) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s (%s) missing from the shortcuts", c.title, theme.KeyLabel(key))
		}
	}
}

func TestShortcutsOverlay(t *testing.T) {
	m := newModel()
	press(t, m, "<f1>")
	s, ok := m.overlay.(*shortcuts)
	if !ok || !strings.HasSuffix(line(m, menuLine), "HELP") {
		t.Fatalf("F1 opened %T", m.overlay)
	}
	scr := screen(m)
	for _, want := range []string{"Keyboard shortcuts", "Moving around", "Ctrl+G   F5"} {
		if !strings.Contains(scr, want) {
			t.Errorf("shortcuts missing %q:\n%s", want, scr)
		}
	}
	// Taller than 20 lines, so it scrolls, and says where it is.
	if !strings.Contains(scr, "1-") {
		t.Errorf("no scroll position in footer:\n%s", scr)
	}
	press(t, m, "<down>", "<down>")
	if s.top != 2 {
		t.Errorf("down scrolled to %d", s.top)
	}
	send(m, tea.MouseWheelMsg{X: 40, Y: 10, Button: tea.MouseWheelUp})
	if m.View(); s.top != 0 {
		t.Errorf("wheel up scrolled to %d", s.top)
	}
	press(t, m, "<end>")
	bottom := s.top
	press(t, m, "<down>")
	if bottom == 0 || s.top != bottom {
		t.Errorf("End scrolled to %d, then Down to %d", bottom, s.top)
	}
	press(t, m, "<esc>")
	if m.overlay != nil || m.mode != modeReady {
		t.Error("Esc did not close the shortcuts")
	}
	press(t, m, "<ctrl+/>")
	if _, ok := m.overlay.(*shortcuts); !ok {
		t.Error("Ctrl+/ did not open the shortcuts")
	}
	leftClick(m, 0, m.height-1)
	if m.overlay != nil {
		t.Error("clicking outside did not close the shortcuts")
	}
}

func TestShortcutsTwoColumnsWhenWide(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	press(t, m, "<f1>")
	lines, w := m.overlay.(*shortcuts).lines(m)
	if w < 100 || len(lines) > len(helpRows()) {
		t.Errorf("width %d, %d lines for %d rows", w, len(lines), len(helpRows()))
	}
}

func TestFunctionListInsertsFunction(t *testing.T) {
	m := newModel()
	m.runCommand("help.functions")
	press(t, m, "iferr")
	if got := selected(t, m); !strings.HasPrefix(got, "IFERROR(") {
		t.Fatalf("selected %q", got)
	}
	if !strings.Contains(line(m, m.height-1), "Enter  insert") {
		t.Errorf("status %q", line(m, m.height-1))
	}
	press(t, m, "<enter>")
	if m.mode != modeEnter || m.line.text() != "=IFERROR(" {
		t.Errorf("mode %v buf %q", m.mode, m.line.text())
	}
}

// Overlays must survive screens smaller than they are.
func TestOverlaysOnTinyScreens(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {20, 6}, {3, 2}} {
		for _, keys := range [][]string{{"<alt+o>", "<right>"}, {"<ctrl+k>", "s"}, {"<f1>", "<end>"}, {"1", "<enter>", "<ctrl+q>"}, {"=su"}, {"<alt+d>", "n"}} {
			m := newModel()
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			press(t, m, keys...)
			m.View()
			rightClick(m, size[0]-1, size[1]-1)
			m.View()
		}
	}
}

func TestAbout(t *testing.T) {
	m := newModel()
	m.runCommand("help.about")
	if l := line(m, contextLine); !strings.HasPrefix(l, "012 devel") || !strings.Contains(l, "Esc  Close") {
		t.Errorf("about %q", l)
	}
	press(t, m, "<esc>")
	if m.overlay != nil || line(m, contextLine) != "" {
		t.Errorf("Esc left %T %q", m.overlay, line(m, contextLine))
	}
}
