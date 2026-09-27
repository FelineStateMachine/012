package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// themePicker is File > Settings > Theme: every theme in a picker, fuzzy
// searched by name, "dark" or "light". The highlighted theme is drawn
// live; Enter keeps it and saves it to the config file, Esc goes back to
// the theme there was.
type themePicker struct {
	*picker
	shownName string // the theme being previewed
}

func newThemePicker(m *Model) *themePicker {
	current := m.prefs.Config.Theme().Pick(!m.prefs.light)
	entries := theme.List(m.prefs.ThemesDir)
	items := make([]pickItem, 0, len(entries))
	sel := 0
	for _, e := range entries {
		kind := "light"
		if e.Dark {
			kind = "dark"
		}
		desc := "A " + kind + " color scheme, drawn in its own colors with solid bars"
		switch {
		case e.Name == theme.Terminal:
			kind, desc = "your terminal's colors", "The terminal's own 16-color palette, following its light or dark background"
		case e.User:
			kind += ", your file"
		}
		if strings.EqualFold(e.Name, current) {
			sel = len(items)
		}
		name := e.Name
		items = append(items, pickItem{title: name, name: len(name), detail: kind, desc: desc,
			pick: func(m *Model) tea.Cmd { m.keepTheme(name); return nil }})
	}
	tp := &themePicker{picker: newPicker(m, "Theme", "Type a name, dark or light", 64, items)}
	tp.action = "keep"
	tp.Sel = sel
	tp.preview(m)
	return tp
}

func (tp *themePicker) Key(k tea.KeyPressMsg) tea.Cmd {
	m := tp.m
	if k.String() == "esc" {
		tp.restore(m)
	}
	cmd := tp.picker.Key(k)
	if m.overlay == tp {
		tp.preview(m)
	}
	return cmd
}

func (tp *themePicker) Mouse(e overlay.MouseEvent) tea.Cmd {
	m := tp.m
	if e.Box != pickerID && e.Kind == overlay.MousePress {
		tp.restore(m)
	}
	cmd := tp.picker.Mouse(e)
	if m.overlay == tp {
		tp.preview(m)
	}
	return cmd
}

func (tp *themePicker) Changed() {
	m := tp.m
	tp.picker.Changed()
	tp.preview(m)
}

// preview draws the highlighted theme.
func (tp *themePicker) preview(m *Model) {
	if tp.Sel >= len(tp.shown) {
		return
	}
	name := tp.shown[tp.Sel].item.title
	if name == tp.shownName {
		return
	}
	tp.shownName = name
	m.prefs.preview = name
	m.applyTheme()
}

// restore goes back to the configured theme.
func (tp *themePicker) restore(m *Model) {
	m.prefs.preview = ""
	m.applyTheme()
}

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
