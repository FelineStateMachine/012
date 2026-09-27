package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/cmdline"
)

// The command line (: with vim keys, or from the palette) takes vim's
// file commands (:w, :q, :wq, :x, :e file), a cell, range, named range or
// row number to go to (:B12, :Sheet2!A1, :40), or any registered command
// by its ID or title (:edit.fill_down, :fill down). Completions come from
// the command registry as you type, in a box under the context line, so
// there's no second list of commands to keep up.

func init() {
	register(&command{id: "vim.command", macro: macroNever, title: "Command line",
		desc: "Type a command: a cell to go to (B12), w, q, wq, e file, or any command by name",
		run: func(m *Model) tea.Cmd {
			m.openOverlay(cmdline.New(m.host()))
			return nil
		}})
}

// commandIDs are the registered commands' IDs, by title.
func commandIDs() []string {
	ids := make([]string, 0, len(commands))
	for id := range commands {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		return cmp.Or(cmp.Compare(commands[a].title, commands[b].title), cmp.Compare(a, b))
	})
	return ids
}

// runCmdLine carries out a command line and reports whether it was one.
func (m *Model) runCmdLine(text string) (tea.Cmd, bool) {
	word, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	bang := strings.HasSuffix(word, "!")
	switch strings.TrimSuffix(word, "!") {
	case "w":
		return m.writeTo(arg), true
	case "q":
		if bang {
			return m.exit(), true
		}
		return m.runCommand("quit"), true
	case "wq", "x":
		if word == "x" && !m.changed && arg == "" {
			return m.exit(), true
		}
		m.quitAfterSave = true
		return m.writeTo(arg), true
	case "e":
		return m.editFile(arg, bang), true
	}
	if n, err := strconv.Atoi(text); err == nil {
		m.clearSelection()
		m.cur.Row = m.visibleRow(clamp(n-1, 0, sheet.MaxRows-1))
		return nil, true
	}
	if m.gotoText(text) {
		return nil, true
	}
	id, ok := commandNamed(text)
	if !ok {
		return nil, false
	}
	if c := commands[id]; !c.available(m) {
		m.note = c.title + " isn't available now"
		return nil, true
	}
	return m.runCommand(id), true
}

// commandNamed finds a command by its ID or title, in any case.
func commandNamed(text string) (string, bool) {
	ids := commandIDs()
	for _, id := range ids {
		if strings.EqualFold(id, text) {
			return id, true
		}
	}
	for _, id := range ids {
		if strings.EqualFold(commands[id].title, text) {
			return id, true
		}
	}
	return "", false
}

// writeTo is :w. Without a name it saves, as File > Save; with one it
// saves as that name, as File > Save as, or downloads when the name is
// another format's (:w out.csv), as File > Download. It goes the menus'
// ways, so files are named, checked and replaced the same.
func (m *Model) writeTo(name string) tea.Cmd {
	if name == "" {
		return m.runCommand("file.save")
	}
	k, ok := fileio.KindOf(name)
	switch {
	case !ok:
		return m.saveAsFile(name)
	case !k.CanExport():
		m.quitAfterSave = false
		m.fail("012 can't write " + k.String() + " files")
		return nil
	}
	m.quitAfterSave = false // a download isn't the sheet's file
	return m.downloadFile(k, sheet.Rect{}, name)
}

// editFile is :e name: open a sheet or import a file, refusing to drop
// unsaved changes unless forced with :e!.
func (m *Model) editFile(name string, force bool) tea.Cmd {
	switch {
	case name == "":
		return m.runCommand("file.open")
	case m.changed && !force:
		m.fail("Unsaved changes: :w saves them, :e! " + name + " discards them")
		return nil
	}
	return m.openFile(name)
}
