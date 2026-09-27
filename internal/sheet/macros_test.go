package sheet

import (
	"bytes"
	"strings"
	"testing"
)

func TestSaveMacroIsUndoable(t *testing.T) {
	w := NewBook()
	if err := w.SaveMacro("", Macro{Name: "Totals", Key: "1", Source: "enter(\"x\")\n"}, "record macro Totals"); err != nil {
		t.Fatal(err)
	}
	if err := w.SaveMacro("totals", Macro{Name: "Sums", Key: "1", Source: "enter(\"y\")\n"}, "edit macro Sums"); err != nil {
		t.Fatal(err)
	}
	if ms := w.Macros(); len(ms) != 1 || ms[0].Name != "Sums" || ms[0].API != MacroAPI {
		t.Fatalf("macros %+v", ms)
	}
	c, ok := w.Undo()
	if !ok || c.Label != "edit macro Sums" || !c.Macros {
		t.Fatalf("undo %+v %v", c, ok)
	}
	if mc, ok := w.Macro("TOTALS"); !ok || mc.Source != "enter(\"x\")\n" {
		t.Fatalf("after undo %+v %v", mc, ok)
	}
	w.Undo()
	if len(w.Macros()) != 0 {
		t.Fatalf("macros after undoing the add: %+v", w.Macros())
	}
	w.Redo()
	if _, ok := w.MacroForKey("1"); !ok {
		t.Fatal("redo lost the shortcut")
	}
	if !w.DeleteMacro("Totals") || len(w.Macros()) != 0 {
		t.Fatal("delete")
	}
}

func TestSaveMacroChecksNamesAndKeys(t *testing.T) {
	w := NewBook()
	w.SaveMacro("", Macro{Name: "A", Key: "1"}, "add")
	for _, tc := range []struct {
		old  string
		mc   Macro
		want string
	}{
		{"", Macro{Name: " "}, "needs a name"},
		{"", Macro{Name: "a"}, "already a macro named A"},
		{"", Macro{Name: "B", Key: "1"}, "Ctrl+Alt+Shift+1 already runs A"},
		{"", Macro{Name: "B", Key: "x"}, "is a digit"},
		{"C", Macro{Name: "C"}, "no macro named C"},
	} {
		if err := w.SaveMacro(tc.old, tc.mc, "x"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("SaveMacro(%q, %+v) = %v, want %q", tc.old, tc.mc, err, tc.want)
		}
	}
}

func TestMacrosRoundTrip(t *testing.T) {
	for _, sheets := range []int{1, 2} {
		w := NewBook()
		if sheets == 2 {
			w.AddSheet("Two", 1)
		}
		src := "# a <script> & more\nselect(\"B2\")\nenter(\"=SUM(A1:A3)\")\n"
		w.SaveMacro("", Macro{Name: "Totals", Key: "3", Source: src}, "add")
		w.SaveMacro("", Macro{Name: "Plain", Source: "move(0, 1)\n"}, "add")
		w.SetMacroOrigin("abc123")
		var b bytes.Buffer
		if err := w.Write(&b); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), `{"name": "Totals", "key": "3", "api": 1, "source": "# a <script> & more\nselect(\"B2\")`) {
			t.Errorf("file:\n%s", b.String())
		}
		got, err := ReadBook(&b)
		if err != nil {
			t.Fatal(err)
		}
		ms := got.Macros()
		if len(ms) != 2 || ms[0] != (Macro{Name: "Totals", Key: "3", Source: src, API: 1}) || ms[1].Name != "Plain" || ms[1].Key != "" {
			t.Errorf("%d sheets: macros %+v", sheets, ms)
		}
		if got.MacroOrigin() != "abc123" {
			t.Errorf("origin %q", got.MacroOrigin())
		}
		if got.CanUndo() {
			t.Error("loading is undoable")
		}
	}
}

func TestFileWithoutMacrosHasNoMacroFields(t *testing.T) {
	w := NewBook()
	w.SetMacroOrigin("abc")
	var b bytes.Buffer
	w.Write(&b)
	if strings.Contains(b.String(), "macro") {
		t.Errorf("file:\n%s", b.String())
	}
}

func TestReadMacrosDropsClashingKeys(t *testing.T) {
	src := `{"version": 2, "macros": [{"name": "A", "key": "1", "api": 1, "source": ""}, {"name": "B", "key": "1", "api": 2, "source": ""}], "cells": {}}`
	w, err := ReadBook(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if ms := w.Macros(); ms[1].Key != "" || ms[1].API != 2 {
		t.Errorf("macros %+v", ms)
	}
	dup := `{"version": 2, "macros": [{"name": "A", "api": 1, "source": ""}, {"name": "a", "api": 1, "source": ""}], "cells": {}}`
	if _, err := ReadBook(strings.NewReader(dup)); err == nil {
		t.Error("a name defined twice loads")
	}
}

func TestBeginKeepsOneStepOpen(t *testing.T) {
	s := New()
	w := s.Book()
	end := w.Begin(Change{Label: "run macro M", Sheet: s})
	s.Set(Addr{}, "2")
	s.Set(Addr{Row: 1}, "=A1*10")
	w.Settle()
	if v := s.Value(Addr{Row: 1}); v.Num != 20 {
		t.Fatalf("A2 inside the step = %v", v)
	}
	s.Set(Addr{}, "3")
	if _, ok := w.Undo(); ok {
		t.Fatal("undo ran inside an open step")
	}
	end()
	end() // a second call does nothing
	if v := s.Value(Addr{Row: 1}); v.Num != 30 {
		t.Fatalf("A2 after the step = %v", v)
	}
	c, ok := w.Undo()
	if !ok || c.Label != "run macro M" || s.Cell(Addr{}) != nil || s.Cell(Addr{Row: 1}) != nil {
		t.Fatalf("undo %+v %v: A1 %v", c, ok, s.Cell(Addr{}))
	}
	if w.CanUndo() {
		t.Error("the step was split")
	}
}
