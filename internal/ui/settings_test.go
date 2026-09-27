package ui

import (
	"regexp"
	"strings"
	"testing"
)

// File > Settings > Decimal arithmetic toggles the setting with a ✓ in
// the menu and "decimal" on the status line, marks the file modified,
// and Ctrl+Z turns it back off.
func TestDecimalSetting(t *testing.T) {
	m := newModel()
	press(t, m, "=0.1+0.2=0.3", "<enter>")
	value := func() string { return m.sheet.Value(addr("A1")).String() }
	if value() != "FALSE" {
		t.Fatalf("binary: A1 = %s", value())
	}
	press(t, m, "<alt+f>")
	press(t, m, "<up>", "<up>", "<right>")
	if s := screen(m); !strings.Contains(s, "Decimal arithmetic") || strings.Contains(s, "✓") {
		t.Fatalf("settings submenu:\n%s", s)
	}
	press(t, m, "<enter>")
	if value() != "TRUE" || !strings.Contains(line(m, contextLine), "Decimal arithmetic on") {
		t.Fatalf("on: A1 = %s, context %q", value(), line(m, contextLine))
	}
	last := line(m, m.height-1)
	if !strings.Contains(last, "modified  decimal") {
		t.Errorf("status line: %q", last)
	}
	press(t, m, "<alt+f>", "<up>", "<up>", "<right>")
	if !regexp.MustCompile(`Decimal arithmetic +✓`).MatchString(screen(m)) {
		t.Errorf("no check mark:\n%s", screen(m))
	}
	press(t, m, "<esc>", "<esc>", "<ctrl+z>")
	if value() != "FALSE" || m.sheet.Decimal() || strings.Contains(line(m, m.height-1), "decimal") {
		t.Errorf("undo: A1 = %s, decimal %v, status %q", value(), m.sheet.Decimal(), line(m, m.height-1))
	}
}
