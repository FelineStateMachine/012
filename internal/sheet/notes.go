package sheet

import (
	"strings"
	"unicode"
)

// Notes are Sheets' notes (Insert > Note): plain text kept on a cell,
// shown when the cell is active or hovered. A note is part of its cell
// (Cell.Note), so it moves with the cell when rows are inserted, deleted
// or sorted, copies with it, stays when the contents are cleared, and
// every change to one is an undo step like any other change to a cell.

// MaxNote caps a note's length in bytes, so a file or a paste can't
// make one without bound.
const MaxNote = 4096

// Note returns the note on the cell at a, or "".
func (s *Sheet) Note(a Addr) string {
	if c := s.cells.get(a); c != nil {
		return c.Note
	}
	return ""
}

// CleanNote tidies a note as SetNote stores it: control characters
// other than line breaks dropped, trailing space trimmed, at most
// MaxNote bytes.
func CleanNote(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	if len(text) > MaxNote {
		text = strings.ToValidUTF8(text[:MaxNote], "")
	}
	return text
}

// SetNote sets the note on the cell at a as one undo step; an empty note
// deletes it. A pivot table's results can't take notes: ErrPivotEdit;
// nor can the cells an array spills into: ErrSpillEdit.
func (s *Sheet) SetNote(a Addr, text string) error {
	if s.InPivot(Rect{From: a, To: a}) {
		return ErrPivotEdit
	}
	if _, ok := s.SpillAnchor(a); ok {
		return ErrSpillEdit
	}
	text = CleanNote(text)
	if s.Note(a) == text {
		return nil
	}
	label := "note on " + a.String()
	if text == "" {
		label = "delete note on " + a.String()
	}
	s.change(label, Rect{From: a, To: a}, func() { s.putNote(a, text) })
	return nil
}

// putNote stores a note without recording a step of its own.
func (s *Sheet) putNote(a Addr, text string) {
	old := s.cells.get(a)
	c := old.clone()
	if c == nil {
		c = &Cell{}
	}
	c.Note = text
	if c.Blank() && c.Format.IsZero() && c.Style.IsZero() && text == "" {
		c = nil
	}
	s.place(a, c)
}

// LoadNote sets a note as a loader does: without recording undo.
func (s *Sheet) LoadNote(a Addr, text string) {
	if text = CleanNote(text); a.Valid() && text != "" {
		s.putNote(a, text)
	}
}

// NotesIn returns the cells in r that have notes, in row-major order.
// It walks the cells the sheet holds, not r's addresses.
func (s *Sheet) NotesIn(r Rect) []Addr {
	var out []Addr
	for a := range s.cells.anyInRange(r) {
		if s.cells.get(a).Note != "" {
			out = append(out, a)
		}
	}
	sortAddrs(out)
	return out
}

// ClearNotes deletes the notes in r as one undo step and returns how many
// there were.
func (s *Sheet) ClearNotes(r Rect) int {
	notes := s.NotesIn(r)
	if len(notes) == 0 {
		return 0
	}
	s.change("delete notes in "+r.String(), r, func() {
		for _, a := range notes {
			s.putNote(a, "")
		}
	})
	return len(notes)
}
