package ui

import (
	"cmp"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Notebooks: a notebook tab holds code cells (nushell pipelines) and
// note cells (Markdown), drawn and keyed by package nbview. Every action
// on them is a command registered here, so menus (Data > Shell), the
// palette and help list them; the keys Jupyter uses reach them through
// nbKeys and nbEditKeys. The cells are the workbook's (sheet/notebook.go),
// running them is nbrun.go's, sending an output to a sheet nbsend.go's,
// and drawing the tab nbscreen.go's.

// nbKeys are command mode's keys, as Jupyter's; "d d" is d twice.
var nbKeys = map[string]string{
	"enter":           "nb.edit",
	"shift+enter":     "nb.run_next",
	"ctrl+enter":      "nb.run",
	"r":               "nb.run", // runs where Ctrl+Enter arrives as Enter
	"alt+enter":       "nb.run_insert",
	"a":               "nb.insert_above",
	"b":               "nb.insert_below",
	"!":               "nb.add_code",
	"d d":             "nb.delete",
	"m":               "nb.to_note",
	"y":               "nb.to_code",
	"z":               "edit.undo",
	"c":               "nb.copy",
	"x":               "nb.cut",
	"v":               "nb.paste",
	"o":               "nb.toggle_output",
	"n":               "nb.name",
	"G":               "nb.send",
	"i i":             "nb.stop",
	"f":               "nb.stream", // follow
	"0 0":             "nb.restart",
	"f9":              "nb.run_all",
	"V":               "nb.paste_above",
	"O":               "nb.toggle_whole",
	"alt+up":          "nb.move_up", // as an editor moves lines; JupyterLab's Ctrl+Shift+Up too
	"alt+down":        "nb.move_down",
	"ctrl+shift+up":   "nb.move_up",
	"ctrl+shift+down": "nb.move_down",
	"ctrl+g":          "nb.goto",
}

// nbEditKeys are edit mode's.
var nbEditKeys = map[string]string{
	"esc":         "nb.command_mode",
	"shift+enter": "nb.run_next",
	"ctrl+enter":  "nb.run",
	"alt+enter":   "nb.run_insert",
}

// nbShortcut is the notebook's key for command id, as shown: its
// command mode's, or edit mode's, the shortest first.
func nbShortcut(id string) string {
	return nbKeyLabels(id)[0]
}

// nbKeyLabels are the notebook's keys for id, shown, command mode's
// first; "" alone for none.
func nbKeyLabels(id string) []string {
	var out []string
	for _, keys := range []map[string]string{nbKeys, nbEditKeys} {
		var found []string
		for k, c := range keys {
			if c == id {
				found = append(found, nbview.KeyLabel(k))
			}
		}
		slices.SortFunc(found, func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), cmp.Compare(a, b)) })
		for _, k := range found {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// nbHelpRows are the notebook's keys for help, in the order of its menu.
func nbHelpRows(listed map[string]bool) []helpRow {
	rows := []helpRow{{keys: []string{"Up", "Down", "j", "k"}, action: "Move between cells and outputs"},
		{keys: []string{"Shift+Up", "Shift+Down", "K", "J"}, action: "Select several cells"}}
	for _, it := range append([]menuItem{{cmd: "nb.edit"}, {cmd: "nb.command_mode"}, {cmd: "nb.add_code"}}, notebookItems...) {
		keys := nbKeyLabels(it.cmd)
		if it.sep || keys[0] == "" || listed[it.cmd] || commands[it.cmd] == nil {
			continue
		}
		listed[it.cmd] = true
		rows = append(rows, helpRow{keys: keys, action: commands[it.cmd].title})
	}
	return rows
}

// notebookItems are Data > Shell's items.
var notebookItems = []menuItem{
	{cmd: "nb.open"}, sep,
	{cmd: "nb.run"}, {cmd: "nb.run_next"}, {cmd: "nb.run_all"}, {cmd: "nb.run_above"}, {cmd: "nb.run_below"}, {cmd: "nb.stream"}, {cmd: "nb.stop"}, sep,
	{cmd: "nb.insert_above"}, {cmd: "nb.insert_below"}, {cmd: "nb.delete"}, {cmd: "nb.move_up"}, {cmd: "nb.move_down"}, sep,
	{cmd: "nb.to_code"}, {cmd: "nb.to_note"}, {cmd: "nb.name"}, sep,
	{cmd: "nb.copy"}, {cmd: "nb.cut"}, {cmd: "nb.paste"}, {cmd: "nb.paste_above"}, sep,
	{cmd: "nb.toggle_output"}, {cmd: "nb.toggle_whole"}, {cmd: "nb.open_output"}, {cmd: "nb.clear_output"}, {cmd: "nb.send"}, sep,
	{cmd: "nb.goto"}, {cmd: "nb.toc"}, sep,
	{cmd: "nb.clear_outputs"}, {cmd: "nb.restart"}, {cmd: "nb.reactive"}, sep,
	{cmd: "region.freeze"}, {cmd: "region.delete"},
}

// nbCellMenu is a cell's context menu, on a right click.
var nbCellMenu = []menuItem{
	{cmd: "nb.run"}, {cmd: "nb.run_below"}, {cmd: "nb.stream"}, sep,
	{cmd: "nb.cut"}, {cmd: "nb.copy"}, {cmd: "nb.paste"}, {cmd: "nb.paste_above"}, {cmd: "nb.delete"}, sep,
	{cmd: "nb.move_up"}, {cmd: "nb.move_down"}, {cmd: "nb.to_code"}, {cmd: "nb.to_note"}, {cmd: "nb.name"}, sep,
	{cmd: "nb.toggle_output"}, {cmd: "nb.toggle_whole"}, {cmd: "nb.clear_output"}, {cmd: "nb.open_output"}, {cmd: "nb.send"},
}

func init() {
	onTab := func(m *Model) bool { return m.nbView() != nil }
	onCell := func(m *Model) bool { _, ok := m.nbCell(); return ok }
	onCode := func(m *Model) bool { c, ok := m.nbCell(); return ok && c.Kind == notebook.Code }
	register(
		&command{id: "nb.open", macro: macroNever, title: "Open notebook", desc: "Show the workbook's notebook, or make one: nushell pipelines in cells, their outputs under them",
			run: (*Model).openNotebook},
		&command{id: "nb.edit", macro: macroNever, title: "Edit cell", desc: "Edit the selected cell, or open its output full-screen when the output is selected",
			enabled: onCell, run: (*Model).nbEnter},
		&command{id: "nb.command_mode", macro: macroNever, title: "Stop editing", desc: "Keep what was typed and go back to moving between cells",
			enabled: func(m *Model) bool { v := m.nbView(); return v != nil && v.Editing() },
			run:     func(m *Model) tea.Cmd { m.nbView().StopEdit(); return nil }},
		&command{id: "nb.run", macro: macroNever, title: "Run cell", desc: "Run the selected cell, and whatever it reads that hasn't run",
			enabled: onCode, run: func(m *Model) tea.Cmd { return m.runSelected(0) }},
		&command{id: "nb.run_next", macro: macroNever, title: "Run cell and select next", desc: "Run the selected cell and select the one below, adding one at the end",
			enabled: onCell, run: func(m *Model) tea.Cmd { return m.runSelected(1) }},
		&command{id: "nb.run_insert", macro: macroNever, title: "Run cell and insert below", desc: "Run the selected cell and add a code cell under it",
			enabled: onCell, run: func(m *Model) tea.Cmd { return m.runSelected(2) }},
		&command{id: "nb.run_all", title: "Run all cells", desc: "Run every code cell, in order, each after the cells it reads",
			enabled: onTab, run: func(m *Model) tea.Cmd { return m.runRange(0, -1) }},
		&command{id: "nb.run_above", macro: macroNever, title: "Run cells above", desc: "Run the code cells above the selected one",
			enabled: onCell, run: func(m *Model) tea.Cmd { i, _ := m.nbView().Selected(); return m.runRange(0, i) }},
		&command{id: "nb.run_below", macro: macroNever, title: "Run cell and below", desc: "Run the selected cell and every code cell under it",
			enabled: onCell, run: func(m *Model) tea.Cmd { i, _ := m.nbView().Selected(); return m.runRange(i, -1) }},
		&command{id: "nb.stream", macro: macroNever, title: "Run as stream", desc: "Run the selected cell until it's stopped, its rows arriving as the pipeline prints them (tail -f, watch)",
			enabled: onCode, run: (*Model).streamSelected},
		&command{id: "nb.stop", macro: macroNever, title: "Stop running", desc: "Stop the cell running, killing its process, those waiting and the cells running as streams",
			enabled: func(m *Model) bool { return m.nb.running != nil || len(m.nb.queue) > 0 || len(m.nb.streams) > 0 },
			run:     func(m *Model) tea.Cmd { m.stopCells(); return nil }},
		&command{id: "nb.clear_outputs", title: "Clear outputs", desc: "Clear every cell's output",
			enabled: onTab, run: func(m *Model) tea.Cmd { m.clearOutputs(false); return nil }},
		&command{id: "nb.restart", title: "Restart", desc: "Stop what's running, clear every output and count runs from 1 again",
			enabled: onTab, run: func(m *Model) tea.Cmd { m.clearOutputs(true); return nil }},
		&command{id: "nb.reactive", title: "Reactive notebook", desc: "Run the cells reading a cell again whenever it runs",
			enabled: onTab, checked: func(m *Model) bool { return m.sheet.IsNotebook() && m.sheet.Reactive() },
			run: func(m *Model) tea.Cmd { m.sheet.SetReactive(!m.sheet.Reactive()); return nil }},
	)
	registerCellCommands(onCell, onCode)
	registerOutputCommands(onCode)
}

// nbView is the view of the notebook tab shown, or nil on a sheet.
func (m *Model) nbView() *nbview.View {
	if !m.sheet.IsNotebook() {
		return nil
	}
	return m.viewOf(m.sheet)
}

// viewOf is notebook s's view, made the first time it's asked for.
func (m *Model) viewOf(s *sheet.Sheet) *nbview.View {
	if m.nb.views == nil {
		m.nb.views = map[*sheet.Sheet]*nbview.View{}
	}
	v := m.nb.views[s]
	if v == nil {
		v = nbview.New(nbHost{m, s})
		v.Keys, v.EditKeys = nbKeys, nbEditKeys
		lang := m.nbLang().For()
		v.Providers = nbview.Providers{Highlighter: lang, Completer: lang, Checker: lang}
		m.nb.views[s] = v
	}
	return v
}

// nbCell is the selected cell of the notebook shown.
func (m *Model) nbCell() (notebook.Cell, bool) {
	if v := m.nbView(); v != nil {
		return v.Cell()
	}
	return notebook.Cell{}, false
}

// nbEnter is Enter in command mode: edit the cell, or open its output.
func (m *Model) nbEnter() tea.Cmd {
	v := m.nbView()
	if _, out := v.Selected(); out {
		v.OpenFull()
		return nil
	}
	return v.StartEdit()
}

// setCells replaces the notebook's cells as one undo step, and the user
// who changes cells of a notebook made here trusts it.
func (m *Model) setCells(label string, cells []notebook.Cell) {
	m.sheet.SetNotebookCells(label, cells)
	m.madeCells()
}

// madeCells trusts the workbook's cells on this computer once the user
// changes them, unless cells or macros came from elsewhere untrusted.
func (m *Model) madeCells() {
	if m.macroTrusted() || !m.nb.fromFile && len(m.book().Macros()) == 0 {
		m.trustHere()
	}
}

// openCellName asks for the selected cell's name.
func (m *Model) openCellName() tea.Cmd {
	c, _ := m.nbCell()
	s := m.sheet
	m.openText("Name the cell:", c.Name(), func(m *Model, text string) tea.Cmd {
		if text != "" && text != c.Name() {
			if err := notebook.ValidName(text); err != nil {
				m.fail(err.Error())
				return nil
			}
			if m.book().FreeCellName(text) != text {
				m.fail("$" + text + " is taken: another cell or region has that name")
				return nil
			}
		}
		m.renameCell(s, c, text)
		return nil
	})
	return nil
}

// renameCell names cell c of notebook s, and the output it sent to a
// sheet with it.
func (m *Model) renameCell(s *sheet.Sheet, c notebook.Cell, name string) {
	cells := s.NotebookCells()
	i := slices.IndexFunc(cells, func(x notebook.Cell) bool { return x.ID == c.ID })
	if i < 0 {
		return
	}
	cells[i].Source = notebook.WithName(c.Source, name)
	s.SetNotebookCells("name cell "+strconv.Itoa(i+1), cells)
	m.madeCells()
	if old := c.Name(); old != "" && name != "" {
		if _, r, ok := m.book().Region(old); ok && r.Output {
			m.moveOutput(r, name)
		}
	}
}

// nbHost is how a notebook's view reaches the model.
type nbHost struct {
	m *Model
	s *sheet.Sheet
}

var _ nbview.Host = nbHost{}

func (h nbHost) Theme() *theme.Theme            { return &h.m.th }
func (h nbHost) Locale() *locale.Locale         { return h.m.locale() }
func (h nbHost) Cells() []notebook.Cell         { return h.s.NotebookCells() }
func (h nbHost) Output(id int) *notebook.Output { return h.m.book().Output(id) }
func (h nbHost) State(id int) nbview.State      { return h.m.cellState(h.s, id) }
func (h nbHost) Kernel() nbview.Kernel          { return h.m.nbKernel(h.s) }
func (h nbHost) Run(command string) tea.Cmd     { return h.m.runCommand(command) }

// Edit keeps a cell's new source.
func (h nbHost) Edit(id int, source string) {
	cells := h.s.NotebookCells()
	for i, c := range cells {
		if c.ID == id {
			cells[i].Source = source
			h.s.SetNotebookCells("edit cell "+strconv.Itoa(i+1), cells)
			h.m.madeCells()
			return
		}
	}
}
