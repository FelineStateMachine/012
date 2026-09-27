package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/config"
)

// SetVimKeys turns vim keys on or off, as the setting does, for callers
// that know the user's choice at startup.
func (m *Model) SetVimKeys(on bool) {
	if m.prefs.vim != on {
		m.vim = vimState{}
	}
	m.prefs.vim = on
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
			m.saveKeymap()
			return nil
		},
		checked: func(m *Model) bool { return m.prefs.vim },
	})
}

// saveKeymap writes the keymap option to the config file, so vim keys
// stay as chosen next time.
func (m *Model) saveKeymap() {
	value := "default"
	if m.prefs.vim {
		value = "vim"
	}
	c := m.prefs.Config
	if c == nil || c.Path == "" {
		return
	}
	c.Override("keymap", value, "File > Settings > Vim keys")
	if err := config.SetInFile(c.Path, "keymap", value); err != nil {
		m.fail("Couldn't save the keymap: " + err.Error())
	}
}
