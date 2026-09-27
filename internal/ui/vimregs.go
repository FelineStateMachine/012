package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Vim registers. The unnamed register is the clipboard Ctrl+V pastes, so
// y, d and x in vim keys and Ctrl+C share it. A register named with "
// before an operator ("ayy, "bx, "ap) holds a copy of its own:
//
//   - "a to "z: named, kept until written again;
//   - "0: the last copy made without naming a register;
//   - "1 to "9: the last rows deleted (dd, cc, V d), newest in "1;
//   - "-: the last cells deleted (x, s, v d);
//   - "+: the system clipboard, written with OSC 52 as every copy is,
//     and read back from the terminal for "+p;
//   - "": the unnamed register itself.
//
// Registers live with the workbook, as the clipboard does.

// vimRegister is what a vim register holds: cells, or whole rows that
// paste as new rows.
type vimRegister struct {
	clip  *sheet.Clip
	sheet *sheet.Sheet // where it was copied from
	rows  bool
}

// isRegister reports whether r names a register.
func isRegister(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '"' || r == '+'
}

// storeRegister puts what an operator just copied (the clipboard) in the
// register it named, or when it named none, in "0 for a copy, "1 for
// deleted rows (moving "1 to "8 along to "2 to "9) and "- for deleted
// cells.
func (m *Model) storeRegister(deleted bool) {
	v := &m.vim
	r := vimRegister{clip: m.copied.clip, sheet: m.copied.sheet, rows: m.rowsCopied()}
	if v.regs == nil {
		v.regs = map[rune]vimRegister{}
	}
	switch name := v.reg; {
	case name >= 'a' && name <= 'z', name >= '0' && name <= '9', name == '-':
		v.regs[name] = r
	case name == '+' || name == '"':
		// The clipboard and the system clipboard have it already.
	case !deleted:
		v.regs['0'] = r
	case r.rows:
		for i := '9'; i > '1'; i-- {
			v.regs[i] = v.regs[i-1]
		}
		v.regs['1'] = r
	default:
		v.regs['-'] = r
	}
}

// fromRegister runs put, a paste of the clipboard, with the register
// named with " in the clipboard's place, leaving the clipboard as it was.
// "+ asks the terminal for the system clipboard, which is pasted when it
// answers (pasteClipboard).
func (m *Model) fromRegister(put func() tea.Cmd) tea.Cmd {
	name := m.vim.reg
	switch name {
	case 0, '"':
		return put()
	case '+':
		m.vim.clipPaste = true
		return tea.ReadClipboard
	}
	r, ok := m.vim.regs[name]
	if !ok || r.clip == nil {
		m.note = `Register "` + string(name) + ` is empty`
		return nil
	}
	saved, savedRows := m.copied, m.vim.rows
	m.copied = clipboard{clip: r.clip, sheet: r.sheet}
	m.vim.rows = nil
	if r.rows {
		m.vim.rows = r.clip
	}
	cmd := put()
	m.copied, m.vim.rows = saved, savedRows
	return cmd
}

// pasteClipboard pastes the system clipboard's text, read for "+p, at
// the active cell: a block of cells for tab-separated or multi-line
// text, as a terminal paste, or one value entered in the cell.
func (m *Model) pasteClipboard(text string) {
	m.vim.clipPaste = false
	if m.mode != modeReady || text == "" {
		return
	}
	m.handlePaste(text)
	if m.mode == modeEnter {
		m.commit()
	}
}
