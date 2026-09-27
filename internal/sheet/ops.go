package sheet

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Errors returned by operations that would lose data or don't fit.
var (
	ErrPushedOff  = errors.New("There's data at the edge of the sheet that would be pushed off")
	ErrPasteEdge  = errors.New("The paste doesn't fit: it would go past the edge of the sheet")
	ErrFillTooBig = errors.New("The selection is too large to fill")
)

// maxFill caps how many cells a paste or fill writes at once, so an
// accidental whole-sheet selection can't stall the program.
const maxFill = 1 << 20

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
		for a, c := range s.cells.all() {
			if next[a] != c {
				s.place(a, next[a])
			}
		}
		for a, c := range next {
			if s.cells.get(a) != c {
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
	for range s.cells.inRange(edge) {
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
		s.shiftView(rows, sp)
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

// Clip is a copied range: snapshots of its cells taken at copy time.
type Clip struct {
	Src   Rect
	cells map[Addr]*Cell // by offset from Src.From
}

// Copy snapshots the cells in r for pasting.
func (s *Sheet) Copy(r Rect) *Clip {
	c := &Clip{Src: r, cells: map[Addr]*Cell{}}
	for _, a := range s.cellsIn(r) {
		c.cells[Addr{Col: a.Col - r.From.Col, Row: a.Row - r.From.Row}] = s.cells.get(a).clone()
	}
	return c
}

// Size returns the clip's width and height in cells.
func (c *Clip) Size() (cols, rows int) {
	return c.Src.To.Col - c.Src.From.Col + 1, c.Src.To.Row - c.Src.From.Row + 1
}

// Text returns the clip's values as displayed in General format, row by
// row, for the system clipboard.
func (c *Clip) Text() [][]string {
	cols, rows := c.Size()
	out := make([][]string, rows)
	for r := range out {
		out[r] = make([]string, cols)
		for col := range out[r] {
			if cell := c.cells[Addr{Col: col, Row: r}]; cell != nil {
				out[r][col] = cell.Value.String()
			}
		}
	}
	return out
}

// Paste writes the clip into dst and returns the range written. Formulas
// shift their relative references by the distance pasted, as in Sheets;
// with values set, only the computed values are pasted. Like Sheets, a
// destination that is an exact multiple of the clip's size is tiled;
// otherwise the clip is pasted once at dst's top-left corner.
func (s *Sheet) Paste(c *Clip, dst Rect, values bool) (Rect, error) {
	cols, rows := c.Size()
	w, h := dst.To.Col-dst.From.Col+1, dst.To.Row-dst.From.Row+1
	if w%cols != 0 || h%rows != 0 {
		dst.To = Addr{Col: dst.From.Col + cols - 1, Row: dst.From.Row + rows - 1}
		w, h = cols, rows
	}
	if !dst.To.Valid() {
		return Rect{}, ErrPasteEdge
	}
	if w*h > maxFill {
		return Rect{}, ErrFillTooBig
	}
	label := "paste into " + dst.String()
	if values {
		label = "paste values into " + dst.String()
	}
	s.change(label, dst, func() {
		for r := range h {
			for col := range w {
				off := Addr{Col: col % cols, Row: r % rows}
				to := Addr{Col: dst.From.Col + col, Row: dst.From.Row + r}
				from := Addr{Col: c.Src.From.Col + off.Col, Row: c.Src.From.Row + off.Row}
				s.pasteCell(to, c.cells[off], to.Col-from.Col, to.Row-from.Row, values)
			}
		}
	})
	return dst, nil
}

func (s *Sheet) pasteCell(a Addr, c *Cell, dc, dr int, values bool) {
	switch {
	case c == nil:
		if s.cells.get(a) != nil {
			s.place(a, nil)
		}
	case values:
		s.put(a, valueInput(c.Value))
	default:
		s.place(a, c.rewritten(formula.Shift(dc, dr)).clone())
	}
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
	if w*h > maxFill {
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
					s.pasteCell(a, c, a.Col-origin.Col, a.Row-origin.Row, false)
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
		if !s.cells.get(a).Blank() {
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
