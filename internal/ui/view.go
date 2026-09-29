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
	focus := m.sheet.Grow(sheet.Rect{From: m.active(), To: m.active()}) // a merged cell's columns
	sel, selecting := m.highlight()
	for _, c := range m.screenCols() {
		if c == divider {
			b.WriteString(m.th.FrozenLine.Render("│"))
			continue
		}
		w := m.sheet.ColWidth(c)
		style, plain := m.th.Header, false
		switch {
		case c >= focus.From.Col && c <= focus.To.Col:
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

// gridRow draws a row one line tall.
func (m *Model) gridRow(row int) string {
	if row >= sheet.MaxRows {
		return ""
	}
	if row == divider {
		return m.dividerRow()
	}
	return m.rowLine(row, rowtext.Line{N: 1})
}

// rowLine draws line ln of a row: its number on the last line, beside
// its values, so a tall row ends on the line that numbers it, then its
// cells.
func (m *Model) rowLine(row int, ln rowtext.Line) string {
	lc := m.lineContext(row, ln)
	hdr := m.th.RowHeader
	switch {
	case row == lc.focus.Row || len(lc.merges) > 0 && lc.merged(sheet.Addr{Col: lc.focus.Col, Row: row}, lc.focus):
		hdr = m.th.HeaderActive
	case lc.selecting && row >= lc.sel.From.Row && row <= lc.sel.To.Row:
		hdr = m.th.HeaderSel
	case m.mouse.hover.kind == hitRowHeader && m.mouse.hover.addr.Row == row:
		hdr = m.th.HeaderHover
	}
	var b strings.Builder
	num := ""
	if ln.K == ln.N-1 {
		num = strconv.Itoa(row + 1)
	}
	if m.showRowHandle(row) && ln.K == ln.N-1 {
		b.WriteString(hdr.Render(theme.PadLeft(num, m.hdrW()-1)) + m.th.Handle.Render("▄"))
	} else {
		b.WriteString(hdr.Render(theme.PadLeft(num, m.hdrW()-1) + " "))
	}

	_, fc := m.frozen()
	if fc > 0 {
		b.WriteString(m.cellsText(&lc, 0, m.layoutLine(&lc, 0, fc, 0, fc-1)))
		b.WriteString(m.th.FrozenLine.Render("│"))
	}
	ncols := m.visibleCols(m.left)
	b.WriteString(m.cellsText(&lc, m.left, m.layoutLine(&lc, m.left, ncols, fc, sheet.MaxCols-1)))
	return b.String()
}

// cellsText draws the spans of a line of cells from column first on, in
// the roles for the pointer, the selection, search matches and errors.
func (m *Model) cellsText(lc *lineCtx, first int, spans []rowtext.Span) string {
	row := lc.row
	var b strings.Builder
	spills := m.sheet.HasSpills()
	rules := m.sheet.HasRules()
	var rgb func(int) color.Color
	if rules {
		rgb = m.slotColor
	}
	// A shade is a whole role, so it is kept across the loop and read
	// through a pointer rather than zeroed and copied for every cell.
	var shade theme.Shade
	for i, sp := range spans {
		a := sheet.Addr{Col: first + i, Row: row}
		base, colored := m.cellRole(lc, a, &sp, spills)
		var look sheet.Look
		shaded := false
		if piece, typing := m.entryPiece(lc, a); typing { // gridlines.go
			sp = rowtext.Span{Text: piece}
		} else {
			m.decorate(&sp, row) // links and error marks, see links.go
			if rules {
				look, shade, shaded = m.ruleSpan(a, &sp, colored, rgb) // looks.go
				if look.Checkbox && !m.valueLine(lc, a) {
					sp = rowtext.Span{Trail: m.sheet.ColWidth(a.Col)}
				}
			}
		}
		if shaded {
			base, colored = &shade.Style, true
		}
		// The copy marker is layered on the cell's own colors.
		if lc.bottom() && m.copied.marks(m.sheet, a) {
			marked := theme.Drawable(base.Inherit(m.th.Copied))
			base, colored, shaded = &marked, true, false
		}
		text := m.cellText(lc, a, sp, &look, base, colored, &shade, shaded)
		b.WriteString(m.cellMarks(lc, a, text, &look, base, colored))
	}
	return b.String()
}

// cellText draws a cell's span on its role: as a rule's shade, with its
// data bar or icon, as a dropdown's chip (taking the look's ▾, which the
// chip holds), or plainly.
func (m *Model) cellText(lc *lineCtx, a sheet.Addr, sp rowtext.Span, look *sheet.Look, base *lipgloss.Style, colored bool, shade *theme.Shade, shaded bool) string {
	switch {
	case look.Bar || look.Icon != "" || look.ValueHidden:
		return m.barText(sp, m.sheet.ColWidth(a.Col), look, base, colored, m.valueLine(lc, a))
	case look.Dropdown && look.Display != sheet.DropArrow:
		chip, ok := "", false
		if look.Display == sheet.DropChip && !colored && sp.Owner == a.Col && m.valueLine(lc, a) {
			chip, ok = m.chipText(sp, m.sheet.ColWidth(a.Col))
		}
		if ok || look.Display == sheet.DropPlain {
			look.Dropdown = false // no ▾ at the right
		}
		if ok {
			return chip
		}
	case shaded && plainSpan(sp):
		return shade.Wrap(strings.Repeat(" ", sp.Lead) + sp.Text + strings.Repeat(" ", sp.Trail))
	}
	return renderSpan(&m.th, sp, base, colored)
}

// cellRole is the role a cell is drawn in: the pointer, the selection,
// a search match, a trace, an error or a spill, or the plain cell role
// (colored false). Roles are large, so it points at the theme's.
func (m *Model) cellRole(lc *lineCtx, a sheet.Addr, sp *rowtext.Span, spills bool) (*lipgloss.Style, bool) {
	switch {
	case a == lc.focus || lc.merges != nil && lc.merged(a, lc.focus):
		return &m.th.Pointer, true
	case lc.selecting && lc.sel.Contains(a):
		return &m.th.Selection, true
	case m.found(a):
		return &m.th.Found, true
	case m.trace.covers(m.sheet, a):
		return &m.th.Traced, true
	}
	if lc.labels != nil && (lc.labels.Contains(a) || sp.Text != "" && lc.labels.Contains(sheet.Addr{Col: sp.Owner, Row: lc.row})) {
		return &m.th.Region, true
	}
	switch v := m.sheet.Value(a); {
	case sheet.IsPending(v):
		return &m.th.Muted, true
	case v.Kind == sheet.Error:
		return &m.th.ErrorCell, true
	case spills && sp.Text != "" && m.sheet.Cell(sheet.Addr{Col: sp.Owner, Row: lc.row}).Spilled():
		return &m.th.Spilled, true
	}
	return &m.th.Cell, false
}

// cellMarks draws a cell's marks over its text: the fill handle, a
// note's corner or a dropdown's ▾, and the border along its left.
func (m *Model) cellMarks(lc *lineCtx, a sheet.Addr, text string, look *sheet.Look, base *lipgloss.Style, colored bool) string {
	w := m.sheet.ColWidth(a.Col)
	switch {
	case lc.bottom() && m.showFillHandle(a):
		text = ansi.Truncate(text, w-1, "") + base.Render("▟")
	case lc.ln.K == 0 && m.sheet.Note(a) != "":
		text = m.noteMark(text, a, *base, colored)
	case look.Dropdown && m.valueLine(lc, a):
		text = m.dropdownMark(text, w, *base, colored)
	}
	if lc.shaped {
		text = m.leftEdge(a, text)
	}
	return text
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
// Roles are large, so base is read through a pointer.
func renderSpan(th *theme.Theme, sp rowtext.Span, base *lipgloss.Style, colored bool) string {
	lead, trail := strings.Repeat(" ", sp.Lead), strings.Repeat(" ", sp.Trail)
	plain := plainSpan(sp)
	switch {
	case plain && !colored:
		return lead + sp.Text + trail
	case plain:
		return base.Render(lead + sp.Text + trail)
	case !colored:
		return lead + spanStyle(th, *base, sp).Render(sp.Text) + trail
	}
	return base.Render(lead) + spanStyle(th, *base, sp).Render(sp.Text) + base.Render(trail)
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
