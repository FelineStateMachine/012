package lineedit

import (
	"slices"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Area is the multi-line editor of a notebook's cells: Line's text and
// caret, with line breaks, soft-wrapped to a width so no text is ever
// hidden. A line longer than the width breaks before a pipe (|) where it
// can, then after a space, then anywhere; the rows it continues on are
// indented by Indent. Up and Down, Home and End move by those rows.
type Area struct {
	Line
	goal int // one past the column Up and Down keep to, 0 for the caret's own
}

// Indent is how far a wrapped line's continuation rows are indented.
const Indent = 2

// Row is one row of wrapped text: Buf[Start:End], a continuation of the
// row before it (drawn Indent further in) when Cont is set.
type Row struct {
	Start, End int
	Cont       bool
}

// SetText replaces the text, with the caret at its end.
func (a *Area) SetText(text string) {
	a.Buf, a.goal = nil, 0
	a.Pos = 0
	a.InsertText(text)
}

// InsertText types text at the caret, line breaks included; a tab is
// two spaces, and other control characters are dropped.
func (a *Area) InsertText(text string) {
	a.goal = 0
	for _, r := range text {
		switch {
		case r == '\r':
			continue
		case r == '\t':
			a.insert(' ', ' ')
		case r == '\n' || unicode.IsPrint(r):
			a.insert(r)
		}
	}
}

func (a *Area) insert(rs ...rune) {
	a.Buf = slices.Insert(a.Buf, a.Pos, rs...)
	a.Pos += len(rs)
}

// Wrap lays buf out in rows at most width columns wide.
func Wrap(buf []rune, width int) []Row {
	width = max(width, Indent+4)
	var rows []Row
	start := 0
	for i := 0; i <= len(buf); i++ {
		if i < len(buf) && buf[i] != '\n' {
			continue
		}
		rows = wrapLine(rows, buf, start, i, width)
		start = i + 1
	}
	return rows
}

// wrapLine adds the rows of the line buf[from:to].
func wrapLine(rows []Row, buf []rune, from, to, width int) []Row {
	cont := false
	for {
		limit := width
		if cont {
			limit -= Indent
		}
		end, w := from, 0
		for end < to {
			rw := runeWidth(buf[end])
			if w+rw > limit {
				break
			}
			w += rw
			end++
		}
		if end >= to {
			return append(rows, Row{Start: from, End: to, Cont: cont})
		}
		end = breakAt(buf, from, end)
		rows = append(rows, Row{Start: from, End: end, Cont: cont})
		from, cont = end, true
	}
}

// breakAt is where a row that would run to end breaks: before the last
// pipe, else after the last space, else at end.
func breakAt(buf []rune, from, end int) int {
	for i := end; i > from+1; i-- {
		if buf[i] == '|' && buf[i-1] == ' ' {
			return i
		}
	}
	for i := end; i > from+1; i-- {
		if buf[i-1] == ' ' {
			return i
		}
	}
	return end
}

func runeWidth(r rune) int { return max(ansi.StringWidth(string(r)), 1) }

// RowWidth is the columns buf[from:to] takes.
func RowWidth(buf []rune, from, to int) int {
	w := 0
	for _, r := range buf[from:to] {
		w += runeWidth(r)
	}
	return w
}

// Caret is the row the caret is on among rows, and its column there,
// continuation rows' indent included.
func Caret(rows []Row, buf []rune, pos int) (row, col int) {
	for i, r := range rows {
		last := i == len(rows)-1 || !rows[i+1].Cont
		if pos >= r.Start && (pos < r.End || pos == r.End && last) {
			col = RowWidth(buf, r.Start, pos)
			if r.Cont {
				col += Indent
			}
			return i, col
		}
	}
	return max(len(rows)-1, 0), 0
}

// posAt is the position in row r nearest column col.
func posAt(r Row, buf []rune, col int, last bool) int {
	if r.Cont {
		col -= Indent
	}
	w := 0
	for i := r.Start; i < r.End; i++ {
		rw := runeWidth(buf[i])
		if w+rw > col {
			return i
		}
		w += rw
	}
	if !last && r.End > r.Start {
		return r.End - 1 // the next row starts at End
	}
	return r.End
}

// Key applies an editing key at width, reporting whether it was one.
// Enter breaks the line, keeping its indent.
func (a *Area) Key(k tea.KeyPressMsg, width int) bool {
	key := k.String()
	rows := Wrap(a.Buf, width)
	row, col := Caret(rows, a.Buf, a.Pos)
	switch key {
	case "up", "down":
		if a.goal == 0 {
			a.goal = col + 1
		}
		to := row - 1
		if key == "down" {
			to = row + 1
		}
		if to < 0 || to >= len(rows) {
			return true
		}
		last := to == len(rows)-1 || !rows[to+1].Cont
		a.Pos = posAt(rows[to], a.Buf, a.goal-1, last)
		return true
	case "home":
		a.Pos = rows[row].Start
	case "end":
		last := row == len(rows)-1 || !rows[row+1].Cont
		a.Pos = rows[row].End
		if !last {
			a.Pos = max(rows[row].End-1, rows[row].Start)
		}
	case "ctrl+a":
		a.Pos = a.lineStart()
	case "ctrl+e":
		a.Pos = a.lineEnd()
	case "enter":
		indent := a.indent()
		a.insert('\n')
		a.insert(indent...)
	case "left", "right", "backspace", "delete":
		a.Line.Key(k)
	default:
		text := Typed(k)
		if text == "" {
			return false
		}
		a.InsertText(text)
	}
	a.goal = 0
	return true
}

// lineStart and lineEnd are the ends of the caret's line.
func (a *Area) lineStart() int {
	i := a.Pos
	for i > 0 && a.Buf[i-1] != '\n' {
		i--
	}
	return i
}

func (a *Area) lineEnd() int {
	i := a.Pos
	for i < len(a.Buf) && a.Buf[i] != '\n' {
		i++
	}
	return i
}

// indent is the spaces the caret's line starts with.
func (a *Area) indent() []rune {
	var out []rune
	for i := a.lineStart(); i < len(a.Buf) && a.Buf[i] == ' '; i++ {
		out = append(out, ' ')
	}
	return out
}

// Offset is the caret as a byte offset into the text, what a completer
// or checker is given.
func (a *Area) Offset() int { return len(string(a.Buf[:a.Pos])) }
