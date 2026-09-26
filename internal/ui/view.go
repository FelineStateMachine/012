package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"one23/internal/sheet"
)

// active is the cell drawn as the cell pointer: the pointer while
// pointing, otherwise the active cell (which stays put while a selection is
// extended, as in Sheets).
func (m *Model) active() sheet.Addr {
	if m.mode == modePoint || m.pointing() {
		return m.point.at
	}
	return m.cur
}

// highlight is the range drawn as selected: the pointer's range while
// pointing, otherwise the selection.
func (m *Model) highlight() (sheet.Rect, bool) {
	if m.mode == modePoint || m.pointing() {
		return m.point.rect(), true
	}
	return m.selection(), m.hasRange()
}

func (m *Model) headerRow() string {
	var b strings.Builder
	b.WriteString(m.th.header.Render(strings.Repeat(" ", rowHdrW)))
	focus := m.active()
	sel, selecting := m.highlight()
	for i := range m.visibleCols(m.left) {
		c := m.left + i
		w := m.sheet.ColWidth(c)
		style := m.th.header
		switch {
		case c == focus.Col:
			style = m.th.headerActive
		case selecting && c >= sel.From.Col && c <= sel.To.Col:
			style = m.th.headerSel
		case m.hover.addr.Col == c && (m.hover.kind == hitColHeader || m.hover.kind == hitColBorder):
			style = m.th.headerHover
		}
		label := center(sheet.ColName(c), w)
		if m.showHandle(c) && w > 1 {
			// Draw the resize handle in the header's last cell.
			b.WriteString(style.Render(label[:len(label)-1]) + m.th.handle.Render("▐"))
			continue
		}
		b.WriteString(style.Render(label))
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
	focus := m.active()
	sel, selecting := m.highlight()
	hdr := m.th.header
	switch {
	case row == focus.Row:
		hdr = m.th.headerActive
	case selecting && row >= sel.From.Row && row <= sel.To.Row:
		hdr = m.th.headerSel
	case m.hover.kind == hitRowHeader && m.hover.addr.Row == row:
		hdr = m.th.headerHover
	}
	var b strings.Builder
	b.WriteString(hdr.Render(padLeft(strconv.Itoa(row+1), rowHdrW-1) + " "))

	for i, sp := range m.rowText(row) {
		a := sheet.Addr{Col: m.left + i, Row: row}
		base, colored := m.th.cell, true
		switch {
		case a == focus:
			base = m.th.pointer
		case selecting && sel.Contains(a):
			base = m.th.selection
		case m.sheet.Value(a).Kind == sheet.Error:
			base = m.th.errorCell
		default:
			colored = false
		}
		// The copy marker is layered on the cell's own colors.
		if m.copyMarked(a) {
			base, colored = base.Inherit(m.th.copied), true
		}
		if a == m.cur && (m.mode == modeEnter || m.mode == modeEdit) {
			sp = span{text: m.inCellText(m.sheet.ColWidth(a.Col))}
		}
		b.WriteString(m.renderSpan(sp, base, colored))
	}
	return b.String()
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
}

// renderSpan draws a span on base, one of the cell roles. Plain cells
// with no text style are written without escape codes.
func (m *Model) renderSpan(sp span, base lipgloss.Style, colored bool) string {
	lead, trail := strings.Repeat(" ", sp.lead), strings.Repeat(" ", sp.trail)
	st := sp.style
	st.Align = sheet.AlignAuto
	switch {
	case st.IsZero() && !colored:
		return lead + sp.text + trail
	case st.IsZero():
		return base.Render(lead + sp.text + trail)
	case !colored:
		return lead + m.th.text(base, st).Render(sp.text) + trail
	}
	return base.Render(lead) + m.th.text(base, st).Render(sp.text) + base.Render(trail)
}

// inCellText shows the entry being typed inside the cell, keeping the end
// of long entries visible, as Sheets does.
func (m *Model) inCellText(w int) string {
	text := " " + string(m.buf)
	if over := ansi.StringWidth(text) - w; over > 0 {
		text = ansi.TruncateLeft(text, over+1, "…")
	}
	return padRight(text, w)
}

// rowText lays out the visible columns of row, each span exactly its
// column's width. Values are formatted with their cell's number format
// and aligned as Sheets does: numbers right, text left, booleans and
// errors centered, unless the cell sets an alignment. Text runs on into
// blank neighbors: to the right when left-aligned, to the left when
// right-aligned, both ways when centered. It can come from cells outside
// the viewport, so the scan starts at the nearest filled cell on each
// side.
func (m *Model) rowText(row int) []span {
	ncols := m.visibleCols(m.left)
	lo, hi := m.left, m.left+ncols-1
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
	for c := lo - 1; c >= 0; c-- {
		if content(c) != nil {
			first = c
			break
		}
	}
	for c := hi + 1; c < sheet.MaxCols; c++ {
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
		for k := max(first, lo); k <= min(last, hi); k++ {
			kx0, kx1 := col(k)
			if kx1 <= from || kx0 >= to {
				continue
			}
			sp := span{trail: kx1 - kx0, style: cell.Style, owner: c}
			if seg0, seg1 := max(start, kx0, from), min(start+tw, kx1, to); seg1 > seg0 {
				sp.lead, sp.text, sp.trail = seg0-kx0, ansi.Cut(text, seg0-start, seg1-start), kx1-seg1
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

// keyLabel formats a key binding for display, e.g. "ctrl+s" -> "Ctrl+S".
func keyLabel(k string) string {
	switch k {
	case "delete":
		return "Del"
	case "backspace":
		return "Backspace"
	case "esc":
		return "Esc"
	}
	parts := strings.Split(k, "+")
	for i, p := range parts {
		switch {
		case len(p) == 1:
			parts[i] = strings.ToUpper(p)
		case p[0] == 'f' && len(p) <= 3 && p[1] >= '0' && p[1] <= '9':
			parts[i] = strings.ToUpper(p)
		default:
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "+")
}

func padRight(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

func padLeft(s string, w int) string {
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}

func center(s string, w int) string {
	pad := max(w-ansi.StringWidth(s), 0)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}
