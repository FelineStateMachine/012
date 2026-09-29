package ui

import (
	"cmp"
	"runtime/debug"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/shortcuts"
)

func init() {
	register(
		&command{id: "help", macro: macroNever, title: "Keyboard shortcuts", desc: "Show every key, grouped by what it does", run: func(m *Model) tea.Cmd {
			m.openOverlay(shortcuts.New(m.host()))
			return nil
		}},
		&command{id: "help.functions", macro: macroNever, title: "Function list", desc: "Search the functions formulas can use", run: func(m *Model) tea.Cmd {
			p := m.newPicker("Functions", "Type a function name, e.g. sum or if", 100, functionItems(m))
			p.Action = "insert"
			m.openOverlay(p)
			return nil
		}},
		&command{id: "help.about", macro: macroNever, title: "About 012", desc: "Show the version", run: func(m *Model) tea.Cmd {
			m.ask(question{
				msg: "012 " + version() + ": Lotus 1-2-3 looks, Google Sheets keys.",
				choices: []choice{
					{key: "esc", label: "Close", run: func(*Model) tea.Cmd { return nil }},
				},
			})
			return nil
		}},
	)
	keymap["f1"] = "help"
	keymap["ctrl+/"] = "help"
}

// version is the release 012 was built from, or "devel" for a source
// build (including Go's pseudo-versions such as v0.0.0-2026...+dirty).
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && strings.HasPrefix(bi.Main.Version, "v") && !strings.ContainsAny(bi.Main.Version, "-+") {
		return bi.Main.Version
	}
	return "devel"
}

// functionItems lists the spreadsheet functions for the function list;
// picking one starts a formula with it.
func functionItems(m *Model) []picker.Item {
	fns := sheet.Funcs()
	items := make([]picker.Item, len(fns))
	for i, f := range fns {
		items[i] = picker.Item{
			Title: f.Name + "(" + f.Args + ")", Name: len(f.Name), Detail: f.Desc, Desc: f.Desc,
			Pick: func() tea.Cmd {
				m.closeOverlay()
				m.startEntry(modeEnter, "="+f.Name+"(")
				return nil
			},
		}
	}
	return items
}

// helpRow is a line of the shortcuts view: a group heading, or keys and
// what they do.
type helpRow struct {
	heading string
	keys    []string
	action  string
}

// helpRows lists the shortcuts, generated from the keymap and the command
// registry so help can't drift from behavior. Movement and typing, which
// aren't commands, come first; then commands grouped by the menu they're
// in. With vim keys on, the vim keys come first.
func helpRows(vim bool) []helpRow {
	if vim {
		return append(vimHelpRows(), keyHelpRows(true)...)
	}
	return keyHelpRows(false)
}

// keyHelpRows are the Sheets keys and every command's shortcuts.
func keyHelpRows(vim bool) []helpRow {
	listed := map[string]bool{}
	var rows []helpRow
	group := func(title string, static []helpRow, ids ...string) {
		var cmds []helpRow
		for _, id := range ids {
			keys := keysFor(id, vim)
			if listed[id] || len(keys) == 0 || commands[id] == nil {
				continue
			}
			listed[id] = true
			for i, k := range keys {
				keys[i] = keyLabel(k)
			}
			cmds = append(cmds, helpRow{keys: keys, action: commands[id].title})
		}
		if len(static)+len(cmds) > 0 {
			rows = append(rows, helpRow{heading: title})
			rows = append(rows, static...)
			rows = append(rows, cmds...)
		}
	}
	byTitle := func(pred func(id string) bool) []string {
		var ids []string
		for id := range commands {
			if pred(id) {
				ids = append(ids, id)
			}
		}
		slices.SortFunc(ids, func(a, b string) int { return cmp.Compare(commands[a].title, commands[b].title) })
		return ids
	}

	group("Moving around", []helpRow{
		{keys: []string{"Arrows"}, action: "Move"},
		{keys: []string{"Ctrl+arrows"}, action: "Jump to the edge of the data"},
		{keys: []string{"Tab", "Shift+Tab"}, action: "Right, left"},
		{keys: []string{"PgUp", "PgDn"}, action: "Screen up, down"},
		{keys: []string{"Alt+PgUp", "Alt+PgDn"}, action: "Screen left, right"},
		{keys: []string{"Home", "Ctrl+Home"}, action: "Column A, cell A1"},
		{keys: []string{"Ctrl+End"}, action: "Last used cell"},
	}, "goto")
	group("Sheets", []helpRow{
		{keys: []string{"Click a tab"}, action: "Show it; double-click renames"},
	}, "sheet.next", "sheet.prev", "sheet.new", "sheet.goto")
	group("Selecting", []helpRow{
		{keys: []string{"Shift+arrows"}, action: "Extend the selection"},
		{keys: []string{"Ctrl+Shift+arrows"}, action: "Extend to the edge of the data"},
		{keys: []string{"Mouse"}, action: "Select by dragging or Shift+click"},
	}, byTitle(func(id string) bool { return strings.HasPrefix(id, "select.") })...)
	group("Entering data", []helpRow{
		{keys: []string{"Type"}, action: "Replace the cell; = starts a formula"},
		{keys: []string{"Enter", "Tab"}, action: "Accept and move down, right"},
		{keys: []string{"Arrows"}, action: "Pick cells for a formula"},
		{keys: []string{"Esc"}, action: "Cancel the entry"},
	}, "edit")
	group("Macros", []helpRow{
		{keys: []string{"Ctrl+Alt+Shift+0-9"}, action: "Run the macro with that shortcut"},
		{keys: []string{"Esc"}, action: "Stop a macro while it runs"},
	})
	group("Notebooks", nbHelpRows(listed))
	group("Menus and search", []helpRow{
		{keys: []string{"Alt+letter"}, action: "Open a menu by its underlined letter"},
	}, "palette", "menu", "menu.context", "help")
	for _, d := range menuBar {
		var ids []string
		var walk func(items []menuItem)
		walk = func(items []menuItem) {
			for _, it := range items {
				if it.items != nil {
					walk(it.items)
				} else if !it.sep {
					ids = append(ids, it.cmd)
				}
			}
		}
		walk(visibleItems(d.items))
		group(d.title, nil, ids...)
	}
	group("Other", nil, byTitle(func(string) bool { return true })...)
	return rows
}

// Shortcuts are the rows of the shortcuts view (package shortcuts),
// with the keys in use.
func (h host) Shortcuts() []shortcuts.Row {
	rows := helpRows(h.m.prefs.vim)
	out := make([]shortcuts.Row, len(rows))
	for i, r := range rows {
		out[i] = shortcuts.Row{Heading: r.heading, Keys: r.keys, Action: r.action}
	}
	return out
}
