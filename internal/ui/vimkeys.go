package ui

import (
	tea "charm.land/bubbletea/v2"
)

// The vim key tables. Each binding runs a registered command, so vim keys
// show next to commands in the palette and the shortcuts, and do what the
// menus do; the few that only change mode (v, V, =) say so in help.

// vimSpan is what a count means to a binding.
type vimSpan int

const (
	spanRepeat vimSpan = iota // run the command n times (3u)
	spanOnce                  // the count means nothing (:)
	spanRows                  // act on n rows from the active cell's (3dd)
	spanCols                  // act on n cells from the active cell rightwards (4x)
)

// vimBinding is what a key sequence does in NORMAL or VISUAL mode.
type vimBinding struct {
	id    string // the command run, or shown in help for do
	span  vimSpan
	label string // for help, when not the command's title
	// do, when set, acts instead of running id: for keys that change
	// mode or choose between commands.
	do func(m *Model, n int) tea.Cmd
}

// run carries out the binding with count n.
func (b vimBinding) run(m *Model, n int) tea.Cmd {
	if b.do != nil {
		return b.do(m, n)
	}
	c := commands[b.id]
	if !c.available(m) {
		m.note = c.title + " isn't available now"
		return nil
	}
	switch b.span {
	case spanOnce:
		return m.runCommand(b.id)
	case spanRows:
		m.spanRows(n)
	case spanCols:
		m.spanCols(n)
	default:
		cmds := make([]tea.Cmd, 0, n)
		for range n {
			cmds = append(cmds, m.runCommand(b.id))
		}
		return tea.Batch(cmds...)
	}
	cmd := m.runCommand(b.id)
	if m.mode == modeReady {
		m.clearSelection()
	}
	return cmd
}

// vimNormal binds key sequences in NORMAL mode, in the order help lists
// them.
var vimNormal = map[string]vimBinding{
	"i": {id: "edit", label: "Edit the cell, caret at the start", do: func(m *Model, _ int) tea.Cmd {
		cmd := m.runCommand("edit")
		m.line.Pos = 0
		return cmd
	}},
	"a":      {id: "edit", span: spanOnce},
	"=":      {label: "Start a formula", do: func(m *Model, _ int) tea.Cmd { m.startEntry(modeEnter, "="); return nil }},
	"o":      {id: "row.open_below", span: spanOnce},
	"O":      {id: "row.open_above", span: spanOnce},
	"x":      {id: "clear", span: spanCols},
	"dd":     {id: "row.cut", span: spanRows},
	"yy":     {id: "row.yank", span: spanRows},
	"p":      {id: "row.paste_below", label: "Paste rows below, or cells here", do: putAfter},
	"P":      {id: "row.paste_above", label: "Paste rows above, or cells here", do: putBefore},
	"u":      {id: "edit.undo"},
	"ctrl+r": {id: "edit.redo"},
	"v":      {label: "Select cells (VISUAL)", do: func(m *Model, _ int) tea.Cmd { m.startVisual(visualCells); return nil }},
	"V":      {label: "Select rows (VISUAL)", do: func(m *Model, _ int) tea.Cmd { m.startVisual(visualRows); return nil }},
	"/":      {id: "edit.find", span: spanOnce},
	"n":      {id: "edit.find_next"},
	"N":      {id: "edit.find_prev"},
	":":      {id: "vim.command", span: spanOnce},
	"gt":     {id: "sheet.next"},
	"gT":     {id: "sheet.prev"},
}

// vimVisual binds keys in VISUAL mode; motions extend the selection.
var vimVisual = map[string]vimBinding{
	"d": {label: "Delete the selection, keeping a copy", do: visualDelete},
	"x": {label: "Delete the selection, keeping a copy", do: visualDelete},
	"y": {label: "Copy the selection", do: func(m *Model, _ int) tea.Cmd {
		id := "edit.copy"
		if m.vim.visual == visualRows {
			id = "row.yank"
		}
		return m.endVisual(m.runCommand(id))
	}},
	"p": {id: "edit.paste", label: "Paste over the selection", do: func(m *Model, _ int) tea.Cmd {
		m.vim.visual = visualNone
		return m.runCommand("edit.paste")
	}},
	"o": {label: "Go to the other corner", do: func(m *Model, _ int) tea.Cmd {
		m.cur, m.ext = m.ext, m.cur
		return nil
	}},
	"v": {label: "Select cells, or back to NORMAL", do: func(m *Model, _ int) tea.Cmd { return m.switchVisual(visualCells) }},
	"V": {label: "Select rows, or back to NORMAL", do: func(m *Model, _ int) tea.Cmd { return m.switchVisual(visualRows) }},
	":": {id: "vim.command", span: spanOnce},
}

// vimHelpOrder lists the NORMAL and VISUAL bindings for help, grouped.
var vimHelpOrder = []string{"i", "a", "=", "o", "O", "x", "dd", "yy", "p", "P", "u", "ctrl+r", "v", "V", "/", "n", "N", ":", "gt", "gT"}

// visualDelete deletes the selection, keeping a copy to paste: whole
// rows with V, the cells' contents with v.
func visualDelete(m *Model, _ int) tea.Cmd {
	if m.vim.visual == visualRows {
		return m.endVisual(m.runCommand("row.cut"))
	}
	cmd := m.runCommand("edit.copy")
	m.runCommand("clear")
	return m.endVisual(cmd)
}

// endVisual goes back to NORMAL mode after an operator.
func (m *Model) endVisual(cmd tea.Cmd) tea.Cmd {
	m.vim.visual = visualNone
	if m.mode == modeReady {
		m.clearSelection()
	}
	return cmd
}

// switchVisual changes a visual selection to kind, or ends it when it
// already is one.
func (m *Model) switchVisual(kind visualKind) tea.Cmd {
	if m.vim.visual == kind {
		return m.endVisual(nil)
	}
	m.startVisual(kind)
	return nil
}

// putAfter is p: rows copied with yy or dd go in as new rows below the
// active one; cells copied any other way are pasted at the active cell.
func putAfter(m *Model, _ int) tea.Cmd {
	if !m.rowsCopied() {
		return m.runCommand("edit.paste")
	}
	return m.runCommand("row.paste_below") // it reads the count, n

}

// putBefore is P: as p, but rows go in above the active one.
func putBefore(m *Model, _ int) tea.Cmd {
	if !m.rowsCopied() {
		return m.runCommand("edit.paste")
	}
	return m.runCommand("row.paste_above")
}

// vimKeysFor returns the vim keys bound to a command, NORMAL mode first.
func vimKeysFor(id string) []string {
	var keys []string
	for _, k := range vimHelpOrder {
		if vimNormal[k].id == id {
			keys = append(keys, k)
		}
	}
	return keys
}

// vimShadows reports whether vim keys take key from its Sheets binding:
// Ctrl+D and Ctrl+U scroll, Ctrl+R redoes.
func vimShadows(key string) bool {
	_, bound := vimNormal[key]
	_, moves := vimMotions[key]
	return bound || moves
}

// isVimKey reports whether k is one of the vim tables' keys, which are
// shown as typed (dd, G, $) rather than as a named key.
func isVimKey(k string) bool {
	_, bound := vimNormal[k]
	_, moves := vimMotions[k]
	_, visual := vimVisual[k]
	return (bound || moves || visual) && !isNamedKey(k)
}

// isNamedKey reports whether k names a key rather than a character.
func isNamedKey(k string) bool {
	return len(k) > 2 || k == "up"
}

// vimVisualOrder lists the VISUAL bindings for help.
var vimVisualOrder = []string{"d", "x", "y", "p", "o", "v", "V"}

// vimHelpRows are the vim keys for the shortcuts view: motions, then the
// NORMAL and VISUAL bindings, from the tables above.
func vimHelpRows() []helpRow {
	rows := []helpRow{
		{heading: "Vim keys: NORMAL"},
		{keys: []string{"h", "j", "k", "l"}, action: "Left, down, up, right"},
		{keys: []string{"w", "b"}, action: "Jump to the edge of the data"},
		{keys: []string{"gg", "G"}, action: "First row, last row of data (5G: row 5)"},
		{keys: []string{"0", "$"}, action: "Column A, last cell of the row"},
		{keys: []string{"H", "M", "L"}, action: "Top, middle, bottom of the screen"},
		{keys: []string{"Ctrl+D", "Ctrl+U"}, action: "Half a screen down, up"},
		{keys: []string{"5j", "3dd"}, action: "A count repeats a move or sizes an operator"},
	}
	rows = append(rows, bindingRows(vimNormal, vimHelpOrder)...)
	rows = append(rows, helpRow{heading: "Vim keys: VISUAL"},
		helpRow{keys: []string{"Motions"}, action: "Extend the selection"})
	rows = append(rows, bindingRows(vimVisual, vimVisualOrder)...)
	return append(rows, helpRow{keys: []string{"Esc"}, action: "Back to NORMAL"})
}

// bindingRows lists bindings in order, joining neighbors that do the
// same thing (d and x).
func bindingRows(table map[string]vimBinding, order []string) []helpRow {
	var rows []helpRow
	for _, k := range order {
		b := table[k]
		action := b.label
		if action == "" {
			action = commands[b.id].title
		}
		if n := len(rows); n > 0 && rows[n-1].action == action {
			rows[n-1].keys = append(rows[n-1].keys, keyLabel(k))
			continue
		}
		rows = append(rows, helpRow{keys: []string{keyLabel(k)}, action: action})
	}
	return rows
}
