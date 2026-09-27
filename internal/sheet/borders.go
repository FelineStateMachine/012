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

// Borders are the lines on a cell's four edges.
type Borders struct {
	Top, Bottom, Left, Right Line
}

// IsZero reports whether no edge has a line.
func (b Borders) IsZero() bool { return b == Borders{} }

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

// apply sets the edges of e on b to l.
func (e edgeSet) apply(b *Borders, l Line) {
	for _, on := range [...]struct {
		set  bool
		edge *Line
	}{{e.top, &b.Top}, {e.bottom, &b.Bottom}, {e.left, &b.Left}, {e.right, &b.Right}} {
		if on.set {
			*on.edge = l
		}
	}
}

// SetBorders draws the edges k names in r with line l, or removes every
// line of r for BorderNone, as one undo step. Neighbors outside r lose
// the lines on the edges they share with the ones set.
func (s *Sheet) SetBorders(r Rect, k BorderKind, l Line) {
	label := "borders " + k.String() + " " + r.String()
	if k == BorderNone {
		l, label = LineNone, "remove borders from "+r.String()
	}
	s.change(label, r, func() {
		s.eachFormat(r, true, func(a Addr, f *lineFmt) {
			k.edges(r, a).apply(&f.Style.Borders, l)
		})
		s.clearFacing(r, k)
	})
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
		s.eachFormat(n, true, func(_ Addr, f *lineFmt) { e.apply(&f.Style.Borders, LineNone) })
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
func (s *Sheet) EdgeAbove(a Addr) Line {
	l := s.CellStyle(a).Borders.Top
	if a.Row > 0 {
		l = Heavier(l, s.CellStyle(Addr{Col: a.Col, Row: a.Row - 1}).Borders.Bottom)
	}
	return l
}

// EdgeLeft is the line on the edge between the cell at a and the one to
// its left.
func (s *Sheet) EdgeLeft(a Addr) Line {
	l := s.CellStyle(a).Borders.Left
	if a.Col > 0 {
		l = Heavier(l, s.CellStyle(Addr{Col: a.Col - 1, Row: a.Row}).Borders.Right)
	}
	return l
}
