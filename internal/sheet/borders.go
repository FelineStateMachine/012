package sheet

// Borders, as Sheets' Format > Borders. Each cell keeps the line on each
// of its four edges in its style, so borders fall back on rows and
// columns, travel with copies and go with Clear formatting as the rest
// of the style does. Neighbors share an edge, which shows the heavier of
// the lines either keeps; setting an edge of a range from inside clears
// the facing edge of the neighbor outside, so the last change shows.

// Line is how an edge is drawn.
type Line uint8

const (
	LineNone Line = iota
	LineThin
	LineThick
	LineDouble
)

var lineNames = [...]string{"", "thin", "thick", "double"}

// String returns the line's name as stored in files.
func (l Line) String() string {
	if int(l) < len(lineNames) {
		return lineNames[l]
	}
	return ""
}

// ParseLine is the inverse of Line.String.
func ParseLine(s string) (Line, bool) {
	for i, n := range lineNames {
		if n == s {
			return Line(i), true
		}
	}
	return LineNone, false
}

// Heavier is the line drawn where two edges meet: double over thick over
// thin.
func Heavier(a, b Line) Line { return max(a, b) }

// Borders are the lines on a cell's four edges, two bits each in the
// low byte, and their colors, four bits each above: ColorNone draws in
// the ink of the text (see Color and theme.CellBorder).
type Borders uint32

// Edge is one of a cell's edges.
type Edge uint8

const (
	EdgeTop Edge = iota
	EdgeBottom
	EdgeLeft
	EdgeRight
)

// BordersOf is the borders with these lines.
func BordersOf(top, bottom, left, right Line) Borders {
	return Borders(0).With(EdgeTop, top).With(EdgeBottom, bottom).With(EdgeLeft, left).With(EdgeRight, right)
}

// Line is the line on edge e.
func (b Borders) Line(e Edge) Line { return Line(b >> (2 * e) & 3) }

// Color is the color of edge e's line.
func (b Borders) Color(e Edge) Color { return Color(b >> (8 + 4*e) & 15) }

// Stroke is edge e's line and color.
func (b Borders) Stroke(e Edge) Stroke { return Stroke{b.Line(e), b.Color(e)} }

// With is b with edge e drawn with l, in the color it had; no line has
// no color.
func (b Borders) With(e Edge, l Line) Borders {
	b = b&^(3<<(2*e)) | Borders(l&3)<<(2*e)
	if l == LineNone {
		b = b.WithColor(e, ColorNone)
	}
	return b
}

// WithColor is b with edge e's line in color c.
func (b Borders) WithColor(e Edge, c Color) Borders {
	return b&^(15<<(8+4*e)) | Borders(c&15)<<(8+4*e)
}

// WithStroke is b with edge e drawn as st.
func (b Borders) WithStroke(e Edge, st Stroke) Borders {
	return b.With(e, st.Line).WithColor(e, st.Color)
}

// Stroke is a line and its color.
type Stroke struct {
	Line  Line
	Color Color
}

// heavier is the stroke drawn where a and b share an edge: the heavier
// line, and a's when they are as heavy.
func heavier(a, b Stroke) Stroke {
	if b.Line > a.Line {
		return b
	}
	return a
}

func (b Borders) Top() Line    { return b.Line(EdgeTop) }
func (b Borders) Bottom() Line { return b.Line(EdgeBottom) }
func (b Borders) Left() Line   { return b.Line(EdgeLeft) }
func (b Borders) Right() Line  { return b.Line(EdgeRight) }

// IsZero reports whether no edge has a line.
func (b Borders) IsZero() bool { return b == 0 }

// BorderKind is which edges of a range Format > Borders sets.
type BorderKind uint8

const (
	BorderAll    BorderKind = iota // every edge of every cell
	BorderOuter                    // the range's outline
	BorderInner                    // the edges between its cells
	BorderTop                      // its top edge
	BorderBottom                   //
	BorderLeft                     //
	BorderRight                    //
	BorderNone                     // no edges: removes every line
)

var borderKindNames = [...]string{"all", "outer", "inner", "top", "bottom", "left", "right", "none"}

// String names the kind, as undo labels and macros write it.
func (k BorderKind) String() string {
	if int(k) < len(borderKindNames) {
		return borderKindNames[k]
	}
	return ""
}

// ParseBorderKind is the inverse of BorderKind.String.
func ParseBorderKind(s string) (BorderKind, bool) {
	for i, n := range borderKindNames {
		if n == s {
			return BorderKind(i), true
		}
	}
	return 0, false
}

// edgeSet is which of a cell's edges a change sets.
type edgeSet struct{ top, bottom, left, right bool }

// edges are the edges k sets on the cell at a within r. Along the axis
// r covers whole (whole columns run down every row), every cell counts
// as inside, and so do lines (see lineAddr).
func (k BorderKind) edges(r Rect, a Addr) edgeSet {
	firstRow, lastRow := a.Row >= 0 && a.Row == r.From.Row, a.Row >= 0 && a.Row == r.To.Row
	firstCol, lastCol := a.Col >= 0 && a.Col == r.From.Col, a.Col >= 0 && a.Col == r.To.Col
	if r.AllRows() {
		firstRow, lastRow = false, false
	}
	if r.AllCols() {
		firstCol, lastCol = false, false
	}
	switch k {
	case BorderAll, BorderNone:
		return edgeSet{true, true, true, true}
	case BorderOuter:
		return edgeSet{firstRow, lastRow, firstCol, lastCol}
	case BorderInner:
		return edgeSet{!firstRow, !lastRow, !firstCol, !lastCol}
	case BorderTop:
		return edgeSet{top: firstRow}
	case BorderBottom:
		return edgeSet{bottom: lastRow}
	case BorderLeft:
		return edgeSet{left: firstCol}
	}
	return edgeSet{right: lastCol}
}

// apply sets the edges of e on b to st.
func (e edgeSet) apply(b *Borders, st Stroke) {
	for edge, set := range [...]bool{EdgeTop: e.top, EdgeBottom: e.bottom, EdgeLeft: e.left, EdgeRight: e.right} {
		if set {
			*b = b.WithStroke(Edge(edge), st)
		}
	}
}

// SetBorders draws the edges k names in r with line l, or removes every
// line of r for BorderNone, as one undo step. Neighbors outside r lose
// the lines on the edges they share with the ones set.
func (s *Sheet) SetBorders(r Rect, k BorderKind, l Line) {
	s.SetBorderStroke(r, k, Stroke{Line: l})
}

// SetBorderStroke is SetBorders with lines of a color. An outline of
// whole columns or rows draws the sheet's first and last edges too.
func (s *Sheet) SetBorderStroke(r Rect, k BorderKind, st Stroke) {
	label := k.String() + " border " + r.String() // "top border B2:C4"
	if k <= BorderInner {
		label = k.String() + " borders " + r.String()
	}
	if st.Color != ColorNone {
		label += " in " + st.Color.String()
	}
	if k == BorderNone {
		st, label = Stroke{}, "remove borders from "+r.String()
	}
	s.change(label, r, func() {
		s.eachFormat(r, true, func(a Addr, f *lineFmt) {
			k.edges(r, a).apply(&f.Style.Borders, st)
		})
		s.sheetEdges(r, k, st)
		s.clearFacing(r, k)
	})
}

// sheetEdges draws the edges of whole columns or rows that lie on the
// sheet's own edges, which k.edges leaves out along the axis they cover
// whole: the top of row 1 and the bottom of the last row under whole
// columns, the left of column A and the right of the last column beside
// whole rows. Across the whole sheet those are the lines of its first
// and last rows and columns; otherwise the cells along them.
func (s *Sheet) sheetEdges(r Rect, k BorderKind, st Stroke) {
	var e edgeSet
	switch k {
	case BorderOuter:
		e = edgeSet{true, true, true, true}
	case BorderTop, BorderBottom, BorderLeft, BorderRight:
		e = edgeSet{top: k == BorderTop, bottom: k == BorderBottom, left: k == BorderLeft, right: k == BorderRight}
	default:
		return
	}
	last := Addr{Col: MaxCols - 1, Row: MaxRows - 1}
	set := func(on bool, n Rect, edge edgeSet) {
		if on {
			s.eachFormat(n, true, func(_ Addr, f *lineFmt) { edge.apply(&f.Style.Borders, st) })
		}
	}
	if r.AllRows() {
		set(e.top, Rect{From: Addr{Col: r.From.Col}, To: Addr{Col: r.To.Col}}, edgeSet{top: true})
		set(e.bottom, Rect{From: Addr{Col: r.From.Col, Row: last.Row}, To: Addr{Col: r.To.Col, Row: last.Row}}, edgeSet{bottom: true})
	}
	if r.AllCols() {
		set(e.left, Rect{From: Addr{Row: r.From.Row}, To: Addr{Row: r.To.Row}}, edgeSet{left: true})
		set(e.right, Rect{From: Addr{Col: last.Col, Row: r.From.Row}, To: Addr{Col: last.Col, Row: r.To.Row}}, edgeSet{right: true})
	}
}

// clearFacing removes the lines of the neighbors of r on the edges that
// setting k on r draws.
func (s *Sheet) clearFacing(r Rect, k BorderKind) {
	outer := k.edges(r, r.From)
	last := k.edges(r, r.To)
	outer.bottom, outer.right = last.bottom, last.right
	strip := func(from, to Addr, e edgeSet) {
		n := Rect{From: from, To: to}
		if !n.From.Valid() || !n.To.Valid() {
			return
		}
		s.eachFormat(n, true, func(_ Addr, f *lineFmt) { e.apply(&f.Style.Borders, Stroke{}) })
	}
	if outer.top && !r.AllRows() {
		strip(Addr{Col: r.From.Col, Row: r.From.Row - 1}, Addr{Col: r.To.Col, Row: r.From.Row - 1}, edgeSet{bottom: true})
	}
	if outer.bottom && !r.AllRows() {
		strip(Addr{Col: r.From.Col, Row: r.To.Row + 1}, Addr{Col: r.To.Col, Row: r.To.Row + 1}, edgeSet{top: true})
	}
	if outer.left && !r.AllCols() {
		strip(Addr{Col: r.From.Col - 1, Row: r.From.Row}, Addr{Col: r.From.Col - 1, Row: r.To.Row}, edgeSet{right: true})
	}
	if outer.right && !r.AllCols() {
		strip(Addr{Col: r.To.Col + 1, Row: r.From.Row}, Addr{Col: r.To.Col + 1, Row: r.To.Row}, edgeSet{left: true})
	}
}

// EdgeAbove is the line on the edge between the cell at a and the one
// above it: the heavier of their facing lines.
func (s *Sheet) EdgeAbove(a Addr) Line { return s.StrokeAbove(a).Line }

// EdgeLeft is the line on the edge between the cell at a and the one to
// its left.
func (s *Sheet) EdgeLeft(a Addr) Line { return s.StrokeLeft(a).Line }

// StrokeAbove is EdgeAbove with its color: the heavier line's, or the
// cell's own where the lines are as heavy.
func (s *Sheet) StrokeAbove(a Addr) Stroke {
	st := s.CellStyle(a).Borders.Stroke(EdgeTop)
	if a.Row > 0 {
		st = heavier(st, s.CellStyle(Addr{Col: a.Col, Row: a.Row - 1}).Borders.Stroke(EdgeBottom))
	}
	return st
}

// StrokeLeft is EdgeLeft with its color.
func (s *Sheet) StrokeLeft(a Addr) Stroke {
	st := s.CellStyle(a).Borders.Stroke(EdgeLeft)
	if a.Col > 0 {
		st = heavier(st, s.CellStyle(Addr{Col: a.Col - 1, Row: a.Row}).Borders.Stroke(EdgeRight))
	}
	return st
}
