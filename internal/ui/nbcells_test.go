package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// notebookOf is a notebook model with code cells of srcs, the first
// selected, in command mode.
func notebookOf(t *testing.T, out map[string]string, srcs ...string) (*Model, *fakeNu) {
	t.Helper()
	m, nu := notebookModel(t, out)
	var cells []notebook.Cell
	for _, s := range srcs {
		cells = append(cells, notebook.Cell{Source: s})
	}
	m.setCells("cells", cells)
	m.nbView().Select(0, false)
	return m, nu
}

// screenX is the column text starts at on screen line y.
func screenX(t *testing.T, m *Model, y int, text string) int {
	t.Helper()
	l := line(m, y)
	i := strings.Index(l, text)
	if i < 0 {
		t.Fatalf("line %d lacks %q: %q", y, text, l)
	}
	return ansi.StringWidth(l[:i])
}

func TestNotebookMovesCells(t *testing.T) {
	m, _ := notebookOf(t, nil, "a", "b", "c")
	press(t, m, "<down>", "<alt+down>")
	if got := cells(m); !slices.Equal(got, []string{"a", "c", "b"}) {
		t.Fatalf("Alt+Down: %q", got)
	}
	if i, _ := m.nbView().Selected(); i != 2 {
		t.Errorf("the cell moved isn't selected: %d", i)
	}
	press(t, m, "<shift+up>", "<alt+up>")
	if got := cells(m); !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("two cells up: %q", got)
	}
	if from, to := m.nbView().Range(); from != 0 || to != 1 {
		t.Errorf("the cells moved aren't selected: %d to %d", from, to)
	}
	press(t, m, "z")
	if got := cells(m); !slices.Equal(got, []string{"a", "c", "b"}) {
		t.Errorf("undo: %q", got)
	}
}

func TestNotebookActsOnSeveralCells(t *testing.T) {
	m, nu := notebookOf(t, map[string]string{"a": "1", "b": "2"}, "a", "b", "c")
	press(t, m, "<shift+down>", "c", "<end>", "V")
	if got := cells(m); !slices.Equal(got, []string{"a", "b", "a", "b", "c"}) {
		t.Fatalf("copy, paste above: %q", got)
	}
	if from, to := m.nbView().Range(); from != 2 || to != 3 {
		t.Errorf("what was pasted isn't selected: %d to %d", from, to)
	}
	press(t, m, "d", "d")
	if got := cells(m); !slices.Equal(got, []string{"a", "b", "c"}) || !strings.Contains(m.note, "cells 3 to 4") {
		t.Fatalf("d d: %q, %q", got, m.note)
	}
	press(t, m, "z")
	if got := cells(m); len(got) != 5 {
		t.Errorf("z: %q", got)
	}
	press(t, m, "<home>", "<shift+down>", "<ctrl+enter>")
	if got := nu.ran(); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("running two cells ran %q", got)
	}
	press(t, m, "m")
	if c := m.sheet.NotebookCells(); c[0].Kind != notebook.Note || c[1].Kind != notebook.Note || c[2].Kind != notebook.Code {
		t.Error("m made the two cells notes")
	}
}

// The toolbar's buttons run their commands; Code ▾ opens the kinds.
func TestNotebookToolbar(t *testing.T) {
	m, nu := notebookOf(t, map[string]string{"ls": lsOut}, "ls")
	if got := line(m, formulaLine); !strings.Contains(got, "▶ Run") || !strings.Contains(got, "nu ○ idle") {
		t.Fatalf("the toolbar: %q", got)
	}
	send(m, tea.MouseClickMsg{X: screenX(t, m, formulaLine, "▶ Run") + 1, Y: formulaLine, Button: tea.MouseLeft})
	if len(nu.jobs) != 1 || !strings.Contains(screen(m), "Out[1]:") {
		t.Fatalf("▶ Run ran %v:\n%s", nu.ran(), screen(m))
	}
	send(m, tea.MouseClickMsg{X: screenX(t, m, formulaLine, "Code ▾"), Y: formulaLine, Button: tea.MouseLeft})
	if m.overlay == nil {
		t.Fatal("Code ▾ didn't open the kinds")
	}
	press(t, m, "<down>", "<enter>")
	if c := m.sheet.NotebookCells(); c[len(c)-1].Kind != notebook.Note {
		t.Errorf("Markdown didn't make the cell a note: %+v", c)
	}
}

// A click on a cell's ▶ runs it, and a click in its box edits it.
func TestNotebookMouse(t *testing.T) {
	m, nu := notebookOf(t, map[string]string{"ls": lsOut}, "ls", "b")
	y := nbBody + 1 // the first cell's code
	send(m, tea.MouseClickMsg{X: m.width - 1, Y: y, Button: tea.MouseLeft})
	if len(nu.jobs) != 1 {
		t.Fatalf("▶ ran %v", nu.ran())
	}
	send(m, tea.MouseClickMsg{X: 14, Y: y, Button: tea.MouseLeft})
	if !m.nbView().Editing() || !strings.Contains(line(m, menuLine), "EDIT") {
		t.Error("a click in the box didn't edit the cell")
	}
	send(m, tea.MouseClickMsg{X: 14, Y: y, Button: tea.MouseRight})
	if m.overlay == nil {
		t.Error("a right click didn't open the cell menu")
	}
}

func TestNotebookGoToCell(t *testing.T) {
	m, _ := notebookOf(t, nil, "a", "sales = open x.csv", "c")
	press(t, m, "<ctrl+g>")
	if m.overlay == nil {
		t.Fatal("Ctrl+G opened nothing")
	}
	m.line.Set("sales")
	m.overlay.(interface{ Changed() }).Changed()
	press(t, m, "<enter>")
	if i, _ := m.nbView().Selected(); i != 1 || m.overlay != nil {
		t.Errorf("went to cell %d", i+1)
	}
	run(m, m.runCommand("nb.toc"))
	if !strings.Contains(m.note, "no headings") {
		t.Errorf("a table of contents without headings: %q", m.note)
	}
}

func TestNotebookClearsOneOutput(t *testing.T) {
	m, _ := notebookOf(t, map[string]string{"a": "1", "b": "2"}, "a", "b")
	run(m, m.runCommand("nb.run_all"))
	run(m, m.runCommand("nb.clear_output"))
	c := m.sheet.NotebookCells()
	if m.book().Output(c[0].ID) != nil || m.book().Output(c[1].ID) == nil {
		t.Error("Clear output cleared the wrong outputs")
	}
}
