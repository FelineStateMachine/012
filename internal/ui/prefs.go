package ui

import tea "charm.land/bubbletea/v2"

// Preferences belong to the user, not the sheet: they aren't saved in
// .012 files or undone, and File > New and Open keep them. Each one is a
// checked command in File > Settings.

// prefs are the user's preferences.
type prefs struct {
	vim bool // vim keys in READY mode: see vim.go
}

// initialPrefs is what a new Model starts with. It is the seam where
// preferences are read from the user's configuration (for vim keys, the
// option keymap = default|vim); until that's wired, the defaults.
func initialPrefs() prefs {
	return prefs{}
}

// prefsChanged is called after the user changes a preference, with the
// new preferences in m.prefs. It is the seam where they are written back
// to the user's configuration; nothing persists them yet.
func (m *Model) prefsChanged() tea.Cmd {
	return nil
}

// SetVimKeys turns vim keys on or off, as the setting does, for callers
// that know the user's choice at startup.
func (m *Model) SetVimKeys(on bool) {
	m.prefs.vim = on
	m.vim = vimState{}
}

func init() {
	register(&command{id: "settings.vim", title: "Vim keys",
		desc: "Move with hjkl, counts and operators (dd, yy, p), visual selection with v and V, and : commands",
		run: func(m *Model) tea.Cmd {
			m.SetVimKeys(!m.prefs.vim)
			m.note = "Vim keys off: typing replaces the cell, as in Sheets"
			if m.prefs.vim {
				m.note = "Vim keys on: i or Enter edits, : runs a command, F1 lists the keys"
			}
			return m.prefsChanged()
		},
		checked: func(m *Model) bool { return m.prefs.vim },
	})
}
