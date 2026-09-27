package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Cells whose text is a URL, and HYPERLINK formulas, are drawn as OSC 8
// hyperlinks: blue and underlined, and Cmd- or Ctrl-clickable in
// terminals that support them. Error values get a curly underline on top
// of their color, and the context line explains the active cell's error.

// decorate marks a span's text as a link or an error, from the cell the
// text belongs to.
func (g *grid) decorate(sp *rowtext.Span, row int) {
	if sp.Text == "" {
		return
	}
	a := sheet.Addr{Col: sp.Owner, Row: row}
	switch v := g.sheet.Value(a); {
	case v.Kind == sheet.Error && !sheet.IsPending(v):
		sp.Error = true
	case v.Kind == sheet.Text:
		sp.Link = g.sheet.Link(a)
	}
}

// spanStyle is the style of a span's text on base: the cell's text style,
// then the link or error mark. On a colored role (the pointer, the
// selection) a link keeps the role's colors and adds its underline.
func spanStyle(th *theme.Theme, base lipgloss.Style, sp rowtext.Span) lipgloss.Style {
	st := sp.Style
	st.Align = sheet.AlignAuto
	s := th.Text(base, st)
	switch {
	case sp.Link != "":
		s = s.Inherit(th.Link).Hyperlink(sp.Link)
	case sp.Error:
		s = s.Inherit(th.ErrorMark)
	case sp.Invalid:
		s = s.Inherit(th.Invalid)
	}
	return s
}

// errorLine explains the active cell's error on the context line, e.g.
// "#DIV/0!  Division by zero in B3/0".
func (m *Model) errorLine() string {
	v := m.sheet.Value(m.cur)
	why := m.sheet.ExplainError(m.cur)
	if v.Kind != sheet.Error || why == "" {
		return ""
	}
	return m.th.ErrorCell.Render(v.Str) + "  " + m.th.Muted.Render(why)
}
