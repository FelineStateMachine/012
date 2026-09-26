package ui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"one23/internal/sheet"
)

// command is a user-facing action. Every action is registered once here
// and reached from the slash menu, key bindings, help and the command
// palette, so titles, descriptions and shortcuts stay consistent.
type command struct {
	id    string
	title string // shown in the palette and help, e.g. "File Save"
	desc  string // one line, shown on the third panel line
	run   func(m *Model) tea.Cmd
}

var commands = map[string]*command{}

func register(cmds ...*command) {
	for _, c := range cmds {
		if _, dup := commands[c.id]; dup {
			panic("duplicate command " + c.id)
		}
		commands[c.id] = c
	}
}

// keymap binds READY-mode keys to command IDs. Movement and typing are
// handled separately.
var keymap = map[string]string{
	"/":      "menu",
	"<":      "menu",
	"f1":     "help",
	"f2":     "edit",
	"f5":     "goto",
	"delete": "cell.erase",
}

// keysFor returns the shortcuts bound to a command, sorted.
func keysFor(id string) []string {
	var keys []string
	for k, c := range keymap {
		if c == id {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// runCommand runs a registered command by ID.
func (m *Model) runCommand(id string) tea.Cmd {
	c, ok := commands[id]
	if !ok {
		panic("unknown command " + id)
	}
	return c.run(m)
}

func init() {
	register(
		&command{id: "menu", title: "Menu", desc: "Open the slash menu", run: func(m *Model) tea.Cmd {
			m.openMenu()
			return nil
		}},
		&command{id: "help", title: "Help", desc: "Show keys and functions", run: func(m *Model) tea.Cmd {
			m.mode = modeHelp
			return nil
		}},
		&command{id: "edit", title: "Edit Cell", desc: "Edit the current cell's contents", run: (*Model).startEdit},
		&command{id: "goto", title: "Go To", desc: "Move the cell pointer to an address", run: func(m *Model) tea.Cmd {
			m.openGoto()
			return nil
		}},
		&command{id: "cell.erase", title: "Erase Cell", desc: "Erase the current cell", run: func(m *Model) tea.Cmd {
			m.set(m.cur, "")
			return nil
		}},
		&command{id: "column.width", title: "Worksheet Column Set-Width", desc: "Specify a width for the current column", run: (*Model).openWidth},
		&command{id: "column.reset", title: "Worksheet Column Reset-Width", desc: "Return the current column to the default width", run: func(m *Model) tea.Cmd {
			m.sheet.SetColWidth(m.cur.Col, 0)
			m.changed = true
			return nil
		}},
		&command{id: "worksheet.erase", title: "Worksheet Erase", desc: "Erase the entire worksheet from memory", run: func(m *Model) tea.Cmd {
			m.reset(sheet.New(), "")
			return nil
		}},
		&command{id: "range.erase", title: "Range Erase", desc: "Erase the cell or range", run: func(m *Model) tea.Cmd {
			m.openRange("Enter range to erase:", func(m *Model, r sheet.Rect) tea.Cmd {
				m.sheet.EraseRange(r)
				m.changed = true
				return nil
			})
			return nil
		}},
		&command{id: "file.save", title: "File Save", desc: "Store the entire worksheet in a file", run: (*Model).openSave},
		&command{id: "file.retrieve", title: "File Retrieve", desc: "Erase the current worksheet and display the selected file", run: (*Model).openRetrieve},
		&command{id: "quit", title: "Quit", desc: "End the session", run: func(*Model) tea.Cmd { return tea.Quit }},
	)
}
