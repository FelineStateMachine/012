package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNoteEdit(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "12", "<enter>", "<up>", "<shift+f2>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Note on B1:") || m.indicator() != "NOTE" {
		t.Fatalf("prompt %q, %s", l, m.indicator())
	}
	press(t, m, "Checked", "<alt+enter>", "twice", "<enter>")
	if got := m.sheet.Note(addr("B1")); got != "Checked\ntwice" {
		t.Fatalf("note %q", got)
	}
	// The active cell's note is on the context line, and its cell has
	// the mark in its corner.
	if l := line(m, contextLine); !strings.Contains(l, "Note  Checked ↵ twice") {
		t.Errorf("context line %q", l)
	}
	if row := line(m, gridTop); !strings.Contains(row, "12▝") {
		t.Errorf("no mark: %q", row)
	}
	// Editing starts from the note, line breaks shown as ↵.
	press(t, m, "<shift+f2>")
	if l := line(m, contextLine); !strings.Contains(l, "Checked↵twice") {
		t.Errorf("editing %q", l)
	}
	press(t, m, "<esc>")
	// Clearing the cell keeps the note; undo takes it away.
	press(t, m, "<delete>")
	if m.sheet.Note(addr("B1")) == "" {
		t.Error("Delete removed the note")
	}
	press(t, m, "<ctrl+z>", "<ctrl+z>")
	if m.sheet.Note(addr("B1")) != "" {
		t.Errorf("undo left %q", m.sheet.Note(addr("B1")))
	}
	press(t, m, "<ctrl+y>")
	m.runCommand("note.delete")
	if m.sheet.Note(addr("B1")) != "" || !strings.Contains(line(m, contextLine), "Deleted 1 note") {
		t.Errorf("delete: %q, %q", m.sheet.Note(addr("B1")), line(m, contextLine))
	}
}

func TestNoteHover(t *testing.T) {
	m := newModel()
	m.sheet.SetNote(addr("B2"), "A long note that wraps over more than one line in its box")
	send(m, tea.MouseMotionMsg{X: cellX(1), Y: gridTop + 1})
	s := screen(m)
	if !strings.Contains(s, "┌─ Note ─") || !strings.Contains(s, "A long note that wraps over") || !strings.Contains(s, "more than one line in its box") {
		t.Errorf("no hover box:\n%s", s)
	}
	send(m, tea.MouseMotionMsg{X: cellX(3), Y: gridTop + 1})
	if strings.Contains(screen(m), "Note") {
		t.Errorf("box stays after leaving the cell:\n%s", screen(m))
	}
}
