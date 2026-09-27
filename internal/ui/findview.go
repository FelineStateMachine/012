package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/charmbracelet/x/ansi"
)

// The find bar as the context line draws it, and its mouse targets: the
// fields, then the option chips, with the match count on the right.

// Pieces of the bar, left to right: each field, then the option chips.
type findPart struct {
	text   string
	field  int    // -1 for chips
	toggle string // the Alt key a chip toggles
}

func (f *findBar) parts(m *Model) []findPart {
	label := func(name string, i int) string {
		style := m.th.Muted
		if f.field == i {
			style = m.th.Key
		}
		return style.Render(name) + m.th.Muted.Render(picker.SearchPrompt)
	}
	parts := []findPart{{text: label("Find", 0) + f.fieldText(m, 0), field: 0}}
	if f.replace {
		parts = append(parts, findPart{text: label("Replace", 1) + f.fieldText(m, 1), field: 1})
	}
	chip := func(on bool, name, key string) findPart {
		style := m.th.Muted
		if on {
			style = m.th.MenuSelected
		}
		return findPart{text: style.Render(" " + name + " "), field: -1, toggle: key}
	}
	parts = append(parts,
		chip(f.opts.MatchCase, "Aa", "alt+c"),
		chip(f.opts.WholeCell, "Whole", "alt+w"),
		chip(f.opts.Regex, ".*", "alt+r"),
		chip(f.opts.InFormulas, "=", "alt+="))
	switch {
	case f.where == inRange:
		parts = append(parts, chip(true, "in "+f.scope.String(), "alt+s"))
	case f.where == inAll:
		parts = append(parts, chip(true, "in all sheets", "alt+s"))
	case f.scope != nil || len(m.book().Visible()) > 1:
		parts = append(parts, chip(false, "in "+m.sheet.Name(), "alt+s"))
	}
	return parts
}

// fieldText is a field's text: the live edit buffer when focused.
func (f *findBar) fieldText(m *Model, i int) string {
	if f.field == i {
		return m.line.Text()
	}
	return f.fields[i]
}

// gapAfter separates a part from the next: wider after a field, so the
// query and the chips read as separate groups.
func gapAfter(p findPart) string {
	if p.field >= 0 {
		return "   "
	}
	return " "
}

// spans returns each part with the x where it starts on the context line.
func (f *findBar) spans(m *Model) ([]findPart, []int) {
	parts := f.parts(m)
	xs := make([]int, len(parts))
	x := 0
	for i, p := range parts {
		xs[i] = x
		x += ansi.StringWidth(p.text) + len(gapAfter(p))
	}
	return parts, xs
}

// ContextLine renders the bar for the context line, with the match count
// on the right.
func (f *findBar) ContextLine() (left, right string) {
	m := f.m
	var b strings.Builder
	parts := f.parts(m)
	for i, p := range parts {
		b.WriteString(p.text)
		if i < len(parts)-1 {
			b.WriteString(gapAfter(p))
		}
	}
	switch {
	case f.err != "":
		right = m.th.Warning.Render(f.err)
	case f.fields[0] == "":
	case len(f.matches) == 0:
		right = m.th.Warning.Render("No matches")
	default:
		where := ""
		if f.where == inAll {
			where = " on " + f.matches[f.cur].s.Name()
		}
		right = m.th.Muted.Render(strconv.Itoa(f.cur+1) + " of " + strconv.Itoa(len(f.matches)) + where)
	}
	return b.String(), right
}

// Cursor puts the terminal cursor in the focused field, whose text ends
// its part.
func (f *findBar) Cursor() (x, y int) {
	m := f.m
	parts, xs := f.spans(m)
	for i, p := range parts {
		if p.field == f.field {
			return xs[i] + ansi.StringWidth(p.text) - ansi.StringWidth(m.line.Tail()), contextLine
		}
	}
	return 0, contextLine
}

// Mouse focuses a field or flips a chip on the bar; a click anywhere
// else closes the bar and lands where it was clicked.
func (f *findBar) Mouse(e overlay.MouseEvent) tea.Cmd {
	m := f.m
	if e.Kind != overlay.MousePress {
		return nil
	}
	if e.Y != contextLine {
		m.find = f
		m.closeOverlay()
		return m.handlePress(tea.Mouse{X: e.X, Y: e.Y, Button: e.Button})
	}
	parts, xs := f.spans(m)
	for i, p := range parts {
		if e.X >= xs[i] && e.X < xs[i]+ansi.StringWidth(p.text) {
			if p.field >= 0 {
				f.focus(m, p.field)
				return nil
			}
			return f.Key(keyFor(p.toggle))
		}
	}
	return nil
}

// keyFor makes a key press from a keystroke like "alt+c".
func keyFor(s string) tea.KeyPressMsg {
	k := tea.KeyPressMsg{}
	parts := strings.Split(s, "+")
	for _, p := range parts[:len(parts)-1] {
		if p == "alt" {
			k.Mod |= tea.ModAlt
		}
	}
	k.Code = rune(parts[len(parts)-1][0])
	return k
}

// Status shows the keys for moving and replacing, and a reminder of the
// option keys when there's room. Narrow screens keep the most useful keys.
func (f *findBar) Status() (string, string) {
	m := f.m
	pairs := []string{"Enter", "next", "Shift+Enter", "previous", "Esc", "close"}
	if f.replace {
		pairs = []string{"Enter", "replace", "Ctrl+Enter", "all", "Tab", "field", "Esc", "close"}
	}
	desc := "Alt+C/W/R/= options"
	if f.scope != nil || len(m.book().Visible()) > 1 {
		desc = "Alt+C/W/R/=/S options"
	}
	for {
		keys := m.th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= m.width:
			return m.th.Muted.Render(desc), keys
		case desc != "":
			desc = ""
		case len(pairs) > 2:
			pairs = append(pairs[:len(pairs)-4], pairs[len(pairs)-2:]...) // keep Esc
		default:
			return "", keys
		}
	}
}
