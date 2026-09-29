package sheet

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// sent adds a notebook cell named name to a new notebook tab and sends
// its output to s at a, returning the notebook.
func sent(t *testing.T, s *Sheet, name, a string) *Sheet {
	t.Helper()
	w := s.Book()
	nb := w.Notebook()
	if nb == nil {
		var err error
		if nb, err = w.AddNotebook("", s); err != nil {
			t.Fatal(err)
		}
	}
	nb.SetNotebookCells("add cell", append(nb.NotebookCells(), notebook.Cell{Source: name + " = ls"}))
	if err := s.AddRegion(Region{Name: name, At: at(a), Output: true}); err != nil {
		t.Fatal(err)
	}
	return nb
}

func TestSentOutputFollowsRuns(t *testing.T) {
	s := New()
	w := s.Book()
	sent(t, s, "files", "B2")
	other, _ := w.AddSheet("Sheet2", 1)
	other.Set(at("A1"), "=SUM(nu.files)")
	wantShown(t, other, map[string]string{"A1": "#REF!"}) // not run yet
	apply(t, s, LiveOp{Region: "files", Reset: true, Header: liveRow("n", "size"), Rows: []LiveRow{liveRow("a", "1"), liveRow("b", "2")}})
	wantShown(t, s, map[string]string{"B2": "n", "C3": "1", "B4": "b", "B1": ""})
	wantShown(t, other, map[string]string{"A1": "3"})
	// A run that gives fewer rows clears the rest.
	apply(t, s, LiveOp{Region: "files", Reset: true, Header: liveRow("n", "size"), Rows: []LiveRow{liveRow("c", "10")}})
	wantShown(t, s, map[string]string{"B3": "c", "B4": ""})
	wantShown(t, other, map[string]string{"A1": "10"})
	if err := s.Set(at("B3"), "x"); err != ErrOutputEdit {
		t.Errorf("typing over an output: %v", err)
	}
	if w.UndoLabel() != "edit A1" {
		t.Errorf("rows arriving made an undo step: %q", w.UndoLabel())
	}
}

// Undoing a deleted column puts back the cells a region moved over and
// was sent again at, rather than keep the region's values there.
func TestUndoPutsBackCellsARegionMovedOver(t *testing.T) {
	s := New()
	w := s.Book()
	s.Set(at("D19"), "=SEQUENCE(2)")
	sent(t, s, "r1", "E19")
	op := LiveOp{Region: "r1", Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("3")}}
	apply(t, s, op)
	s.DeleteCols(1, 1)
	apply(t, s, op) // sent again where it moved, as the UI does
	wantShown(t, s, map[string]string{"C19": "1", "D19": "n"})
	w.Undo()
	apply(t, s, op)
	wantShown(t, s, map[string]string{"D19": "1", "D20": "2", "E19": "n", "E20": "3"})
}

// A formula naming a region whose table is blocked reads #REF!, even
// when the table it showed was only its header, where the region's
// first cell stays.
func TestFormulaNamingABlockedRegion(t *testing.T) {
	s := New()
	s.Set(at("D1"), "=SUM(nu.r1)")
	s.Set(at("F12"), "5")
	sent(t, s, "r1", "E12")
	apply(t, s, LiveOp{Region: "r1", Reset: true, Header: liveRow("n")})
	wantShown(t, s, map[string]string{"D1": "0"})
	apply(t, s, LiveOp{Region: "r1", Reset: true, Header: liveRow("n", "m"), Rows: []LiveRow{liveRow("1", "2")}})
	wantShown(t, s, map[string]string{"E12": "#REF!", "D1": "#REF!"})
}

// Undoing a move puts back the cell a region's table grew over once the
// cell left, and the region, sent again, is blocked by it as before.
func TestUndoPutsBackCellsARegionGrewOver(t *testing.T) {
	s := New()
	w := s.Book()
	s.Set(at("F14"), "=1+1")
	sent(t, s, "r1", "E13")
	op := LiveOp{Region: "r1", Reset: true, Header: liveRow("n", "m"), Rows: []LiveRow{liveRow("1", "2")}}
	apply(t, s, op)
	wantShown(t, s, map[string]string{"E13": "#REF!", "F14": "2"})
	if _, err := s.Move(rect("F14"), at("A1")); err != nil {
		t.Fatal(err)
	}
	apply(t, s, op)
	wantShown(t, s, map[string]string{"E13": "n", "F14": "2", "A1": "2"})
	w.Undo()
	apply(t, s, op)
	wantShown(t, s, map[string]string{"E13": "#REF!", "F14": "2", "A1": ""})
	if s.Cell(at("F14")).Input != "=1+1" {
		t.Errorf("F14 = %q", s.Cell(at("F14")).Input)
	}
}

// A region whose column is deleted is gone, and formulas naming it say
// so, as after reopening.
func TestFormulaNamingARegionDeleted(t *testing.T) {
	s := New()
	s.Set(at("D1"), "=SUM(nu.r1)")
	sent(t, s, "r1", "H3")
	apply(t, s, LiveOp{Region: "r1", Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("56")}})
	wantShown(t, s, map[string]string{"D1": "56"})
	s.DeleteCols(7, 2)
	wantShown(t, s, map[string]string{"D1": "#NAME?"})
}

func TestSentOutputNames(t *testing.T) {
	s := New()
	w := s.Book()
	sent(t, s, "files", "A1")
	if err := s.AddRegion(Region{Name: "files", At: at("D1"), Output: true}); err == nil {
		t.Error("two regions share a name")
	}
	if _, err := s.AddLinked(at("F1"), LinkSource{Path: "files.csv"}); err != nil {
		t.Fatal(err)
	}
	if _, r, _ := w.Region("files_2"); !r.Linked() {
		t.Error("a linked file took a cell's name")
	}
	if got := w.FreeCellName("files"); got != "files_3" {
		t.Errorf("a free name like files: %q", got)
	}
	if err := w.DefineName("nu.big", s, rect("A9")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRegion(Region{Name: "big", At: at("H1"), Output: true}); err == nil {
		t.Error("a region took a named range's name")
	}
}

// Undo puts a region back emptied and stale, for its source to send its
// rows again; moving it with inserted rows does the same.
func TestSentOutputUndoAndShift(t *testing.T) {
	s := New()
	w := s.Book()
	sent(t, s, "t", "A2")
	apply(t, s, LiveOp{Region: "t", Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("1")}})
	if err := s.DeleteRegion("t"); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"A2": "", "A3": ""})
	w.Undo()
	if !s.regionMeta[nameKey("t")].stale {
		t.Error("the region undo put back isn't stale")
	}
	apply(t, s, LiveOp{Region: "t", Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("1")}})
	s.InsertRows(0, 2)
	if _, r, _ := w.Region("t"); r.At != at("A4") {
		t.Errorf("after inserting rows the region is at %v", r.At)
	}
	if !s.regionMeta[nameKey("t")].stale {
		t.Error("the region moved isn't stale")
	}
	if err := s.FreezeRegion("t"); err != nil {
		t.Fatal(err)
	}
}

func TestNotebookCellsUndo(t *testing.T) {
	w := NewBook()
	nb, err := w.AddNotebook("", w.Sheet(0))
	if err != nil || nb.Name() != "Notebook" || !nb.IsNotebook() {
		t.Fatalf("%v %v", nb, err)
	}
	nb.SetNotebookCells("add cell", []notebook.Cell{{Source: "ls"}})
	cells := nb.NotebookCells()
	if len(cells) != 1 || cells[0].ID == 0 {
		t.Fatalf("cells %+v", cells)
	}
	nb.SetNotebookCells("edit cell 1", []notebook.Cell{{ID: cells[0].ID, Source: "files = ls"}})
	w.Undo()
	if got := nb.NotebookCells(); got[0].Source != "ls" {
		t.Errorf("after undo: %+v", got)
	}
	nb.SetReactive(true)
	w.Undo()
	if nb.Reactive() {
		t.Error("undo left the notebook reactive")
	}
	if w.Notebook() != nb {
		t.Error("the workbook's notebook")
	}
}

func writeBook(t *testing.T, w *Workbook) string {
	t.Helper()
	var b bytes.Buffer
	if err := w.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestNotebookRoundTrip(t *testing.T) {
	w := NewBook()
	nb, _ := w.AddNotebook("", w.Sheet(0))
	nb.SetNotebookCells("add cells", []notebook.Cell{
		{Kind: notebook.Note, Source: "# Files"},
		{Source: "files = ls"},
		{Source: "$files | where size > 1kb"},
		{Source: "nope"},
	})
	nb.SetReactive(true)
	cells := nb.NotebookCells()
	w.SetOutput(cells[1].ID, &notebook.Output{NUON: []byte(`[[name, size]; [a, 2kb]]`), Count: 1, Source: cells[1].Source})
	w.SetOutput(cells[2].ID, &notebook.Output{NUON: []byte(strings.Repeat("x", 100)), Count: 2})
	w.SetOutput(cells[3].ID, &notebook.Output{Err: "Command `nope` not found", Detail: "help: try ls"})
	w.SetOutputCaps(notebook.Caps{Cell: 50, Total: 1000})
	if got := w.UnsavedOutputs(); len(got) != 1 || got[0] != "cell 3" {
		t.Errorf("unsaved %v", got)
	}
	text := writeBook(t, w)
	for _, want := range []string{`"tab": "notebook"`, `"reactive": true`, `{"kind":"note","source":"# Files"}`,
		`"output":"[[name, size]; [a, 2kb]]"`, `"unsaved":true`, `"error":"Command`} {
		if !strings.Contains(text, want) {
			t.Errorf("the file lacks %s:\n%s", want, text)
		}
	}
	back, err := ReadBook(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	nb2 := back.Notebook()
	if nb2 == nil || !nb2.Reactive() || nb2.Name() != "Notebook" {
		t.Fatalf("read back %v", nb2)
	}
	got := nb2.NotebookCells()
	if len(got) != 4 || got[0].Kind != notebook.Note || got[1].Source != "files = ls" {
		t.Fatalf("cells %+v", got)
	}
	if o := back.Output(got[1].ID); o == nil || string(o.NUON) != `[[name, size]; [a, 2kb]]` || o.Count != 0 {
		t.Errorf("output of files: %+v", o)
	}
	if o := back.Output(got[2].ID); o == nil || !o.Unsaved || o.NUON != nil {
		t.Errorf("the output over the cap: %+v", o)
	}
	if o := back.Output(got[3].ID); o == nil || o.Err == "" || o.Detail != "help: try ls" {
		t.Errorf("the error: %+v", o)
	}
	if st := notebook.Stale(got, back.Output); len(st) != 0 {
		t.Errorf("stale on opening: %v", st)
	}
	if back.OutputsChanged() != 0 {
		t.Error("reading outputs counts as a change")
	}
	// Written again, it's the same file.
	if again := writeBook(t, back); again != text {
		t.Errorf("written again:\n%s\nwas\n%s", again, text)
	}
}

// A notebook sheet of an earlier build opens with its commands as code
// cells of a notebook tab, their tables sent where they were.
func TestOldNotebookConverted(t *testing.T) {
	old := `{"version": 4, "sheets": [
  {"name": "Shell 1", "notebook": true, "cells": {}, "regions": [
    {"name": "r1", "command": "ls", "at": "A1", "rows": 3, "cols": 2},
    {"name": "big", "command": "$r1 | where size > 1kb", "at": "A6", "reads": ["r1"]},
    {"name": "sel", "command": "$in | math sum", "at": "A9", "input": "Data 1!A1:A4"}
  ]},
  {"name": "Data 1", "cells": {"B1": "=SUM(nu.big)"}}
]}`
	w, err := ReadBook(strings.NewReader(old))
	if err != nil {
		t.Fatal(err)
	}
	nb := w.Notebook()
	if nb == nil || w.Index(nb) != 1 || nb.Name() != "Notebook" {
		t.Fatalf("notebook %v at %d", nb, w.Index(nb))
	}
	cells := nb.NotebookCells()
	want := []string{"r1 = ls", "big = $r1 | where size > 1kb", "sel = $sheet.'Data 1'!A1:A4 | do {\n$in | math sum\n}"}
	for i, c := range cells {
		if c.Source != want[i] {
			t.Errorf("cell %d: %q, want %q", i+1, c.Source, want[i])
		}
	}
	shell := w.Lookup("Shell 1")
	if shell.IsNotebook() {
		t.Error("the old notebook sheet is a notebook tab")
	}
	if _, r, ok := w.Region("big"); !ok || !r.Output || r.At != at("A7") {
		t.Errorf("big's table: %+v %v", r, ok)
	}
	if notes := w.LoadNotes(); len(notes) != 1 || !strings.Contains(notes[0], "Shell 1") {
		t.Errorf("notes %v", notes)
	}
	apply(t, shell, LiveOp{Region: "big", Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("4")}})
	wantShown(t, w.Lookup("Data 1"), map[string]string{"B1": "4"})
	if text := writeBook(t, w); strings.Contains(text, `"command"`) || !strings.Contains(text, `"output":true`) {
		t.Errorf("saved again:\n%s", text)
	}
}
