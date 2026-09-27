package ui

import (
	"errors"
	"fmt"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/formula"
)

// Undo and redo, fill, inserting and deleting rows and columns, and F4 in
// formulas. The engine does the work and records each change as one undo
// step; this file maps commands onto the selection and reports back on the
// context line.

func init() {
	register(
		&command{id: "edit.undo", macro: macroNever, title: "Undo", desc: "Undo the last change", run: (*Model).undo},
		&command{id: "edit.redo", macro: macroNever, title: "Redo", desc: "Redo the last undone change", run: (*Model).redo},
		&command{id: "edit.fill_down", title: "Fill down", desc: "Copy the top row of the selection into the rows below it", edits: (*Model).selection, run: func(m *Model) tea.Cmd {
			return m.fill(m.sheet.FillDown)
		}},
		&command{id: "edit.fill_right", title: "Fill right", desc: "Copy the left column of the selection into the columns right of it", edits: (*Model).selection, run: func(m *Model) tea.Cmd {
			return m.fill(m.sheet.FillRight)
		}},
		&command{id: "insert.row_above", title: "Insert rows above", desc: "Insert as many rows above the selection as it spans", edits: rowsTarget, changes: noCells, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			return m.structural(m.sheet.InsertRows(r.From.Row, r.To.Row-r.From.Row+1))
		}},
		&command{id: "insert.row_below", title: "Insert rows below", desc: "Insert as many rows below the selection as it spans", edits: rowsBelowTarget, changes: noCells, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			if r.To.Row == sheet.MaxRows-1 {
				return m.structural(sheet.ErrPushedOff)
			}
			return m.structural(m.sheet.InsertRows(r.To.Row+1, r.To.Row-r.From.Row+1))
		}},
		&command{id: "insert.col_left", title: "Insert columns left", desc: "Insert as many columns left of the selection as it spans", edits: colsTarget, changes: noCells, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			return m.structural(m.sheet.InsertCols(r.From.Col, r.To.Col-r.From.Col+1))
		}},
		&command{id: "insert.col_right", title: "Insert columns right", desc: "Insert as many columns right of the selection as it spans", edits: colsRightTarget, changes: noCells, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			if r.To.Col == sheet.MaxCols-1 {
				return m.structural(sheet.ErrPushedOff)
			}
			return m.structural(m.sheet.InsertCols(r.To.Col+1, r.To.Col-r.From.Col+1))
		}},
		&command{id: "delete.row", title: "Delete rows", desc: "Delete the selected rows; references to them become #REF!", edits: rowsTarget, changes: selectedRowsTarget, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			m.sheet.DeleteRows(r.From.Row, r.To.Row-r.From.Row+1)
			return nil
		}},
		&command{id: "delete.col", title: "Delete columns", desc: "Delete the selected columns; references to them become #REF!", edits: colsTarget, changes: selectedColsTarget, keepsSpills: true, run: func(m *Model) tea.Cmd {
			r := m.selection()
			m.sheet.DeleteCols(r.From.Col, r.To.Col-r.From.Col+1)
			return nil
		}},
		// Sheets' Ctrl+Alt+= and Ctrl+Alt+- act on rows, or on columns when
		// whole columns are selected.
		&command{id: "insert.selection", title: "Insert rows or columns", desc: "Insert rows above the selection, or columns left of it when whole columns are selected", run: func(m *Model) tea.Cmd {
			if m.whole == wholeCols {
				return m.runCommand("insert.col_left")
			}
			return m.runCommand("insert.row_above")
		}},
		&command{id: "delete.selection", title: "Delete rows or columns", desc: "Delete the selected rows, or columns when whole columns are selected", run: func(m *Model) tea.Cmd {
			if m.whole == wholeCols {
				return m.runCommand("delete.col")
			}
			return m.runCommand("delete.row")
		}},
	)
	keymap["ctrl+alt+="] = "insert.selection"
	keymap["ctrl+alt+-"] = "delete.selection"
	keymap["ctrl+z"] = "edit.undo"
	keymap["ctrl+y"] = "edit.redo"
	keymap["ctrl+shift+z"] = "edit.redo"
	keymap["ctrl+d"] = "edit.fill_down"
	keymap["ctrl+r"] = "edit.fill_right"
}

func (m *Model) undo() tea.Cmd {
	c, ok := m.sheet.Undo()
	m.afterHistory("Undid", "Nothing to undo", c, ok)
	return nil
}

func (m *Model) redo() tea.Cmd {
	c, ok := m.sheet.Redo()
	m.afterHistory("Redid", "Nothing to redo", c, ok)
	return nil
}

// afterHistory selects what an undo or redo changed, as Sheets does, and
// says what it was on the context line.
func (m *Model) afterHistory(verb, none string, c sheet.Change, ok bool) {
	if !ok {
		m.note = none
		return
	}
	m.note = verb + ": " + c.Label
	i := m.book().Index(m.sheet)
	if i < 0 {
		i = m.book().Active()
	}
	m.afterSheetsChange(c.Sheet, i)
	if !c.Tabs && !c.Macros {
		m.selectRect(c.Focus)
	}
}

// selectRect selects r, as whole columns or rows when it spans the sheet.
func (g *grid) selectRect(r sheet.Rect) {
	allRows := r.From.Row == 0 && r.To.Row == sheet.MaxRows-1
	allCols := r.From.Col == 0 && r.To.Col == sheet.MaxCols-1
	switch {
	case allRows && allCols:
		g.selecting, g.whole, g.ext = true, wholeAll, g.cur
	case allRows:
		g.cur.Col = r.From.Col
		g.ext = sheet.Addr{Col: r.To.Col, Row: g.cur.Row}
		g.selecting, g.whole = true, wholeCols
	case allCols:
		g.cur.Row = r.From.Row
		g.ext = sheet.Addr{Col: g.cur.Col, Row: r.To.Row}
		g.selecting, g.whole = true, wholeRows
	default:
		g.cur, g.ext = r.From, r.To
		g.selecting, g.whole = r.From != r.To, wholeNone
	}
}

func (m *Model) fill(fn func(sheet.Rect) (sheet.Rect, error)) tea.Cmd {
	m.writeChecked("Fill", func() (sheet.Rect, error) { return fn(m.selection()) })
	return nil
}

// structural reports an insert that couldn't be done.
func (m *Model) structural(err error) tea.Cmd {
	if err != nil {
		m.fail(err.Error())
	}
	return nil
}

// fillEntry stores the entry being typed in every selected cell, adjusting
// references as if it were copied from the active cell (Ctrl+Enter).
func (m *Model) fillEntry() bool {
	input := m.storedEntry(m.line.Text())
	if err := m.entrySheet().FillEntry(m.selection(), m.cur, input); err != nil {
		m.entryError(err, input)
		return false
	}
	if m.checkEntryFill(m.entrySheet().InvalidIn(m.selection()), input) {
		return false
	}
	m.cancelEntry()
	m.recordEntry(input, true)
	return true
}

// entryError keeps an entry that can't be stored in EDIT mode, with the
// caret at the problem and the reason on line 3.
func (m *Model) entryError(err error, input string) {
	var pe *sheet.ParseError
	m.mode = modeEdit
	m.entry.hint = sheet.LocalizeError(err, m.locale()).Error()
	if errors.As(err, &pe) {
		m.line.Pos = utf8.RuneCountInString(input[:min(pe.Pos, len(input))])
	}
}

// toggleAbsolute cycles the reference at the caret through A1, $A$1, A$1
// and $A1, as F4 does in Sheets. A range cycles both corners.
func (m *Model) toggleAbsolute() {
	if buf, pos, ok := formula.CycleRef(m.line.Buf, m.line.Pos); ok {
		m.line.Buf, m.line.Pos = buf, pos
	}
}

// countCells describes a paste for the context line.
func countCells(r sheet.Rect) string {
	cols, rows := r.To.Col-r.From.Col+1, r.To.Row-r.From.Row+1
	switch {
	case r.AllRows() && r.AllCols():
		return "the whole sheet"
	case r.AllRows() && cols == 1:
		return "1 column"
	case r.AllRows():
		return fmt.Sprintf("%d columns", cols)
	case r.AllCols():
		return rowCount(rows)
	}
	n := cols * rows
	if n == 1 {
		return "1 cell"
	}
	return fmt.Sprintf("%d cells", n)
}
