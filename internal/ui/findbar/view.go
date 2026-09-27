package findbar

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// The find bar as the context line draws it, and its mouse targets: the
// fields, then the option chips, with the match count on the right.

// Pieces of the bar, left to right: each field, then the option chips.
type part struct {
	text   string
	field  int    // -1 for chips
	toggle string // the Alt key a chip toggles
}

func (f *Bar) parts() []part {
	th := f.h.Theme()
	label := func(name string, i int) string {
		style := th.Muted
		if f.field == i {
			style = th.Key
		}
		return style.Render(name) + th.Muted.Render(overlay.SearchPrompt)
	}
	parts := []part{{text: label("Find", 0) + f.fieldText(0), field: 0}}
	if f.replace {
		parts = append(parts, part{text: label("Replace", 1) + f.fieldText(1), field: 1})
	}
	chip := func(on bool, name, key string) part {
		style := th.Muted
		if on {
			style = th.MenuSelected
		}
		return part{text: style.Render(" " + name + " "), field: -1, toggle: key}
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
	case f.scope != nil || len(f.h.Book().Visible()) > 1:
		s, _ := f.h.At()
		parts = append(parts, chip(false, "in "+s.Name(), "alt+s"))
	}
	return parts
}

// fieldText is a field's text: the live edit buffer when focused.
func (f *Bar) fieldText(i int) string {
	if f.field == i {
		return f.h.Line().Text()
	}
	return f.fields[i]
}

// gapAfter separates a part from the next: wider after a field, so the
// query and the chips read as separate groups.
func gapAfter(p part) string {
	if p.field >= 0 {
		return "   "
	}
	return " "
}

// spans returns each part with the x where it starts on the context line.
func (f *Bar) spans() ([]part, []int) {
	parts := f.parts()
	xs := make([]int, len(parts))
	x := 0
	for i, p := range parts {
		xs[i] = x
		x += ansi.StringWidth(p.text) + len(gapAfter(p))
	}
	return parts, xs
}

// Layout is empty: the bar is drawn on the context line.
func (f *Bar) Layout() []overlay.Box { return nil }

// ContextLine renders the bar for the context line, with the match count
// on the right.
func (f *Bar) ContextLine() (left, right string) {
	th := f.h.Theme()
	var b strings.Builder
	parts := f.parts()
	for i, p := range parts {
		b.WriteString(p.text)
		if i < len(parts)-1 {
			b.WriteString(gapAfter(p))
		}
	}
	switch {
	case f.err != "":
		right = th.Warning.Render(f.err)
	case f.fields[0] == "":
	case len(f.matches) == 0:
		right = th.Warning.Render("No matches")
	default:
		where := ""
		if f.where == inAll {
			where = " on " + f.matches[f.cur].Sheet.Name()
		}
		right = th.Muted.Render(strconv.Itoa(f.cur+1) + " of " + strconv.Itoa(len(f.matches)) + where)
	}
	return b.String(), right
}

// Cursor puts the terminal cursor in the focused field, whose text ends
// its part.
func (f *Bar) Cursor() (x, y int) {
	parts, xs := f.spans()
	for i, p := range parts {
		if p.field == f.field {
			return xs[i] + ansi.StringWidth(p.text) - ansi.StringWidth(f.h.Line().Tail()), overlay.ContextLine
		}
	}
	return 0, overlay.ContextLine
}

// Mouse focuses a field or flips a chip on the bar; a click anywhere
// else closes the bar and lands where it was clicked.
func (f *Bar) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Kind != overlay.MousePress {
		return nil
	}
	if e.Y != overlay.ContextLine {
		f.h.Leave()
		return f.h.Press(e.X, e.Y, e.Button)
	}
	parts, xs := f.spans()
	for i, p := range parts {
		if e.X >= xs[i] && e.X < xs[i]+ansi.StringWidth(p.text) {
			if p.field >= 0 {
				f.focus(p.field)
				return nil
			}
			return f.Key(overlay.KeyFor(p.toggle))
		}
	}
	return nil
}

// Status shows the keys for moving and replacing, and a reminder of the
// option keys when there's room. Narrow screens keep the most useful keys.
func (f *Bar) Status() (string, string) {
	th := f.h.Theme()
	width, _ := f.h.Size()
	pairs := []string{"Enter", "next", "Shift+Enter", "previous", "Esc", "close"}
	if f.replace {
		pairs = []string{"Enter", "replace", "Ctrl+Enter", "all", "Tab", "field", "Esc", "close"}
	}
	desc := "Alt+C/W/R/= options"
	if f.scope != nil || len(f.h.Book().Visible()) > 1 {
		desc = "Alt+C/W/R/=/S options"
	}
	for {
		keys := th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= width:
			return th.Muted.Render(desc), keys
		case desc != "":
			desc = ""
		case len(pairs) > 2:
			pairs = append(pairs[:len(pairs)-4], pairs[len(pairs)-2:]...) // keep Esc
		default:
			return "", keys
		}
	}
}
