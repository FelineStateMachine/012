package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Notes follow Sheets' Insert > Note (Shift+F2): a cell with a note shows
// a mark in its top-right corner, the note is on the context line while
// the cell is active, and in a small box beside the cell while the mouse
// is over it. The note is typed on the context line, where Shift+Enter
// or Alt+Enter starts a new line, shown as ↵.

// noteBreak stands for a line break in a note being typed on the context
// line, which holds one line.
const noteBreak = "↵"

func init() {
	register(
		&command{id: "note.edit", title: "Note", desc: "Add or edit the active cell's note, shown when the cell is active or hovered",
			edits: cellTarget, run: (*Model).openNote},
		&command{id: "note.delete", title: "Delete notes", desc: "Delete the notes of the selected cells",
			edits: (*Model).selection, run: (*Model).deleteNotes},
	)
	keymap["shift+f2"] = "note.edit"
}

// openNote asks for the active cell's note on the context line, starting
// from the note it has.
func (m *Model) openNote() tea.Cmd {
	a := m.cur
	m.clearSelection()
	initial := strings.ReplaceAll(m.sheet.Note(a), "\n", noteBreak)
	m.openText("Note on "+a.String()+":", initial, func(m *Model, text string) tea.Cmd {
		note := strings.ReplaceAll(text, noteBreak, "\n")
		had := m.sheet.Note(a) != ""
		if err := m.sheet.SetNote(a, note); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.changed = true
		if strings.TrimSpace(note) == "" && had {
			m.note = "Deleted the note on " + a.String() + "; Ctrl+Z brings it back"
		}
		return nil
	})
	m.prompt.indicator = "NOTE"
	m.prompt.breaks = true
	m.prompt.fresh = false // typing adds to the note rather than replacing it
	return nil
}

// deleteNotes deletes the notes in the selection.
func (m *Model) deleteNotes() tea.Cmd {
	r := m.selection()
	switch n := m.sheet.ClearNotes(r); n {
	case 0:
		m.note = "No notes in " + r.String()
	case 1:
		m.changed = true
		m.note = "Deleted 1 note"
	default:
		m.changed = true
		m.note = "Deleted " + strconv.Itoa(n) + " notes"
	}
	return nil
}

// noteLine shows the active cell's note on the context line, its lines
// run together.
func (m *Model) noteLine() string {
	note := m.sheet.Note(m.cur)
	if note == "" {
		return ""
	}
	return m.th.Key.Render("Note") + "  " + strings.ReplaceAll(note, "\n", " "+m.th.Muted.Render(noteBreak)+" ")
}

// noteMark draws a cell's note mark over the last column of its text,
// like Sheets' corner triangle: in the cell's own role where it has one
// (the pointer, the selection), so it reads on any of them.
func (m *Model) noteMark(text string, a sheet.Addr, base lipgloss.Style, colored bool) string {
	w := m.sheet.ColWidth(a.Col)
	if w < 2 {
		return text
	}
	mark := m.th.NoteMark
	if colored {
		mark = base
	}
	return ansi.Truncate(text, w-1, "") + mark.Render("▝")
}

// noteMaxRows is how many lines of a note the hover box shows.
const noteMaxRows = 8

// noteBox is the note of the cell under the mouse, in a box beside the
// cell, while nothing else is going on.
func (m *Model) noteBox() (overlay.Box, bool) {
	h := m.mouse.hover
	if h.kind != hitCell && h.kind != hitCheckbox && h.kind != hitDropdown || m.mode != modeReady || m.mouse.drag != dragNone {
		return overlay.Box{}, false
	}
	note := m.sheet.Note(h.addr)
	if note == "" {
		return overlay.Box{}, false
	}
	inner := min(32, m.width-2)
	if inner < 8 {
		return overlay.Box{}, false
	}
	wrapped := strings.Split(ansi.Wrap(note, inner-2, ""), "\n")
	if len(wrapped) > noteMaxRows {
		wrapped = wrapped[:noteMaxRows]
		wrapped[noteMaxRows-1] = ansi.Truncate(wrapped[noteMaxRows-1], inner-3, "") + "…"
	}
	rows := make([]string, len(wrapped))
	for i, line := range wrapped {
		rows[i] = m.th.MenuBar.Render(theme.PadRight(" "+strings.TrimRight(line, " "), inner))
	}
	return m.besideCell(noteBoxID, h.addr, m.th.Frame(inner, "Note", "", rows)), true
}

// besideCell places a framed box beside the cell at a, its top on the
// cell's row: right of the cell, or left of it when there's no room,
// moved up as far as it must to stay above the status line, and never
// over the column headers.
func (m *Model) besideCell(id string, a sheet.Addr, lines []string) overlay.Box {
	w := ansi.StringWidth(lines[0])
	x, y := m.cellPos(a)
	x += m.sheet.ColWidth(a.Col)
	if x+w > m.width { // no room on the right: the left side
		x = max(x-m.sheet.ColWidth(a.Col)-w, 0)
	}
	x, y = m.clampBox(x, y, w, len(lines)+1) // the status line's row stays clear
	return overlay.Box{ID: id, X: x, Y: max(y, gridTop), Lines: lines}
}

const noteBoxID = "note"
