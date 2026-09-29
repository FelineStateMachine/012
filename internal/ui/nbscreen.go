package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// A notebook tab on the screen: the control panel as for a sheet (the
// menu bar with NOTEBOOK or EDIT as the mode, the formula bar naming the
// selected cell and what it reads, the context line with the keys), the
// tab's own bar where the column letters would be, and the cells where
// the grid would be.

// nbBody is the screen row the notebook's cells start at, under its bar.
const nbBody = gridTop

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
	lines := []string{m.menuBarLine(), m.formulaBar(), m.contextLineText(), m.notebookBar(v)}
	return append(lines, v.Lines()...)
}

// notebookBar is the tab's bar: its name, its cells, what's running,
// and whether it's reactive.
func (m *Model) notebookBar(v *nbview.View) string {
	cells := m.sheet.NotebookCells()
	parts := []string{" " + m.sheet.Name(), plural(len(cells), strconv.Itoa(len(cells))+" cell", strconv.Itoa(len(cells))+" cells")}
	if f := v.Full(); f != nil {
		parts = []string{" " + f.Title + ", full-screen"}
	}
	if r := m.nb.running; r != nil {
		waiting := len(m.nb.queue)
		run := "running 1"
		if waiting > 0 {
			run += ", " + strconv.Itoa(waiting) + " waiting"
		}
		parts = append(parts, run)
	}
	if m.sheet.Reactive() {
		parts = append(parts, "reactive")
	}
	return strings.Join(parts, "   ")
}

// notebookFormulaBar is the formula bar on a notebook tab: the selected
// cell's name, then what it reads or what its output holds.
func (m *Model) notebookFormulaBar(v *nbview.View) string {
	name, text := v.Head()
	box := m.th.Header.Render(theme.PadRight(" "+ansi.Truncate(name, nameBoxW-1, "…"), nameBoxW)) + " "
	return box + m.th.Muted.Render(text)
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
	switch {
	case m.warn != "":
		return m.th.Warning.Render(m.warn), ""
	case m.note != "":
		return m.th.Hint.Render(m.note), ""
	}
	return v.ContextLine()
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

// notebookMouse takes a click or the wheel over the cells.
func (m *Model) notebookMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	v := m.nbView()
	mouse := msg.Mouse()
	if v == nil || m.mode != modeReady || m.overlay != nil || mouse.Y < nbBody || mouse.Y >= m.height-1 {
		return nil, false
	}
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return v.Click(mouse.Y - nbBody), true
		}
		return nil, true
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			v.Wheel(-3)
		case tea.MouseWheelDown:
			v.Wheel(3)
		}
		return nil, true
	}
	return nil, true
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
	}
	for _, v := range m.nb.views {
		if cmd, ok := v.Update(msg); ok {
			return cmd, true
		}
	}
	return nil, false
}

// notebookSync is what follows any message: outputs sent again where
// they're stale, and the highlighter asked about what was drawn.
func (m *Model) notebookSync() tea.Cmd {
	m.syncOutputs()
	if m.book().OutputsChanged() != m.nb.saved {
		m.changed = true
	}
	if v := m.nbView(); v != nil {
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
