package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Drawing the sheet's rules (see sheet.Look): a conditional format's
// fill, text color and text styles, a color scale's shade, checkboxes,
// the ▾ of a dropdown and the mark of an invalid entry. Only the visible
// cells are asked for their looks, and the sheet keeps them until the
// next recalculation, so a rule over thousands of rows costs a frame
// what the screen shows.

// checkGlyphs are a checkbox's text, unchecked and checked, drawn
// centered like a boolean. ASCII brackets keep one width in every
// terminal, where ballot box glyphs don't.
var checkGlyphs = [2]string{"[ ]", "[✓]"}

// ruleSpan applies the looks of cell a and of the owner of its text to
// a span: its text styles and a checkbox's glyph, and, where the cell
// isn't colored already, the shade it's drawn on, if any (shaded).
func (m *Model) ruleSpan(a sheet.Addr, sp *rowtext.Span, colored bool, rgb func(int) color.Color) (look sheet.Look, shade theme.Shade, shaded bool) {
	look = m.sheet.Look(a)
	own := look
	if sp.Owner != a.Col && sp.Text != "" {
		own = m.sheet.Look(sheet.Addr{Col: sp.Owner, Row: a.Row})
	}
	if look.Checkbox {
		*sp = checkSpan(look.Checked, m.sheet.ColWidth(a.Col), m.sheet.CellStyle(a))
		own = look
	}
	if own.Styled {
		st := own.Style
		sp.Style.Bold = sp.Style.Bold || st.Bold
		sp.Style.Italic = sp.Style.Italic || st.Italic
		sp.Style.Underline = sp.Style.Underline || st.Underline
		sp.Style.Strikethrough = sp.Style.Strikethrough || st.Strikethrough
	}
	sp.Invalid = own.Invalid && sp.Text != ""
	switch {
	case colored:
	case look.Scaled:
		return look, m.th.ScaleFill(look.From, look.To, look.Pos, rgb), true
	case look.Styled || own.Styled:
		st := sheet.RuleStyle{}
		if look.Styled {
			st.Fill = look.Style.Fill
		}
		if own.Styled {
			st.Text = own.Style.Text
		}
		if st.Fill != sheet.ColorNone || st.Text != sheet.ColorNone {
			return look, m.th.RuleShade(st), true
		}
	}
	return look, theme.Shade{}, false
}

// checkSpan is a checkbox drawn in a column w wide.
func checkSpan(on bool, w int, st sheet.Style) rowtext.Span {
	g := checkGlyphs[0]
	if on {
		g = checkGlyphs[1]
	}
	if w < 3 {
		g = ansi.Truncate(g[1:], w, "")
	}
	gw := ansi.StringWidth(g)
	lead := (w - gw) / 2
	st.Underline = false // an underline would run under the brackets
	return rowtext.Span{Lead: lead, Text: g, Trail: w - gw - lead, Style: st}
}

// dropdownMark puts the ▾ of a dropdown cell in its last column.
func (m *Model) dropdownMark(text string, w int, base lipgloss.Style, colored bool) string {
	if w < 3 {
		return text
	}
	mark := m.th.Dropdown.Render("▾")
	if colored {
		mark = base.Render("▾")
	}
	return ansi.Truncate(text, w-1, "") + mark
}

// slotColor is the terminal's color for an ANSI slot, as color scales
// blend it.
func (m *Model) slotColor(i int) color.Color { return m.term.color(i) }

// checkboxAt reports where the glyph of the checkbox at a is on screen,
// as columns from..to of the row, or false if a isn't a checkbox.
func (m *Model) checkboxAt(a sheet.Addr, start int) (from, to int, ok bool) {
	if !m.sheet.HasRules() || !m.sheet.Look(a).Checkbox {
		return 0, 0, false
	}
	sp := checkSpan(false, m.sheet.ColWidth(a.Col), sheet.Style{})
	return start + sp.Lead, start + sp.Lead + ansi.StringWidth(sp.Text) - 1, true
}

// ruleHit is what a click at x on the cell at a starting at start hits
// among its rules: a checkbox's glyph or a dropdown's ▾.
func (m *Model) ruleHit(a sheet.Addr, x, start int) hitKind {
	if m.mode != modeReady || !m.sheet.HasRules() {
		return hitCell
	}
	if from, to, ok := m.checkboxAt(a, start); ok && x >= from && x <= to {
		return hitCheckbox
	}
	if w := m.sheet.ColWidth(a.Col); w >= 3 && x == start+w-1 && m.sheet.Look(a).Dropdown {
		return hitDropdown
	}
	return hitCell
}

// validationLine is the context line for the active cell's validation:
// why its entry is invalid, the rule's help text, or the keys of a
// checkbox or dropdown.
func (m *Model) validationLine() string {
	if !m.sheet.HasRules() {
		return ""
	}
	v, ok := m.sheet.Validation(m.cur)
	if !ok {
		return ""
	}
	look := m.sheet.Look(m.cur)
	var keys string
	switch {
	case look.Checkbox:
		keys = m.th.KeyHints("Space", "check or uncheck")
	case look.Dropdown:
		keys = m.th.KeyHints(m.shortcut("data.dropdown"), "choose from the list")
	}
	var text string
	switch {
	case look.Invalid:
		text = m.th.Warning.Render("Invalid: " + v.HelpText())
	case strings.TrimSpace(v.Help) != "":
		text = m.th.Muted.Render(v.Help)
	}
	switch {
	case text == "":
		return keys
	case keys == "" || ansi.StringWidth(text)+3+ansi.StringWidth(keys) > m.width:
		return text
	}
	return text + "   " + keys
}
