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

// bottom reports whether the line is the row's last, where values sit
// unless aligned otherwise, and the marks of a cell's box are drawn.
func (lc *lineCtx) bottom() bool { return lc.ln.K == lc.ln.N-1 }

// valueLine reports whether the line is the one the value of the cell
// at a sits on: the row's last, first or middle, as its vertical
// alignment says.
func (m *Model) valueLine(lc *lineCtx, a sheet.Addr) bool {
	return lc.ln.N == 1 || lc.ln.K == m.sheet.CellStyle(a).VAlign.Offset(1, lc.ln.N, false)
}

// entryPiece is the part of the entry being typed that the cell at a
// shows on this line, if any: all of it in the active cell, on the line
// its value sits on, or the piece of a merged active cell's columns
// that falls in a's, on the line that shows the merge's value, so the
// entry is as wide as the merge.
func (m *Model) entryPiece(lc *lineCtx, a sheet.Addr) (string, bool) {
	if m.mode != modeEnter && m.mode != modeEdit || a.Row != m.cur.Row && lc.merges == nil || m.away() {
		return "", false
	}
	if mg, ok := lc.mergeOf(a.Col); ok && mg.From == m.cur {
		if row, k := m.mergeText(mg); row != lc.row || k != lc.ln.K {
			return "", false
		}
		x0 := m.sheet.ColsWidth(mg.From.Col, a.Col-1)
		text := m.inCellText(m.sheet.ColsWidth(mg.From.Col, mg.To.Col))
		return ansi.Cut(text, x0, x0+m.sheet.ColWidth(a.Col)), true
	}
	if a != m.cur || !m.valueLine(lc, a) {
		return "", false
	}
	return m.inCellText(m.sheet.ColWidth(a.Col)), true
}

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
// crossing them drawn over. The spans are the model's scratch, good
// until the next line is laid out.
func (m *Model) layoutLine(lc *lineCtx, lo, ncols, minCol, maxCol int) []rowtext.Span {
	spans := m.rowScratch.LayoutLine(m.sheet, lc.row, lo, ncols, minCol, maxCol, lc.ln)
	hi := lo + ncols - 1
	for _, mg := range lc.merges {
		from, to := max(lo, mg.From.Col), min(hi, mg.To.Col)
		if from > to {
			continue
		}
		row, k := m.mergeText(mg)
		// While its value is being typed, the entry shows in its place.
		typing := (m.mode == modeEnter || m.mode == modeEdit) && !m.away() && mg.From == m.cur
		copy(spans[from-lo:], rowtext.Merged(m.sheet, mg, from, to, row == lc.row && k == lc.ln.K && !typing))
	}
	return spans
}

// mergeText returns the line that shows a merge's value: among the lines
// its shown rows take on screen, rule lines between them included, the
// middle one (or the top or bottom one, as its top-left cell's vertical
// alignment says), or the nearest line of text above it. Each merge's is
// worked out once a frame.
func (m *Model) mergeText(mg sheet.Rect) (row, k int) {
	if at, ok := m.mergeLines[mg]; ok {
		return at[0], at[1]
	}
	row, k = m.mergeLine(mg)
	if m.mergeLines == nil {
		m.mergeLines = map[sheet.Rect][2]int{}
	}
	m.mergeLines[mg] = [2]int{row, k}
	return row, k
}

func (m *Model) mergeLine(mg sheet.Rect) (row, k int) {
	va := m.sheet.CellStyle(mg.From).VAlign
	if mg.To.Row-mg.From.Row >= maxMergeWalk {
		row = (mg.From.Row + mg.To.Row) / 2
		return row, va.Offset(1, m.shape(row).Lines, true)
	}
	type spot struct{ row, k, at int }
	var spots []spot
	at := 0 // lines so far
	for r := mg.From.Row; r <= mg.To.Row; r++ {
		if m.sheet.RowHidden(r) {
			continue
		}
		sh := m.shape(r)
		if sh.Rule && len(spots) > 0 {
			at++
		}
		for k := range sh.Lines {
			spots = append(spots, spot{r, k, at})
			at++
		}
	}
	if len(spots) == 0 {
		return mg.From.Row, 0
	}
	target, best := va.Offset(1, at, true), spots[0]
	for _, s := range spots[1:] {
		if s.at <= target {
			best = s
		}
	}
	return best.row, best.k
}

// maxMergeWalk is the tallest merge whose lines are counted to find its
// middle; a taller one takes the middle of all its rows.
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

// hEdge is the line drawn along the top of the cell at a, with its
// color: none inside a merge.
func (m *Model) hEdge(a sheet.Addr) sheet.Stroke {
	if a.Col < 0 || a.Row < 0 {
		return sheet.Stroke{}
	}
	if mg, ok := m.sheet.MergeAt(a); ok && a.Row > mg.From.Row {
		return sheet.Stroke{}
	}
	return m.sheet.StrokeAbove(a)
}

// vEdge is the line drawn along the left of the cell at a, with its
// color: none inside a merge.
func (m *Model) vEdge(a sheet.Addr) sheet.Stroke {
	if a.Col < 0 || a.Row < 0 {
		return sheet.Stroke{}
	}
	if mg, ok := m.sheet.MergeAt(a); ok && a.Col > mg.From.Col {
		return sheet.Stroke{}
	}
	return m.sheet.StrokeLeft(a)
}

// borderRole is the role a border line of color c is drawn in: the
// text's ink, or its color as a text color of the rules draws it, from
// the terminal's palette.
func (m *Model) borderRole(c sheet.Color) *lipgloss.Style {
	if c == sheet.ColorNone || int(c) >= sheet.NumColors {
		return &m.th.CellBorder
	}
	return &m.th.RuleText[c]
}

// leftEdge draws the line along the left of the cell at a over the
// first column of its text, in the border's role even in a highlighted
// cell, so the table's lines run on through the pointer and selection.
func (m *Model) leftEdge(a sheet.Addr, text string) string {
	st := m.vEdge(a)
	e := st.Line
	if e == sheet.LineNone {
		return text
	}
	glyph := m.paint(m.borderRole(st.Color), theme.Junction(e, e, sheet.LineNone, sheet.LineNone))
	if strings.HasPrefix(text, " ") { // padding, as it mostly is
		return glyph + text[1:]
	}
	return glyph + ansi.Cut(text, 1, m.sheet.ColWidth(a.Col))
}

// paintKey is a text drawn in a role.
type paintKey struct {
	role *lipgloss.Style
	text string
}

// paint draws text in role, as role.Render does, keeping what it drew
// until the next frame: borders draw the same few strings over and over.
func (m *Model) paint(role *lipgloss.Style, text string) string {
	if s, ok := m.painted[paintKey{role, text}]; ok {
		return s
	}
	if m.painted == nil {
		m.painted = map[paintKey]string{}
	}
	s := role.Render(text)
	m.painted[paintKey{role, text}] = s
	return s
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

// ruleJoint is where the edges at the top-left corner of a cell meet on
// a rule line: its character and color, and whether the double lines to
// its left and right start thick beside it (see theme.Bridges).
type ruleJoint struct {
	glyph               string
	color               sheet.Color
	bridgeL, bridgeR    bool
	hidden              bool // inside a merge, where no lines are drawn
	up, down, left, top sheet.Stroke
}

// ruleJointAt works out the joint at the top-left of the cell at column
// c of lc's row, left being the edge along the top of the column before.
func (m *Model) ruleJointAt(lc *lineCtx, above, c int, left sheet.Stroke) ruleJoint {
	if c >= sheet.MaxCols {
		return ruleJoint{glyph: " "}
	}
	a := sheet.Addr{Col: c, Row: lc.row}
	j := ruleJoint{up: m.vEdge(sheet.Addr{Col: c, Row: above}), down: m.vEdge(a), left: left, top: m.hEdge(a)}
	j.glyph = theme.Junction(j.up.Line, j.down.Line, j.left.Line, j.top.Line)
	j.bridgeL, j.bridgeR = theme.Bridges(j.up.Line, j.down.Line, j.left.Line, j.top.Line)
	// The joint takes the color of the heaviest line it draws: a thick
	// one where it draws double lines thick (see theme.Junction).
	arms := [...]sheet.Stroke{j.up, j.down, j.left, j.top}
	weight := func(l sheet.Line) int {
		if l == sheet.LineDouble && theme.Mixed(j.up.Line, j.down.Line, j.left.Line, j.top.Line) {
			return int(sheet.LineThin)
		}
		return int(l)
	}
	heaviest := sheet.Stroke{}
	for _, st := range arms {
		if weight(st.Line) > weight(heaviest.Line) {
			heaviest = st
		}
	}
	j.color = heaviest.Color
	if mg, ok := lc.mergeOf(c); ok && lc.row > mg.From.Row && c > mg.From.Col {
		j.hidden, j.glyph = true, " "
	}
	return j
}

// ruleCols draws the rule line of ncols columns from first.
func (m *Model) ruleCols(b *strings.Builder, lc *lineCtx, above, first, ncols int) {
	j := m.ruleJointAt(lc, above, first, sheet.Stroke{})
	for c := first; c < first+ncols; c++ {
		a := sheet.Addr{Col: c, Row: lc.row}
		w := m.sheet.ColWidth(c)
		h := j.top
		style, colored := m.borderRole(h.Color), false
		if lc.selecting && lc.sel.Contains(a) && lc.sel.Contains(sheet.Addr{Col: c, Row: above}) {
			style, colored = &m.th.Selection, true
		}
		if mg, ok := lc.mergeOf(c); ok && lc.row > mg.From.Row && mg.From == lc.focus {
			style, colored = &m.th.Pointer, true
		}
		next := m.ruleJointAt(lc, above, c+1, h)
		switch {
		case !colored && j.glyph == " " && h.Line == sheet.LineNone:
			b.WriteString(strings.Repeat(" ", w))
		case j.glyph == " ":
			b.WriteString(m.paint(style, j.glyph+strings.Repeat(hLine(h.Line), w-1)))
		default: // the joint in the border's role, as leftEdge draws the lines through it
			b.WriteString(m.paint(m.borderRole(j.color), j.glyph))
			m.ruleRun(b, style, colored, h.Line, w-1, j, next)
		}
		j = next
	}
}

// hLine is the character of a horizontal line drawn with l.
func hLine(l sheet.Line) string { return theme.Junction(sheet.LineNone, sheet.LineNone, l, l) }

// ruleRun draws the n characters of line l along the top of a cell,
// between joints from and to, in style: a double line meeting a thick
// joint starts or ends with a thick stub in the joint's color, as the
// joint's arm running on (see theme.Bridges).
func (m *Model) ruleRun(b *strings.Builder, style *lipgloss.Style, colored bool, l sheet.Line, n int, from, to ruleJoint) {
	if n <= 0 {
		return
	}
	left, right := from.bridgeR, to.bridgeL && (n > 1 || !from.bridgeR)
	stub := hLine(sheet.LineThick)
	stubStyle := func(j ruleJoint) *lipgloss.Style {
		if colored {
			return style
		}
		return m.borderRole(j.color)
	}
	mid := n
	if left {
		b.WriteString(m.paint(stubStyle(from), stub))
		mid--
	}
	if right {
		mid--
	}
	b.WriteString(m.paint(style, strings.Repeat(hLine(l), max(mid, 0))))
	if right {
		b.WriteString(m.paint(stubStyle(to), stub))
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
