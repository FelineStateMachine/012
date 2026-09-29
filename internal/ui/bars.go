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
	text := base
	if !plainSpan(sp) {
		st := spanStyle(&m.th, *base, sp)
		text = &st
	}
	// A run is drawn in the cell's role (penBase), its text's style
	// (penText) or a role of the theme (penRole, role). The theme's roles
	// are held apart from base, so base, often a shade on the caller's
	// stack, never flows into the frame's cache of shades.
	var b strings.Builder
	var run strings.Builder
	cur, curRole := penBase, (*lipgloss.Style)(nil)
	flush := func() {
		switch {
		case run.Len() == 0:
		case cur == penBase && !colored:
			b.WriteString(run.String())
		case cur == penBase:
			b.WriteString(base.Render(run.String()))
		case cur == penText:
			b.WriteString(text.Render(run.String()))
		default: // a theme's role, whose address stays put for the frame
			b.WriteString(m.shade(curRole).Wrap(run.String()))
		}
		run.Reset()
	}
	put := func(p pen, role *lipgloss.Style, s string) {
		if p == penText && text == base {
			p = penBase
		}
		if p != cur || role != curRole {
			flush()
			cur, curRole = p, role
		}
		run.WriteString(s)
	}
	for x, ch := range cols {
		switch n := coverAt(cover, x); {
		case ch == "":
		case icon && x == iconColumn(w):
			p, role := m.ruleRole(look.IconColor, colored)
			put(p, role, look.Icon)
		case n > 0 && ch == " ":
			p, role := m.ruleRole(look.BarColor, colored)
			put(p, role, theme.Block(n))
		case n >= 4 && !colored:
			put(penRole, &m.th.BarOn[look.BarColor], ch)
		case ch == " ":
			put(penBase, nil, ch)
		default:
			put(penText, nil, ch)
		}
	}
	flush()
	return b.String()
}

// shade is role with its escape codes, kept for the frame.
func (m *Model) shade(role *lipgloss.Style) theme.Shade {
	if sh, ok := m.shaded[role]; ok {
		return sh
	}
	if m.shaded == nil {
		m.shaded = map[*lipgloss.Style]theme.Shade{}
	}
	sh := theme.ShadeOf(*role)
	m.shaded[role] = sh
	return sh
}

func coverAt(cover []int, x int) int {
	if x < len(cover) {
		return cover[x]
	}
	return 0
}

// pen is what barText draws a run of a cell's columns in.
type pen uint8

const (
	penBase pen = iota // the cell's role
	penText            // its text's style
	penRole            // a role of the theme, kept for the frame
)

// ruleRole is the pen a rule color draws a glyph in: the color's role,
// or the cell's own role on a highlighted cell.
func (m *Model) ruleRole(c sheet.Color, colored bool) (pen, *lipgloss.Style) {
	if colored || c == sheet.ColorNone || int(c) >= sheet.NumColors {
		return penBase, nil
	}
	return penRole, &m.th.RuleText[c]
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
