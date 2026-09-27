// Package themepicker is File > Settings > Theme: every theme in a
// picker, fuzzy searched by name, narrowed to one kind by a search
// starting with "dark" or "light". The highlighted theme
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
	// KeyReleases reports whether the terminal says when a key is let
	// go (the kitty keyboard protocol), so Space can be held to peek.
	KeyReleases() bool
}

// Picker is the open theme picker.
type Picker struct {
	*picker.Picker
	h     Host
	open  bool   // not yet closed or kept
	shown string // the theme being previewed
	peek  bool   // Space is held: the list is hidden to show the sheet
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
		case e.Name == theme.HighContrast:
			kind, desc = "high contrast", "White on black or black on white, following the terminal, with WCAG AAA contrast"
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
	tp.Narrow = byKind(entries)
	tp.Sel = sel
	tp.preview()
	return tp
}

// byKind narrows a search starting with the word "dark" or "light" to
// the schemes of that kind (by their meta.isDark), the rest of it then
// matching their names. So "light" lists light schemes, not dark ones
// named like "Bright Lights", and "light sol" finds Solarized Light.
// The terminal's theme is neither kind.
func byKind(entries []theme.Entry) func(string) (string, func(*picker.Item) bool) {
	dark := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Name != theme.Terminal {
			dark[e.Name] = e.Dark
		}
	}
	return func(q string) (string, func(*picker.Item) bool) {
		word, rest, _ := strings.Cut(q, " ")
		var want bool
		switch strings.ToLower(word) {
		case "dark":
			want = true
		case "light":
		default:
			return q, nil
		}
		return rest, func(it *picker.Item) bool {
			d, ok := dark[it.Title]
			return ok && d == want
		}
	}
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

// Holding Space, on a terminal that reports key releases, hides the list
// so the whole sheet shows in the highlighted theme; letting go brings
// the list back. It needs an empty search, so a space can still be typed
// into one. Elsewhere Space is typed, as any other key.

// holdsSpace reports whether k, a press of Space, starts or continues a
// hold rather than typing.
func (tp *Picker) holdsSpace(k tea.KeyPressMsg) bool {
	return k.String() == "space" && tp.h.KeyReleases() && (tp.peek || tp.h.Line().Text() == "")
}

// Release ends a hold of Space.
func (tp *Picker) Release(k tea.KeyReleaseMsg) tea.Cmd {
	if k.String() == "space" {
		tp.peek = false
	}
	return nil
}

func (tp *Picker) Layout() []overlay.Box {
	if tp.peek {
		return nil
	}
	return tp.Picker.Layout()
}

func (tp *Picker) Cursor() (int, int) {
	if tp.peek {
		return -1, 0
	}
	return tp.Picker.Cursor()
}

func (tp *Picker) Status() (string, string) {
	desc, keys := tp.Picker.Status()
	th := tp.h.Theme()
	switch {
	case tp.peek:
		return "Theme " + tp.shown, th.KeyHints("Space", "let go to go back")
	case tp.h.KeyReleases() && tp.h.Line().Text() == "":
		keys = th.KeyHints("Space", "hold to peek") + "  " + keys
	}
	return desc, keys
}

func (tp *Picker) Key(k tea.KeyPressMsg) tea.Cmd {
	if tp.holdsSpace(k) {
		tp.peek = true
		return nil
	}
	tp.peek = false
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
