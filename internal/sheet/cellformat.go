package sheet

// Formatting changes go through these methods rather than by editing
// cells directly, so history (undo) can hook a single place.

// materializeLimit bounds how many blank cells a formatting change
// creates. Formatting whole columns fits under it; for the whole sheet
// only cells with contents or formatting are changed.
const materializeLimit = 1 << 16

// DisplayFormat returns the format the cell at a is shown with: its own,
// its row's or column's, or for Automatic, the one inferred from its
// formula (=DATE() shows a date, =SUM(B2:B4) of currency shows
// currency).
func (s *Sheet) DisplayFormat(a Addr) Format {
	f, sty, ok := s.cells.look(a)
	switch {
	case !f.IsZero():
		return f
	case s.lines.none():
	case ok && sty.own:
	default:
		if f := s.inherited(a).Format; !f.IsZero() {
			return f
		}
	}
	if c := s.cells.richAt(a); c != nil { // only formulas infer formats
		return c.auto
	}
	return Format{}
}

// SetFormat gives every cell in r the number format f, including blank
// cells, which keep it for when something is typed. Whole columns and
// rows keep it as their line's format rather than on each cell.
func (s *Sheet) SetFormat(r Rect, f Format) {
	s.change("format "+r.String()+" as "+kindLabel(f.Kind), r, func() {
		s.eachFormat(r, !f.IsZero(), func(_ Addr, l *lineFmt) { l.Format = f })
	})
}

// SetStyle changes the text style of every cell in r with fn, e.g. to
// turn on bold while keeping italics. Callers wanting a specific undo
// label ("bold B2:B5") wrap it in Batch.
func (s *Sheet) SetStyle(r Rect, fn func(*Style)) {
	s.change("style "+r.String(), r, func() {
		s.eachFormat(r, true, func(_ Addr, l *lineFmt) { fn(&l.Style) })
	})
}

// ClearFormatting resets the format and style of every cell in r, as
// Sheets' Format > Clear formatting.
func (s *Sheet) ClearFormatting(r Rect) {
	s.change("clear formatting "+r.String(), r, func() {
		s.eachFormat(r, false, func(_ Addr, l *lineFmt) { *l = lineFmt{} })
	})
}

// AdjustDecimals shows delta more (or fewer) decimal places in every
// non-blank number cell of r, starting from what each cell shows now.
func (s *Sheet) AdjustDecimals(r Rect, delta int) {
	label := "more decimals in "
	if delta < 0 {
		label = "fewer decimals in "
	}
	s.change(label+r.String(), r, func() {
		s.eachCell(r, false, func(a Addr, l *lineFmt) {
			if c := s.cells.get(a); !c.Blank() {
				l.Format = s.DisplayFormat(a).WithDecimals(delta, c.Value.Num)
			}
		})
	})
}

// eachFormat applies fn to the formatting of r: to its lines' formats
// when r is whole columns or rows, and to each of its cells otherwise.
func (s *Sheet) eachFormat(r Rect, create bool, fn func(Addr, *lineFmt)) {
	if !s.formatLines(r, func(l *lineFmt) { fn(Addr{}, l) }) {
		s.eachCell(r, create, fn)
	}
}

// eachCell applies fn to what each cell in r shows and has the cell
// show the result, placing a copy so the change can be undone. With
// create, blank cells get a cell to hold the formatting (up to
// materializeLimit cells); cells left with neither contents nor
// formatting are removed. Cells that fn leaves unchanged are not touched.
func (s *Sheet) eachCell(r Rect, create bool, fn func(Addr, *lineFmt)) {
	area := (r.To.Col - r.From.Col + 1) * (r.To.Row - r.From.Row + 1)
	var addrs []Addr
	if create && area <= materializeLimit {
		for row := r.From.Row; row <= r.To.Row; row++ {
			for col := r.From.Col; col <= r.To.Col; col++ {
				addrs = append(addrs, Addr{Col: col, Row: row})
			}
		}
	} else {
		addrs = s.cellsIn(r)
	}
	for _, a := range addrs {
		want := s.effective(a)
		fn(a, &want)
		s.placeFormat(a, want)
	}
}
