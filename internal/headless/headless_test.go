package headless

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

func addr(s string) sheet.Addr {
	a, _ := sheet.ParseAddr(s)
	return a
}

// book is a workbook of two sheets, Sheet1 and 'Q3 plan', and a name.
func book(t *testing.T) *sheet.Workbook {
	t.Helper()
	w := sheet.NewBook()
	s := w.Sheet(0)
	for a, in := range map[string]string{"A1": "Item", "B1": "Price", "A2": "Apple", "B2": "1.5", "A3": "Pear", "B3": "$2", "B4": "=SUM(B2:B3)", "C1": "=1/0"} {
		if err := s.Set(addr(a), in); err != nil {
			t.Fatal(err)
		}
	}
	q, err := w.AddSheet("Q3 plan", 1)
	if err != nil {
		t.Fatal(err)
	}
	q.Set(addr("A1"), "2026-09-29")
	q.Set(addr("B2"), "=Sheet1!B4*2")
	if err := w.DefineName("Prices", s, sheet.NewRect(addr("B2"), addr("B3"))); err != nil {
		t.Fatal(err)
	}
	w.SetActive(s)
	return w
}

func TestResolve(t *testing.T) {
	w := book(t)
	for ref, want := range map[string]string{
		"":             "Sheet1",
		"B7":           "Sheet1!B7",
		"$B$7":         "Sheet1!B7",
		"A1:C9":        "Sheet1!A1:C9",
		"'Q3 plan'!B2": "'Q3 plan'!B2",
		"Q3 plan!B2":   "'Q3 plan'!B2",
		"Q3 plan":      "'Q3 plan'",
		"'Q3 plan'!":   "'Q3 plan'",
		"Prices":       "Sheet1!B2:B3",
	} {
		got, err := Resolve(w, ref)
		if err != nil || got.String() != want {
			t.Errorf("Resolve(%q) = %s, %v; want %s", ref, got, err, want)
		}
	}
	for ref, want := range map[string]string{
		"Q4!A1": `no sheet named "Q4" (sheets: Sheet1, Q3 plan)`,
		"what":  `"what" isn't a cell, a range, a named range or a sheet`,
	} {
		if _, err := Resolve(w, ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Resolve(%q): %v, want %q", ref, err, want)
		}
	}
	if _, _, err := ResolveCell(w, "A1:B2"); err == nil || !strings.Contains(err.Error(), "name one cell") {
		t.Errorf("ResolveCell of a range: %v", err)
	}
}

func get(t *testing.T, w *sheet.Workbook, ref string, o GetOptions) string {
	t.Helper()
	target, err := Resolve(w, ref)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Get(&b, target, o); err != nil {
		t.Fatalf("get %s: %v", ref, err)
	}
	return b.String()
}

func TestGet(t *testing.T) {
	w := book(t)
	for _, c := range []struct {
		ref  string
		o    GetOptions
		want string
	}{
		{"B4", GetOptions{}, "3.5\n"},
		{"B3", GetOptions{}, "$2\n"},
		{"B3", GetOptions{Format: "nuon"}, "2\n"},
		{"B3", GetOptions{Format: "json"}, "2\n"},
		{"B4", GetOptions{Input: true}, "=SUM(B2:B3)\n"},
		{"B4", GetOptions{Input: true, Format: "json"}, "\"=SUM(B2:B3)\"\n"},
		{"A1", GetOptions{Format: "nuon"}, "Item\n"},
		{"Z9", GetOptions{Format: "nuon"}, "null\n"},
		{"Z9", GetOptions{}, "\n"},
		{"C1", GetOptions{}, "#DIV/0!\n"},
		{"'Q3 plan'!B2", GetOptions{}, "7\n"},
		{"A1:B3", GetOptions{}, "Item   Price\nApple    1.5\nPear      $2\n"},
		{"A1:B3", GetOptions{Format: "csv"}, "Item,Price\nApple,1.5\nPear,$2\n"},
		{"A1:B3", GetOptions{Format: "nuon"}, "[[Item, Price]; [Apple, 1.5],\n[Pear, 2]]\n"},
		{"A2:B3", GetOptions{Format: "json", NoHeader: true}, "[\n{\"A\":\"Apple\",\"B\":1.5},\n{\"A\":\"Pear\",\"B\":2}\n]\n"},
		{"B1:B4", GetOptions{Format: "json", Input: true}, "[\n{\"Price\":\"1.5\"},\n{\"Price\":\"$2\"},\n{\"Price\":\"=SUM(B2:B3)\"}\n]\n"},
	} {
		if got := get(t, w, c.ref, c.o); got != c.want {
			t.Errorf("get %s %+v = %q, want %q", c.ref, c.o, got, c.want)
		}
	}
	if got := get(t, w, "'Q3 plan'!A1", GetOptions{Format: "nuon"}); !strings.HasPrefix(got, "2026-09-29T00:00:00") {
		t.Errorf("a date as NUON: %q", got)
	}
	target, _ := Resolve(w, "A1:B2")
	if err := Get(io.Discard, target, GetOptions{Format: "xml"}); err == nil {
		t.Error("--format xml was taken")
	}
}

func TestSet(t *testing.T) {
	w := book(t)
	warn, err := Set(w, []Entry{{"B2", "10"}, {"'Q3 plan'!C1", "=B2+1"}, {"A2", ""}}, SetOptions{})
	if err != nil || len(warn) > 0 {
		t.Fatal(warn, err)
	}
	s := w.Sheet(0)
	if v := s.Value(addr("B4")); v.Num != 12 {
		t.Errorf("B4 = %v after setting B2", v)
	}
	if s.Filled(addr("A2")) {
		t.Error("an empty input didn't clear A2")
	}
	if !w.CanUndo() {
		t.Error("set wasn't a change")
	}
	for _, c := range []struct {
		e    Entry
		want string
	}{
		{Entry{"B5", "=SUM(B2:B3"}, "Sheet1!B5: Expected , or ) in SUM, at character 11 of =SUM(B2:B3"},
		{Entry{"A1:A2", "x"}, "A1:A2 is the range A1:A2: name one cell"},
		{Entry{"Q9!A1", "x"}, `no sheet named "Q9"`},
	} {
		if _, err := Set(w, []Entry{c.e}, SetOptions{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("set %v: %v, want %q", c.e, err, c.want)
		}
	}
}

func TestSetRules(t *testing.T) {
	w := book(t)
	s := w.Sheet(0)
	s.Protect(sheet.Protection{Range: sheet.NewRect(addr("D1"), addr("D9"))})
	if _, err := Set(w, []Entry{{"D2", "1"}}, SetOptions{}); err == nil || !strings.Contains(err.Error(), "Sheet1!D2 is protected (D1:D9): --force") {
		t.Errorf("a protected cell: %v", err)
	}
	if _, err := Set(w, []Entry{{"D2", "1"}}, SetOptions{Force: true}); err != nil {
		t.Errorf("--force: %v", err)
	}
	s.AddValidation(sheet.Validation{Ranges: []sheet.Rect{sheet.NewRect(addr("E1"), addr("E9"))}, Kind: sheet.ValidNumber, Op: sheet.RuleBetween, Args: [2]string{"1", "9"}, Reject: true})
	s.AddValidation(sheet.Validation{Ranges: []sheet.Rect{sheet.NewRect(addr("F1"), addr("F9"))}, Kind: sheet.ValidNumber, Op: sheet.RuleBetween, Args: [2]string{"1", "9"}})
	if _, err := Set(w, []Entry{{"E1", "5"}, {"E2", "50"}}, SetOptions{}); err == nil || !strings.HasPrefix(err.Error(), "Sheet1!E2: ") {
		t.Errorf("a rejected entry: %v", err)
	}
	warn, err := Set(w, []Entry{{"F1", "50"}}, SetOptions{})
	if err != nil || len(warn) != 1 || !strings.HasPrefix(warn[0], "Sheet1!F1: ") {
		t.Errorf("an entry marked invalid: %v %v", warn, err)
	}
}

func TestOpenSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "book.012")
	if _, err := Open(path, false); err == nil {
		t.Error("opened a missing file")
	}
	f, err := Open(path, true)
	if err != nil || !f.New {
		t.Fatal(f, err)
	}
	if _, err := Set(f.Book, []Entry{{"A1", "5"}, {"A2", "=A1*2"}}, SetOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	os.Chmod(path, 0o600)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(path, old, old)
	f, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if v := f.Book.Sheet(0).Value(addr("A2")); v.Num != 10 {
		t.Errorf("A2 = %v", v)
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); !st.ModTime().Equal(old) {
		t.Error("saving an unchanged workbook rewrote it")
	}
	Set(f.Book, []Entry{{"A1", "6"}}, SetOptions{})
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if st.ModTime().Equal(old) || st.Mode().Perm() != 0o600 {
		t.Errorf("saved %v with mode %v", st.ModTime(), st.Mode())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left files behind: %v", entries)
	}
	os.WriteFile(path, []byte("{nope"), 0o644)
	if _, err := Open(path, false); err == nil || !strings.HasPrefix(err.Error(), path+": ") {
		t.Errorf("a broken file: %v", err)
	}
}

func TestRecalc(t *testing.T) {
	w := book(t)
	w.Sheet(0).Set(addr("D1"), "=D2")
	w.Sheet(0).Set(addr("D2"), "=D1")
	problems, circular := Recalc(w)
	if !circular {
		t.Error("no circular reference")
	}
	var got []string
	for _, p := range problems {
		got = append(got, p.At()+" "+p.Value)
	}
	if strings.Join(got, ", ") != "Sheet1!C1 #DIV/0!, Sheet1!D1 #REF!, Sheet1!D2 #REF!" {
		t.Errorf("problems: %v", got)
	}
	if problems[0].Why == "" {
		t.Error("no explanation")
	}
}

// fakeNu answers commands with NUON.
type fakeNu map[string]string

func (f fakeNu) Run(_ context.Context, job nushell.Job, _ string, stdout io.Writer) error {
	out, ok := f[job.Command]
	if !ok {
		return &nushell.Error{Msg: "Command `" + job.Command + "` not found"}
	}
	_, err := io.WriteString(stdout, out)
	return err
}

func TestRunNotebooks(t *testing.T) {
	w := sheet.NewBook()
	grid := w.Sheet(0)
	grid.Set(addr("D1"), "7")
	nb, err := w.AddNotebook("Notebook", grid)
	if err != nil {
		t.Fatal(err)
	}
	nb.SetNotebookCells("add cells", []notebook.Cell{
		{Kind: notebook.Code, Source: "files = ls"},
		{Kind: notebook.Note, Source: "# Files"},
		{Kind: notebook.Code, Source: "n = $files | length"},
		{Kind: notebook.Code, Source: "$sheet.D1:D1"},
		{Kind: notebook.Code, Source: "boom"},
		{Kind: notebook.Code, Source: "last = 1"},
	})
	if err := grid.AddRegion(sheet.Region{Name: "files", At: addr("A1"), Output: true}); err != nil {
		t.Fatal(err)
	}
	nu := fakeNu{"ls": "[[name, size]; [a, 1], [b, 2]]", "$files | length": "2", "$__sheet1": "[[D]; [7]]"}
	failed := RunNotebooks(context.Background(), w, NotebookOptions{Runner: nu, Timeout: time.Second})
	if len(failed) != 1 || failed[0] != "Notebook cell 5: Command `boom` not found" {
		t.Errorf("failed: %q", failed)
	}
	target, _ := Resolve(w, "A1:B3")
	var b bytes.Buffer
	Get(&b, target, GetOptions{Format: "csv"})
	if b.String() != "name,size\na,1\nb,2\n" {
		t.Errorf("the output sent to Sheet1:\n%s", b.String())
	}
	cells := nb.NotebookCells()
	if o := w.Output(cells[2].ID); o == nil || string(o.NUON) != "2" || o.Reads["files"] == 0 {
		t.Errorf("$files | length: %+v", o)
	}
	if o := w.Output(cells[4].ID); o == nil || o.Err != "Command `boom` not found" {
		t.Errorf("boom: %+v", o)
	}
	if o := w.Output(cells[5].ID); o != nil {
		t.Errorf("a cell after a failure ran: %+v", o)
	}
	if o := w.Output(cells[1].ID); o != nil {
		t.Errorf("a note ran: %+v", o)
	}
}

func TestOpenSendsOutputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nb.012")
	os.WriteFile(path, []byte(`{"version": 4, "sheets": [
{"name": "Sheet1", "regions": [{"name": "files", "at": "B2", "output": true}], "cells": {}},
{"name": "Notebook", "tab": "notebook", "notebookCells": [{"source": "files = ls", "output": "[[name]; [a.txt]]"}], "cells": {}}]}`), 0o644)
	f, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Book.Sheet(0).LocalText(addr("B3")); got != "a.txt" {
		t.Errorf("B3 = %q, want the output the file kept", got)
	}
}
