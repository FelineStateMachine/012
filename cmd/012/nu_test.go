package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// 012 nu opens a new file as a notebook, at its prompt.
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
	if !strings.Contains(lines[0], " NU ") || !strings.Contains(lines[1], "nu❯") {
		t.Errorf("not at the prompt:\n%s", screen)
	}
	if !strings.Contains(lines[len(lines)-1], "Shell 1") {
		t.Errorf("no Shell 1 tab: %q", lines[len(lines)-1])
	}
}
