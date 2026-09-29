package ui

import (
	"cmp"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// The command palette ("Search the menus" in Sheets, Alt+/) finds any
// registered command by fuzzy matching its title and menu path, and shows
// its shortcut. The function list uses the same picker.

func init() {
	register(&command{id: "palette", macro: macroNever, title: "Search the menus", desc: "Find and run any command by name", run: func(m *Model) tea.Cmd {
		m.openOverlay(m.newPicker("Search the menus", "Type a command, e.g. save or width", 76, paletteItems(m)))
		return nil
	}})
	for _, k := range []string{"alt+/", "ctrl+k", "ctrl+shift+p"} {
		keymap[k] = "palette"
	}
}

// paletteItems lists every command: those in menus first, in menu order
// with their menu path, then the rest by title.
func paletteItems(m *Model) []picker.Item {
	var items []picker.Item
	seen := map[string]bool{"palette": true}
	add := func(id, path string) {
		c := commands[id]
		if seen[id] || c.hidden != nil && c.hidden(m) {
			return
		}
		seen[id] = true
		items = append(items, picker.Item{
			Title: c.title, Name: len(c.title), Detail: path, Key: m.shortcut(id), Desc: c.desc,
			Off: !c.available(m), Pick: func() tea.Cmd { return m.runFromOverlay(id) },
		})
	}
	var walk func(items []menuItem, path string)
	walk = func(items []menuItem, path string) {
		for _, it := range items {
			switch {
			case it.items != nil:
				walk(it.items, path+" › "+it.label())
			case !it.sep:
				add(it.cmd, path)
			}
		}
	}
	for _, d := range menuBar {
		walk(visibleItems(d.items), d.title)
	}
	items = append(items, macroRunItems(m)...)
	rest := make([]string, 0, len(commands))
	for id := range commands {
		rest = append(rest, id)
	}
	slices.SortFunc(rest, func(a, b string) int { return cmp.Compare(commands[a].title, commands[b].title) })
	for _, id := range rest {
		add(id, "")
	}
	return items
}
