package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/confine"
)

// Save as onto another file that exists asks first: Enter replaces it,
// Esc keeps it. Saving as the open file, or a new name, doesn't ask.
func TestSaveAsAsksBeforeReplacing(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSheet(t, "other.012", "theirs")
	m := newModel()
	press(t, m, "mine", "<enter>")

	run(m, m.openSave())
	press(t, m, "other", "<enter>")
	if _, ok := m.overlay.(*choiceBar); !ok || !strings.Contains(line(m, contextLine), "other.012 exists.") ||
		!strings.Contains(line(m, contextLine), "Replace") {
		t.Fatalf("no question: overlay %T, %q", m.overlay, line(m, contextLine))
	}
	press(t, m, "<esc>")
	if got, _ := readSheet("other.012"); a1Of(got) != "theirs" || m.filename != "" || !m.changed {
		t.Fatalf("Esc wrote it: A1 %q, file %q, changed %v", a1Of(got), m.filename, m.changed)
	}

	run(m, m.openSave())
	press(t, m, "other", "<enter>", "<enter>")
	if got, _ := readSheet("other.012"); a1Of(got) != "mine" || m.filename != "other.012" || m.changed {
		t.Fatalf("Enter didn't replace: A1 %q, file %q, changed %v", a1Of(got), m.filename, m.changed)
	}

	// Now it's the open file: Save as onto it saves without asking, as
	// Save does.
	press(t, m, "<up>", "again", "<enter>")
	run(m, m.openSave())
	press(t, m, "<enter>")
	if got, _ := readSheet("other.012"); m.overlay != nil || a1Of(got) != "again" {
		t.Fatalf("Save as the open file: overlay %T, A1 %q", m.overlay, a1Of(got))
	}
	// A new name doesn't ask.
	run(m, m.openSave())
	press(t, m, "fresh", "<enter>")
	if _, err := os.Stat("fresh.012"); err != nil || m.overlay != nil {
		t.Fatalf("new name: %v, overlay %T", err, m.overlay)
	}
}

// In a served session the file is looked for inside the served
// directory, and :w name and :wq name go the same way; cancelling :wq's
// question cancels its quit.
func TestServedSaveAsAndVimAsk(t *testing.T) {
	top := t.TempDir()
	served := filepath.Join(top, "served")
	os.Mkdir(served, 0o755)
	writeSheet(t, filepath.Join(served, "kept.012"), "kept")
	t.Chdir(top)
	root, err := confine.New(served)
	if err != nil {
		t.Fatal(err)
	}
	m := vimModel(t)
	m.Serve(root, []string{"TERM=xterm-256color"})
	press(t, m, "x", ":w kept", "<enter>")
	if !strings.Contains(line(m, contextLine), "kept.012 exists.") {
		t.Fatalf(":w kept: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if got, _ := readSheet(filepath.Join(served, "kept.012")); a1Of(got) != "kept" {
		t.Fatalf("Esc wrote it: %q", a1Of(got))
	}
	if _, ok := press(t, m, ":wq kept", "<enter>", "<esc>").(tea.QuitMsg); ok {
		t.Fatal("cancelled :wq quit")
	}
	if m.quitAfterSave {
		t.Error("cancelled :wq still waits to quit")
	}
	if _, ok := press(t, m, ":wq kept", "<enter>", "<enter>").(tea.QuitMsg); !ok {
		t.Fatal(":wq kept, Replace didn't quit")
	}
	if got, _ := readSheet(filepath.Join(served, "kept.012")); a1Of(got) == "kept" {
		t.Error("Replace didn't write the served file")
	}
	// A file by that name in the server's working directory isn't the
	// served one: no question for a name that's new inside.
	writeSheet(t, filepath.Join(top, "cwd.012"), "cwd")
	press(t, m, ":w cwd", "<enter>")
	if m.overlay != nil {
		t.Errorf(":w cwd asked about the working directory's file: %q", line(m, contextLine))
	}
}
