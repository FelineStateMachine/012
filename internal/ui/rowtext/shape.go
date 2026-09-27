package rowtext

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Wrap breaks text into lines at most w columns wide, as Sheets wraps a
// cell: at spaces where it can, inside a word longer than a line, and at
// every line break the text holds. It returns at least one line.
func Wrap(text string, w int) []string {
	w = max(w, 1)
	var out []string
	for para := range strings.SplitSeq(text, "\n") {
		out = wrapLine(out, para, w)
	}
	return out
}

// wrapLine appends the lines of one paragraph to out.
func wrapLine(out []string, para string, w int) []string {
	line, lw := "", 0
	for word := range strings.FieldsSeq(para) {
		ww := ansi.StringWidth(word)
		switch {
		case lw > 0 && lw+1+ww <= w:
			line, lw = line+" "+word, lw+1+ww
			continue
		case lw > 0:
			out = append(out, line)
		}
		for ww > w { // a word longer than a line breaks where it must
			c := textCutter{s: word}
			head := c.cut(0, w)
			if head == "" { // a character wider than the line
				n, _ := nextCluster(word)
				head = word[:n]
			}
			out = append(out, head)
			word = word[len(head):]
			ww = ansi.StringWidth(word)
		}
		line, lw = word, ww
	}
	return append(out, line)
}

// Shape is how a row is laid out: whether a border line comes above it,
// and how many lines its cells take.
type Shape struct {
	Rule  bool
	Lines int
}

// Shapes works out the shape of rows as the grid asks for them, and
// keeps them until the sheet changes.
type Shapes struct {
	s       *sheet.Sheet
	version uint64
	rows    map[int]Shape
}

// Row returns the shape of row on s: its height when one is set, or
// enough lines for the text its cells wrap, and a rule line when a
// border lies along its top. Rows of a sheet that nothing shapes are a
// line each.
func (x *Shapes) Row(s *sheet.Sheet, row int) Shape {
	if !s.Shaped() {
		return Shape{Lines: 1}
	}
	if x.s != s || x.version != s.Version() || x.rows == nil {
		x.s, x.version, x.rows = s, s.Version(), map[int]Shape{}
	}
	if sh, ok := x.rows[row]; ok {
		return sh
	}
	sh := Shape{Rule: s.RuleAbove(row), Lines: fit(s, row)}
	x.rows[row] = sh
	return sh
}

// fit is how many lines row takes: its height, or enough for the text
// it wraps.
func fit(s *sheet.Sheet, row int) int {
	if h, ok := s.RowHeight(row); ok {
		return h
	}
	n := 1
	for _, c := range s.WrappedIn(row) {
		a := sheet.Addr{Col: c, Row: row}
		v := s.Value(a)
		if v.Kind != sheet.Text {
			continue
		}
		n = max(n, len(Wrap(v.Str, s.ColWidth(c)-2)))
	}
	return min(n, sheet.MaxRowHeight)
}

// Merged lays out a merged cell m over its columns from lo to hi (those
// on screen): its top-left cell's value when show is set, on the line of
// the merge that shows it, centered unless that cell is aligned and cut
// to the merge's width; blank otherwise.
func Merged(s *sheet.Sheet, m sheet.Rect, lo, hi int, show bool) []Span {
	x := make([]int, hi-lo+2) // where each column starts, from the merge's first
	x[0] = s.ColsWidth(m.From.Col, lo-1)
	for c := lo; c <= hi; c++ {
		x[c-lo+1] = x[c-lo] + s.ColWidth(c)
	}
	w := s.ColsWidth(m.From.Col, m.To.Col)
	out := make([]Span, hi-lo+1)
	for c := lo; c <= hi; c++ {
		out[c-lo] = Span{Trail: s.ColWidth(c), Owner: m.From.Col}
	}
	cell := s.Cell(m.From)
	if !show || cell.Blank() {
		return out
	}
	f := s.DisplayFormat(m.From)
	st := s.CellStyle(m.From)
	text, align := sheet.Display(cell.Value, f, w)
	if st.Align != sheet.AlignAuto && align != sheet.AlignFill {
		align = st.Align
	} else if align != sheet.AlignFill {
		align = sheet.AlignCenter
	}
	text = ansi.Truncate(text, max(w-2, 0), "")
	tw := ansi.StringWidth(text)
	start := (w - tw) / 2
	switch align {
	case sheet.AlignLeft:
		start = 1
	case sheet.AlignRight:
		start = w - 1 - tw
	case sheet.AlignFill:
		start = 0
	}
	cut := textCutter{s: text}
	for c := lo; c <= hi; c++ {
		kx0, kx1 := x[c-lo], x[c-lo+1]
		sp := Span{Trail: kx1 - kx0, Style: st, Owner: m.From.Col}
		if seg0, seg1 := max(start, kx0), min(start+tw, kx1); seg1 > seg0 {
			sp.Lead, sp.Text, sp.Trail = seg0-kx0, cut.cut(seg0-start, seg1-start), kx1-seg1
		}
		out[c-lo] = sp
	}
	return out
}
