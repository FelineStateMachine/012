package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// A notebook tab on the screen, as Jupyter's: the menu bar with
// NOTEBOOK or EDIT as the mode, the notebook's toolbar where a sheet's
// formula bar is, the context line (what the selection is and the keys
// that apply, or prompts and messages) drawn as a rule closing off the
// toolbar, and the cells under it where the grid would be.

// nbBody is the screen row the notebook's cells start at, under the
// context line.
const nbBody = contextLine + 1

// openNotebook shows the workbook's notebook, making one after the sheet
// shown if it has none.
func (m *Model) openNotebook() tea.Cmd {
	w := m.book()
	s := w.Notebook()
	if s == nil {
		var err error
		if s, err = w.AddNotebook("", m.sheet); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.note = "Added " + s.Name() + ": b adds a code cell, Enter edits it, Shift+Enter runs it"
	}
	m.showSheet(s)
	return m.loadWords()
}

// OpenNotebook starts on the workbook's notebook, as 012 nu does: a new
// file's empty sheet becomes the notebook, with a code cell ready to
// type in; a workbook without one gets one.
func (m *Model) OpenNotebook() {
	w := m.book()
	s := w.Notebook()
	if s == nil {
		if first := w.Sheet(0); w.Len() == 1 && first.Len() == 0 && !w.CanUndo() && w.RenameSheet(first, "Notebook") == nil {
			first.MakeNotebook()
			s = first
		} else if s, _ = w.AddNotebook("", m.sheet); s == nil {
			return
		}
	}
	if len(s.NotebookCells()) == 0 {
		s.SetNotebookCells("add cell", []notebook.Cell{{}})
		w.ClearHistory()
		m.saved = w.StateID()
	}
	m.showSheet(s)
	m.nb.startup = true
}

// startNotebook edits the new notebook's first cell as the program
// starts, when 012 nu asked.
func (m *Model) startNotebook() tea.Cmd {
	if !m.nb.startup {
		return nil
	}
	m.nb.startup = false
	v := m.nbView()
	if v == nil {
		return nil
	}
	cmd := m.loadWords()
	if c, ok := v.Cell(); ok && c.Source == "" && len(m.sheet.NotebookCells()) == 1 {
		return tea.Batch(cmd, v.StartEdit())
	}
	return cmd
}

// notebookKey gives a key to the notebook shown, reporting whether it
// took it.
func (m *Model) notebookKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	v := m.nbView()
	if v == nil {
		return nil, false
	}
	m.sizeNotebook(v)
	cmd, ok := v.Key(k)
	if !ok && v.Editing() {
		v.Commit() // a key of the UI's, such as Ctrl+S, sees what's typed
	}
	return cmd, ok
}

// sizeNotebook tells the view how much room it has.
func (m *Model) sizeNotebook(v *nbview.View) {
	v.Resize(m.width, m.height-nbBody-1)
}

// notebookLines are the screen's lines on a notebook tab, from the
// menu bar down to the line above the status line.
func (m *Model) notebookLines(v *nbview.View) []string {
	m.sizeNotebook(v)
	lines := []string{m.menuBarLine(), m.formulaBar(), m.contextLineText()}
	return append(lines, v.Lines()...)
}

// notebookToolbar is the toolbar, where a sheet's formula bar is.
func (m *Model) notebookToolbar(v *nbview.View) string {
	return v.Toolbar(m.width)
}

// nbKernel is how running stands in notebook s, for its toolbar.
func (m *Model) nbKernel(s *sheet.Sheet) nbview.Kernel {
	k := nbview.Kernel{Off: m.shellOff(), Reactive: s.Reactive(), Clip: len(m.nb.clip)}
	if r := m.nb.runs.running; r != nil && r.s == s {
		k.Busy = true
	}
	for _, q := range m.nb.runs.queue {
		if q.s == s {
			k.Waiting++
		}
	}
	for _, st := range m.nb.runs.streams {
		if st.s == s {
			k.Live++
		}
	}
	return k
}

// pasteMsg takes pasted text: into a notebook's cell being edited, and
// nowhere else on a notebook tab, or as on a sheet.
func (m *Model) pasteMsg(text string) tea.Cmd {
	if v := m.nbView(); v != nil && m.mode == modeReady && m.overlay == nil {
		return v.Paste(text)
	}
	m.handlePaste(text)
	return nil
}

// notebookContext is the context line on a notebook tab: what the last
// action said, or the notebook's own line.
func (m *Model) notebookContext(v *nbview.View) (string, string) {
	left, right := v.ContextLine()
	switch {
	case m.warn != "":
		left, right = m.th.Warning.Render(m.warn), ""
	case m.note != "":
		left, right = m.th.Hint.Render(m.note), ""
	}
	if v.FullOpen() {
		return left, right
	}
	return v.Rule(left, right, m.width), ""
}

// notebookIndicator is the mode on a notebook tab.
func notebookIndicator(v *nbview.View) string {
	switch {
	case v.FullOpen():
		return "OUTPUT"
	case v.Editing():
		return "EDIT"
	}
	return "NOTEBOOK"
}

// notebookCursor is where the terminal's caret goes on a notebook tab.
func (m *Model) notebookCursor(v *nbview.View) (int, int, bool) {
	if f := v.Full(); f != nil {
		if x, ok := f.Cursor(); ok {
			return x, contextLine, true
		}
		return 0, 0, false
	}
	x, y, ok := v.Cursor()
	return x, nbBody + y, ok
}

// notebookBoxes are the notebook's floating boxes: its completions.
func (m *Model) notebookBoxes() []overlay.Box {
	if v := m.nbView(); v != nil && m.overlay == nil && m.mode == modeReady {
		return v.Boxes(nbBody)
	}
	return nil
}

// notebookMouse takes a click on the toolbar, or a click or the wheel
// over the cells.
func (m *Model) notebookMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	v := m.nbView()
	mouse := msg.Mouse()
	if v == nil || m.mode != modeReady || m.overlay != nil {
		return nil, false
	}
	if click, ok := msg.(tea.MouseClickMsg); ok && mouse.Y == formulaLine && click.Button == tea.MouseLeft {
		return m.toolbarClick(v, mouse.X), true
	}
	if mouse.Y < nbBody || mouse.Y >= m.height-1 {
		return nil, false
	}
	m.sizeNotebook(v)
	y := mouse.Y - nbBody
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		switch msg.Button {
		case tea.MouseLeft:
			return v.Click(mouse.X, y, mouse.Mod.Contains(tea.ModShift)), true
		case tea.MouseRight:
			v.RightClick(y)
			m.showContextMenu(nbCellMenu, mouse.X, mouse.Y+1)
		}
		return nil, true
	case tea.MouseWheelMsg:
		m.nb.follow = false // the user looks elsewhere while cells run
		switch msg.Button {
		case tea.MouseWheelUp:
			v.Wheel(y, -3)
		case tea.MouseWheelDown:
			v.Wheel(y, 3)
		}
		return nil, true
	}
	return nil, true
}

// toolbarClick runs the toolbar's button at column x: the cell's kind
// opens a menu of the kinds under it.
func (m *Model) toolbarClick(v *nbview.View, x int) tea.Cmd {
	switch id := v.ToolbarAt(x, m.width); id {
	case "":
		return nil
	case nbview.Back:
		v.CloseFull()
		return nil
	case "nb.kind":
		m.showContextMenu([]menuItem{{cmd: "nb.to_code"}, {cmd: "nb.to_note"}}, x, formulaLine+1)
		return nil
	default:
		return m.runCommand(id)
	}
}

// notebookMsg takes the notebooks' own messages.
func (m *Model) notebookMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case nbDoneMsg:
		return m.finishCell(msg), true
	case nbWordsMsg:
		m.nb.words = msg.words
		return nil, true
	case nbTickMsg:
		return m.ticked(msg), true
	case nbStreamMsg:
		return m.streamPolled(msg), true
	case nbStreamTickMsg:
		return m.streamTicked(), true
	}
	for _, v := range m.nb.views {
		if cmd, ok := v.Update(msg); ok {
			return cmd, true
		}
	}
	return nil, false
}

// notebookSync is what follows any message: outputs sent again where
// they're stale, the language told what the notebook binds, and the
// highlighter asked about what was drawn.
func (m *Model) notebookSync() tea.Cmd {
	m.syncOutputs()
	if m.book().OutputsChanged() != m.nb.saved {
		m.changed = true
	}
	if v := m.nbView(); v != nil {
		m.syncLang(m.sheet, v)
		return v.Fetch()
	}
	return nil
}

// bookOpened notes what a workbook opened with: whether it has cells of
// its own, which may come from another computer, and what converting
// it said.
func (m *Model) bookOpened() {
	w := m.book()
	for _, s := range w.Sheets() {
		m.nb.fromFile = m.nb.fromFile || len(s.NotebookCells()) > 0
	}
	if notes := w.LoadNotes(); len(notes) > 0 {
		m.note = strings.Join(notes, "; ")
	}
}

// outputCaps are the caps on the outputs a file keeps, from the config.
func (m *Model) outputCaps() notebook.Caps {
	c := notebook.DefaultCaps
	if cfg := m.prefs.Config; cfg != nil {
		c = notebook.Caps{Cell: cfg.Int("nu-save-cell-kb") << 10, Total: cfg.Int("nu-save-notebook-kb") << 10}
	}
	return c
}

// leftGrid notes the sheet left, when it's a grid, for $selection.
func (m *Model) leftGrid() {
	if !m.sheet.IsNotebook() {
		m.nb.grid, m.nb.sel = m.sheet, m.selection()
	}
}

// notebookSafe reports whether a command applies on a notebook tab:
// those about files, sheets, settings, help and the notebook itself,
// not those acting on a grid's cells.
func notebookSafe(id string) bool {
	for _, p := range []string{"file.", "sheet.", "nb.", "settings.", "help", "palette", "menu", "quit", "vim.command", "macro.manage"} {
		if strings.HasPrefix(id, p) {
			return id != "sheet.duplicate"
		}
	}
	return id == "edit.undo" || id == "edit.redo"
}
