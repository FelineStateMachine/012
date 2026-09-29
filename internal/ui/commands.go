package ui

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
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
	// hidden, when set, reports that the command doesn't apply to this
	// session at all, as sending to a pipeline outside one: menus and
	// the palette leave it out rather than dim it.
	hidden func(m *Model) bool

	// checked, when set, makes the command a setting: menus show a ✓
	// while it is on.
	checked func(m *Model) bool

	// edits, when set, is the range of the sheet shown that the command
	// changes. A command that would change a pivot table's results or
	// cells an array formula spills is refused, as Sheets refuses (see
	// pivot.go).
	edits func(m *Model) sheet.Rect
	// keepsSpills is set when the command moves the cells of edits
	// (inserting or deleting rows or columns) or changes only their
	// formatting: an array spilling there keeps its values, so only
	// pivots refuse it.
	keepsSpills bool

	// changes, when set, is the range whose cells the command changes,
	// where that's narrower than edits, and false for none: inserting
	// rows moves the cells below without changing them. Protected ranges
	// ask before a change to them (see protect.go).
	changes func(m *Model) (sheet.Rect, bool)

	// macro says how recording and scripts treat the command; see
	// macroUse.
	macro macroUse

	// answer, when set, runs the command with the answer to the dialog
	// it opens, as a script's run(id, answer=...) does: the dialog's
	// choices as a macro records them, a JSON object, made as if picked
	// and applied. It checks what it needs itself, in place of enabled.
	answer func(m *Model, text string) (tea.Cmd, error)

	// typed, when set, lets a key bound to the command type itself
	// where the command isn't available, as Space checks a checkbox and
	// starts an entry anywhere else.
	typed bool
}

// macroUse is how macros treat a command.
type macroUse int

const (
	// macroRecord commands change the workbook: a recording calls them
	// by id, run("format.bold"), and scripts may run them. The default.
	macroRecord macroUse = iota
	// macroView commands change only what's shown or selected, or open
	// something to look through: a recording keeps the selection they
	// leave, not the command, so replaying doesn't depend on the screen.
	// Scripts may run them.
	macroView
	// macroNever commands are about the session (files, menus, help,
	// undo, macros themselves): never recorded, and scripts can't run
	// them.
	macroNever
)

// available reports whether the command can run in m's current state.
func (c *command) available(m *Model) bool {
	if m.sheet.IsNotebook() && !notebookSafe(c.id) {
		return false
	}
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
// Alt combinations and named keys. With vim keys on, it leaves out the
// Sheets keys vim takes and adds the vim keys (dd, u) last.
func keysFor(id string, vim bool) []string {
	var keys []string
	for k, c := range keymap {
		if c == id && !(vim && vimShadows(k)) {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(keyRank(a), keyRank(b)), cmp.Compare(a, b))
	})
	if vim {
		keys = append(keys, vimKeysFor(id)...)
	}
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

// keyLabel is how a key is shown: "Ctrl+S", "Del", or a vim key as typed,
// e.g. "dd" or "G".
func keyLabel(k string) string {
	if isVimKey(k) {
		return k
	}
	return theme.KeyLabel(k)
}

// shortcut is the key shown next to a command in menus and the palette,
// e.g. "Ctrl+S", or "" if it has none. It ignores vim keys; see
// Model.shortcut.
func shortcut(id string) string {
	if keys := keysFor(id, false); len(keys) > 0 {
		return keyLabel(keys[0])
	}
	return nbShortcut(id)
}

// shortcut is the key shown for a command with the keys in use: with vim
// keys on, the vim key when Sheets' key is taken or there is none.
func (m *Model) shortcut(id string) string {
	if keys := keysFor(id, m.prefs.vim); len(keys) > 0 {
		return keyLabel(keys[0])
	}
	return nbShortcut(id)
}

// runCommand runs a registered command by ID. It is the one place a
// command runs, whatever reached it (a key, a menu, the palette, a click
// on a tab or a chart), so a command log or macro recorder attaches here.
func (m *Model) runCommand(id string) tea.Cmd {
	c, ok := commands[id]
	if !ok {
		panic("unknown command " + id)
	}
	// Sort, filter, find, fill and the rest are all commands, so this
	// one span times every one of them, and holds what they cause: the
	// recalculation, a pivot refresh, a sort.
	span := m.spans.Start("command", slog.String("id", id))
	defer span.End()
	if m.sheet.IsNotebook() && !notebookSafe(id) {
		m.note = c.title + " works on a sheet's cells: this tab is a notebook"
		return nil
	}
	if c.edits != nil && m.refuseEdit(c.edits(m), c.keepsSpills) || edits(c) && !m.mayEdit() {
		return nil
	}
	if m.askCommand(c) {
		return nil
	}
	if c.edits != nil && m.askUndoCost(c.edits(m), func(m *Model) tea.Cmd { return m.runCommand(c.id) }) {
		return nil
	}
	if m.rec != nil {
		return m.recordingCommand(c) // macrorec.go
	}
	return c.run(m)
}

func init() {
	register(
		&command{id: "edit", macro: macroView, title: "Edit cell", desc: "Edit the active cell's contents", run: (*Model).startEdit, edits: cellTarget},
		&command{id: "goto", macro: macroView, title: "Go to", desc: "Move to a cell address", run: func(m *Model) tea.Cmd {
			m.openGoto()
			return nil
		}},
		&command{id: "clear", title: "Clear", desc: "Clear the contents of the selected cells", edits: (*Model).selection, run: func(m *Model) tea.Cmd {
			m.sheet.EraseRange(m.selection())
			m.changed = true
			return nil
		}},
		&command{id: "select.none", macro: macroView, title: "Deselect", desc: "Collapse the selection and clear the copy marker", run: func(m *Model) tea.Cmd {
			m.clearSelection()
			m.copied.clearMark()
			return nil
		}},
		&command{id: "select.all", macro: macroView, title: "Select all", desc: "Select the data, then the whole sheet", run: (*Model).selectAll},
		&command{id: "select.columns", macro: macroView, title: "Select columns", desc: "Select the whole columns of the selection", run: (*Model).selectColumns},
		&command{id: "select.rows", macro: macroView, title: "Select rows", desc: "Select the whole rows of the selection", run: (*Model).selectRows},
		&command{id: "column.width", title: "Column width", desc: "Set the width of the selected columns", run: (*Model).openWidth},
		&command{id: "column.reset", title: "Reset column width", desc: "Return the selected columns to the default width", run: func(m *Model) tea.Cmd {
			r := m.selection()
			m.sheet.Batch(sheet.Change{Label: "reset column widths", Focus: r}, func() error {
				for c := range m.sheet.Widths() { // only the columns that have a width
					if c >= r.From.Col && c <= r.To.Col {
						m.sheet.SetColWidth(c, 0)
					}
				}
				return nil
			})
			m.changed = true
			return nil
		}},
		&command{id: "file.new", macro: macroNever, title: "New", desc: "Start a new, empty sheet", run: func(m *Model) tea.Cmd {
			m.reset(sheet.New(), "")
			return nil
		}},
		&command{id: "file.save", macro: macroNever, title: "Save", desc: "Save the sheet", run: (*Model).save},
		&command{id: "file.saveas", macro: macroNever, title: "Save as", desc: "Save the sheet under a new name", run: (*Model).openSave},
		&command{id: "file.open", macro: macroNever, title: "Open", desc: "Open a sheet, replacing this one", run: (*Model).openRetrieve},
		&command{id: "quit", macro: macroNever, title: "Quit", desc: "Close 012", run: (*Model).quit},
	)
}
