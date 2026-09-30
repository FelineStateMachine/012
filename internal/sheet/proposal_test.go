package sheet

import (
	"errors"
	"testing"
)

// proposalBook is a workbook with 1, 2, 3 in A1:A3 and their sum in A4,
// its history shared by two authors, 1 the person and 2 the agent.
func proposalBook(t *testing.T) *Workbook {
	t.Helper()
	w := New().Book()
	s := w.Sheet(0)
	for i, in := range []string{"1", "2", "3", "=SUM(A1:A3)"} {
		if err := s.Set(Addr{Row: i}, in); err != nil {
			t.Fatal(err)
		}
	}
	w.ShareHistory()
	return w
}

func setCells(entries map[Addr]string) func(*Workbook) error {
	return func(w *Workbook) error {
		for a, in := range entries {
			if err := w.Sheet(0).Set(a, in); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestProposeLeavesTheWorkbook(t *testing.T) {
	w := proposalBook(t)
	state := w.StateID()
	p, err := Propose(w, "set", setCells(map[Addr]string{{Row: 0}: "10", {Row: 1, Col: 1}: "x"}))
	if err != nil {
		t.Fatal(err)
	}
	if w.StateID() != state || w.Sheet(0).Value(Addr{Row: 3}).Num != 6 {
		t.Fatal("proposing changed the workbook")
	}
	if len(p.Cells) != 2 || p.Whole() || p.Remote {
		t.Fatalf("cells %+v, other %v", p.Cells, p.Other)
	}
	if c := p.Cells[0]; c.At != (Addr{Row: 0}) || c.Was != "1" || c.Now != "10" {
		t.Fatalf("first cell %+v", c)
	}
	if got := p.Result().Sheet(0).Value(Addr{Row: 3}).Num; got != 15 {
		t.Fatalf("the copy's sum is %v", got)
	}
}

func TestProposalAcceptedWholeIsOneStepOfItsAuthor(t *testing.T) {
	w := proposalBook(t)
	p, _ := Propose(w, "fix", setCells(map[Addr]string{{Row: 0}: "10", {Row: 1}: "20"}))
	w.SetAuthor(1)
	w.Sheet(0).Set(Addr{Col: 2}, "mine")
	w.SetAuthor(2)
	if err := p.Apply(w, nil); err != nil {
		t.Fatal(err)
	}
	if got := w.Sheet(0).Value(Addr{Row: 3}).Num; got != 33 {
		t.Fatalf("sum %v after accepting", got)
	}
	w.SetAuthor(2)
	if w.UndoLabel() != "fix" {
		t.Fatalf("the agent's undo is %q", w.UndoLabel())
	}
	w.Undo()
	if got := w.Sheet(0).Value(Addr{Row: 3}).Num; got != 6 || w.Sheet(0).Cell(Addr{Col: 2}).Input != "mine" {
		t.Fatal("the agent's undo took back more than its step")
	}
}

func TestProposalAcceptedByCell(t *testing.T) {
	w := proposalBook(t)
	p, _ := Propose(w, "fix", setCells(map[Addr]string{{Row: 0}: "10", {Row: 1}: "20", {Row: 2}: ""}))
	if len(p.Cells) != 3 {
		t.Fatalf("cells %+v", p.Cells)
	}
	if err := p.Apply(w, func(i int) bool { return i != 1 }); err != nil {
		t.Fatal(err)
	}
	s := w.Sheet(0)
	if inputOf(s, Addr{Row: 0}) != "10" || inputOf(s, Addr{Row: 1}) != "2" || inputOf(s, Addr{Row: 2}) != "" {
		t.Fatal("the picked cells weren't the ones set")
	}
	if got := s.Value(Addr{Row: 3}).Num; got != 12 {
		t.Fatalf("sum %v", got)
	}
}

func TestProposalRejectedChangesNothing(t *testing.T) {
	w := proposalBook(t)
	state := w.StateID()
	p, _ := Propose(w, "fix", setCells(map[Addr]string{{Row: 0}: "10"}))
	if err := p.Apply(w, func(int) bool { return false }); err != nil || w.StateID() != state {
		t.Fatal("picking no cell made a step")
	}
}

func TestProposalKeepsFormats(t *testing.T) {
	w := proposalBook(t)
	p, _ := Propose(w, "bold", func(w *Workbook) error {
		w.Sheet(0).SetStyle(Rect{From: Addr{Row: 0}, To: Addr{Row: 0}}, func(st *Style) { st.Bold = true })
		return nil
	})
	if len(p.Cells) != 1 || !p.Cells[0].Look || p.Cells[0].Was != p.Cells[0].Now {
		t.Fatalf("cells %+v", p.Cells)
	}
	p.Apply(w, nil)
	if !w.Sheet(0).Cell(Addr{Row: 0}).Style.Bold {
		t.Fatal("the style wasn't applied")
	}
}

func TestProposalOfRowsIsWhole(t *testing.T) {
	w := proposalBook(t)
	p, _ := Propose(w, "insert", func(w *Workbook) error { return w.Sheet(0).InsertRows(0, 1) })
	if !p.Whole() || len(p.Sheets) != 1 || p.Sheets[0] != "Sheet1" {
		t.Fatalf("other %v, sheets %v", p.Other, p.Sheets)
	}
	if err := p.Apply(w, func(int) bool { return false }); !errors.Is(err, ErrWhole) && len(p.Cells) > 0 {
		t.Fatalf("part of a whole change: %v", err)
	}
	if err := p.Apply(w, nil); err != nil {
		t.Fatal(err)
	}
	if w.Sheet(0).Cell(Addr{Row: 1}).Input != "1" {
		t.Fatal("the row wasn't inserted")
	}
}

func TestProposalNotesRemoteFormulas(t *testing.T) {
	w := proposalBook(t)
	p, _ := Propose(w, "ask", setCells(map[Addr]string{{Col: 1}: `=JEV.CLASSIFY(A1, "parity", "odd, even")`}))
	if !p.Remote {
		t.Fatalf("a JEV formula isn't remote: %+v", p.Cells)
	}
}

func TestAdoptKeepsEarlierStepsTheirs(t *testing.T) {
	w := New().Book()
	w.Sheet(0).Set(Addr{}, "1")
	w.ShareHistory()
	w.Adopt(1)
	w.SetAuthor(1)
	if !w.CanUndo() {
		t.Fatal("the adopted step isn't the author's")
	}
	w.SetAuthor(2)
	if w.CanUndo() {
		t.Fatal("another author can undo it")
	}
}

// inputOf is the input at a, "" for a blank cell.
func inputOf(s *Sheet, a Addr) string {
	if c := s.Cell(a); c != nil {
		return c.Input
	}
	return ""
}
