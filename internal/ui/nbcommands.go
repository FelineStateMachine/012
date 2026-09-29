package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// The commands that change a notebook's cells and how their outputs
// show, and finding a cell: each acts on the cells selected (nbcells.go).

// registerCellCommands registers the commands that change cells.
func registerCellCommands(onCell, onCode func(*Model) bool) {
	onTab := func(m *Model) bool { return m.nbView() != nil }
	isKind := func(k notebook.Kind) func(m *Model) bool {
		return func(m *Model) bool { c, ok := m.nbCell(); return ok && c.Kind == k }
	}
	register(
		&command{id: "nb.insert_above", macro: macroNever, title: "Add cell above", desc: "Add a code cell above the selected ones",
			enabled: onTab, run: func(m *Model) tea.Cmd { return m.addCell(0, false) }},
		&command{id: "nb.insert_below", macro: macroNever, title: "Add cell below", desc: "Add a code cell below the selected ones",
			enabled: onTab, run: func(m *Model) tea.Cmd { return m.addCell(1, false) }},
		&command{id: "nb.add_code", macro: macroNever, title: "New code cell", desc: "Add a code cell below the selected ones and edit it",
			enabled: onTab, run: func(m *Model) tea.Cmd { return m.addCell(1, true) }},
		&command{id: "nb.delete", macro: macroNever, title: "Delete cells", desc: "Delete the selected cells; z or Undo brings them back",
			enabled: onCell, run: func(m *Model) tea.Cmd { m.deleteCells(); return nil }},
		&command{id: "nb.move_up", macro: macroNever, title: "Move cells up", desc: "Move the selected cells above the cell over them",
			enabled: func(m *Model) bool { from, _ := m.nbRangeIf(); return from > 0 }, run: func(m *Model) tea.Cmd { m.moveCells(-1); return nil }},
		&command{id: "nb.move_down", macro: macroNever, title: "Move cells down", desc: "Move the selected cells below the cell under them",
			enabled: func(m *Model) bool { _, to := m.nbRangeIf(); return to >= 0 && to < len(m.sheet.NotebookCells())-1 },
			run:     func(m *Model) tea.Cmd { m.moveCells(1); return nil }},
		&command{id: "nb.to_note", macro: macroNever, title: "Markdown", desc: "Make the selected cells notes, written in Markdown",
			enabled: onCell, checked: isKind(notebook.Note), run: func(m *Model) tea.Cmd { m.setKind(notebook.Note); return nil }},
		&command{id: "nb.to_code", macro: macroNever, title: "Code", desc: "Make the selected cells code, nushell pipelines",
			enabled: onCell, checked: isKind(notebook.Code), run: func(m *Model) tea.Cmd { m.setKind(notebook.Code); return nil }},
		&command{id: "nb.name", macro: macroNever, title: "Name cell", desc: "Name the cell's output, which later cells read as $name and formulas as nu.name",
			enabled: onCode, run: (*Model).openCellName},
		&command{id: "nb.copy", macro: macroNever, title: "Copy cells", desc: "Copy the selected cells, to paste with v or V",
			enabled: onCell, run: func(m *Model) tea.Cmd { m.copyCells(false); return nil }},
		&command{id: "nb.cut", macro: macroNever, title: "Cut cells", desc: "Cut the selected cells, to paste with v or V",
			enabled: onCell, run: func(m *Model) tea.Cmd { m.copyCells(true); return nil }},
		&command{id: "nb.paste", macro: macroNever, title: "Paste cells below", desc: "Paste the cells copied below the selected ones",
			enabled: func(m *Model) bool { return m.nbView() != nil && len(m.nb.clip) > 0 }, run: func(m *Model) tea.Cmd { m.pasteCells(false); return nil }},
		&command{id: "nb.paste_above", macro: macroNever, title: "Paste cells above", desc: "Paste the cells copied above the selected ones",
			enabled: func(m *Model) bool { return m.nbView() != nil && len(m.nb.clip) > 0 }, run: func(m *Model) tea.Cmd { m.pasteCells(true); return nil }},
		&command{id: "nb.goto", macro: macroNever, title: "Go to cell", desc: "Jump to a cell by its number, name, code or heading",
			enabled: onCell, run: func(m *Model) tea.Cmd { m.openCellPicker(false); return nil }},
		&command{id: "nb.toc", macro: macroNever, title: "Table of contents", desc: "Jump to a heading of the notebook's notes",
			enabled: onTab, run: func(m *Model) tea.Cmd { m.openCellPicker(true); return nil }},
	)
}

// registerOutputCommands registers the commands for outputs.
func registerOutputCommands(onCode func(*Model) bool) {
	register(
		&command{id: "nb.toggle_output", macro: macroNever, title: "Hide output", desc: "Fold the selected cells' outputs to a line, or show them again",
			enabled: onCode, checked: func(m *Model) bool { c, ok := m.nbCell(); return ok && m.nbView().Hidden(c.ID) },
			run: func(m *Model) tea.Cmd { m.nbView().ToggleHidden(); return nil }},
		&command{id: "nb.toggle_whole", macro: macroNever, title: "Show whole output", desc: "Show every row of the selected cells' outputs, or scroll them in a window again",
			enabled: onCode, checked: func(m *Model) bool { c, ok := m.nbCell(); return ok && m.nbView().Whole(c.ID) },
			run: func(m *Model) tea.Cmd { m.nbView().ToggleWhole(); return nil }},
		&command{id: "nb.clear_output", macro: macroNever, title: "Clear output", desc: "Clear the selected cells' outputs",
			enabled: onCode, run: func(m *Model) tea.Cmd { m.clearCellOutputs(); return nil }},
		&command{id: "nb.open_output", macro: macroNever, title: "Open output", desc: "Show the selected cell's output full-screen, to sort and filter it",
			enabled: onCode, run: func(m *Model) tea.Cmd {
				if !m.nbView().OpenFull() {
					m.note = "The cell has no output to open"
				}
				return nil
			}},
		&command{id: "nb.send", macro: macroNever, title: "Send to sheet", desc: "Put the cell's output on a sheet, kept up to date whenever the cell runs",
			enabled: onCode, run: (*Model).sendCell},
		&command{id: "region.freeze", title: "Freeze output", desc: "Turn the output sent here into plain values you can edit; the cell no longer updates it",
			enabled: onOutputRegion, run: func(m *Model) tea.Cmd { return m.freezeOutput() }},
		&command{id: "region.delete", title: "Remove output", desc: "Remove the output sent here and its values from the sheet",
			enabled: onOutputRegion, run: func(m *Model) tea.Cmd { return m.deleteOutput() }},
	)
}

// nbRangeIf is the selected cells of the notebook shown, or -1, -1 on a
// sheet.
func (m *Model) nbRangeIf() (from, to int) {
	v := m.nbView()
	if v == nil || len(m.sheet.NotebookCells()) == 0 {
		return -1, -1
	}
	return v.Range()
}

// openCellPicker opens a picker of the notebook's cells to jump to, or
// of its headings alone, a table of contents.
func (m *Model) openCellPicker(toc bool) {
	v := m.nbView()
	var items []picker.Item
	for _, e := range v.Outline(!toc) {
		title := e.Title
		detail := "cell " + strconv.Itoa(e.Cell+1)
		if e.Level > 0 {
			title = strings.Repeat("  ", e.Level-1) + title
		}
		i := e.Cell
		items = append(items, picker.Item{Title: title, Name: len(title), Detail: detail, Desc: "Go to " + detail,
			Pick: func() tea.Cmd { m.closeOverlay(); v.Select(i, false); return nil }})
	}
	title, empty := "Go to cell", "a cell's number, name or code"
	if toc {
		title, empty = "Table of contents", "a heading"
		if len(items) == 0 {
			m.note = "The notes have no headings: # starts one"
			return
		}
	}
	p := m.newPicker(title, empty, 60, items)
	p.Action = "go"
	m.openOverlay(p)
}
