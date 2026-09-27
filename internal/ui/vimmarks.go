package ui

import (
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Vim marks: m{a-z} marks the active cell, `{a-z} goes back to it (on
// its sheet) and '{a-z} to its row, in column A. Jumps (gg, G, H, M, L,
// /, n, N, a mark, a cell or row from the : line) remember where they
// left from, and '' or `` goes back there. A mark stays on its address:
// rows inserted above it don't move it. Marks live with the workbook.

// mark is a marked cell.
type mark struct {
	sheet *sheet.Sheet
	at    sheet.Addr
}

// jumpMotions are the motions that count as jumps.
var jumpMotions = map[string]bool{"gg": true, "G": true, "H": true, "M": true, "L": true}

// jumped remembers the active cell before a jump, to go back to with a
// quote typed twice.
func (m *Model) jumped() {
	if m.visual() == visualNone {
		m.vim.back = &mark{sheet: m.sheet, at: m.cur}
	}
}

// vimArgKey finishes a sequence that takes a name: the register after ",
// the mark after m, ' or `.
func (m *Model) vimArgKey(key string) tea.Cmd {
	v := &m.vim
	prefix := v.keys
	v.keys = ""
	r, size := utf8.DecodeRuneInString(key)
	if size != len(key) {
		r = 0 // a named key, such as "up"
	}
	switch prefix {
	case `"`:
		if !isRegister(r) {
			m.note = `No register "` + key + `: a-z, 0-9, -, " and +`
			v.reset()
			return nil
		}
		v.reg = r // the count typed before it stays, and more may follow
	case "m":
		v.reset()
		m.setMark(r, key)
	default:
		v.reset()
		m.jumpToMark(r, key, prefix == "`")
	}
	return nil
}

// setMark is m{a-z}.
func (m *Model) setMark(r rune, key string) {
	if r < 'a' || r > 'z' {
		m.note = "Marks are a to z, not " + key
		return
	}
	if m.vim.marks == nil {
		m.vim.marks = map[rune]mark{}
	}
	m.vim.marks[r] = mark{sheet: m.sheet, at: m.cur}
	m.note = "Mark " + key + " at " + m.cur.String() + "; `" + key + " comes back"
}

// jumpToMark is `{a-z} (the cell) and '{a-z} (its row, in column A);
// after ' or `, back to where the last jump left from. In VISUAL mode it
// moves the selection's corner, on the sheet shown.
func (m *Model) jumpToMark(r rune, key string, exact bool) {
	var to mark
	switch mk, ok := m.vim.marks[r]; {
	case r == '\'' || r == '`':
		if m.vim.back == nil {
			m.note = "No jump to go back from yet"
			return
		}
		to = *m.vim.back
	case ok:
		to = mk
	case r >= 'a' && r <= 'z':
		m.note = "Mark " + key + " isn't set: m" + key + " sets it"
		return
	default:
		m.note = "Marks are a to z, not " + key
		return
	}
	if !to.sheet.Live() {
		m.note = "Mark " + key + " was on a deleted sheet"
		return
	}
	if !exact {
		to.at.Col = 0
	}
	if m.visual() != visualNone {
		if to.sheet != m.sheet {
			m.note = "Mark " + key + " is on " + to.sheet.Name()
			return
		}
		m.ext = clampAddr(to.at)
		m.ext.Row = m.visibleRow(m.ext.Row)
		return
	}
	m.jumped()
	m.showSheet(to.sheet)
	m.clearSelection()
	m.cur = clampAddr(to.at)
	m.cur.Row = m.visibleRow(m.cur.Row)
}
