package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// panicOn makes F12 panic where, as the crashtest build does, for the
// test.
func panicOn(t *testing.T, where string) {
	t.Helper()
	crashTest = func(msg tea.Msg) string {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.Code == tea.KeyF12 {
			return where
		}
		return ""
	}
	t.Cleanup(func() { crashTest = nil })
}

var f12 = tea.KeyPressMsg{Code: tea.KeyF12}

// isQuit reports whether cmd quits the program.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// A panic in Update quits the program, and the crash says where and
// what with; the model's unsaved work is kept in the recovery directory
// and offered back when the file is opened again.
func TestGuardUpdatePanicKeepsWork(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(t.TempDir(), "recovery")
	panicOn(t, "update")
	m := newModel()
	m.prefs.RecoveryDir = dir
	m.filename = "book.012"
	press(t, m, "mine", "<enter>")
	g := Guard(m)
	next, cmd := g.Update(f12)
	if !isQuit(cmd) {
		t.Fatal("a panic in Update didn't quit")
	}
	if next != g {
		t.Fatalf("Update returned %v after a panic; Bubble Tea draws what it returns", next)
	}
	next.View()
	if _, cmd := g.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd != nil {
		t.Error("a crashed model took more input")
	}
	c := g.Finish(nil)
	if c == nil || c.Where != "update" || fmt.Sprint(c.Value) != "crash test in update" || !strings.Contains(string(c.Stack), "crash.go") {
		t.Fatalf("crash %+v", c)
	}
	kept, err := g.Keep(time.Now())
	if err != nil || filepath.Dir(kept) != dir || !strings.Contains(filepath.Base(kept), "book-") {
		t.Fatalf("kept %q, %v", kept, err)
	}

	again := New(m.sheet, "book.012") // the file on disk doesn't matter here
	again.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	again.prefs.RecoveryDir = dir
	again.OfferKept()
	run(again, again.Init())
	if !strings.Contains(line(again, contextLine), "Unsaved changes to book.012 were kept.") {
		t.Fatalf("no offer: %q", line(again, contextLine))
	}
	// Another directory's book.012 is another file.
	other := newModel()
	other.prefs.RecoveryDir = dir
	t.Chdir(t.TempDir())
	other.filename = "book.012"
	other.OfferKept()
	run(other, other.Init())
	if strings.Contains(line(other, contextLine), "were kept") {
		t.Errorf("offered another directory's book: %q", line(other, contextLine))
	}
}

// Nothing unsaved, nothing kept.
func TestGuardKeepsOnlyUnsaved(t *testing.T) {
	m := newModel()
	m.prefs.RecoveryDir = t.TempDir()
	if kept, err := Guard(m).Keep(time.Now()); kept != "" || err != nil {
		t.Errorf("kept %q, %v", kept, err)
	}
}

// A panic in a command, or in one of a batch it returns, comes back to
// Update as a crash, which quits.
func TestGuardCommandPanic(t *testing.T) {
	panicOn(t, "command")
	g := Guard(newModel())
	_, cmd := g.Update(f12)
	msg := cmd()
	if _, ok := msg.(crashMsg); !ok {
		t.Fatalf("got %T", msg)
	}
	if _, cmd := g.Update(msg); !isQuit(cmd) {
		t.Error("a crash didn't quit")
	}
	if c := g.Finish(nil); c == nil || c.Where != "a command" {
		t.Errorf("crash %+v", c)
	}

	g = Guard(newModel())
	fine := func() tea.Msg { return nil }
	boom := func() tea.Msg { panic("in a batch") }
	batch, ok := g.guard(tea.Batch(fine, boom))().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("batch %v", batch)
	}
	if c, ok := batch[1]().(crashMsg); !ok || fmt.Sprint(c.crash.Value) != "in a batch" {
		t.Errorf("a batch's panic: %v", batch[1]())
	}
}

// A panic in View goes on to Bubble Tea, to stop the program, once the
// crash is noted; the last frame is shown after it.
func TestGuardViewPanic(t *testing.T) {
	panicOn(t, "view")
	g := Guard(newModel())
	before := g.View()
	g.Update(f12)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("View didn't panic")
			}
		}()
		g.View()
	}()
	if c := g.Finish(errors.Join(tea.ErrProgramKilled, tea.ErrProgramPanic)); c == nil || c.Where != "view" {
		t.Fatalf("crash %+v", c)
	}
	if g.View().Content != before.Content {
		t.Error("after the crash, View isn't the last frame")
	}
}

// A panic only Bubble Tea caught is still a crash.
func TestGuardFinishBubbleTeaPanic(t *testing.T) {
	g := Guard(newModel())
	if c := g.Finish(nil); c != nil {
		t.Errorf("a clean quit crashed: %+v", c)
	}
	if c := g.Finish(tea.ErrProgramPanic); c == nil || c.Where != "a command" {
		t.Errorf("crash %+v", c)
	}
}

// The report says where, what with, the build, the file and where its
// work went, with the stack; at most crashReportsKept are left.
func TestCrashReport(t *testing.T) {
	dir := t.TempDir()
	c := &Crash{Where: "update", Value: "boom", Stack: []byte("goroutine 1 [running]:\nmain.main()\n")}
	at := time.Date(2026, 9, 29, 10, 11, 12, 0, time.Local)
	path, err := c.Report(dir, "v9.9.9", "book.012", "/r/book-1.012", nil, at)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "crash-20260929-101112.txt" {
		t.Errorf("path %s", path)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"a panic in update", "Version: v9.9.9", "File:    book.012", "Kept:    /r/book-1.012", "Panic: boom", "main.main()"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("report lacks %q:\n%s", want, data)
		}
	}
	for i := range crashReportsKept + 3 {
		if _, err := c.Report(dir, "", "", "", errors.New("disk full"), at.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	names, _ := filepath.Glob(filepath.Join(dir, "crash-*.txt"))
	if len(names) != crashReportsKept || filepath.Base(names[0]) != "crash-20260929-101116.txt" {
		t.Errorf("kept %d reports, first %s", len(names), filepath.Base(names[0]))
	}
	data, _ = os.ReadFile(names[0])
	if !strings.Contains(string(data), "couldn't keep the unsaved changes: disk full") || !strings.Contains(string(data), "(untitled)") {
		t.Errorf("report:\n%s", data)
	}
}

// A path too long for a file name still makes a key that fits, and
// different long paths make different keys.
func TestRecoveryKeyLong(t *testing.T) {
	long := "/" + strings.Repeat("dir/", 80)
	a, b := recoveryKey(long+"a.012"), recoveryKey(long+"b.012")
	if len(a) > recoveryKeyMax || a == b || !strings.HasSuffix(a, "a") {
		t.Errorf("keys %q, %q", a, b)
	}
}
