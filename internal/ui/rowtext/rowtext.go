// Package rowtext lays out the text of one row of cells across the
// columns on screen, as Sheets does: values formatted and aligned, text
// running on into blank neighbors, cut to column boundaries in a single
// pass. It reads the sheet and nothing else, so drawing the grid can be
// measured and tested on its own.
package rowtext

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Span is what one grid column shows in a row: blank columns, the text,
// blank columns. Only the text takes the owning cell's text style, so an
// underline doesn't run into the padding.
type Span struct {
	Lead  int
	Text  string
	Trail int
	Style sheet.Style
	Owner int // column of the cell the text belongs to

	// Left for the caller to fill in from the owner's value.
	Link  string // the owner's link target, drawn as a hyperlink
	Error bool   // the owner shows an error: its text gets the error mark
}

// Layout lays out ncols columns of row from lo, each span exactly its
// column's width. Text may run in from columns between minCol and maxCol,
// so it stops at the frozen columns' divider. Values are formatted with
// their cell's number format and aligned as Sheets does: numbers right,
// text left, booleans and errors centered, unless the cell sets an
// alignment. Text runs on into blank neighbors: to the right when
// left-aligned, to the left when right-aligned, both ways when centered.
// It can come from cells outside the viewport, so the scan starts at the
// nearest filled cell on each side.
func Layout(s *sheet.Sheet, row, lo, ncols, minCol, maxCol int) []Span {
	l := rowLayout{s: s, row: row, lo: lo, hi: lo + ncols - 1, out: make([]Span, ncols)}
	for i := range l.out {
		l.out[i] = Span{Trail: s.ColWidth(lo + i)}
	}
	l.first, l.last = l.reach(minCol, maxCol)
	// x[k] is where column first+k starts, relative to column first.
	l.x = make([]int, l.last-l.first+2)
	for c := l.first; c <= l.last; c++ {
		l.x[c-l.first+1] = l.x[c-l.first] + s.ColWidth(c)
	}
	for c := l.first; c <= l.last; c++ {
		if cell := l.content(c); cell != nil {
			l.place(c, cell)
		}
	}
	keepGaps(l.out)
	return l.out
}

// rowLayout is Layout's work in progress on one row.
type rowLayout struct {
	s           *sheet.Sheet
	row         int
	lo, hi      int // the columns laid out
	first, last int // the columns whose text may reach them
	x           []int
	claimed     int // text of earlier cells reaches up to here
	out         []Span
}

// content is the cell in column c of the row, or nil if it's blank.
func (l *rowLayout) content(c int) *sheet.Cell {
	if cell := l.s.Cell(sheet.Addr{Col: c, Row: l.row}); !cell.Blank() {
		return cell
	}
	return nil
}

// col returns where column c starts and ends, relative to column first.
func (l *rowLayout) col(c int) (int, int) { return l.x[c-l.first], l.x[c-l.first+1] }

// maxReach is how many columns away text may run in from: 2,560
// characters in default columns.
const maxReach = 256

// reach returns the nearest filled columns outside lo..hi, within minCol
// and maxCol and maxReach of the viewport, whose text might run into
// view; lo and hi if there are none. The filled cells are found through
// the sheet's index, not by looking at every column between.
func (l *rowLayout) reach(minCol, maxCol int) (first, last int) {
	first, last = l.lo, l.hi
	if c, ok := l.s.NextFilledCol(l.row, l.lo-1, -1, max(minCol, l.lo-maxReach)); ok {
		first = c
	}
	if c, ok := l.s.NextFilledCol(l.row, l.hi+1, 1, min(maxCol, l.hi+maxReach)); ok {
		last = c
	}
	return first, last
}

// place lays out the text of the cell in column c over the columns it
// reaches.
func (l *rowLayout) place(c int, cell *sheet.Cell) {
	x0, x1 := l.col(c)
	text, align, pad := l.display(c, cell, x1-x0)
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
	from, to := l.room(c, cell, start, tw)
	l.claimed = to
	cut := textCutter{s: text}
	style := l.s.CellStyle(sheet.Addr{Col: c, Row: l.row})
	for k := max(l.first, l.lo); k <= min(l.last, l.hi); k++ {
		kx0, kx1 := l.col(k)
		if kx1 <= from || kx0 >= to {
			continue
		}
		sp := Span{Trail: kx1 - kx0, Style: style, Owner: c}
		if seg0, seg1 := max(start, kx0, from), min(start+tw, kx1, to); seg1 > seg0 {
			sp.Lead, sp.Text, sp.Trail = seg0-kx0, cut.cut(seg0-start, seg1-start), kx1-seg1
		}
		l.out[k-l.lo] = sp
	}
}

// display formats the cell in column c for a column w wide, returning
// the text, its alignment and the padding on its aligned side.
func (l *rowLayout) display(c int, cell *sheet.Cell, w int) (string, sheet.Align, int) {
	f := l.s.DisplayFormat(sheet.Addr{Col: c, Row: l.row})
	text, align := sheet.Display(cell.Value, f, w)
	pad := 1
	// A number one character too wide (12/31/2026 in a default column)
	// may use the padding when nothing is to its right, rather than
	// turning into #s.
	if cell.Value.Kind == sheet.Number && strings.Trim(text, "#") == "" && l.content(c+1) == nil {
		if wider, _ := sheet.Display(cell.Value, f, w+1); strings.Trim(wider, "#") != "" {
			text, pad = wider, 0
		}
	}
	if a := l.s.CellStyle(sheet.Addr{Col: c, Row: l.row}).Align; a != sheet.AlignAuto && align != sheet.AlignFill {
		align = a
	}
	return text, align, pad
}

// room returns where the text of the cell in column c, tw wide from
// start, may go: its own column and, for text, the blank neighbors it
// runs into, but not over text already placed to its left.
func (l *rowLayout) room(c int, cell *sheet.Cell, start, tw int) (from, to int) {
	x0, x1 := l.col(c)
	from, to = max(x0, l.claimed), x1
	if cell.Value.Kind != sheet.Text {
		return from, to
	}
	for k := c + 1; k <= l.last && start+tw > to && l.content(k) == nil; k++ {
		_, to = l.col(k)
	}
	for k := c - 1; k >= l.first && start < from && l.content(k) == nil; k-- {
		kx0, _ := l.col(k)
		if kx0 < l.claimed {
			return l.claimed, to
		}
		from = kx0
	}
	return from, to
}

// keepGaps keeps a gap where text that runs to the edge of its column
// would touch a neighbor that starts at its own edge
// ("Groceries9/28/2026").
func keepGaps(out []Span) {
	for i := 0; i+1 < len(out); i++ {
		l, r := &out[i], &out[i+1]
		if l.Text != "" && l.Trail == 0 && r.Text != "" && r.Lead == 0 && l.Owner != r.Owner {
			l.Text = ansi.Truncate(l.Text, ansi.StringWidth(l.Text)-1, "")
			l.Trail = 1
		}
	}
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
