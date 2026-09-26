package ui

import (
	"strings"
	"testing"
)

func TestControlPanelLayout(t *testing.T) {
	m := newModel()
	if l := line(m, menuLine); !strings.HasPrefix(l, " File  Edit") || !strings.HasSuffix(l, "READY") {
		t.Errorf("menu bar %q", l)
	}
	press(t, m, "<shift+down>", "<shift+right>")
	if l := line(m, formulaLine); strings.TrimSpace(l) != "A1:B2" {
		t.Errorf("name box should show the selection: %q", l)
	}
	press(t, m, "<esc>", "=1+")
	if x, y, _ := m.cursorPos(); y != formulaLine || x != formulaBarTextX()+3 {
		t.Errorf("entry cursor at %d,%d", x, y)
	}
	if !strings.Contains(line(m, contextLine), "Enter accept") {
		t.Errorf("context line %q", line(m, contextLine))
	}
	press(t, m, "<esc>", "<ctrl+g>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Go to: A1") || !strings.HasSuffix(l, "Esc cancel") {
		t.Errorf("prompt line %q", l)
	}
	if x, y, _ := m.cursorPos(); y != contextLine || x != len("Go to: A1") {
		t.Errorf("prompt cursor at %d,%d", x, y)
	}
}
