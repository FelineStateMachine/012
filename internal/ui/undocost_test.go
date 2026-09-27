package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A change whose undo step would pass the ceiling asks first: Esc backs
// out, Enter runs it without undo and forgets the history.
func TestUndoCostAsks(t *testing.T) {
	defer func(n int64) { maxStepBytes = n }(maxStepBytes)
	maxStepBytes = 100
	m := newModel()
	press(t, m, "1", "<enter>", "2", "<enter>", "3", "<enter>", "4", "<enter>", "5", "<enter>", "6", "<enter>", "7", "<enter>")
	press(t, m, "<up>", "<shift+up>", "<shift+up>", "<shift+up>", "<shift+up>", "<shift+up>", "<shift+up>")
	m.runCommand("clear")
	l := line(m, contextLine)
	if _, ok := m.overlay.(*choiceBar); !ok || !strings.Contains(l, "This can't be undone: it would take 0 MB of undo history.") {
		t.Fatalf("clear didn't ask: %q", l)
	}
	press(t, m, "<esc>")
	if m.sheet.Value(addr("A1")).Num != 1 || !m.sheet.CanUndo() {
		t.Fatalf("Esc went on: A1 %v", m.sheet.Value(addr("A1")))
	}
	m.runCommand("clear")
	press(t, m, "<enter>")
	if m.sheet.Value(addr("A1")).Kind != sheet.Empty || m.sheet.CanUndo() || !m.changed {
		t.Fatalf("after Enter: A1 %v, can undo %v, changed %v", m.sheet.Value(addr("A1")), m.sheet.CanUndo(), m.changed)
	}
	// Under the ceiling nothing asks.
	maxStepBytes = sheet.MaxStepBytes
	press(t, m, "8", "<enter>", "<up>")
	m.runCommand("clear")
	if m.overlay != nil || m.sheet.Value(addr("A1")).Kind != sheet.Empty || !m.sheet.CanUndo() {
		t.Errorf("small clear: overlay %T, A1 %v", m.overlay, m.sheet.Value(addr("A1")))
	}
}
