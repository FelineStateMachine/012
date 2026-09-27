package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
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
	case m.drag == dragFill:
		return m.fillTo, true
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
	b.WriteString(m.th.Header.Render(strings.Repeat(" ", rowHdrW)))
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
		case m.hover.addr.Col == c && (m.hover.kind == hitColHeader || m.hover.kind == hitColBorder || m.hover.kind == hitFilterButton):
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
	if m.drag == dragResize {
		return m.resizeCol == c
	}
	return m.hover.kind == hitColBorder && m.hover.addr.Col == c
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
	hdr := m.th.Header
	switch {
	case row == focus.Row:
		hdr = m.th.HeaderActive
	case selecting && row >= sel.From.Row && row <= sel.To.Row:
		hdr = m.th.HeaderSel
	case m.hover.kind == hitRowHeader && m.hover.addr.Row == row:
		hdr = m.th.HeaderHover
	}
	var b strings.Builder
	b.WriteString(hdr.Render(theme.PadLeft(strconv.Itoa(row+1), rowHdrW-1) + " "))

	_, fc := m.frozen()
	if fc > 0 {
		b.WriteString(m.cellsText(row, 0, m.rowText(row, 0, fc, 0, fc-1), focus, sel, selecting))
		b.WriteString(m.th.FrozenLine.Render("│"))
	}
	ncols := m.visibleCols(m.left)
	b.WriteString(m.cellsText(row, m.left, m.rowText(row, m.left, ncols, fc, sheet.MaxCols-1), focus, sel, selecting))
	return b.String()
}

// cellsText draws the spans of a row's columns from first on, in the
// roles for the pointer, the selection, search matches and errors.
func (m *Model) cellsText(row, first int, spans []span, focus sheet.Addr, sel sheet.Rect, selecting bool) string {
	var b strings.Builder
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
		case m.traced(a):
			base = m.th.Traced
		case sheet.IsPending(m.sheet.Value(a)):
			base = m.th.Muted
		case m.sheet.Value(a).Kind == sheet.Error:
			base = m.th.ErrorCell
		default:
			colored = false
		}
		// The copy marker is layered on the cell's own colors.
		if m.copyMarked(a) {
			base, colored = base.Inherit(m.th.Copied), true
		}
		if a == m.cur && (m.mode == modeEnter || m.mode == modeEdit) && !m.away() {
			sp = span{text: m.inCellText(m.sheet.ColWidth(a.Col))}
		} else {
			m.decorate(&sp, row) // links and error marks, see links.go
		}
		text := m.renderSpan(sp, base, colored)
		if m.showFillHandle(a) {
			w := m.sheet.ColWidth(a.Col)
			text = ansi.Truncate(text, w-1, "") + base.Render("▟")
		}
		b.WriteString(text)
	}
	return b.String()
}

// dividerRow is the line under the frozen rows, crossing the one right of
// the frozen columns.
func (m *Model) dividerRow() string {
	var b strings.Builder
	b.WriteString(strings.Repeat("─", rowHdrW))
	for _, c := range m.screenCols() {
		if c == divider {
			b.WriteString("┼")
			continue
		}
		b.WriteString(strings.Repeat("─", m.sheet.ColWidth(c)))
	}
	return m.th.FrozenLine.Render(ansi.Truncate(b.String(), m.width, ""))
}

// span is what one grid column shows in a row: blank columns, the text,
// blank columns. Only the text takes the owning cell's text style, so an
// underline doesn't run into the padding.
type span struct {
	lead  int
	text  string
	trail int
	style sheet.Style
	owner int // column of the cell the text belongs to

	link  string // the owner's link target, drawn as a hyperlink
	error bool   // the owner shows an error: its text gets the error mark
}

// renderSpan draws a span on base, one of the cell roles. Plain cells
// with no text style are written without escape codes.
func (m *Model) renderSpan(sp span, base lipgloss.Style, colored bool) string {
	lead, trail := strings.Repeat(" ", sp.lead), strings.Repeat(" ", sp.trail)
	st := sp.style
	st.Align = sheet.AlignAuto
	plain := st.IsZero() && sp.link == "" && !sp.error
	switch {
	case plain && !colored:
		return lead + sp.text + trail
	case plain:
		return base.Render(lead + sp.text + trail)
	case !colored:
		return lead + m.textStyle(base, sp).Render(sp.text) + trail
	}
	return base.Render(lead) + m.textStyle(base, sp).Render(sp.text) + base.Render(trail)
}

// inCellText shows the entry being typed inside the cell, keeping the end
// of long entries visible, as Sheets does.
func (m *Model) inCellText(w int) string {
	text := " " + string(m.buf)
	if over := ansi.StringWidth(text) - w; over > 0 {
		text = ansi.TruncateLeft(text, over+1, "…")
	}
	return theme.PadRight(text, w)
}

// rowText lays out ncols columns of row from lo, each span exactly its
// column's width. Text may run in from columns between minCol and maxCol,
// so it stops at the frozen columns' divider. Values are formatted with their cell's number format
// and aligned as Sheets does: numbers right, text left, booleans and
// errors centered, unless the cell sets an alignment. Text runs on into
// blank neighbors: to the right when left-aligned, to the left when
// right-aligned, both ways when centered. It can come from cells outside
// the viewport, so the scan starts at the nearest filled cell on each
// side.
func (m *Model) rowText(row, lo, ncols, minCol, maxCol int) []span {
	hi := lo + ncols - 1
	out := make([]span, ncols)
	for i := range out {
		out[i] = span{trail: m.sheet.ColWidth(lo + i)}
	}
	content := func(c int) *sheet.Cell {
		if c := m.sheet.Cell(sheet.Addr{Col: c, Row: row}); !c.Blank() {
			return c
		}
		return nil
	}
	first, last := lo, hi
	for c := lo - 1; c >= minCol; c-- {
		if content(c) != nil {
			first = c
			break
		}
	}
	for c := hi + 1; c <= maxCol; c++ {
		if content(c) != nil {
			last = c
			break
		}
	}
	// x[k] is where column first+k starts, relative to column first.
	x := make([]int, last-first+2)
	for c := first; c <= last; c++ {
		x[c-first+1] = x[c-first] + m.sheet.ColWidth(c)
	}
	col := func(c int) (int, int) { return x[c-first], x[c-first+1] }

	claimed := 0 // text of earlier cells reaches up to here
	for c := first; c <= last; c++ {
		cell := content(c)
		if cell == nil {
			continue
		}
		x0, x1 := col(c)
		f := m.sheet.DisplayFormat(sheet.Addr{Col: c, Row: row})
		text, align := sheet.Display(cell.Value, f, x1-x0)
		pad := 1
		// A number one character too wide (12/31/2026 in a default
		// column) may use the padding when nothing is to its right,
		// rather than turning into #s.
		if cell.Value.Kind == sheet.Number && strings.Trim(text, "#") == "" && content(c+1) == nil {
			if wider, _ := sheet.Display(cell.Value, f, x1-x0+1); strings.Trim(wider, "#") != "" {
				text, pad = wider, 0
			}
		}
		if a := cell.Style.Align; a != sheet.AlignAuto && align != sheet.AlignFill {
			align = a
		}
		tw := ansi.StringWidth(text)
		start := x0 + pad
		switch align {
		case sheet.AlignFill:
			start = x0
		case sheet.AlignRight:
			start = x1 - pad - tw
		case sheet.AlignCenter:
			start = x0 + (x1-x0-tw)/2
		}
		from, to := max(x0, claimed), x1 // where this cell's text may go
		if cell.Value.Kind == sheet.Text {
			for k := c + 1; k <= last && start+tw > to && content(k) == nil; k++ {
				_, to = col(k)
			}
			for k := c - 1; k >= first && start < from && content(k) == nil; k-- {
				if kx0, _ := col(k); kx0 >= claimed {
					from = kx0
				} else {
					from = claimed
					break
				}
			}
		}
		claimed = to
		cut := textCutter{s: text}
		for k := max(first, lo); k <= min(last, hi); k++ {
			kx0, kx1 := col(k)
			if kx1 <= from || kx0 >= to {
				continue
			}
			sp := span{trail: kx1 - kx0, style: cell.Style, owner: c}
			if seg0, seg1 := max(start, kx0, from), min(start+tw, kx1, to); seg1 > seg0 {
				sp.lead, sp.text, sp.trail = seg0-kx0, cut.cut(seg0-start, seg1-start), kx1-seg1
			}
			out[k-lo] = sp
		}
	}
	// Text that runs to the edge of its column would touch a neighbor
	// that starts at its own edge ("Groceries9/28/2026"); keep a gap.
	for i := 0; i+1 < len(out); i++ {
		l, r := &out[i], &out[i+1]
		if l.text != "" && l.trail == 0 && r.text != "" && r.lead == 0 && l.owner != r.owner {
			l.text = ansi.Truncate(l.text, ansi.StringWidth(l.text)-1, "")
			l.trail = 1
		}
	}
	return out
}

// textCutter cuts successive column ranges, left to right, out of one
// line of text in a single pass, as ansi.Cut would one at a time: a
// cluster is in [l, r) when its right edge is past l and not past r.
// Cutting each column's piece with ansi.Cut rescanned the text from its
// start, so a screen of 500-character text took 40 ms to draw.
type textCutter struct {
	s     string
	i     int // byte offset of the next cluster
	right int // right edge, in columns, of the text before s[i:]
}

func (c *textCutter) cut(l, r int) string {
	for c.i < len(c.s) {
		n, w := nextCluster(c.s[c.i:])
		if c.right+w > l {
			break
		}
		c.i, c.right = c.i+n, c.right+w
	}
	from := c.i
	for c.i < len(c.s) {
		n, w := nextCluster(c.s[c.i:])
		if c.right+w > r {
			break
		}
		c.i, c.right = c.i+n, c.right+w
	}
	return c.s[from:c.i]
}

// nextCluster returns the length in bytes and width in columns of the
// grapheme cluster s starts with, measured as ansi.Cut measures it.
func nextCluster(s string) (n, w int) {
	switch b := s[0]; {
	case b >= 0x20 && b < 0x7f:
		return 1, 1
	case b < 0x80:
		return 1, 0
	}
	g, w := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
	return len(g), w
}
