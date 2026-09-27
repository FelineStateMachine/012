package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/findbar"
)

// Find and replace is a bar on the context line (package findbar); here
// are its commands and what it asks of the model.

func init() {
	register(
		&command{id: "edit.find", macro: macroView, title: "Find", desc: "Find text in the sheet as you type", run: func(m *Model) tea.Cmd {
			m.openFind(false)
			return nil
		}},
		&command{id: "edit.replace", macro: macroView, title: "Find and replace", desc: "Find text and replace it", run: func(m *Model) tea.Cmd {
			m.openFind(true)
			return nil
		}},
	)
	register(
		&command{id: "edit.find_next", title: "Find next", desc: "Go to the next match of the last search", run: func(m *Model) tea.Cmd {
			m.findAgain(1)
			return nil
		}},
		&command{id: "edit.find_prev", title: "Find previous", desc: "Go to the previous match of the last search", run: func(m *Model) tea.Cmd {
			m.findAgain(-1)
			return nil
		}},
	)
	keymap["ctrl+f"] = "edit.find"
	keymap["ctrl+h"] = "edit.replace"
}

// findAgain goes to the next (d = 1) or previous (d = -1) match of the
// last search from the active cell, without opening the bar, as n and N
// do in vim and less.
func (m *Model) findAgain(d int) {
	if m.find == nil || !m.find.Searched() {
		m.note = "No search yet: " + m.shortcut("edit.find") + " finds"
		return
	}
	m.find.Again(d)
}

// openFind opens the bar, keeping the last search. A multi-cell selection
// becomes the search scope, as Sheets' "specific range".
func (m *Model) openFind(replace bool) {
	f, ok := m.overlay.(*findbar.Bar)
	if !ok {
		f = m.find
		if f == nil {
			f = findbar.New(m.host())
		}
		var sel *sheet.Rect
		if m.hasRange() {
			r := m.selection()
			sel = &r
		}
		f.Reset(sel)
		f.Vim = m.prefs.vim
		m.clearSelection()
		m.openOverlay(f)
	}
	f.Open(replace)
}

// found reports whether a is a match of the open find bar.
func (m *Model) found(a sheet.Addr) bool {
	f, ok := m.overlay.(*findbar.Bar)
	return ok && f.Found(m.sheet, a)
}

// cellCount is n cells in words: "1 cell", "3 cells".
func cellCount(n int) string {
	if n == 1 {
		return "1 cell"
	}
	return strconv.Itoa(n) + " cells"
}
