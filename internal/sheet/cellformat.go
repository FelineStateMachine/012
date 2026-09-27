package sheet

import "github.com/FelineStateMachine/012/internal/formula"

// Formatting changes go through these methods rather than by editing
// cells directly, so history (undo) can hook a single place.

// materializeLimit bounds how many blank cells a formatting change
// creates. Formatting whole columns fits under it; for the whole sheet
// only cells with contents or formatting are changed.
const materializeLimit = 1 << 16

// DisplayFormat returns the format the cell at a is shown with: its own,
// or for Automatic, the one inferred from its formula (=DATE() shows a
// date, =SUM(B2:B4) of currency shows currency).
func (s *Sheet) DisplayFormat(a Addr) Format {
	c := s.cells.get(a)
	switch {
	case c == nil:
		return Format{}
	case c.Format.IsZero():
		return c.auto
	}
	return c.Format
}

// SetFormat gives every cell in r the number format f, including blank
// cells, which keep it for when something is typed.
func (s *Sheet) SetFormat(r Rect, f Format) {
	s.change("format "+r.String()+" as "+f.Kind.label(), r, func() {
		s.eachCell(r, !f.IsZero(), func(_ Addr, c *Cell) { c.Format = f })
	})
}

// SetStyle changes the text style of every cell in r with fn, e.g. to
// turn on bold while keeping italics. Callers wanting a specific undo
// label ("bold B2:B5") wrap it in Batch.
func (s *Sheet) SetStyle(r Rect, fn func(*Style)) {
	s.change("style "+r.String(), r, func() {
		s.eachCell(r, true, func(_ Addr, c *Cell) { fn(&c.Style) })
	})
}

// ClearFormatting resets the format and style of every cell in r, as
// Sheets' Format > Clear formatting.
func (s *Sheet) ClearFormatting(r Rect) {
	s.change("clear formatting "+r.String(), r, func() {
		s.eachCell(r, false, func(_ Addr, c *Cell) { c.Format, c.Style = Format{}, Style{} })
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
		s.eachCell(r, false, func(a Addr, c *Cell) {
			if !c.Blank() {
				c.Format = s.DisplayFormat(a).WithDecimals(delta, c.Value.Num)
			}
		})
	})
}

// eachCell applies fn to a copy of each cell in r and places the copy,
// so the change can be undone. With create, blank cells get a cell to
// hold the formatting (up to materializeLimit cells); cells left with
// neither contents nor formatting are removed. Cells that fn leaves
// unchanged are not touched.
func (s *Sheet) eachCell(r Rect, create bool, fn func(Addr, *Cell)) {
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
		old := s.cells.get(a)
		c := old.clone()
		if c == nil {
			c = &Cell{}
		}
		fn(a, c)
		switch {
		case c.Blank() && c.Format.IsZero() && c.Style.IsZero():
			if old != nil {
				s.place(a, nil)
			}
		case old == nil || c.Format != old.Format || c.Style != old.Style:
			s.place(a, c)
		}
	}
}

// isVolatile reports whether n calls a function whose result changes
// without its inputs changing (TODAY, NOW, RAND).
func isVolatile(n Node) bool {
	switch n := n.(type) {
	case formula.Unary:
		return isVolatile(n.X)
	case formula.Binary:
		return isVolatile(n.L) || isVolatile(n.R)
	case formula.Call:
		if funcOf(n).Volatile {
			return true
		}
		for _, a := range n.Args {
			if isVolatile(a) {
				return true
			}
		}
	}
	return false
}

// inferFormat picks the format Sheets shows a formula's result in when
// the cell is Automatic: dates from date functions, and otherwise the
// format of the first formatted input, so =B2+B3 of currency shows
// currency and a date plus days shows a date.
func inferFormat(n Node, at func(string, Addr) Format) Format {
	switch n := n.(type) {
	case formula.Ref:
		return at(n.Sheet, n.Addr)
	case formula.Range:
		return at(n.Sheet, n.Rect.From)
	case formula.Unary:
		if n.Op == "-" || n.Op == "+" {
			return inferFormat(n.X, at)
		}
	case formula.Binary:
		l, r := inferFormat(n.L, at), inferFormat(n.R, at)
		switch n.Op {
		case "+", "-":
			if n.Op == "-" && l.Kind.isTime() && r.Kind.isTime() {
				return Format{} // days between two dates
			}
			return firstFormat(l, r)
		case "*", "/":
			if l.Kind.isTime() || r.Kind.isTime() {
				return Format{}
			}
			return firstFormat(l, r)
		}
	case formula.Call:
		if funcOf(n).format != nil {
			return funcOf(n).format(n.Args, func(n Node) Format { return inferFormat(n, at) })
		}
	}
	return Format{}
}

func firstFormat(fs ...Format) Format {
	for _, f := range fs {
		if !f.IsZero() && f.Kind != FmtText {
			return f
		}
	}
	return Format{}
}

// Result formats for the function table.

// returns makes a function's result show in format f.
func returns(f Format) func([]Node, func(Node) Format) Format {
	return func([]Node, func(Node) Format) Format { return f }
}

// inherit makes a function's result take the format of its first
// formatted argument (SUM, MIN, ROUND...).
func inherit(args []Node, infer func(Node) Format) Format {
	for _, a := range args {
		if f := infer(a); !f.IsZero() && f.Kind != FmtText {
			return f
		}
	}
	return Format{}
}

// inheritFrom takes the format of argument i only.
func inheritFrom(i int) func([]Node, func(Node) Format) Format {
	return func(args []Node, infer func(Node) Format) Format {
		if i < len(args) {
			return infer(args[i])
		}
		return Format{}
	}
}
