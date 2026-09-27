package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Whole-row commands, as vim's line operators: copy rows (yy), cut them
// (dd), paste them back as new rows (p, P), and open a new row to type in
// (o, O). They work without vim keys too, from the palette.

func init() {
	register(
		&command{id: "row.yank", title: "Copy rows", desc: "Copy the selected rows whole, to paste as new rows", run: func(m *Model) tea.Cmd {
			r := m.selectedRows()
			cmd := m.copyRows(r)
			m.note = "Copied " + rowCount(r.To.Row-r.From.Row+1)
			return cmd
		}},
		&command{id: "row.cut", title: "Cut rows", desc: "Delete the selected rows, keeping them to paste as new rows", run: func(m *Model) tea.Cmd {
			r := m.selectedRows()
			cmd := m.copyRows(r)
			n := r.To.Row - r.From.Row + 1
			m.sheet.DeleteRows(r.From.Row, n)
			m.note = "Deleted " + rowCount(n) + "; p pastes them back"
			return cmd
		}},
		&command{id: "row.paste_below", title: "Paste rows below", desc: "Insert the copied rows below the active cell's row",
			run: func(m *Model) tea.Cmd { return m.pasteRows(m.cur.Row + 1) }, enabled: (*Model).rowsCopied},
		&command{id: "row.paste_above", title: "Paste rows above", desc: "Insert the copied rows above the active cell's row",
			run: func(m *Model) tea.Cmd { return m.pasteRows(m.cur.Row) }, enabled: (*Model).rowsCopied},
		&command{id: "row.open_below", title: "Open a row below", desc: "Insert a row below the active cell and start typing in it",
			run: func(m *Model) tea.Cmd { return m.openRow(m.cur.Row + 1) }},
		&command{id: "row.open_above", title: "Open a row above", desc: "Insert a row above the active cell and start typing in it",
			run: func(m *Model) tea.Cmd { return m.openRow(m.cur.Row) }},
	)
}

// selectedRows is the rows of the selection across the columns in use.
func (m *Model) selectedRows() sheet.Rect {
	r := m.selection()
	last := 0
	if used, ok := m.sheet.UsedRange(); ok {
		last = used.To.Col
	}
	return sheet.Rect{From: sheet.Addr{Row: r.From.Row}, To: sheet.Addr{Col: last, Row: r.To.Row}}
}

// copyRows puts rows r on the clipboard, marked as whole rows so pasting
// inserts them, and on the system clipboard.
func (m *Model) copyRows(r sheet.Rect) tea.Cmd {
	clip := m.sheet.Copy(r)
	m.copied = clipboard{clip: clip, sheet: m.sheet}
	m.vim.rows = clip
	return tea.SetClipboard(formatTSV(clip.Text()))
}

// rowsCopied reports whether the clipboard holds whole rows.
func (m *Model) rowsCopied() bool {
	return m.vim.rows != nil && m.copied.clip == m.vim.rows
}

// pasteRows inserts the copied rows at row at, as many times as the vim
// count says, as one undo step, and moves to the first of them.
func (m *Model) pasteRows(at int) tea.Cmd {
	clip := m.copied.clip
	cols, rows := clip.Size()
	n := rows * m.vim.n()
	dst := sheet.Rect{From: sheet.Addr{Row: at}, To: sheet.Addr{Col: cols - 1, Row: at + n - 1}}
	if !dst.To.Valid() {
		return m.structural(sheet.ErrPushedOff)
	}
	err := m.sheet.Batch(sheet.Change{Label: "paste " + rowCount(n), Focus: dst, Sheet: m.sheet}, func() error {
		if err := m.sheet.InsertRows(at, n); err != nil {
			return err
		}
		_, err := m.sheet.Paste(clip, dst, false)
		return err
	})
	if err != nil {
		return m.structural(err)
	}
	m.clearSelection()
	m.cur.Row = at
	m.note = "Pasted " + rowCount(n)
	return nil
}

// openRow inserts a blank row at row at and starts an entry in it, in the
// active cell's column.
func (m *Model) openRow(at int) tea.Cmd {
	if at >= sheet.MaxRows {
		return m.structural(sheet.ErrPushedOff)
	}
	if err := m.sheet.InsertRows(at, 1); err != nil {
		return m.structural(err)
	}
	m.clearSelection()
	m.cur.Row = at
	m.startEntry(modeEnter, "")
	return nil
}
