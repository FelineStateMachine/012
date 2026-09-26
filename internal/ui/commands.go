package ui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"one23/internal/sheet"
)

// command is a user-facing action. Every action is registered once here
// and reached from menus, key bindings, help and the command palette, so
// titles, descriptions and shortcuts stay consistent.
type command struct {
	id    string
	title string // shown in menus, the palette and help, e.g. "Save"
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

// keymap binds READY-mode keys to command IDs, following Google Sheets
// where it has a shortcut. Movement and typing are handled separately.
var keymap = map[string]string{
	"f10":         "menu",
	"f1":          "help",
	"ctrl+/":      "help",
	"enter":       "edit",
	"f2":          "edit",
	"f5":          "goto",
	"ctrl+g":      "goto",
	"delete":      "clear",
	"backspace":   "clear",
	"esc":         "select.none",
	"ctrl+a":      "select.all",
	"ctrl+space":  "select.columns",
	"shift+space": "select.rows",
	"ctrl+s":      "file.save",
	"ctrl+o":      "file.open",
	"ctrl+q":      "quit",
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
		&command{id: "menu", title: "Menu", desc: "Open the menu", run: func(m *Model) tea.Cmd {
			m.openMenu()
			return nil
		}},
		&command{id: "help", title: "Keyboard shortcuts", desc: "Show keys and functions", run: func(m *Model) tea.Cmd {
			m.mode = modeHelp
			return nil
		}},
		&command{id: "edit", title: "Edit cell", desc: "Edit the active cell's contents", run: (*Model).startEdit},
		&command{id: "goto", title: "Go to", desc: "Move to a cell address", run: func(m *Model) tea.Cmd {
			m.openGoto()
			return nil
		}},
		&command{id: "clear", title: "Clear", desc: "Clear the contents of the selected cells", run: func(m *Model) tea.Cmd {
			m.sheet.EraseRange(m.selection())
			m.changed = true
			return nil
		}},
		&command{id: "select.none", title: "Deselect", desc: "Collapse the selection to the active cell", run: func(m *Model) tea.Cmd {
			m.clearSelection()
			return nil
		}},
		&command{id: "select.all", title: "Select all", desc: "Select the data, then the whole sheet", run: (*Model).selectAll},
		&command{id: "select.columns", title: "Select columns", desc: "Select the whole columns of the selection", run: (*Model).selectColumns},
		&command{id: "select.rows", title: "Select rows", desc: "Select the whole rows of the selection", run: (*Model).selectRows},
		&command{id: "column.width", title: "Column width", desc: "Set the width of the selected columns", run: (*Model).openWidth},
		&command{id: "column.reset", title: "Reset column width", desc: "Return the selected columns to the default width", run: func(m *Model) tea.Cmd {
			r := m.selection()
			for c := r.From.Col; c <= r.To.Col; c++ {
				m.sheet.SetColWidth(c, 0)
			}
			m.changed = true
			return nil
		}},
		&command{id: "file.new", title: "New", desc: "Start a new, empty sheet", run: func(m *Model) tea.Cmd {
			m.reset(sheet.New(), "")
			return nil
		}},
		&command{id: "file.save", title: "Save", desc: "Save the sheet", run: (*Model).save},
		&command{id: "file.saveas", title: "Save as", desc: "Save the sheet under a new name", run: (*Model).openSave},
		&command{id: "file.open", title: "Open", desc: "Open a sheet, replacing this one", run: (*Model).openRetrieve},
		&command{id: "quit", title: "Quit", desc: "Close one23", run: (*Model).quit},
	)
}

// save writes to the current file, asking for a name the first time.
func (m *Model) save() tea.Cmd {
	if m.filename == "" {
		return m.openSave()
	}
	return saveCmd(m.sheet, m.filename)
}

// quit exits, confirming first when there are unsaved changes.
func (m *Model) quit() tea.Cmd {
	if !m.changed {
		return tea.Quit
	}
	m.openMenu()
	m.menu = append(m.menu, menuLevel{items: quitConfirm})
	return nil
}
