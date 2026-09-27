package sheet

import (
	"bytes"
	"strings"
	"testing"
)

func TestNotes(t *testing.T) {
	s := New()
	s.Set(at("B2"), "10")
	if err := s.SetNote(at("B2"), "Checked\r\nwith \x1bthe bank  "); err != nil {
		t.Fatal(err)
	}
	if got := s.Note(at("B2")); got != "Checked\nwith the bank" {
		t.Errorf("note %q", got)
	}
	// Typing over the cell and clearing it keep the note, as in Sheets.
	s.Set(at("B2"), "12")
	s.EraseRange(rng("A1:C3"))
	if got := s.Note(at("B2")); got != "Checked\nwith the bank" {
		t.Errorf("after clearing: note %q", got)
	}
	// A note on a blank cell is a cell of its own, without contents.
	s.SetNote(at("D5"), "Later")
	if c := s.Cell(at("D5")); c == nil || !c.Blank() || c.Note != "Later" {
		t.Errorf("blank noted cell %+v", c)
	}
	// Notes move with their rows.
	s.InsertRows(0, 2)
	if s.Note(at("B4")) == "" || s.Note(at("D7")) != "Later" {
		t.Errorf("after inserting rows: %q %q", s.Note(at("B4")), s.Note(at("D7")))
	}
	if got := s.NotesIn(rng("A1:Z99")); len(got) != 2 || got[0] != at("B4") {
		t.Errorf("NotesIn %v", got)
	}
	// Undo steps back one change at a time.
	s.Undo()
	if s.Note(at("D5")) != "Later" {
		t.Errorf("undo insert: %q", s.Note(at("D5")))
	}
	s.Undo()
	if s.Cell(at("D5")) != nil {
		t.Errorf("undo note: %+v", s.Cell(at("D5")))
	}
	// Deleting the last note of a blank, plain cell removes the cell.
	if n := s.ClearNotes(rng("A1:C3")); n != 1 || s.Cell(at("B2")) != nil {
		t.Errorf("ClearNotes = %d, cell %+v", n, s.Cell(at("B2")))
	}
}

func TestNotesCopyAndFill(t *testing.T) {
	s := New()
	s.Set(at("A1"), "1")
	s.SetNote(at("A1"), "Source")
	s.SetNote(at("B1"), "Mine")
	if _, err := s.Paste(s.Copy(rng("A1")), rng("A2"), false); err != nil {
		t.Fatal(err)
	}
	if s.Note(at("A2")) != "Source" {
		t.Errorf("pasted note %q", s.Note(at("A2")))
	}
	// Ctrl+Enter fills contents, each cell keeping its own note.
	s.FillEntry(rng("A1:B1"), at("A1"), "7")
	if s.Note(at("A1")) != "Source" || s.Note(at("B1")) != "Mine" || s.Value(at("B1")).Num != 7 {
		t.Errorf("fill: %q %q %v", s.Note(at("A1")), s.Note(at("B1")), s.Value(at("B1")))
	}
}

func TestNotesInPivotRefused(t *testing.T) {
	s := New()
	for a, v := range map[string]string{"A1": "Region", "B1": "Sales", "A2": "East", "B2": "5"} {
		s.Set(at(a), v)
	}
	p, err := s.Book().CreatePivot(s, rng("A1:B2"), "", NewPivot(s, rng("A1:B2")))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := p.PivotRange()
	if err := p.SetNote(out.From, "x"); err != ErrPivotEdit {
		t.Errorf("note on pivot results: %v", err)
	}
}

func TestNotesFile(t *testing.T) {
	s := New()
	s.Set(at("A1"), "Rent")
	s.SetNote(at("A1"), "Due on\nthe 1st")
	s.SetNote(at("C3"), "Blank")
	var b bytes.Buffer
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"A1": {"input":"Rent","note":"Due on\nthe 1st"}`) {
		t.Errorf("file:\n%s", b.String())
	}
	got, err := Read(&b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Note(at("A1")) != "Due on\nthe 1st" || got.Note(at("C3")) != "Blank" || got.Value(at("A1")).Str != "Rent" {
		t.Errorf("read back %q %q", got.Note(at("A1")), got.Note(at("C3")))
	}
}
