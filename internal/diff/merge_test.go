package diff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

func merge(t *testing.T, base, ours, theirs []byte) (*sheet.Workbook, []string) {
	t.Helper()
	out, conflicts, err := Merge(base, ours, theirs)
	if err != nil {
		t.Fatal(err)
	}
	w, err := sheet.ReadBook(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("the merge doesn't read: %v\n%s", err, out)
	}
	var said []string
	for _, c := range conflicts {
		said = append(said, c.String())
	}
	return w, said
}

func input(w *sheet.Workbook, ref string) string {
	name, rest := sheet.SplitSheet(ref)
	s := w.Lookup(name)
	if s == nil {
		return "(no sheet " + name + ")"
	}
	c := s.Cell(addr(rest))
	if c == nil {
		return ""
	}
	return c.Input
}

func TestMergeCells(t *testing.T) {
	base := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "Item", "B1": "1", "B2": "2", "B3": "3", "B4": "=SUM(B1:B3)"}}, nil)
	ours := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "Item", "B1": "10", "B2": "2", "B3": "30", "B4": "=SUM(B1:B3)", "C1": "ours"}}, nil)
	theirs := file(t, []string{"Sheet1"}, []map[string]string{{"A1": "Item", "B1": "1", "B2": "20", "B3": "31", "B4": "=SUM(B1:B3)", "D1": "theirs"}}, func(w *sheet.Workbook) {
		s := w.Sheet(0)
		s.SetStyle(sheet.NewRect(addr("B1"), addr("B1")), func(st *sheet.Style) { st.Bold = true })
		s.SetColWidth(3, 20)
	})
	w, said := merge(t, base, ours, theirs)
	for ref, want := range map[string]string{"Sheet1!B1": "10", "Sheet1!B2": "20", "Sheet1!B3": "30", "Sheet1!C1": "ours", "Sheet1!D1": "theirs"} {
		if got := input(w, ref); got != want {
			t.Errorf("%s = %q, want %q", ref, got, want)
		}
	}
	s := w.Sheet(0)
	if !s.CellStyle(addr("B1")).Bold || s.ColWidth(3) != 20 {
		t.Error("theirs' formatting and width were lost")
	}
	if v := s.Value(addr("B4")); v.Num != 60 {
		t.Errorf("B4 = %v", v)
	}
	if len(said) != 1 || said[0] != "Sheet1!B3 input: ours 30, theirs 31, base 3" {
		t.Errorf("conflicts: %v", said)
	}
	if note := s.Note(addr("B3")); note != "Merge conflict, kept ours; theirs had input 31" {
		t.Errorf("B3's note: %q", note)
	}
}

func TestMergeSheets(t *testing.T) {
	cells := map[string]string{"A1": "a", "A2": "b", "A3": "c"}
	base := file(t, []string{"Plan", "Old", "Keep"}, []map[string]string{cells, {"A1": "old"}, {"A1": "k"}}, nil)
	// Ours renames Plan and adds a sheet; theirs edits Plan, removes Old
	// and adds a sheet of the same name as ours'.
	ours := file(t, []string{"Q3 plan", "Old", "Keep", "Notes"}, []map[string]string{cells, {"A1": "old"}, {"A1": "k"}, {"A1": "ours"}}, nil)
	edited := map[string]string{"A1": "a", "A2": "B", "A3": "c"}
	theirs := file(t, []string{"Plan", "Keep", "Notes"}, []map[string]string{edited, {"A1": "k"}, {"B1": "theirs"}}, nil)
	w, said := merge(t, base, ours, theirs)
	var names []string
	for _, s := range w.Sheets() {
		names = append(names, s.Name())
	}
	if strings.Join(names, ",") != "Q3 plan,Keep,Notes" {
		t.Errorf("sheets: %v", names)
	}
	if input(w, "'Q3 plan'!A2") != "B" || input(w, "Notes!A1") != "ours" || input(w, "Notes!B1") != "theirs" {
		t.Errorf("cells: %q %q %q", input(w, "'Q3 plan'!A2"), input(w, "Notes!A1"), input(w, "Notes!B1"))
	}
	if len(said) != 0 {
		t.Errorf("conflicts: %v", said)
	}

	// A sheet removed on one side and changed on the other conflicts;
	// ours stays.
	oursEdit := file(t, []string{"Plan", "Old", "Keep"}, []map[string]string{cells, {"A1": "older"}, {"A1": "k"}}, nil)
	w, said = merge(t, base, oursEdit, theirs)
	if w.Lookup("Old") == nil || len(said) != 1 || said[0] != "sheet Old: changed in ours, removed in theirs" {
		t.Errorf("removed in theirs: %v %v", w.Lookup("Old"), said)
	}
	theirsEdit := file(t, []string{"Plan", "Old", "Keep"}, []map[string]string{cells, {"A1": "older"}, {"A1": "k"}}, nil)
	oursGone := file(t, []string{"Plan", "Keep"}, []map[string]string{cells, {"A1": "k"}}, nil)
	w, said = merge(t, base, oursGone, theirsEdit)
	if w.Lookup("Old") != nil || len(said) != 1 || said[0] != "sheet Old: removed in ours, changed in theirs" {
		t.Errorf("removed in ours: %v %v", w.Lookup("Old"), said)
	}

	// Both renaming one sheet differently conflicts.
	theirsRenamed := file(t, []string{"Plan B", "Old", "Keep"}, []map[string]string{cells, {"A1": "old"}, {"A1": "k"}}, nil)
	_, said = merge(t, base, ours, theirsRenamed)
	if len(said) != 1 || said[0] != "sheet Q3 plan name: ours Q3 plan, theirs Plan B, base Plan" {
		t.Errorf("renamed twice: %v", said)
	}
}

func TestMergeTrust(t *testing.T) {
	book := func(origin, command string) []byte {
		return []byte(`{"version": 4, "macroOrigin": "` + origin + `", "sheets": [{"name": "S", "notebook": true, "regions": [{"name": "r1", "command": "` + command + `", "at": "A1"}], "cells": {}}]}`)
	}
	base := book("here", "ls")
	cases := []struct {
		ours, theirs []byte
		want         string
	}{
		{book("here", "ls -a"), book("here", "ls"), "here"},     // theirs brought no commands
		{book("here", "ls"), book("there", "ls"), "here"},       // only theirs' origin differs
		{book("here", "echo"), book("there", "echo"), "here"},   // both made the same change
		{book("here", "ls -a"), book("there", "ls -b"), "here"}, // a conflict keeps ours' command
		{book("here", "ls"), book("here", "ls -l"), "here"},     // theirs trusted on this computer too
		{book("there", "ls"), book("there", "ls -l"), "there"},  // both from elsewhere, as ours was
		{book("here", "ls"), book("there", "rm -rf ~"), ""},     // theirs' command from elsewhere
		{book("here", "ls"), book("", "ls -l"), ""},             // theirs' command, never trusted
		{book("here", "ls"), book("here2", "ls -l"), ""},        // another computer's id
		{book("", "ls"), book("here", "ls -l"), ""},             // ours stays untrusted
	}
	for i, c := range cases {
		w, _ := merge(t, base, c.ours, c.theirs)
		if got := w.MacroOrigin(); got != c.want {
			t.Errorf("case %d: origin %q, want %q", i, got, c.want)
		}
	}
}

func TestMergeBroken(t *testing.T) {
	if _, _, err := Merge([]byte("{"), nil, nil); err == nil {
		t.Error("a broken base merged")
	}
	// Names that end up pointing at a sheet no longer there make a
	// workbook the engine refuses, which is an error, not a merge.
	base := []byte(`{"version": 4, "sheets": [{"name": "A", "cells": {}}, {"name": "B", "cells": {}}]}`)
	ours := []byte(`{"version": 4, "sheets": [{"name": "A", "cells": {}}]}`)
	theirs := []byte(`{"version": 4, "names": {"N": "B!A1"}, "sheets": [{"name": "A", "cells": {}}, {"name": "B", "cells": {}}]}`)
	if _, _, err := Merge(base, ours, theirs); err == nil {
		t.Error("a merge naming a sheet removed")
	}
}
