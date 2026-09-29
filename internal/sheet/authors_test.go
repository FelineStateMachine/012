package sheet

import (
	"testing"
)

// sharedBook is a workbook with its history shared, as a room's is.
func sharedBook() (*Workbook, *Sheet) {
	s := New()
	w := s.Book()
	w.ShareHistory()
	return w, s
}

// as makes one step as author a.
func as(w *Workbook, a int, fn func()) {
	w.SetAuthor(a)
	fn()
}

// Operations are numbered in the order they run, whoever runs them, and
// OpsSince hands them out in that order.
func TestOpsInOrder(t *testing.T) {
	w, s := sharedBook()
	as(w, 1, func() { s.Set(at("A1"), "1") })
	as(w, 2, func() { s.Set(at("B1"), "2") })
	as(w, 1, func() { w.Undo() })
	ops, last := w.OpsSince(0)
	if last != 3 || len(ops) != 3 {
		t.Fatalf("ops %+v, last %d", ops, last)
	}
	want := []struct {
		author int
		kind   OpKind
		label  string
	}{{1, OpDo, "edit A1"}, {2, OpDo, "edit B1"}, {1, OpUndo, "edit A1"}}
	for i, o := range ops {
		if o.Seq != uint64(i+1) || o.Author != want[i].author || o.Kind != want[i].kind || o.Label != want[i].label {
			t.Errorf("op %d: %+v, want %+v", i, o, want[i])
		}
	}
	if more, _ := w.OpsSince(2); len(more) != 1 || more[0].Kind != OpUndo {
		t.Errorf("since 2: %+v", more)
	}
}

// Undo takes back only the author's own latest step, from under later
// steps of others that changed other cells; those stay.
func TestUndoIsPerAuthor(t *testing.T) {
	w, s := sharedBook()
	as(w, 1, func() { s.Set(at("A1"), "mine") })
	as(w, 2, func() { s.Set(at("B1"), "theirs") })
	as(w, 2, func() { s.Set(at("B2"), "=A1") })
	before := w.StateID()
	w.SetAuthor(1)
	if w.UndoLabel() != "edit A1" {
		t.Errorf("author 1 would undo %q", w.UndoLabel())
	}
	if _, ok := w.Undo(); !ok {
		t.Fatal("author 1's undo refused")
	}
	if s.Filled(at("A1")) || s.Value(at("B1")).Str != "theirs" {
		t.Errorf("A1 %v, B1 %v", s.Value(at("A1")), s.Value(at("B1")))
	}
	if v := s.Value(at("B2")); v.Str == "mine" {
		t.Errorf("B2, reading A1, is %v", v)
	}
	if w.StateID() == before {
		t.Error("an undo from under others' steps kept the state's ID")
	}
	if w.CanUndo() {
		t.Error("author 1 has nothing left to undo")
	}
	w.SetAuthor(2)
	if !w.CanUndo() || w.UndoLabel() != "edit B2" {
		t.Errorf("author 2 would undo %q", w.UndoLabel())
	}
	// Redo is per author too.
	w.SetAuthor(1)
	if !w.CanRedo() {
		t.Fatal("author 1 can't redo")
	}
	w.Redo()
	if s.Value(at("A1")).Str != "mine" || s.Value(at("B2")).Str != "mine" {
		t.Errorf("after redo A1 %v, B2 %v", s.Value(at("A1")), s.Value(at("B2")))
	}
	w.SetAuthor(2)
	if w.CanRedo() {
		t.Error("author 2 can redo author 1's undo")
	}
}

// Undo refuses when a later step of someone else's changed what the
// step changed, and says whose; once that step is undone, it goes.
func TestUndoRefusesOverlaps(t *testing.T) {
	w, s := sharedBook()
	as(w, 1, func() { s.Set(at("A1"), "first") })
	as(w, 2, func() { s.Set(at("A1"), "second") })
	w.SetAuthor(1)
	b, blocked := w.UndoBlocked()
	if !blocked || b.Author != 2 || b.Label != "edit A1" {
		t.Fatalf("blocked %v by %+v", blocked, b)
	}
	if _, ok := w.Undo(); ok || s.Value(at("A1")).Str != "second" {
		t.Fatalf("undo went ahead: A1 %v", s.Value(at("A1")))
	}
	as(w, 2, func() { w.Undo() })
	w.SetAuthor(1)
	if _, blocked := w.UndoBlocked(); blocked {
		t.Fatal("still blocked")
	}
	w.Undo()
	if s.Filled(at("A1")) {
		t.Errorf("A1 %v", s.Value(at("A1")))
	}
	// Author 2's redo is now in the way of nothing, and redoes onto
	// the blank cell what it undid.
	w.SetAuthor(2)
	if _, ok := w.Redo(); !ok || s.Value(at("A1")).Str != "second" {
		t.Errorf("author 2's redo: A1 %v", s.Value(at("A1")))
	}
	// Author 1's redo would overwrite it: refused.
	w.SetAuthor(1)
	if b, blocked := w.RedoBlocked(); !blocked || b.Author != 2 {
		t.Errorf("author 1's redo not blocked: %+v", b)
	}
}

// Inserting or deleting lines conflicts with any other step on the same
// sheet, and with none on another.
func TestUndoRefusesAcrossShifts(t *testing.T) {
	w, s := sharedBook()
	s2, _ := w.AddSheet("S2", 1)
	as(w, 1, func() { s.Set(at("A5"), "x") })
	as(w, 1, func() { s.InsertRows(2, 1) })
	as(w, 2, func() { s2.Set(at("A1"), "elsewhere") })
	w.SetAuthor(1)
	if _, blocked := w.UndoBlocked(); blocked {
		t.Fatal("a step on another sheet blocks the insert")
	}
	as(w, 2, func() { s.Set(at("Z1"), "far away") })
	w.SetAuthor(1)
	if _, blocked := w.UndoBlocked(); !blocked {
		t.Fatal("a step on the shifted sheet doesn't block the insert")
	}
}

// The sheet list conflicts with everything after it.
func TestUndoRefusesAfterSheetChanges(t *testing.T) {
	w, s := sharedBook()
	as(w, 1, func() { w.AddSheet("S2", 1) })
	as(w, 2, func() { s.Set(at("A1"), "1") })
	w.SetAuthor(1)
	if _, blocked := w.UndoBlocked(); !blocked {
		t.Error("adding a sheet undone past a later step")
	}
}

// Unshared, undo takes back the latest step whoever made it, as a
// workbook one person edits does.
func TestUndoUnshared(t *testing.T) {
	s := New()
	w := s.Book()
	as(w, 1, func() { s.Set(at("A1"), "1") })
	as(w, 2, func() { s.Set(at("A1"), "2") })
	w.SetAuthor(1)
	w.Undo()
	if s.Value(at("A1")).Num != 1 {
		t.Errorf("A1 %v", s.Value(at("A1")))
	}
}
