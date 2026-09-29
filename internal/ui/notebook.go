package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nuprompt"
)

// Notebooks: ! (or Data > Shell, or 012 nu) opens a nushell prompt on
// the formula bar. Each line run becomes a region of the notebook sheet
// (the one shown, or the first, or a new Shell 1): a live table named
// r1, r2 and so on, or name = pipeline to name it, which later commands
// read as $name. Refreshing a region runs it again, then what reads it
// (nurun.go). The engine keeps the regions (sheet/region.go); this is
// the prompt, the commands and the trust a file's commands need.

func init() {
	onRegion := func(m *Model) bool { r, _, ok := m.sheet.RegionAt(m.cur); return ok && !r.Linked() }
	register(
		&command{id: "nu.prompt", macro: macroNever, title: "Shell",
			desc: "Run a nushell pipeline; its table becomes a live region of the notebook sheet",
			run:  func(m *Model) tea.Cmd { return m.openShell("") }},
		&command{id: "nu.refresh", macro: macroNever, title: "Refresh region",
			desc:    "Run the region's command again, then every region that reads it",
			enabled: onRegion,
			run:     func(m *Model) tea.Cmd { return m.refreshRegion(m.regionHere()) }},
		&command{id: "nu.run_all", macro: macroNever, title: "Run all regions",
			desc:    "Run every region's command, each after the regions it reads",
			enabled: func(m *Model) bool { return len(m.book().RunOrder()) > 0 },
			run:     (*Model).runAllRegions},
		&command{id: "nu.edit", macro: macroNever, title: "Edit region's command",
			desc:    "Open the region's command at the prompt, to change it and run it again",
			enabled: onRegion,
			run: func(m *Model) tea.Cmd {
				r := m.regionHere()
				return m.openShell(r.Name + " = " + r.Command)
			}},
		&command{id: "nu.freeze", macro: macroNever, title: "Freeze region",
			desc:    "Turn the region's table into plain values you can edit, and stop running its command",
			enabled: onRegion,
			run:     func(m *Model) tea.Cmd { return m.freezeRegion(m.regionHere()) }},
		&command{id: "nu.delete", macro: macroNever, title: "Delete region",
			desc:    "Remove the region, its command and its table",
			enabled: onRegion,
			run:     func(m *Model) tea.Cmd { return m.deleteRegion(m.regionHere()) }},
		&command{id: "nu.stop", macro: macroNever, title: "Stop shell command",
			desc:    "Stop the region's command that's running, and those waiting after it",
			enabled: func(m *Model) bool { return m.shell.running != nil },
			run:     func(m *Model) tea.Cmd { m.stopShell(); return nil }},
	)
	keymap["!"] = "nu.prompt"
	keymap["f9"] = "nu.run_all"
}

// regionHere is the region at the active cell.
func (m *Model) regionHere() sheet.Region {
	r, _, _ := m.sheet.RegionAt(m.cur)
	return r
}

// openShell opens the prompt with text on it. What's selected is what
// the command reads as $in.
func (m *Model) openShell(text string) tea.Cmd {
	m.shell.input = ""
	if m.hasRange() && !m.sheet.Notebook() {
		m.shell.input = sheet.Qualified(m.sheet.Name(), m.selection())
	}
	m.shell.said = ""
	m.openOverlay(nuprompt.New(m.host(), text))
	return m.loadWords()
}

// OpenShell starts on a notebook sheet with the prompt open, as 012 nu
// does: the workbook's first notebook, or a new sheet made one. A new
// file's empty sheet becomes the notebook.
func (m *Model) OpenShell() {
	s := m.notebookSheet(false)
	if s == nil {
		w := m.book()
		if first := w.Sheet(0); w.Len() == 1 && first.Len() == 0 && !w.CanUndo() {
			if w.RenameSheet(first, w.NextNotebookName()) == nil {
				first.MakeNotebook()
				w.ClearHistory()
				s = first
			}
		}
	}
	if s != nil {
		m.showSheet(s)
	}
	m.shell.startup = true
}

// notebookSheet is where a command's region goes: the sheet shown when
// it's a notebook, or the first notebook, or with create a new Shell N
// after the sheet shown.
func (m *Model) notebookSheet(create bool) *sheet.Sheet {
	if m.sheet.Notebook() {
		return m.sheet
	}
	for _, s := range m.book().Sheets() {
		if s.Notebook() {
			return s
		}
	}
	if !create {
		return nil
	}
	s, err := m.book().AddNotebook("", m.sheet)
	if err != nil {
		m.shell.said = m.th.Warning.Render(err.Error())
		return nil
	}
	return s
}

// splitName splits "name = pipeline" into the name and the pipeline; a
// line that doesn't start so has no name.
func splitName(line string) (string, string) {
	name, rest, ok := strings.Cut(line, "=")
	name = strings.TrimSpace(name)
	if !ok || strings.HasPrefix(rest, "=") || sheet.ValidRegionName(name) != nil {
		return "", line
	}
	return name, strings.TrimSpace(rest)
}

// submitShell runs a line typed at the prompt: a new region, or a named
// region's new command.
func (m *Model) submitShell(line string) tea.Cmd {
	w := m.book()
	w.AddShellHistory(line)
	if why := m.shellOff(); why != "" {
		m.shell.said = m.th.Warning.Render(why)
		return nil
	}
	name, command := splitName(line)
	if command == "" {
		m.shell.said = m.th.Warning.Render("Type a pipeline after " + name + " =")
		return nil
	}
	input := ""
	if strings.Contains(command, "$in") {
		input = m.shell.input
	}
	deps := w.RegionDeps(command)
	trusted := m.macroTrusted() || len(w.Macros()) == 0 && len(w.RunOrder()) == 0
	var s *sheet.Sheet
	var err error
	if t, r, ok := w.Region(name); ok && name != "" {
		s, name = t, r.Name
		err = s.EditRegion(name, command, deps, input)
	} else {
		if s = m.notebookSheet(true); s == nil {
			return nil
		}
		if name == "" {
			name = w.NextRegionName()
		}
		err = s.AddRegion(sheet.Region{Name: name, Command: command, Deps: deps, Input: input})
	}
	if err != nil {
		m.shell.said = m.th.Warning.Render(err.Error())
		return nil
	}
	if trusted {
		m.trustHere() // what you type is yours
	}
	m.showSheet(s)
	if _, r, ok := w.Region(name); ok {
		m.clearSelection()
		m.cur = r.At
	}
	return m.runRegions(m.withInputs(w.RefreshOrder(name)))
}

// shellOff says why commands can't run here, or "".
func (m *Model) shellOff() string {
	switch {
	case m.shell.served && !m.configBool("serve-shell", false):
		return "Shell commands are off in 012 serve (serve-shell in the server's config)"
	case m.configString("shell", "ask") == "off":
		return "Shell commands are off (shell = off in your config)"
	}
	return ""
}

// refreshRegion runs a region's command again and then what reads it,
// asking first when the file's commands came from elsewhere.
func (m *Model) refreshRegion(r sheet.Region) tea.Cmd {
	order := m.withInputs(m.book().RefreshOrder(r.Name))
	return m.trustShell(func(m *Model) tea.Cmd { return m.runRegions(order) })
}

// runAllRegions runs every region, each after those it reads.
func (m *Model) runAllRegions() tea.Cmd {
	order := m.book().RunOrder()
	return m.trustShell(func(m *Model) tea.Cmd { return m.runRegions(order) })
}

// withInputs adds before order the regions its commands read that
// haven't run yet, so they have tables to read.
func (m *Model) withInputs(order []string) []string {
	w := m.book()
	var before []string
	for _, name := range w.RunOrder() {
		if slices.Contains(order, name) {
			continue
		}
		if s, _, ok := w.Region(name); ok && !s.RegionShown(name) && m.readBy(name, order) {
			before = append(before, name)
		}
	}
	return append(before, order...)
}

// readBy reports whether a region of names reads name, directly or
// through others.
func (m *Model) readBy(name string, names []string) bool {
	for _, n := range names {
		if slices.Contains(m.book().RefreshOrder(name), n) {
			return true
		}
	}
	return false
}

// trustShell runs run once commands may run: shell = on, or the file's
// commands were made or trusted here, or the user agrees now.
func (m *Model) trustShell(run func(*Model) tea.Cmd) tea.Cmd {
	if why := m.shellOff(); why != "" {
		m.fail(why)
		return nil
	}
	if m.configString("shell", "ask") == "on" || m.macroTrusted() {
		return run(m)
	}
	m.ask(question{
		msg:  "Run this file's shell commands?",
		desc: "It was saved on another computer: its commands run as you, with your files",
		choices: []choice{
			{key: "enter", label: "Run", run: func(m *Model) tea.Cmd { m.trustHere(); return run(m) }},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}

// sortRegion sorts the table of a region a sort's range r reaches, by
// its columns in keys: the rows below its header, whatever else r
// holds. The order is the region's, kept when it runs again.
func (m *Model) sortRegion(reg sheet.Region, r sheet.Rect, keys []sheet.SortKey) tea.Cmd {
	if reg.Linked() {
		m.note = "A linked file's rows keep the file's order: unlink it to sort them"
		return nil
	}
	t, ok := m.sheet.RegionTable(reg.Name)
	inTable := func(k sheet.SortKey) bool { return k.Col >= t.From.Col && k.Col <= t.To.Col }
	if !ok || len(keys) == 0 || !slices.ContainsFunc(keys, inTable) || r.To.Row <= t.From.Row {
		m.note = "Sort a region by a column of its table"
		return nil
	}
	keys = slices.DeleteFunc(slices.Clone(keys), func(k sheet.SortKey) bool { return !inTable(k) })
	if err := m.sheet.SortRegion(reg.Name, keys); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.note = "Sorted " + reg.Name + "; it stays sorted when it runs again"
	return nil
}

// freezeRegion turns a region's table into plain values.
func (m *Model) freezeRegion(r sheet.Region) tea.Cmd {
	if m.shell.running != nil && m.shell.running.name == r.Name {
		m.stopShell()
	}
	if err := m.sheet.FreezeRegion(r.Name); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.note = "Froze " + r.Name + " into values" + m.orphans(r.Name)
	return nil
}

// deleteRegion removes a region.
func (m *Model) deleteRegion(r sheet.Region) tea.Cmd {
	if m.shell.running != nil && m.shell.running.name == r.Name {
		m.stopShell()
	}
	if err := m.sheet.DeleteRegion(r.Name); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.note = "Deleted " + r.Name + m.orphans(r.Name)
	return nil
}

// orphans says which regions read one that's gone.
func (m *Model) orphans(name string) string {
	if by := m.book().DependsOn(name); len(by) > 0 {
		return "; " + strings.Join(by, ", ") + " read it"
	}
	return ""
}

// notebookKey is Enter and F2 on a region's label (refresh it, edit its
// command) and Esc while a command runs (stop it), in READY.
func (m *Model) notebookKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc":
		if m.shell.running != nil {
			return m.runCommand("nu.stop"), true
		}
	case "enter", "f2":
		if !m.sheet.HasRegions() || m.hasRange() {
			return nil, false
		}
		if _, label, ok := m.sheet.RegionAt(m.cur); ok && label {
			if key == "f2" {
				return m.runCommand("nu.edit"), true
			}
			return m.runCommand("nu.refresh"), true
		}
	}
	return nil, false
}

// regionLine is the context line on a region: what it is and the keys
// for it, or, on a notebook with regions not run, the key that runs them.
func (m *Model) regionLine() string {
	r, label, ok := m.sheet.RegionAt(m.cur)
	if !ok {
		if m.sheet.Notebook() && m.notRun() > 0 {
			return m.th.KeyHints(m.shortcut("nu.run_all"), "run all regions", m.shortcut("nu.prompt"), "shell")
		}
		return ""
	}
	if r.Linked() {
		return m.linkedLine() // linked.go
	}
	if f := m.shell.failed[strings.ToUpper(r.Name)]; f != "" && label {
		return m.th.Warning.Render(r.Name+" failed: "+f) + "   " + m.th.KeyHints("F2", "edit")
	}
	what := r.Name
	if n := m.sheet.RegionNote(r.Name); n != "" {
		what += ": " + n
	}
	if label {
		return m.th.Muted.Render(what+"   ") + m.th.KeyHints("Enter", "refresh", "F2", "edit", m.shortcut("nu.run_all"), "run all")
	}
	return m.th.Muted.Render("Region " + what + " of " + r.Command)
}

// notRun counts the sheet's regions that haven't run since the file
// opened.
func (m *Model) notRun() int {
	n := 0
	for _, r := range m.sheet.Regions() {
		if !r.Linked() && !m.sheet.RegionShown(r.Name) {
			n++
		}
	}
	return n
}

// regionEntry is what the formula bar shows on a region: its command.
func (m *Model) regionEntry() (string, bool) {
	if !m.sheet.HasRegions() {
		return "", false
	}
	r, _, ok := m.sheet.RegionAt(m.cur)
	if !ok || r.Linked() {
		return "", false
	}
	return m.th.Muted.Render(nuprompt.Mark + r.Command), true
}

// shellWords are the prompt's completions: the regions, and nu's
// commands once they're known.
func (m *Model) shellWords() []nuprompt.Word {
	var out []nuprompt.Word
	for _, s := range m.book().Sheets() {
		for _, r := range s.Regions() {
			out = append(out, nuprompt.Word{Text: "$" + r.Name, Desc: r.Command})
		}
	}
	for _, c := range m.shell.words {
		out = append(out, nuprompt.Word{Text: c, Desc: "command"})
	}
	return out
}
