package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"012/internal/sheet"
)

// Undo and redo, fill, inserting and deleting rows and columns, and F4 in
// formulas. The engine does the work and records each change as one undo
// step; this file maps commands onto the selection and reports back on the
// context line.

func init() {
	register(
		&command{id: "edit.undo", title: "Undo", desc: "Undo the last change", run: (*Model).undo},
		&command{id: "edit.redo", title: "Redo", desc: "Redo the last undone change", run: (*Model).redo},
		&command{id: "edit.fill_down", title: "Fill down", desc: "Copy the top row of the selection into the rows below it", run: func(m *Model) tea.Cmd {
			return m.fill(m.sheet.FillDown)
		}},
		&command{id: "edit.fill_right", title: "Fill right", desc: "Copy the left column of the selection into the columns right of it", run: func(m *Model) tea.Cmd {
			return m.fill(m.sheet.FillRight)
		}},
		&command{id: "insert.row_above", title: "Insert rows above", desc: "Insert as many rows above the selection as it spans", run: func(m *Model) tea.Cmd {
			r := m.selection()
			return m.structural(m.sheet.InsertRows(r.From.Row, r.To.Row-r.From.Row+1))
		}},
		&command{id: "insert.row_below", title: "Insert rows below", desc: "Insert as many rows below the selection as it spans", run: func(m *Model) tea.Cmd {
			r := m.selection()
			if r.To.Row == sheet.MaxRows-1 {
				return m.structural(sheet.ErrPushedOff)
			}
			return m.structural(m.sheet.InsertRows(r.To.Row+1, r.To.Row-r.From.Row+1))
		}},
		&command{id: "insert.col_left", title: "Insert columns left", desc: "Insert as many columns left of the selection as it spans", run: func(m *Model) tea.Cmd {
			r := m.selection()
			return m.structural(m.sheet.InsertCols(r.From.Col, r.To.Col-r.From.Col+1))
		}},
		&command{id: "insert.col_right", title: "Insert columns right", desc: "Insert as many columns right of the selection as it spans", run: func(m *Model) tea.Cmd {
			r := m.selection()
			if r.To.Col == sheet.MaxCols-1 {
				return m.structural(sheet.ErrPushedOff)
			}
			return m.structural(m.sheet.InsertCols(r.To.Col+1, r.To.Col-r.From.Col+1))
		}},
		&command{id: "delete.row", title: "Delete rows", desc: "Delete the selected rows; references to them become #REF!", run: func(m *Model) tea.Cmd {
			r := m.selection()
			m.sheet.DeleteRows(r.From.Row, r.To.Row-r.From.Row+1)
			return nil
		}},
		&command{id: "delete.col", title: "Delete columns", desc: "Delete the selected columns; references to them become #REF!", run: func(m *Model) tea.Cmd {
			r := m.selection()
			m.sheet.DeleteCols(r.From.Col, r.To.Col-r.From.Col+1)
			return nil
		}},
		// Sheets' Ctrl+Alt+= and Ctrl+Alt+- act on rows, or on columns when
		// whole columns are selected.
		&command{id: "insert.selection", title: "Insert rows or columns", desc: "Insert rows above the selection, or columns left of it when whole columns are selected", run: func(m *Model) tea.Cmd {
			if m.whole == wholeCols {
				return m.runCommand("insert.col_left")
			}
			return m.runCommand("insert.row_above")
		}},
		&command{id: "delete.selection", title: "Delete rows or columns", desc: "Delete the selected rows, or columns when whole columns are selected", run: func(m *Model) tea.Cmd {
			if m.whole == wholeCols {
				return m.runCommand("delete.col")
			}
			return m.runCommand("delete.row")
		}},
	)
	keymap["ctrl+alt+="] = "insert.selection"
	keymap["ctrl+alt+-"] = "delete.selection"
	keymap["ctrl+z"] = "edit.undo"
	keymap["ctrl+y"] = "edit.redo"
	keymap["ctrl+shift+z"] = "edit.redo"
	keymap["ctrl+d"] = "edit.fill_down"
	keymap["ctrl+r"] = "edit.fill_right"
}

func (m *Model) undo() tea.Cmd {
	c, ok := m.sheet.Undo()
	m.afterHistory("Undid", "Nothing to undo", c, ok)
	return nil
}

func (m *Model) redo() tea.Cmd {
	c, ok := m.sheet.Redo()
	m.afterHistory("Redid", "Nothing to redo", c, ok)
	return nil
}

// afterHistory selects what an undo or redo changed, as Sheets does, and
// says what it was on the context line.
func (m *Model) afterHistory(verb, none string, c sheet.Change, ok bool) {
	if !ok {
		m.note = none
		return
	}
	m.note = verb + ": " + c.Label
	m.selectRect(c.Focus)
}

// selectRect selects r, as whole columns or rows when it spans the sheet.
func (m *Model) selectRect(r sheet.Rect) {
	allRows := r.From.Row == 0 && r.To.Row == sheet.MaxRows-1
	allCols := r.From.Col == 0 && r.To.Col == sheet.MaxCols-1
	switch {
	case allRows && allCols:
		m.selecting, m.whole, m.ext = true, wholeAll, m.cur
	case allRows:
		m.cur.Col = r.From.Col
		m.ext = sheet.Addr{Col: r.To.Col, Row: m.cur.Row}
		m.selecting, m.whole = true, wholeCols
	case allCols:
		m.cur.Row = r.From.Row
		m.ext = sheet.Addr{Col: m.cur.Col, Row: r.To.Row}
		m.selecting, m.whole = true, wholeRows
	default:
		m.cur, m.ext = r.From, r.To
		m.selecting, m.whole = r.From != r.To, wholeNone
	}
}

func (m *Model) fill(fn func(sheet.Rect) (sheet.Rect, error)) tea.Cmd {
	if _, err := fn(m.selection()); err != nil {
		m.fail(err.Error())
	}
	return nil
}

// structural reports an insert that couldn't be done.
func (m *Model) structural(err error) tea.Cmd {
	if err != nil {
		m.fail(err.Error())
	}
	return nil
}

// fillEntry stores the entry being typed in every selected cell, adjusting
// references as if it were copied from the active cell (Ctrl+Enter).
func (m *Model) fillEntry() bool {
	input := string(m.buf)
	if err := m.sheet.FillEntry(m.selection(), m.cur, input); err != nil {
		m.entryError(err, input)
		return false
	}
	m.cancelEntry()
	return true
}

// entryError keeps an entry that can't be stored in EDIT mode, with the
// caret at the problem and the reason on line 3.
func (m *Model) entryError(err error, input string) {
	var pe *sheet.ParseError
	m.mode = modeEdit
	m.hint = err.Error()
	if errors.As(err, &pe) {
		m.bufPos = utf8.RuneCountInString(input[:min(pe.Pos, len(input))])
	}
}

// toggleAbsolute cycles the reference at the caret through A1, $A$1, A$1
// and $A1, as F4 does in Sheets. A range cycles both corners.
func (m *Model) toggleAbsolute() {
	if buf, pos, ok := cycleRef(m.buf, m.bufPos); ok {
		m.buf, m.bufPos = buf, pos
	}
}

// cycleRef finds the reference (or range) at or just before pos in a
// formula and returns the text with its absolute markers cycled, and the
// caret moved to the reference's end.
func cycleRef(buf []rune, pos int) ([]rune, int, bool) {
	quoted := make([]bool, len(buf))
	in := false
	for i, r := range buf {
		if r == '"' {
			in = !in
		}
		quoted[i] = in || r == '"'
	}
	isRefRune := func(i int) bool {
		r := buf[i]
		return !quoted[i] && (r == '$' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}
	// word returns the bounds of the run of reference characters around i.
	word := func(i int) (int, int) {
		start, end := i, i
		for start > 0 && isRefRune(start-1) {
			start--
		}
		for end < len(buf) && isRefRune(end) {
			end++
		}
		return start, end
	}
	isRef := func(start, end int) bool {
		if start == end || start > 0 && buf[start-1] == '@' || end < len(buf) && buf[end] == '(' {
			return false
		}
		_, ok := sheet.ParseAddr(string(buf[start:end]))
		return ok
	}
	start, end := word(pos)
	if !isRef(start, end) {
		return nil, 0, false
	}
	// Widen to a range written A1:B2 or A1..B2.
	sep := func(i int) int {
		switch {
		case i < len(buf) && buf[i] == ':':
			return 1
		case i+1 < len(buf) && buf[i] == '.' && buf[i+1] == '.':
			return 2
		}
		return 0
	}
	corners := [][2]int{{start, end}}
	if n := sep(end); n > 0 {
		if s2, e2 := word(end + n); s2 == end+n && isRef(s2, e2) {
			corners = append(corners, [2]int{s2, e2})
		}
	} else {
		for n := 1; n <= 2 && start-n > 0; n++ {
			if sep(start-n) == n {
				if s0, e0 := word(start - n - 1); e0 == start-n && isRef(s0, e0) {
					corners = [][2]int{{s0, e0}, {start, end}}
				}
				break
			}
		}
	}
	next := nextMarkers(string(buf[corners[0][0]:corners[0][1]]))
	out := slices.Clone(buf[:corners[0][0]])
	for i, c := range corners {
		if i > 0 {
			out = append(out, buf[corners[i-1][1]:c[0]]...)
		}
		out = append(out, []rune(withMarkers(string(buf[c[0]:c[1]]), next))...)
	}
	caret := len(out)
	out = append(out, buf[corners[len(corners)-1][1]:]...)
	return out, caret, true
}

// nextMarkers returns the absolute markers that follow ref's in F4's
// cycle: A1 -> $A$1 -> A$1 -> $A1 -> A1.
func nextMarkers(ref string) markers {
	col := strings.HasPrefix(ref, "$")
	row := strings.Contains(ref[1:], "$")
	switch {
	case !col && !row:
		return markers{true, true}
	case col && row:
		return markers{false, true}
	case row:
		return markers{true, false}
	}
	return markers{}
}

// markers says which parts of a reference are absolute.
type markers struct{ col, row bool }

// withMarkers rewrites ref with the given absolute markers.
func withMarkers(ref string, marks markers) string {
	ref = strings.ToUpper(strings.ReplaceAll(ref, "$", ""))
	i := strings.IndexAny(ref, "0123456789")
	var b strings.Builder
	if marks.col {
		b.WriteByte('$')
	}
	b.WriteString(ref[:i])
	if marks.row {
		b.WriteByte('$')
	}
	b.WriteString(ref[i:])
	return b.String()
}

// countCells describes a paste for the context line.
func countCells(r sheet.Rect) string {
	n := (r.To.Col - r.From.Col + 1) * (r.To.Row - r.From.Row + 1)
	if n == 1 {
		return "1 cell"
	}
	return fmt.Sprintf("%d cells", n)
}
