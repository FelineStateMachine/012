package ui

import "testing"

// Word keys edit the entry by word in ENTER and EDIT.
func TestEntryWordKeys(t *testing.T) {
	m := newModel()
	press(t, m, "=SUM(A1:B2", "<ctrl+backspace>")
	if got := m.line.Text(); got != "=SUM(A1:" {
		t.Fatalf("ctrl+backspace left %q", got)
	}
	press(t, m, "<alt+backspace>", "<f2>", "<ctrl+left>")
	if m.mode != modeEdit || m.line.Pos != 5 || m.cur != addr("A1") {
		t.Errorf("ctrl+left in EDIT: mode %v, caret %d, cell %v", m.mode, m.line.Pos, m.cur)
	}
	press(t, m, "<ctrl+u>")
	if got := m.line.Text(); got != "A1" {
		t.Errorf("ctrl+u left %q", got)
	}
}
