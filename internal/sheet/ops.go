package sheet

import (
	"errors"
	"fmt"
	"strconv"
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
	rw := relocate(s.onThis(s), cell, rng)
	next := make(map[Addr]*Cell, len(s.cells))
	for a, c := range s.cells {
		if to, ok := cell(a); ok {
			next[to] = c.rewritten(rw)
		}
	}
	others := map[loc]*Cell{}
	for _, l := range s.wb.crossList() {
		if l.s == s {
			continue
		}
		c := l.s.cells[l.a]
		if nc := c.rewritten(relocate(s.onThis(l.s), cell, rng)); nc != c {
			others[l] = nc
		}
	}
	s.change(label, focus, func() {
		for l, c := range others {
			l.s.place(l.a, c)
		}
		for a, c := range s.cells {
			if next[a] != c {
				s.place(a, next[a])
			}
		}
		for a, c := range next {
			if s.cells[a] != c {
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
	s.restructure(true, span{at, -n, MaxRows})
}

// DeleteCols deletes n columns starting at column at.
func (s *Sheet) DeleteCols(at, n int) {
	s.restructure(false, span{at, -n, MaxCols})
}

func (s *Sheet) insert(rows bool, at, n int) error {
	size := MaxCols
	if rows {
		size = MaxRows
	}
	for a := range s.cells {
		line := a.Col
		if rows {
			line = a.Row
		}
		if line >= at && line >= size-n {
			return ErrPushedOff
		}
	}
	s.restructure(rows, span{at, n, size})
	return nil
}

func (s *Sheet) restructure(rows bool, sp span) {
	cell, rng := axisRewrite(rows, sp)
	last := sp.at + abs(sp.n) - 1
	focus := colRect(sp.at, min(last, MaxCols-1))
	if rows {
		focus = rowRect(sp.at, min(last, MaxRows-1))
	}
	verb, noun := "insert", "column"
	if sp.n < 0 {
		verb = "delete"
	}
	if rows {
		noun = "row"
	}
	label := fmt.Sprintf("%s %d %s", verb, abs(sp.n), noun)
	if abs(sp.n) > 1 {
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
func (s *Sheet) shiftWidths(sp span) {
	next := map[int]int{}
	for c, w := range s.widths {
		if to, ok := sp.point(c); ok {
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

// Move moves the cells in src so its top-left corner lands on to, as
// cut and paste does in Sheets. Formulas anywhere that referred to the
// moved cells follow them; references to cells the move overwrote become
// #REF!. It returns the destination range.
func (s *Sheet) Move(src Rect, to Addr) (Rect, error) {
	dc, dr := to.Col-src.From.Col, to.Row-src.From.Row
	dst := Rect{to, Addr{Col: src.To.Col + dc, Row: src.To.Row + dr}}
	if !dst.To.Valid() {
		return Rect{}, ErrPasteEdge
	}
	shift := func(a Addr) Addr { return Addr{Col: a.Col + dc, Row: a.Row + dr} }
	cell := func(a Addr) (Addr, bool) {
		switch {
		case src.Contains(a):
			return shift(a), true
		case dst.Contains(a):
			return a, false
		}
		return a, true
	}
	rng := func(r Rect) (Rect, bool) {
		if src.Contains(r.From) && src.Contains(r.To) {
			return Rect{shift(r.From), shift(r.To)}, true
		}
		return r, true
	}
	label := "move " + src.String() + " to " + dst.String()
	s.change(label, dst, func() {
		s.remap(label, dst, cell, rng)
		s.remapNames(rng) // a name for exactly the moved cells follows them
	})
	return dst, nil
}

// MoveTo moves the cells in src to sheet dst, src's top-left corner
// landing on to, as cutting on one sheet and pasting on another does in
// Sheets. Formulas anywhere that read the moved cells follow them to dst,
// naming its sheet where they need to; the moved formulas keep reading
// what they read, naming this sheet for cells that stayed behind.
// References to cells the move overwrote become #REF!.
func (s *Sheet) MoveTo(dst *Sheet, src Rect, to Addr) (Rect, error) {
	if dst == s {
		return s.Move(src, to)
	}
	dc, dr := to.Col-src.From.Col, to.Row-src.From.Row
	d := Rect{to, Addr{Col: src.To.Col + dc, Row: src.To.Row + dr}}
	if !d.To.Valid() {
		return Rect{}, ErrPasteEdge
	}
	shift := func(a Addr) Addr { return Addr{Col: a.Col + dc, Row: a.Row + dr} }
	rw := func(orig, home *Sheet) refRewrite { return s.moveRewrite(dst, src, d, orig, home) }

	moved := map[Addr]*Cell{}
	for _, a := range s.cellsIn(src) {
		moved[shift(a)] = s.cells[a].rewritten(rw(s, dst))
	}
	rest := map[loc]*Cell{}
	keep := func(l loc) {
		if src.Contains(l.a) && l.s == s || d.Contains(l.a) && l.s == dst {
			return
		}
		c := l.s.cells[l.a]
		if nc := c.rewritten(rw(l.s, l.s)); nc != c {
			rest[l] = nc
		}
	}
	for _, t := range []*Sheet{s, dst} {
		for a := range t.cells {
			keep(loc{t, a})
		}
	}
	for _, l := range s.wb.crossList() {
		if l.s != s && l.s != dst {
			keep(l)
		}
	}
	label := "move " + src.String() + " to " + quoteSheet(dst.name) + "!" + d.String()
	dst.change(label, d, func() {
		for _, a := range s.cellsIn(src) {
			s.place(a, nil)
		}
		for _, a := range dst.cellsIn(d) {
			dst.place(a, nil)
		}
		for a, c := range moved {
			dst.place(a, c)
		}
		for l, c := range rest {
			l.s.place(l.a, c)
		}
		for k, n := range s.wb.names {
			if n.Sheet == s && !n.Lost && src.Contains(n.Range.From) && src.Contains(n.Range.To) {
				n.Sheet, n.Range = dst, Rect{shift(n.Range.From), shift(n.Range.To)}
				s.wb.putName(k, &n)
			}
		}
	})
	return d, nil
}

// moveRewrite maps the references of a formula that was on orig and is
// on home after cells in src on s move to d on dst. A reference names its
// sheet unless it points at home.
func (s *Sheet) moveRewrite(dst *Sheet, src, d Rect, orig, home *Sheet) refRewrite {
	dc, dr := d.From.Col-src.From.Col, d.From.Row-src.From.Row
	shift := func(a Addr) Addr { return Addr{Col: a.Col + dc, Row: a.Row + dr} }
	// written is how a reference names now, having named was as written.
	written := func(as string, was, now *Sheet) string {
		switch {
		case now == home:
			return ""
		case now == was && as != "":
			return as
		}
		return now.name
	}
	return refRewrite{
		ref: func(n refNode) Node {
			t := s.wb.resolve(orig, n.sheet)
			switch {
			case t == nil:
				return n
			case t == s && src.Contains(n.a):
				return refNode{shift(n.a), n.abs, written(n.sheet, t, dst)}
			case t == dst && d.Contains(n.a):
				return refErrNode{}
			}
			return refNode{n.a, n.abs, written(n.sheet, t, t)}
		},
		rng: func(n rangeNode) Node {
			t := s.wb.resolve(orig, n.sheet)
			switch {
			case t == nil:
				return n
			case t == s && src.Contains(n.r.From) && src.Contains(n.r.To):
				return rangeNode{Rect{shift(n.r.From), shift(n.r.To)}, n.abs, written(n.sheet, t, dst)}
			}
			return rangeNode{n.r, n.abs, written(n.sheet, t, t)}
		},
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
		c.cells[Addr{Col: a.Col - r.From.Col, Row: a.Row - r.From.Row}] = s.cells[a].clone()
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
		if s.cells[a] != nil {
			s.place(a, nil)
		}
	case values:
		s.put(a, valueInput(c.Value))
	default:
		s.place(a, c.rewritten(shiftRefs(dc, dr)).clone())
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
	src := Rect{r.From, Addr{Col: r.To.Col, Row: r.From.Row}}
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
	src := Rect{r.From, Addr{Col: r.From.Col, Row: r.To.Row}}
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
		c := s.cells[origin].clone()
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
		if !s.cells[a].Blank() {
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
