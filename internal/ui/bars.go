package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Drawing data bars, icon sets and dropdown chips (see sheet.Look), in
// the terminal's own glyphs so each reads without color: a bar is
// eighth blocks across the blank columns it covers and reverse video
// under the text it runs beneath; an icon is a glyph at the cell's
// left; a chip is the value in reverse video between rounded ends.

// cellColumns splits a span into what each of its w columns shows: a
// space, or a grapheme with "" after it for each further column it's
// wide.
func cellColumns(sp rowtext.Span, w int) []string {
	cols := make([]string, 0, w)
	for range sp.Lead {
		cols = append(cols, " ")
	}
	for s := sp.Text; s != "" && len(cols) < w; {
		g, gw := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
		s = s[len(g):]
		if gw == 0 {
			continue
		}
		cols = append(cols, g)
		for range gw - 1 {
			cols = append(cols, "")
		}
	}
	for len(cols) < w {
		cols = append(cols, " ")
	}
	return cols[:w]
}

// iconColumn is where an icon goes in a column w wide.
func iconColumn(w int) int { return min(1, w-1) }

// withIcon makes room for an icon at the left of a cell's text: text
// that would start within two columns of it moves right, cut at the
// cell's edge; a value shown without its icon's value is dropped.
func withIcon(sp rowtext.Span, w int, hidden bool) rowtext.Span {
	if hidden {
		return rowtext.Span{Trail: w, Owner: sp.Owner}
	}
	start := iconColumn(w) + 2
	if sp.Text == "" || sp.Lead >= start {
		return sp
	}
	sp.Text = ansi.Truncate(sp.Text, max(w-start, 0), "")
	sp.Lead = start
	sp.Trail = max(w-start-ansi.StringWidth(sp.Text), 0)
	return sp
}

// barText draws the cell at a, of w columns, with its data bar or icon:
// sp is its text on this line, drawn on base (colored when that's a
// highlight, which the bar's and icon's colors give way to), and value
// whether this is the line its value is on.
func (m *Model) barText(sp rowtext.Span, w int, look *sheet.Look, base *lipgloss.Style, colored, value bool) string {
	icon := value && look.Icon != ""
	if icon || look.ValueHidden {
		sp = withIcon(sp, w, look.ValueHidden)
	}
	cols := cellColumns(sp, w)
	var cover []int
	if look.Bar {
		cover = theme.BarCover(look.BarLen, w-1) // the last column keeps bars apart
	}
	text := spanStyle(&m.th, *base, sp)
	var b strings.Builder
	var run strings.Builder
	var cur *lipgloss.Style
	flush := func() {
		if run.Len() > 0 {
			b.WriteString(cur.Render(run.String()))
			run.Reset()
		}
	}
	put := func(role *lipgloss.Style, s string) {
		if role != cur {
			flush()
			cur = role
		}
		run.WriteString(s)
	}
	for x, ch := range cols {
		switch n := coverAt(cover, x); {
		case ch == "":
		case icon && x == iconColumn(w):
			put(m.ruleRole(look.IconColor, base, colored), look.Icon)
		case n > 0 && ch == " ":
			put(m.ruleRole(look.BarColor, base, colored), theme.Block(n))
		case n >= 4 && !colored:
			put(&m.th.BarOn[look.BarColor], ch)
		case ch == " ":
			put(base, ch)
		default:
			put(&text, ch)
		}
	}
	flush()
	return b.String()
}

func coverAt(cover []int, x int) int {
	if x < len(cover) {
		return cover[x]
	}
	return 0
}

// ruleRole is the role a rule color draws a glyph in: the color, or the
// highlight's own role on a highlighted cell.
func (m *Model) ruleRole(c sheet.Color, base *lipgloss.Style, colored bool) *lipgloss.Style {
	if colored || c == sheet.ColorNone || int(c) >= sheet.NumColors {
		return base
	}
	return &m.th.RuleText[c]
}

// chipText draws a dropdown's value as a chip in a column w wide: its
// rounded ends around the value and ▾, the value cut to fit; false when
// the cell shows no value to put on one.
func (m *Model) chipText(sp rowtext.Span, w int) (string, bool) {
	value := strings.TrimSpace(sp.Text)
	room := w - 5 // padding, two ends, a space and the ▾
	if value == "" || room < 1 {
		return "", false
	}
	value = ansi.Truncate(value, room, "…")
	chip := m.paint(&m.th.DropdownCap, "▐") + m.th.DropdownChip.Render(value+" ▾") + m.paint(&m.th.DropdownCap, "▌")
	return " " + chip + strings.Repeat(" ", w-4-ansi.StringWidth(value)-1), true
}
