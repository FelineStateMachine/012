package ui

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// Changing a notebook's cells, as Jupyter does, on the cells selected:
// the active cell, or the run Shift+Up and Down selected with it. Each
// change is one undo step (setCells), so z brings back what d d deleted.

// nbRange is the selected cells of the notebook shown, first and last,
// and the cells.
func (m *Model) nbRange() (from, to int, cells []notebook.Cell) {
	from, to = m.nbView().Range()
	return from, to, m.sheet.NotebookCells()
}

// cellWords names cells from to to for a note: "cell 3", "cells 2 to 4".
func cellWords(from, to int) string {
	if from == to {
		return "cell " + strconv.Itoa(from+1)
	}
	return "cells " + strconv.Itoa(from+1) + " to " + strconv.Itoa(to+1)
}

// addCell adds an empty code cell above the selected ones (at 0) or
// below them (at 1), editing it with edit.
func (m *Model) addCell(at int, edit bool) tea.Cmd {
	v := m.nbView()
	from, to, cells := m.nbRange()
	i := from
	if at > 0 && len(cells) > 0 {
		i = to + 1
	}
	v.StopEdit()
	cells = slices.Insert(cells, i, notebook.Cell{})
	m.setCells("add cell", cells)
	v.Select(i, false)
	if edit {
		return v.StartEdit()
	}
	return nil
}

// deleteCells deletes the selected cells.
func (m *Model) deleteCells() {
	from, to, cells := m.nbRange()
	if len(cells) == 0 {
		return
	}
	m.setCells("delete "+cellWords(from, to), slices.Delete(cells, from, to+1))
	m.nbView().Select(from, false)
	m.note = "Deleted " + cellWords(from, to) + ": z brings them back"
	if from == to {
		m.note = "Deleted " + cellWords(from, to) + ": z brings it back"
	}
}

// setKind turns the selected cells into code or note cells.
func (m *Model) setKind(k notebook.Kind) {
	v := m.nbView()
	from, to, cells := m.nbRange()
	changed := false
	for i := from; i <= to && i < len(cells); i++ {
		if cells[i].Kind != k {
			cells[i].Kind, changed = k, true
		}
	}
	if changed {
		v.StopEdit()
		m.setCells("change "+cellWords(from, to), cells)
		v.SelectRange(from, to)
	}
}

// copyCells copies the selected cells, and deletes them with cut.
func (m *Model) copyCells(cut bool) {
	from, to, cells := m.nbRange()
	if len(cells) == 0 {
		return
	}
	m.nb.clip = slices.Clone(cells[from : to+1])
	for i := range m.nb.clip {
		m.nb.clip[i].ID = 0
	}
	if cut {
		m.setCells("cut "+cellWords(from, to), slices.Delete(cells, from, to+1))
		m.nbView().Select(from, false)
		m.note = "Cut " + cellWords(from, to) + ": v pastes below, V above"
		return
	}
	m.note = "Copied " + cellWords(from, to) + ": v pastes below, V above"
}

// pasteCells pastes the cells copied below the selected ones, or above
// them, selecting what it pasted.
func (m *Model) pasteCells(above bool) {
	v := m.nbView()
	from, to, cells := m.nbRange()
	i := from
	if !above && len(cells) > 0 {
		i = to + 1
	}
	cells = slices.Insert(cells, i, m.nb.clip...)
	m.setCells("paste "+plural(len(m.nb.clip), "cell", strconv.Itoa(len(m.nb.clip))+" cells"), cells)
	v.SelectRange(i, i+len(m.nb.clip)-1)
}

// moveCells moves the selected cells one place up (d -1) or down (1),
// together, still selected.
func (m *Model) moveCells(d int) {
	v := m.nbView()
	from, to, cells := m.nbRange()
	if len(cells) == 0 || from+d < 0 || to+d >= len(cells) {
		return
	}
	v.StopEdit()
	moved := slices.Clone(cells[from : to+1])
	cells = slices.Delete(cells, from, to+1)
	cells = slices.Insert(cells, from+d, moved...)
	m.setCells("move "+cellWords(from, to), cells)
	if sel, _ := v.Selected(); sel == to {
		v.SelectRange(from+d, to+d) // the active cell stays the one it was
	} else {
		v.SelectRange(to+d, from+d)
	}
}

// clearCellOutputs clears the selected cells' outputs.
func (m *Model) clearCellOutputs() {
	from, to, cells := m.nbRange()
	for i := from; i <= to && i < len(cells); i++ {
		if m.book().Output(cells[i].ID) != nil {
			m.book().SetOutput(cells[i].ID, nil)
			m.feedOutput(cells[i].Name())
		}
	}
	m.note = "Cleared the output of " + cellWords(from, to)
}
