package ui

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// active is the cell drawn as the cell pointer: the pointer while
// pointing, otherwise the active cell (which stays put while a selection is
// extended, as in Sheets).
func (m *Model) active() sheet.Addr {
	if m.mode == modePoint || m.pointing() || m.away() {
		return m.point.at
	}
	if a, ok := m.barActive(); ok {
		return a
	}
	return m.cur
}

// highlight is the range drawn as selected: the pointer's range while
// pointing, otherwise the selection.
func (m *Model) highlight() (sheet.Rect, bool) {
	switch {
	case m.mode == modePoint || m.pointing():
		return m.point.rect(), true
	case m.away(): // the selection is on the entry's sheet
		return sheet.Rect{}, false
	case m.mouse.drag == dragFill:
		return m.mouse.fillTo, true
	}
	if r, ok := m.barRange(); ok {
		return r, true
	}
	return m.selection(), m.hasRange()
}

// screenCols returns the columns drawn left to right, with divider where
// the frozen columns end.
func (m *Model) screenCols() []int {
	_, fc := m.frozen()
	out := make([]int, 0, fc+1+m.visibleCols(m.left))
	for c := range fc {
		out = append(out, c)
	}
	if fc > 0 {
		out = append(out, divider)
	}
	for i := range m.visibleCols(m.left) {
		out = append(out, m.left+i)
	}
	return out
}

func (m *Model) headerRow() string {
	var b strings.Builder
	b.WriteString(m.th.Header.Render(strings.Repeat(" ", m.hdrW())))
	focus := m.active()
	sel, selecting := m.highlight()
	for _, c := range m.screenCols() {
		if c == divider {
			b.WriteString(m.th.FrozenLine.Render("│"))
			continue
		}
		w := m.sheet.ColWidth(c)
		style, plain := m.th.Header, false
		switch {
		case c == focus.Col:
			style = m.th.HeaderActive
		case selecting && c >= sel.From.Col && c <= sel.To.Col:
			style = m.th.HeaderSel
		case m.mouse.hover.addr.Col == c && (m.mouse.hover.kind == hitColHeader || m.mouse.hover.kind == hitColBorder || m.mouse.hover.kind == hitFilterButton):
			style = m.th.HeaderHover
		default:
			plain = true
		}
		name := sheet.ColName(c)
		label := style.Render(theme.Center(name, w))
		if mark, on := m.filterMark(c); mark != "" && w >= len(name)+3 {
			// The filter's button follows the letter, e.g. "B ▾".
			text := theme.Center(name+" "+mark, w)
			k := strings.Index(text, mark)
			markStyle := style
			if on && plain {
				markStyle = m.th.FilterOn
			}
			label = style.Render(text[:k]) + markStyle.Render(mark) + style.Render(text[k+len(mark):])
		}
		if m.showHandle(c) && w > 1 {
			// Draw the resize handle in the header's last cell.
			b.WriteString(ansi.Truncate(label, w-1, "") + m.th.Handle.Render("▐"))
			continue
		}
		b.WriteString(label)
	}
	return b.String()
}

// showHandle reports whether column c's resize handle is visible: while
// hovering it or dragging it.
func (m *Model) showHandle(c int) bool {
	if m.mouse.drag == dragResize {
		return m.mouse.resizeCol == c
	}
	return m.mouse.hover.kind == hitColBorder && m.mouse.hover.addr.Col == c
}

func (m *Model) gridRow(row int) string {
	if row >= sheet.MaxRows {
		return ""
	}
	if row == divider {
		return m.dividerRow()
	}
	focus := m.active()
	sel, selecting := m.highlight()
	hdr := m.th.RowHeader
	switch {
	case row == focus.Row:
		hdr = m.th.HeaderActive
	case selecting && row >= sel.From.Row && row <= sel.To.Row:
		hdr = m.th.HeaderSel
	case m.mouse.hover.kind == hitRowHeader && m.mouse.hover.addr.Row == row:
		hdr = m.th.HeaderHover
	}
	var b strings.Builder
	b.WriteString(hdr.Render(theme.PadLeft(strconv.Itoa(row+1), m.hdrW()-1) + " "))

	_, fc := m.frozen()
	if fc > 0 {
		b.WriteString(m.cellsText(row, 0, rowtext.Layout(m.sheet, row, 0, fc, 0, fc-1), focus, sel, selecting))
		b.WriteString(m.th.FrozenLine.Render("│"))
	}
	ncols := m.visibleCols(m.left)
	b.WriteString(m.cellsText(row, m.left, rowtext.Layout(m.sheet, row, m.left, ncols, fc, sheet.MaxCols-1), focus, sel, selecting))
	return b.String()
}

// cellsText draws the spans of a row's columns from first on, in the
// roles for the pointer, the selection, search matches and errors.
func (m *Model) cellsText(row, first int, spans []rowtext.Span, focus sheet.Addr, sel sheet.Rect, selecting bool) string {
	var b strings.Builder
	rules := m.sheet.HasRules()
	var rgb func(int) color.Color
	if rules {
		rgb = m.slotColor
	}
	for i, sp := range spans {
		a := sheet.Addr{Col: first + i, Row: row}
		base, colored := m.th.Cell, true
		switch {
		case a == focus:
			base = m.th.Pointer
		case selecting && sel.Contains(a):
			base = m.th.Selection
		case m.found(a):
			base = m.th.Found
		case m.trace.covers(m.sheet, a):
			base = m.th.Traced
		case sheet.IsPending(m.sheet.Value(a)):
			base = m.th.Muted
		case m.sheet.Value(a).Kind == sheet.Error:
			base = m.th.ErrorCell
		default:
			colored = false
		}
		var look sheet.Look
		var shade theme.Shade
		shaded := false
		if a == m.cur && (m.mode == modeEnter || m.mode == modeEdit) && !m.away() {
			sp = rowtext.Span{Text: m.inCellText(m.sheet.ColWidth(a.Col))}
		} else {
			m.decorate(&sp, row) // links and error marks, see links.go
			if rules {
				look, shade, shaded = m.ruleSpan(a, &sp, colored, rgb) // looks.go
			}
		}
		if shaded {
			base, colored = shade.Style, true
		}
		// The copy marker is layered on the cell's own colors.
		if m.copied.marks(m.sheet, a) {
			base, colored, shaded = base.Inherit(m.th.Copied), true, false
		}
		var text string
		if shaded && plainSpan(sp) {
			text = shade.Wrap(strings.Repeat(" ", sp.Lead) + sp.Text + strings.Repeat(" ", sp.Trail))
		} else {
			text = renderSpan(&m.th, sp, base, colored)
		}
		w := m.sheet.ColWidth(a.Col)
		switch {
		case m.showFillHandle(a):
			text = ansi.Truncate(text, w-1, "") + base.Render("▟")
		case m.sheet.Note(a) != "":
			text = m.noteMark(text, a, base, colored)
		case look.Dropdown:
			text = m.dropdownMark(text, w, base, colored)
		}
		b.WriteString(text)
	}
	return b.String()
}

// dividerRow is the line under the frozen rows, crossing the one right of
// the frozen columns.
func (m *Model) dividerRow() string {
	var b strings.Builder
	b.WriteString(strings.Repeat("─", m.hdrW()))
	for _, c := range m.screenCols() {
		if c == divider {
			b.WriteString("┼")
			continue
		}
		b.WriteString(strings.Repeat("─", m.sheet.ColWidth(c)))
	}
	return m.th.FrozenLine.Render(ansi.Truncate(b.String(), m.width, ""))
}

// plainSpan reports whether a span's text has no style of its own: no
// text style, link or mark.
func plainSpan(sp rowtext.Span) bool {
	st := sp.Style
	st.Align = sheet.AlignAuto
	return st.IsZero() && sp.Link == "" && !sp.Error && !sp.Invalid
}

// renderSpan draws a span on base, one of the cell roles. Plain cells
// with no text style are written without escape codes.
func renderSpan(th *theme.Theme, sp rowtext.Span, base lipgloss.Style, colored bool) string {
	lead, trail := strings.Repeat(" ", sp.Lead), strings.Repeat(" ", sp.Trail)
	plain := plainSpan(sp)
	switch {
	case plain && !colored:
		return lead + sp.Text + trail
	case plain:
		return base.Render(lead + sp.Text + trail)
	case !colored:
		return lead + spanStyle(th, base, sp).Render(sp.Text) + trail
	}
	return base.Render(lead) + spanStyle(th, base, sp).Render(sp.Text) + base.Render(trail)
}

// inCellText shows the entry being typed inside the cell, keeping the end
// of long entries visible, as Sheets does.
func (m *Model) inCellText(w int) string {
	text := " " + m.line.Text()
	if over := ansi.StringWidth(text) - w; over > 0 {
		text = ansi.TruncateLeft(text, over+1, "…")
	}
	return theme.PadRight(text, w)
}
