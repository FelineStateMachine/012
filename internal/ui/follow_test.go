package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// pump polls every region's source once, as the ticks would, and
// applies what the polls found.
func pump(t *testing.T, m *Model) {
	t.Helper()
	followInterval = 0
	run(m, m.syncFollowers())
	for range 3 { // a stale region reads, then the tick after
		for _, f := range m.follow.by {
			if f.waiting {
				send(m, followTickMsg{f})
			}
		}
	}
}

func appendFile(t *testing.T, name, text string) {
	t.Helper()
	f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString(text)
}

// linkLog links log.csv at the active cell, keeping every row.
func linkLog(t *testing.T, m *Model) {
	t.Helper()
	m.runCommand("data.link")
	press(t, m, "log.csv", "<enter>")
	if !strings.Contains(line(m, contextLine), "Follow log.csv") {
		t.Fatalf("no question: %q", line(m, contextLine))
	}
	press(t, m, "<enter>")
	pump(t, m)
}

func TestLinkFollowsAppends(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "log.csv", "level,n\ninfo,1\nwarn,2\n")
	m := newModel()
	press(t, m, "<right>", "<down>") // B2
	linkLog(t, m)
	if got := m.sheet.Value(addr("C4")).Num; got != 2 || m.sheet.Value(addr("B2")).Str != "level" {
		t.Fatalf("C4 %v, B2 %v; %q", got, m.sheet.Value(addr("B2")), m.errMsg)
	}
	press(t, m, "<left>", "<up>", "=SUM(C:C)", "<enter>")
	m.changed = false
	appendFile(t, "log.csv", "error,4\npartial,")
	pump(t, m)
	if m.sheet.Value(addr("A1")).Num != 7 || m.sheet.Filled(addr("B6")) {
		t.Fatalf("after appending: SUM %v, B6 %v", m.sheet.Value(addr("A1")), m.sheet.Value(addr("B6")))
	}
	if m.changed {
		t.Error("rows arriving marked the file changed")
	}
	// The tab and the context line say it's following.
	m.cur = addr("B3")
	if l := line(m, contextLine); !strings.Contains(l, "● Following log.csv  3 rows") {
		t.Errorf("context line %q", l)
	}
	if st := status(m); !strings.Contains(st, "Sheet1 ●") {
		t.Errorf("status %q", st)
	}
	// Typing into the region is refused.
	press(t, m, "x", "<enter>")
	if m.sheet.Value(addr("B3")).Str != "info" {
		t.Errorf("typed over a linked cell: %v", m.sheet.Value(addr("B3")))
	}
	m.mode, m.errMsg = modeReady, ""
	m.cur = addr("B3")
	m.runCommand("clear")
	if !strings.Contains(m.note, "linked file") || m.sheet.Value(addr("B3")).Str != "info" {
		t.Errorf("clear: note %q, B3 %v", m.note, m.sheet.Value(addr("B3")))
	}

	// Paused, it reads nothing; resumed, it catches up.
	m.runCommand("data.link_follow")
	appendFile(t, "log.csv", "5\n")
	pump(t, m)
	if m.sheet.Filled(addr("B6")) || !strings.Contains(ansi.Strip(m.linkedLine()), "‖ Paused log.csv") {
		t.Fatalf("paused, yet read: %q", m.linkedLine())
	}
	m.runCommand("data.link_follow")
	pump(t, m)
	if m.sheet.Value(addr("B6")).Str != "partial" || m.sheet.Value(addr("A1")).Num != 12 {
		t.Fatalf("resumed: B6 %v, SUM %v", m.sheet.Value(addr("B6")), m.sheet.Value(addr("A1")))
	}

	// Keeping the last two rows.
	m.runCommand("data.link_rows")
	press(t, m, "l", "2", "<enter>")
	pump(t, m)
	if m.sheet.Value(addr("B3")).Str != "error" || m.sheet.Filled(addr("B5")) || m.sheet.Value(addr("B2")).Str != "level" {
		t.Fatalf("window: B3 %v B5 %v", m.sheet.Value(addr("B3")), m.sheet.Value(addr("B5")))
	}
	if l := line(m, contextLine); !strings.Contains(l, "2 rows (the last 2)") {
		t.Errorf("context line %q", l)
	}

	// Unlinking keeps the values as ordinary cells.
	m.runCommand("data.unlink")
	run(m, m.syncFollowers())
	if m.sheet.HasLinked() || len(m.follow.by) != 0 || m.sheet.Cell(addr("B3")).Spilled() || m.sheet.Value(addr("B3")).Str != "error" {
		t.Fatalf("unlinked: %d followers, B3 %+v", len(m.follow.by), m.sheet.Cell(addr("B3")))
	}
	// Undo brings the region back, reading the file again.
	press(t, m, "<ctrl+z>")
	pump(t, m)
	if !m.sheet.HasLinked() || m.sheet.Value(addr("B3")).Str != "error" || !m.sheet.Cell(addr("B3")).Spilled() {
		t.Fatalf("undo: B3 %+v", m.sheet.Cell(addr("B3")))
	}
}

func TestFollowInNewSheetAndReopen(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "log.csv", "a\n1\n")
	m := newModel()
	m.SetMachine("here")
	press(t, m, "x", "<enter>")
	m.runCommand("file.import")
	press(t, m, "log.csv", "<enter>", "follow", "<enter>", "<enter>")
	pump(t, m)
	if m.sheet.Name() != "log" || m.sheet.Value(addr("A2")).Num != 1 {
		t.Fatalf("sheet %q, A2 %v (%q)", m.sheet.Name(), m.sheet.Value(addr("A2")), m.errMsg)
	}
	// Saved in a folder of its own, the path is kept relative to it.
	os.Mkdir("books", 0o755)
	press(t, m, "<ctrl+s>", filepath.Join("books", "b"), "<enter>")
	data, _ := os.ReadFile(filepath.Join("books", "b.012"))
	if !strings.Contains(string(data), `"path":"../log.csv"`) {
		t.Fatalf("saved:\n%s", data)
	}
	if rs := m.book().LinkedRegions(); rs[0].Source.Path != filepath.Join("..", "log.csv") {
		t.Fatalf("path after saving: %q", rs[0].Source.Path)
	}
	appendFile(t, "log.csv", "2\n")
	pump(t, m)
	if m.sheet.Value(addr("A3")).Num != 2 {
		t.Fatalf("after saving elsewhere, A3 %v", m.sheet.Value(addr("A3")))
	}

	// Reopened, it follows again; a missing file shows in the region.
	m2 := newModel()
	m2.SetMachine("here")
	press(t, m2, "<ctrl+o>", filepath.Join("books", "b"), "<enter>")
	m2.showSheet(m2.book().Lookup("log"))
	pump(t, m2)
	if m2.sheet.Value(addr("A3")).Num != 2 {
		t.Fatalf("reopened: A3 %v", m2.sheet.Value(addr("A3")))
	}
	os.Remove("log.csv")
	pump(t, m2)
	m2.cur = addr("A1")
	if l := line(m2, contextLine); !strings.Contains(l, "! log.csv: the file isn't there") {
		t.Errorf("context line %q", l)
	}
	if st := status(m2); !strings.Contains(st, "log !") {
		t.Errorf("status %q", st)
	}
}

func TestLinksFromElsewhereAskOnce(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	outside := filepath.Join(t.TempDir(), "far.csv")
	os.WriteFile(outside, []byte("a\n1\n"), 0o644)
	book := `{"version": 2, "linkOrigin": "another", "cells": {}, "links": [{"at": "A1", "path": ` + jsonQuote(outside) + `}]}`
	writeFile(t, "b.012", book)
	for _, answer := range []string{"<esc>", "<enter>"} {
		m := newModel()
		m.SetMachine("this-one")
		press(t, m, "<ctrl+o>", "b", "<enter>")
		if !strings.Contains(line(m, contextLine), "made on another computer") {
			t.Fatalf("no question: %q", line(m, contextLine))
		}
		press(t, m, answer)
		pump(t, m)
		followed := m.sheet.Value(addr("A2")).Num == 1
		if followed != (answer == "<enter>") {
			t.Errorf("%s: followed %v", answer, followed)
		}
		if answer == "<enter>" && m.book().LinkOrigin() != "this-one" {
			t.Errorf("trusting didn't record this computer: %q", m.book().LinkOrigin())
		}
	}
	// A file in the workbook's own folder needs no question.
	writeFile(t, "near.csv", "a\n1\n")
	writeFile(t, "c.012", `{"version": 2, "linkOrigin": "another", "cells": {}, "links": [{"at": "A1", "path": "near.csv"}]}`)
	m := newModel()
	m.SetMachine("this-one")
	press(t, m, "<ctrl+o>", "c", "<enter>")
	pump(t, m)
	if m.sheet.Value(addr("A2")).Num != 1 || m.overlay != nil {
		t.Errorf("a file beside the workbook: A2 %v, overlay %T", m.sheet.Value(addr("A2")), m.overlay)
	}
}

func TestLinkPath(t *testing.T) {
	for _, c := range []struct{ name, book, want string }{
		{"log.csv", "", "log.csv"},
		{"logs/a.csv", "books/b.012", "../logs/a.csv"},
		{"books/a.csv", "books/b.012", "a.csv"},
	} {
		if got := linkPath(c.name, c.book); got != filepath.FromSlash(c.want) {
			t.Errorf("linkPath(%q, %q) = %q, want %q", c.name, c.book, got, c.want)
		}
	}
	abs := filepath.Join(t.TempDir(), "x.csv")
	if got := linkPath(abs, "b.012"); got != abs {
		t.Errorf("an absolute path outside: %q", got)
	}
	if got := linkNameIn("../a.csv", "books/b.012"); got != "a.csv" {
		t.Errorf("linkNameIn: %q", got)
	}
}

func TestFreeSheetName(t *testing.T) {
	w := sheet.NewBook()
	w.RenameSheet(w.Sheet(0), "log")
	if got := freeSheetName(w, "log"); got != "log 2" {
		t.Errorf("%q", got)
	}
	if got := freeSheetName(w, "a:b"); got != "a_b" {
		t.Errorf("%q", got)
	}
}

func jsonQuote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }
