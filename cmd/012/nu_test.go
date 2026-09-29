package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// 012 nu opens a new file as a notebook, editing its first cell.
func TestNuOpensNotebook(t *testing.T) {
	e, _, _ := testEnv(t, nil)
	var screen string
	e.runTUI = func(m tea.Model, _ ...tea.ProgramOption) error {
		m.Init()
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
		screen = ansi.Strip(m.View().Content)
		return nil
	}
	if err := run([]string{"nu", filepath.Join(t.TempDir(), "book.012")}, e); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(screen, "\n")
	if !strings.HasSuffix(strings.TrimSpace(lines[0]), "EDIT") || !strings.Contains(screen, "[ ]") {
		t.Errorf("not editing a cell:\n%s", screen)
	}
	if !strings.Contains(lines[len(lines)-1], "❯ Notebook") {
		t.Errorf("no notebook tab: %q", lines[len(lines)-1])
	}
}
