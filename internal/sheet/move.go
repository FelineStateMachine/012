package sheet

import "github.com/FelineStateMachine/012/internal/formula"

// Moving cells, as cut and paste does: formulas that read the moved cells
// follow them.

// Move moves the cells in src so its top-left corner lands on to, as
// cut and paste does in Sheets, with the formatting they show; whole
// columns or rows take their line formats along. Formulas anywhere that referred to the
// moved cells follow them; references to cells the move overwrote become
// #REF!. It returns the destination range.
func (s *Sheet) Move(src Rect, to Addr) (Rect, error) {
	dc, dr := to.Col-src.From.Col, to.Row-src.From.Row
	dst := Rect{From: to, To: Addr{Col: src.To.Col + dc, Row: src.To.Row + dr}}
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
			return Rect{From: shift(r.From), To: shift(r.To)}, true
		}
		return r, true
	}
	label := "move " + src.String() + " to " + dst.String()
	f := s.copyFormats(src)
	s.change(label, dst, func() {
		s.remap(label, dst, cell, rng)
		s.remapNames(rng) // a name for exactly the moved cells follows them
		s.moveFormats(s, &f, src, dst)
		s.moveRules(src, dst, cell, rng) // rulemove.go
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
	d := Rect{From: to, To: Addr{Col: src.To.Col + dc, Row: src.To.Row + dr}}
	if !d.To.Valid() {
		return Rect{}, ErrPasteEdge
	}
	mv := sheetMove{from: s, to: dst, src: src, d: d}
	moved, readers := mv.movedCells(), mv.readers()
	f := s.copyFormats(src)
	label := "move " + src.String() + " to " + formula.QuoteSheet(dst.name) + "!" + d.String()
	dst.change(label, d, func() {
		mv.apply(moved, readers)
		dst.moveFormats(s, &f, src, d)
	})
	return d, nil
}

// sheetMove is the cells in src on one sheet moving to d on another.
type sheetMove struct {
	from, to *Sheet
	src, d   Rect
}

func (mv sheetMove) shift(a Addr) Addr {
	return Addr{Col: a.Col + mv.d.From.Col - mv.src.From.Col, Row: a.Row + mv.d.From.Row - mv.src.From.Row}
}

// movedCells are the cells that move, by where they land, with their
// references rewritten.
func (mv sheetMove) movedCells() map[Addr]*Cell {
	moved := map[Addr]*Cell{}
	for _, a := range mv.from.cellsIn(mv.src) {
		moved[mv.shift(a)] = mv.from.cells.get(a).rewritten(mv.rewrite(mv.from, mv.to))
	}
	return moved
}

// readers are the formulas that stay where they are, rewritten, among
// those whose references the move changes: on either sheet, or on another
// sheet naming one of them.
func (mv sheetMove) readers() map[loc]*Cell {
	out := map[loc]*Cell{}
	add := func(l loc) {
		if mv.src.Contains(l.a) && l.s == mv.from || mv.d.Contains(l.a) && l.s == mv.to {
			return // moved or overwritten
		}
		c := l.s.cells.get(l.a)
		if nc := c.rewritten(mv.rewrite(l.s, l.s)); nc != c {
			out[l] = nc
		}
	}
	for _, t := range []*Sheet{mv.from, mv.to} {
		for a := range t.cells.all() {
			add(loc{t, a})
		}
	}
	for _, l := range mv.from.wb.crossList() {
		if l.s != mv.from && l.s != mv.to {
			add(l)
		}
	}
	return out
}

// apply clears the source and destination, places the moved cells and
// the rewritten readers, and moves the named ranges wholly inside the
// source along with it.
func (mv sheetMove) apply(moved map[Addr]*Cell, readers map[loc]*Cell) {
	for _, a := range mv.from.cellsIn(mv.src) {
		mv.from.place(a, nil)
	}
	for _, a := range mv.to.cellsIn(mv.d) {
		mv.to.place(a, nil)
	}
	for a, c := range moved {
		mv.to.place(a, c)
	}
	for l, c := range readers {
		l.s.place(l.a, c)
	}
	w := mv.from.wb
	for k, n := range w.names {
		if n.Sheet == mv.from && !n.Lost && mv.src.Contains(n.Range.From) && mv.src.Contains(n.Range.To) {
			n.Sheet, n.Range = mv.to, Rect{From: mv.shift(n.Range.From), To: mv.shift(n.Range.To)}
			w.putName(k, &n)
		}
	}
}

// rewrite maps the references of a formula that was on orig and is on
// home after the move. A reference names its sheet unless it points at
// home.
func (mv sheetMove) rewrite(orig, home *Sheet) formula.Rewriter {
	w := mv.from.wb
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
	return formula.Rewriter{
		Ref: func(n formula.Ref) Node {
			t := w.resolve(orig, n.Sheet)
			switch {
			case t == nil:
				return n
			case t == mv.from && mv.src.Contains(n.Addr):
				return formula.Ref{Addr: mv.shift(n.Addr), Abs: n.Abs, Sheet: written(n.Sheet, t, mv.to)}
			case t == mv.to && mv.d.Contains(n.Addr):
				return formula.RefErr{}
			}
			return formula.Ref{Addr: n.Addr, Abs: n.Abs, Sheet: written(n.Sheet, t, t)}
		},
		Range: func(n formula.Range) Node {
			t := w.resolve(orig, n.Sheet)
			switch {
			case t == nil:
				return n
			case t == mv.from && mv.src.Contains(n.Rect.From) && mv.src.Contains(n.Rect.To):
				r := Rect{From: mv.shift(n.Rect.From), To: mv.shift(n.Rect.To)}
				return formula.Range{Rect: r, Abs: n.Abs, Sheet: written(n.Sheet, t, mv.to)}
			}
			return formula.Range{Rect: n.Rect, Abs: n.Abs, Sheet: written(n.Sheet, t, t)}
		},
	}
}
