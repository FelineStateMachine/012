package sheet

import (
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/locale"
)

// Errors returned by operations that would lose data or don't fit.
var (
	ErrPushedOff  = errors.New("There's data at the edge of the sheet that would be pushed off")
	ErrPasteEdge  = errors.New("The paste doesn't fit: it would go past the edge of the sheet")
	ErrFillTooBig = errors.New("That would write more cells than max-cells allows (see File > Settings)")
)

// DefaultMaxCells is the max-cells setting's default: about 200 MB of
// numbers at 20 bytes each (store.go).
const DefaultMaxCells = 10_000_000

// maxCells caps how many cells a paste or fill writes at once, and how
// many an import keeps, so an accidental whole-sheet selection or a huge
// file can't stall the program or exhaust memory. It is the max-cells
// setting of the config file, process-wide like the rest of it.
var maxCells atomic.Int64

func init() { maxCells.Store(DefaultMaxCells) }

// SetMaxCells sets the cell budget; n < 1 restores the default.
func SetMaxCells(n int) {
	if n < 1 {
		n = DefaultMaxCells
	}
	maxCells.Store(int64(n))
}

// MaxCells returns the cell budget.
func MaxCells() int { return int(maxCells.Load()) }

// remap moves every stored cell with cell (dropping those it rejects) and
// rewrites every reference to this sheet's cells, here and on other
// sheets, with cell and rng, as one undo step. References to other sheets
// stay as they are.
func (s *Sheet) remap(label string, focus Rect, cell func(Addr) (Addr, bool), rng func(Rect) (Rect, bool)) {
	rw := formula.Relocate(s.onThis(s), cell, rng)
	next := make(map[Addr]*Cell, s.cells.len())
	for a, c := range s.cells.all() {
		if to, ok := cell(a); ok {
			next[to] = c.rewritten(rw)
		}
	}
	others := map[loc]*Cell{}
	for _, l := range s.wb.crossList() {
		if l.s == s {
			continue
		}
		c := l.s.cells.get(l.a)
		if nc := c.rewritten(formula.Relocate(s.onThis(l.s), cell, rng)); nc != c {
			others[l] = nc
		}
	}
	s.change(label, focus, func() {
		for l, c := range others {
			l.s.place(l.a, c)
		}
		for a := range s.cells.keys() {
			if c := next[a]; !s.cells.holds(a, c) {
				s.place(a, c)
			}
		}
		for a, c := range next {
			if !s.cells.holds(a, c) {
				s.place(a, c)
			}
		}
	})
}

// InsertRows inserts n blank rows before row at, shifting the rows below
// down and adjusting every reference to them.
func (s *Sheet) InsertRows(at, n int) error {
	return s.insert(true, at, n)
}

// InsertCols inserts n blank columns before column at.
func (s *Sheet) InsertCols(at, n int) error {
	return s.insert(false, at, n)
}

// DeleteRows deletes n rows starting at row at. References to deleted
// cells become #REF!; ranges that lose some of their rows shrink.
func (s *Sheet) DeleteRows(at, n int) {
	s.restructure(true, formula.Span{At: at, N: -n, Size: MaxRows})
}

// DeleteCols deletes n columns starting at column at.
func (s *Sheet) DeleteCols(at, n int) {
	s.restructure(false, formula.Span{At: at, N: -n, Size: MaxCols})
}

func (s *Sheet) insert(rows bool, at, n int) error {
	size := MaxCols
	if rows {
		size = MaxRows
	}
	// The lines pushed past the edge must be empty.
	first := max(at, size-n)
	edge := colRect(first, size-1)
	if rows {
		edge = rowRect(first, size-1)
	}
	for range s.cells.keysIn(edge) {
		return ErrPushedOff
	}
	s.restructure(rows, formula.Span{At: at, N: n, Size: size})
	return nil
}

func (s *Sheet) restructure(rows bool, sp formula.Span) {
	cell, rng := formula.AxisMaps(rows, sp)
	last := sp.At + abs(sp.N) - 1
	focus := colRect(sp.At, min(last, MaxCols-1))
	if rows {
		focus = rowRect(sp.At, min(last, MaxRows-1))
	}
	verb, noun := "insert", "column"
	if sp.N < 0 {
		verb = "delete"
	}
	if rows {
		noun = "row"
	}
	label := fmt.Sprintf("%s %d %s", verb, abs(sp.N), noun)
	if abs(sp.N) > 1 {
		label += "s"
	}
	s.change(label, focus, func() {
		s.remap(label, focus, cell, rng)
		s.remapNames(rng)
		s.shiftCharts(cell, rng)
		if !rows {
			s.shiftWidths(sp)
		}
		s.shiftLines(rows, sp)
		s.shiftView(rows, sp)
		s.shiftPivots(rows, sp)
		s.shiftRules(rows, sp)
	})
}

// shiftWidths moves column widths along with inserted or deleted columns.
func (s *Sheet) shiftWidths(sp formula.Span) {
	next := map[int]int{}
	for c, w := range s.widths {
		if to, ok := sp.Point(c); ok {
			next[to] = w
		}
	}
	for c := range s.widths {
		if _, ok := next[c]; !ok {
			s.setWidth(c, 0)
		}
	}
	for c, w := range next {
		if s.widths[c] != w {
			s.setWidth(c, w)
		}
	}
}

// Clip is a copied range: snapshots of its cells, and of the formatting
// they show, taken at copy time.
type Clip struct {
	Range   Rect           // the range copied
	Src     Rect           // Range trimmed to its cells when whole lines
	cells   map[Addr]*Cell // by offset from Src.From
	formats clipFormats
}

// Copy snapshots the cells in r, and what formatting they show, for
// pasting. Whole columns or rows are trimmed to the cells they hold,
// keeping their line formats.
func (s *Sheet) Copy(r Rect) *Clip {
	c := &Clip{Range: r, Src: s.trimLines(r), cells: map[Addr]*Cell{}, formats: s.copyFormats(r)}
	for _, a := range s.cellsIn(r) {
		c.cells[Addr{Col: a.Col - r.From.Col, Row: a.Row - r.From.Row}] = s.cells.get(a).plain()
	}
	return c
}

// trimLines trims whole columns of r to the rows holding cells, and
// whole rows to the columns holding cells: no point copying a million
// blanks.
func (s *Sheet) trimLines(r Rect) Rect {
	if !r.AllRows() && !r.AllCols() {
		return r
	}
	held, ok := s.cells.bounds(r)
	if !ok {
		held.To = r.From
	}
	if r.AllRows() {
		r.To.Row = max(held.To.Row, r.From.Row)
	}
	if r.AllCols() {
		r.To.Col = max(held.To.Col, r.From.Col)
	}
	return r
}

// Size returns the clip's width and height in cells.
func (c *Clip) Size() (cols, rows int) {
	return c.Src.To.Col - c.Src.From.Col + 1, c.Src.To.Row - c.Src.From.Row + 1
}

// TextIn returns the clip's values as General shows them in loc, row by
// row, for the system clipboard. Blank rows and columns past the last
// value are left off, so copying whole columns gives their data; a block
// of more than MaxCells cells, blanks between values included, is too
// big for text, and TextIn returns nil.
func (c *Clip) TextIn(loc *locale.Locale) [][]string {
	cols, rows := 0, 0
	for off, cell := range c.cells {
		if !cell.Blank() {
			cols, rows = max(cols, off.Col+1), max(rows, off.Row+1)
		}
	}
	if cols*rows > MaxCells() {
		return nil
	}
	out := make([][]string, rows)
	for r := range out {
		out[r] = make([]string, cols)
		for col := range out[r] {
			if cell := c.cells[Addr{Col: col, Row: r}]; cell != nil {
				out[r][col] = textIn(cell.Value, loc)
			}
		}
	}
	return out
}

// Paste writes the clip into dst and returns the range written. Formulas
// shift their relative references by the distance pasted, as in Sheets;
// with values set, only the computed values are pasted, keeping the
// destination's formatting. Otherwise each cell shows the formatting its
// source showed (see clipfmt.go). Like Sheets, a destination that is an
// exact multiple of the clip's size is tiled; otherwise the clip is
// pasted once at dst's top-left corner.
func (s *Sheet) Paste(c *Clip, dst Rect, values bool) (Rect, error) {
	p, err := c.layout(dst)
	if err != nil {
		return Rect{}, err
	}
	if len(c.cells)*p.across*p.down > MaxCells() {
		return Rect{}, ErrFillTooBig
	}
	dst = p.dst
	label := "paste into " + dst.String()
	if values {
		label = "paste values into " + dst.String()
	}
	// Blank parts of the clip clear what they land on, and each of its
	// cells is placed in every tile: the cost is the cells written and
	// cleared, not dst's area.
	s.change(label, dst, func() {
		for _, a := range s.cellsIn(dst) {
			if c.cells[p.off(a)] == nil {
				s.place(a, nil)
			}
		}
		for off, cell := range c.cells {
			from := Addr{Col: c.Src.From.Col + off.Col, Row: c.Src.From.Row + off.Row}
			for tr := range p.down {
				for tc := range p.across {
					to := Addr{Col: dst.From.Col + tc*p.tw + off.Col, Row: dst.From.Row + tr*p.th + off.Row}
					s.pasteCell(to, cell, to.Col-from.Col, to.Row-from.Row, values)
				}
			}
		}
		if !values {
			s.pasteFormats(&c.formats, p)
		}
	})
	return dst, nil
}

func (s *Sheet) pasteCell(a Addr, c *Cell, dc, dr int, values bool) {
	switch {
	case c == nil:
		if s.cells.has(a) {
			s.place(a, nil)
		}
	case values:
		s.put(a, valueInput(c.Value))
	default:
		s.place(a, c.rewritten(formula.Shift(dc, dr)).clone())
	}
}

// fillCell pastes c at a, dc and dr away from where it was typed, as
// Ctrl+Enter fills: each cell keeps its own note.
func (s *Sheet) fillCell(a Addr, c *Cell, dc, dr int) {
	if c == nil {
		s.pasteCell(a, nil, dc, dr, false)
		return
	}
	nc := c.rewritten(formula.Shift(dc, dr)).clone()
	nc.Note = ""
	if old := s.cells.get(a); old != nil {
		nc.Note = old.Note
	}
	s.place(a, nc)
}

// valueInput is the entry that stores v as a constant. Text that would
// read as a number, boolean or formula gets a leading ' to stay text.
func valueInput(v Value) string {
	switch v.Kind {
	case Number:
		return strconv.FormatFloat(v.Num, 'f', -1, 64)
	case Text:
		if n, _, _ := classify(v.Str); n != nil || (len(v.Str) > 0 && v.Str[0] == '\'') {
			return "'" + v.Str
		}
	}
	return v.String()
}

// FillDown copies the top row of r into the rest of it, adjusting
// references (Ctrl+D). A single-row range fills from the row above. When
// the top rows of r start a series and the rest is blank (1, 2 and then
// empty cells), it continues the series instead, as dragging the fill
// handle would.
func (s *Sheet) FillDown(r Rect) (Rect, error) {
	if src, ok := s.seriesStart(r, true); ok {
		return s.FillSeries(src, r)
	}
	src := Rect{From: r.From, To: Addr{Col: r.To.Col, Row: r.From.Row}}
	if r.From.Row == r.To.Row {
		if r.From.Row == 0 {
			return r, nil
		}
		src.From.Row, src.To.Row = r.From.Row-1, r.From.Row-1
	} else {
		r.From.Row++
	}
	var err error
	s.change("fill down "+r.String(), r, func() { r, err = s.Paste(s.Copy(src), r, false) })
	return r, err
}

// FillRight copies the left column of r into the rest of it (Ctrl+R). A
// single-column range fills from the column to the left. Like FillDown,
// it continues a series started in the leftmost columns.
func (s *Sheet) FillRight(r Rect) (Rect, error) {
	if src, ok := s.seriesStart(r, false); ok {
		return s.FillSeries(src, r)
	}
	src := Rect{From: r.From, To: Addr{Col: r.From.Col, Row: r.To.Row}}
	if r.From.Col == r.To.Col {
		if r.From.Col == 0 {
			return r, nil
		}
		src.From.Col, src.To.Col = r.From.Col-1, r.From.Col-1
	} else {
		r.From.Col++
	}
	var err error
	s.change("fill right "+r.String(), r, func() { r, err = s.Paste(s.Copy(src), r, false) })
	return r, err
}

// FillEntry stores input in every cell of r as if it had been typed at
// origin and copied to each cell, adjusting references (Ctrl+Enter).
func (s *Sheet) FillEntry(r Rect, origin Addr, input string) error {
	w, h := r.To.Col-r.From.Col+1, r.To.Row-r.From.Row+1
	if w*h > MaxCells() {
		return ErrFillTooBig
	}
	if _, _, err := classify(input); err != nil {
		return err
	}
	return s.Batch(Change{Label: "fill " + r.String(), Focus: r}, func() error {
		if err := s.put(origin, input); err != nil {
			return err
		}
		c := s.cells.get(origin).clone()
		for row := r.From.Row; row <= r.To.Row; row++ {
			for col := r.From.Col; col <= r.To.Col; col++ {
				if a := (Addr{Col: col, Row: row}); a != origin {
					s.fillCell(a, c, a.Col-origin.Col, a.Row-origin.Row)
				}
			}
		}
		return nil
	})
}

// seriesStart finds the leading rows (down) or columns of r that hold
// something, when there are at least two of them and everything after
// them in r is blank: the start of a series to fill.
func (s *Sheet) seriesStart(r Rect, down bool) (Rect, bool) {
	line := func(a Addr) int {
		if down {
			return a.Row - r.From.Row
		}
		return a.Col - r.From.Col
	}
	used := map[int]bool{}
	for _, a := range s.cellsIn(r) {
		if s.cells.filledAt(a) {
			used[line(a)] = true
		}
	}
	n := 0
	for used[n] {
		n++
	}
	if n < 2 || len(used) != n {
		return Rect{}, false
	}
	src := r
	if down {
		src.To.Row = r.From.Row + n - 1
	} else {
		src.To.Col = r.From.Col + n - 1
	}
	return src, src != r
}

func abs(n int) int { return max(n, -n) }
