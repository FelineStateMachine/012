package ui

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// command is a user-facing action. Every action is registered once here
// and reached from menus, key bindings, help and the command palette, so
// titles, descriptions and shortcuts stay consistent.
type command struct {
	id    string
	title string // shown in menus, the palette and help, e.g. "Save"
	desc  string // one line, shown on the third panel line
	run   func(m *Model) tea.Cmd

	// enabled, when set, reports whether the command can run right now.
	// Menus and the palette show unavailable commands dimmed.
	enabled func(m *Model) bool
}

// available reports whether the command can run in m's current state.
func (c *command) available(m *Model) bool {
	return c.enabled == nil || c.enabled(m)
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

// keysFor returns the shortcuts bound to a command, the one to show first
// leading: Ctrl combinations (what Sheets shows), then function keys, then
// Alt combinations and named keys.
func keysFor(id string) []string {
	var keys []string
	for k, c := range keymap {
		if c == id {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(keyRank(a), keyRank(b)), cmp.Compare(a, b))
	})
	return keys
}

func keyRank(k string) int {
	switch {
	case strings.HasPrefix(k, "ctrl+"):
		return 0
	case len(k) >= 2 && k[0] == 'f' && k[1] >= '0' && k[1] <= '9':
		return 1
	case strings.HasPrefix(k, "alt+"):
		return 2
	case k == "delete":
		return 3
	}
	return 4
}

// shortcut is the key shown next to a command in menus and the palette,
// e.g. "Ctrl+S", or "" if it has none.
func shortcut(id string) string {
	if keys := keysFor(id); len(keys) > 0 {
		return keyLabel(keys[0])
	}
	return ""
}

// runCommand runs a registered command by ID.
func (m *Model) runCommand(id string) tea.Cmd {
	c, ok := commands[id]
	if !ok {
		panic("unknown command " + id)
	}
	// Sort, filter, find, fill and the rest are all commands, so this
	// one span times every one of them.
	span := telemetry.Start("command", slog.String("id", id))
	defer span.End()
	return c.run(m)
}

func init() {
	register(
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
		&command{id: "select.none", title: "Deselect", desc: "Collapse the selection and clear the copy marker", run: func(m *Model) tea.Cmd {
			m.clearSelection()
			m.clearCopyMark()
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
		&command{id: "quit", title: "Quit", desc: "Close 012", run: (*Model).quit},
	)
}

// save writes to the current file, asking for a name the first time.
func (m *Model) save() tea.Cmd {
	if m.filename == "" && m.xfer.source != "" {
		return m.saveImported()
	}
	if m.filename == "" {
		return m.openSave()
	}
	return saveCmd(m.sheet, m.filename)
}
