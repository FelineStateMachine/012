package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Drawing rows that wrap, draw borders or merge cells. Borders are
// drawn with box-drawing characters where a terminal can draw lines: a
// vertical edge in the first column of the cell to its right, which is
// padding, and a horizontal one on a rule line of its own above the row
// it tops, where the lines cross in the joints the font has (see
// theme.Junction). A terminal has no thinner lines between character
// rows, and an underline can't draw a thick or a top edge nor meet the
// vertical lines, so a bordered table takes a line per border, as tables
// printed in a terminal do. Merged cells draw their top-left cell's
// value across the merge, and no borders inside it.

// lineCtx is what drawing a line of cells needs, worked out once per
// line.
type lineCtx struct {
	row       int
	ln        rowtext.Line
	focus     sheet.Addr
	sel       sheet.Rect
	selecting bool
	shaped    bool         // the sheet wraps, draws borders or merges
	merges    []sheet.Rect // merges crossing the row
}

func (m *Model) lineContext(row int, ln rowtext.Line) lineCtx {
	lc := lineCtx{row: row, ln: ln, focus: m.active(), shaped: m.sheet.Shaped()}
	lc.sel, lc.selecting = m.highlight()
	if lc.shaped {
		lc.merges = m.sheet.MergesIn(sheet.Rect{From: sheet.Addr{Row: row}, To: sheet.Addr{Col: sheet.MaxCols - 1, Row: row}})
	}
	return lc
}

// bottom reports whether the line is the row's last, where its values
// sit.
func (lc *lineCtx) bottom() bool { return lc.ln.K == lc.ln.N-1 }

// mergeOf returns the merge on the row holding column c.
func (lc *lineCtx) mergeOf(c int) (sheet.Rect, bool) {
	for _, mg := range lc.merges {
		if c >= mg.From.Col && c <= mg.To.Col {
			return mg, true
		}
	}
	return sheet.Rect{}, false
}

// merged reports whether a is in the merge whose top-left cell is focus.
func (lc *lineCtx) merged(a, focus sheet.Addr) bool {
	if len(lc.merges) == 0 {
		return false
	}
	mg, ok := lc.mergeOf(a.Col)
	return ok && mg.From == focus
}

// layoutLine lays out ncols columns of the line from lo, with the merges
// crossing them drawn over.
func (m *Model) layoutLine(lc *lineCtx, lo, ncols, minCol, maxCol int) []rowtext.Span {
	spans := rowtext.LayoutLine(m.sheet, lc.row, lo, ncols, minCol, maxCol, lc.ln)
	hi := lo + ncols - 1
	for _, mg := range lc.merges {
		from, to := max(lo, mg.From.Col), min(hi, mg.To.Col)
		if from > to {
			continue
		}
		row, k := m.mergeText(mg)
		copy(spans[from-lo:], rowtext.Merged(m.sheet, mg, from, to, row == lc.row && k == lc.ln.K))
	}
	return spans
}

// mergeText returns the line that shows a merge's value: the middle line
// of its middle row among those shown.
func (m *Model) mergeText(mg sheet.Rect) (row, k int) {
	row = (mg.From.Row + mg.To.Row) / 2
	if m.sheet.HiddenRows() > 0 && mg.To.Row-mg.From.Row < maxMergeWalk {
		var shown []int
		for r := mg.From.Row; r <= mg.To.Row; r++ {
			if !m.sheet.RowHidden(r) {
				shown = append(shown, r)
			}
		}
		if len(shown) > 0 {
			row = shown[(len(shown)-1)/2]
		}
	}
	return row, (m.shape(row).Lines - 1) / 2
}

// maxMergeWalk is the tallest merge whose shown rows are counted to find
// its middle; a taller one takes the middle of all its rows.
const maxMergeWalk = 10000

// appendBand appends the lines of a band to lines.
func (m *Model) appendBand(lines []string, b band) []string {
	switch {
	case b.row == divider:
		return append(lines, m.dividerRow())
	case b.row >= sheet.MaxRows:
		return lines
	case b.rule:
		lines = append(lines, m.ruleLine(b.row))
	}
	if b.lines == 1 {
		return append(lines, m.rowLine(b.row, rowtext.Line{N: 1}))
	}
	for k := range b.shown {
		lines = append(lines, m.rowLine(b.row, rowtext.Line{K: k, N: b.lines}))
	}
	return lines
}

// hEdge is the line drawn along the top of the cell at a: none inside a
// merge.
func (m *Model) hEdge(a sheet.Addr) sheet.Line {
	if a.Col < 0 || a.Row < 0 {
		return sheet.LineNone
	}
	if mg, ok := m.sheet.MergeAt(a); ok && a.Row > mg.From.Row {
		return sheet.LineNone
	}
	return m.sheet.EdgeAbove(a)
}

// vEdge is the line drawn along the left of the cell at a: none inside
// a merge.
func (m *Model) vEdge(a sheet.Addr) sheet.Line {
	if a.Col < 0 || a.Row < 0 {
		return sheet.LineNone
	}
	if mg, ok := m.sheet.MergeAt(a); ok && a.Col > mg.From.Col {
		return sheet.LineNone
	}
	return m.sheet.EdgeLeft(a)
}

// leftEdge draws the line along the left of the cell at a over the
// first column of its text, in the cell's colors when it has some.
func (m *Model) leftEdge(lc *lineCtx, a sheet.Addr, text string, base lipgloss.Style, colored bool) string {
	e := m.vEdge(a)
	if e == sheet.LineNone {
		return text
	}
	style := m.th.CellBorder
	if colored {
		style = base
	}
	return style.Render(theme.Junction(e, e, sheet.LineNone, sheet.LineNone)) + ansi.Cut(text, 1, m.sheet.ColWidth(a.Col))
}

// ruleLine draws the border line above row: each column's top edge,
// joined where edges meet, and blank inside merges.
func (m *Model) ruleLine(row int) string {
	above := m.stepRow(row, -1)
	if above == row {
		above = -1
	}
	lc := m.lineContext(row, rowtext.Line{N: 1})
	var b strings.Builder
	b.WriteString(m.th.RowHeader.Render(strings.Repeat(" ", m.hdrW())))
	_, fc := m.frozen()
	if fc > 0 {
		m.ruleCols(&b, &lc, above, 0, fc)
		b.WriteString(m.th.FrozenLine.Render("│"))
	}
	m.ruleCols(&b, &lc, above, m.left, m.visibleCols(m.left))
	return b.String()
}

// ruleCols draws the rule line of ncols columns from first.
func (m *Model) ruleCols(b *strings.Builder, lc *lineCtx, above, first, ncols int) {
	for c := first; c < first+ncols; c++ {
		a := sheet.Addr{Col: c, Row: lc.row}
		w := m.sheet.ColWidth(c)
		style, colored := m.th.CellBorder, false
		if lc.selecting && lc.sel.Contains(a) && lc.sel.Contains(sheet.Addr{Col: c, Row: above}) {
			style, colored = m.th.Selection, true
		}
		mg, inMerge := lc.mergeOf(c)
		inside := inMerge && lc.row > mg.From.Row
		if inside && mg.From == lc.focus {
			style, colored = m.th.Pointer, true
		}
		h := m.hEdge(a)
		left := sheet.LineNone
		if c > first {
			left = m.hEdge(sheet.Addr{Col: c - 1, Row: lc.row})
		}
		joint := theme.Junction(m.vEdge(sheet.Addr{Col: c, Row: above}), m.vEdge(a), left, h)
		if inside && c > mg.From.Col {
			joint = " "
		}
		run := strings.Repeat(theme.Junction(sheet.LineNone, sheet.LineNone, h, h), w-1)
		if !colored && joint == " " && h == sheet.LineNone {
			b.WriteString(strings.Repeat(" ", w))
			continue
		}
		b.WriteString(style.Render(joint + run))
	}
}

// showRowHandle reports whether row's resize handle is visible: while
// hovering it or dragging it.
func (m *Model) showRowHandle(row int) bool {
	if m.mouse.drag == dragRowResize {
		return m.mouse.resizeRow == row
	}
	return m.mouse.hover.kind == hitRowBorder && m.mouse.hover.addr.Row == row
}
