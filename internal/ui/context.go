package ui

import (
	tea "charm.land/bubbletea/v2"
)

// Right-click menus follow Sheets: the cell menu offers clipboard, insert
// and delete commands, and the header menus the subset for whole columns
// or rows. Right-clicking outside the selection first moves there, and
// Shift+F10 opens the cell menu from the keyboard. The menus reuse the
// dropdown, so missing commands are hidden the same way.

var (
	clipboardItems = []menuItem{
		{cmd: "edit.cut"}, {cmd: "edit.copy"}, {cmd: "edit.paste"},
	}
	cellMenu = concat(clipboardItems, []menuItem{
		{cmd: "edit.paste_values", title: "Paste values only"}, sep,
		{cmd: "insert.row_above", title: "Insert row above"}, {cmd: "insert.row_below", title: "Insert row below"},
		{cmd: "insert.col_left", title: "Insert column left"}, {cmd: "insert.col_right", title: "Insert column right"}, sep,
		{cmd: "delete.row", title: "Delete row"}, {cmd: "delete.col", title: "Delete column"}, sep,
		{cmd: "clear"}, sep,
		{cmd: "note.edit", title: "Insert note"}, {cmd: "note.delete", title: "Delete notes"},
		{cmd: "data.protect_range", title: "Protect range"},
	})
	columnMenu = concat(clipboardItems, []menuItem{sep,
		{cmd: "insert.col_left", title: "Insert column left"}, {cmd: "insert.col_right", title: "Insert column right"}, sep,
		{cmd: "delete.col", title: "Delete column"}, {cmd: "clear"}, sep,
		{cmd: "column.width", title: "Resize column"}, {cmd: "column.reset"}, sep,
		{cmd: "data.sort_sheet_az"}, {cmd: "data.sort_sheet_za"}, sep,
		{cmd: "data.filter"}, {cmd: "data.filter_remove"},
	})
	rowMenu = concat(clipboardItems, []menuItem{sep,
		{cmd: "insert.row_above", title: "Insert row above"}, {cmd: "insert.row_below", title: "Insert row below"}, sep,
		{cmd: "delete.row", title: "Delete row"}, {cmd: "clear"}, sep,
		{cmd: "row.height", title: "Resize row"}, {cmd: "row.fit"},
	})
)

func concat(lists ...[]menuItem) []menuItem {
	var out []menuItem
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

func init() {
	register(&command{id: "menu.context", macro: macroNever, title: "Cell menu", desc: "Open the right-click menu for the selection", run: func(m *Model) tea.Cmd {
		x, y := m.cellPos(m.cur)
		m.showContextMenu(cellMenu, x, y+1)
		return nil
	}})
	keymap["shift+f10"] = "menu.context"
}

// rightClick opens the menu for what's under the mouse at x, y.
func (m *Model) rightClick(x, y int) {
	h := m.hitTest(x, y)
	sel := m.selection()
	switch h.kind {
	case hitTab, hitTabAdd:
		m.tabRightClick(h, x, y)
	case hitCell, hitCheckbox, hitDropdown:
		if !m.hasRange() || !sel.Contains(h.addr) {
			m.cur = h.addr
			m.clearSelection()
		}
		m.showContextMenu(cellMenu, x, y+1)
	case hitColHeader, hitColBorder, hitFilterButton:
		if m.whole != wholeCols || h.addr.Col < sel.From.Col || h.addr.Col > sel.To.Col {
			m.cur = h.addr
			m.selecting, m.whole, m.ext = true, wholeCols, h.addr
		}
		m.showContextMenu(columnMenu, x, y+1)
	case hitRowHeader:
		if m.whole != wholeRows || h.addr.Row < sel.From.Row || h.addr.Row > sel.To.Row {
			m.cur = h.addr
			m.selecting, m.whole, m.ext = true, wholeRows, h.addr
		}
		m.showContextMenu(rowMenu, x, y+1)
	}
}
