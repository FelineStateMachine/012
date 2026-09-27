// Package themepicker is File > Settings > Theme: every theme in a
// picker, fuzzy searched by name, "dark" or "light". The highlighted theme
// is drawn live; Enter keeps it, Esc goes back to the theme there was. It
// knows the UI only through Host, which draws and saves themes.
package themepicker

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the theme picker needs of the UI: a picker's host, and
// the themes.
type Host interface {
	picker.Host
	// Current is the theme configured for the terminal's background.
	Current() string
	// ThemesDir is where the user's own themes are.
	ThemesDir() string
	// Preview draws the theme named name; "" goes back to the configured
	// one.
	Preview(name string)
	// Keep closes the picker and makes name the theme, saving it.
	Keep(name string)
}

// Picker is the open theme picker.
type Picker struct {
	*picker.Picker
	h     Host
	open  bool   // not yet closed or kept
	shown string // the theme being previewed
}

// New returns a picker of every theme with the current one highlighted
// and drawn.
func New(h Host) *Picker {
	tp := &Picker{h: h, open: true}
	current := h.Current()
	entries := theme.List(h.ThemesDir())
	items := make([]picker.Item, 0, len(entries))
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
		items = append(items, picker.Item{Title: name, Name: len(name), Detail: kind, Desc: desc,
			Pick: func() tea.Cmd {
				tp.open = false
				h.Keep(name)
				return nil
			}})
	}
	tp.Picker = picker.New(closing{h, tp}, "Theme", "Type a name, dark or light", 64, items)
	tp.Action = "keep"
	tp.Sel = sel
	tp.preview()
	return tp
}

// closing is the host as the inner picker sees it: closing it closes the
// theme picker.
type closing struct {
	Host
	tp *Picker
}

func (c closing) Close() {
	c.tp.open = false
	c.Host.Close()
}

func (tp *Picker) Key(k tea.KeyPressMsg) tea.Cmd {
	if k.String() == "esc" {
		tp.h.Preview("")
	}
	cmd := tp.Picker.Key(k)
	if tp.open {
		tp.preview()
	}
	return cmd
}

func (tp *Picker) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Box != picker.ID && e.Kind == overlay.MousePress {
		tp.h.Preview("")
	}
	cmd := tp.Picker.Mouse(e)
	if tp.open {
		tp.preview()
	}
	return cmd
}

func (tp *Picker) Changed() {
	tp.Picker.Changed()
	tp.preview()
}

// preview draws the highlighted theme.
func (tp *Picker) preview() {
	it := tp.Selected()
	if it == nil || it.Title == tp.shown {
		return
	}
	tp.shown = it.Title
	tp.h.Preview(it.Title)
}
