package ui

import (
	"github.com/FelineStateMachine/012/internal/config"
)

// The theme picker is package themepicker; the model draws, keeps and
// saves the themes it picks.

// keepTheme makes name the theme and saves it in the config file. When
// the config picks a theme by the terminal's background, only the one
// for the current background changes.
func (m *Model) keepTheme(name string) {
	m.closeOverlay()
	m.prefs.preview = ""
	choice := m.prefs.Config.Theme()
	if choice.Light != choice.Dark && !m.prefs.light {
		choice.Dark = name
	} else if choice.Light != choice.Dark {
		choice.Light = name
	} else {
		choice = config.ThemeChoice{Light: name, Dark: name}
	}
	c := m.prefs.Config
	if c == nil {
		c = config.Load("", nil, nil)
		m.prefs.Config = c
	}
	// Until the next start, the choice wins over the file, the
	// environment and flags alike.
	was := c.Get("theme").Src
	c.Override("theme", choice.String(), "File > Settings > Theme")
	if p := m.applyTheme(); p != "" {
		m.note = m.th.Warning.Render(p)
		return
	}
	if c.Path == "" {
		m.note = "Theme " + name + " (not saved: no config file)"
		return
	}
	if err := config.SetInFile(c.Path, "theme", choice.String()); err != nil {
		m.fail("Couldn't save the theme: " + err.Error())
		return
	}
	m.note = "Theme " + name + ", saved in " + config.Tilde(c.Path)
	if was.Kind == config.FromEnv || was.Kind == config.FromFlag {
		m.note += " (" + was.String() + " overrides it at startup)"
	}
}
