package ui

import (
	"strings"
	"testing"
)

// hiddenBook is a model of three sheets, Sheet1 reading Data, showing
// Data.
func hiddenBook(t *testing.T) *Model {
	t.Helper()
	m := newModel()
	press(t, m, "=", "D", "a", "t", "a", "!", "A", "1", "*", "2", "<enter>")
	press(t, m, "<shift+f11>")
	run(m, m.runCommand("sheet.rename"))
	m.line.Set("Data")
	press(t, m, "<enter>", "2", "1", "<enter>", "<shift+f11>", "<ctrl+pgup>")
	if sheetNames(m) != "Sheet1,Data,Sheet3" || m.sheet.Name() != "Data" {
		t.Fatalf("sheets %s, on %s", sheetNames(m), m.sheet.Name())
	}
	return m
}

func TestHideSheetCommand(t *testing.T) {
	m := hiddenBook(t)
	run(m, m.runCommand("sheet.hide"))
	if m.sheet.Name() != "Sheet3" || !m.book().Lookup("Data").Hidden() {
		t.Fatalf("after hiding: on %s", m.sheet.Name())
	}
	if s := status(m); strings.Contains(s, "Data") || !strings.HasPrefix(s, " Sheet1   Sheet3   +") {
		t.Errorf("tabs %q", s)
	}
	if !strings.Contains(line(m, contextLine), "Hid Data; View > Hidden sheets shows it again") {
		t.Errorf("context %q", line(m, contextLine))
	}
	press(t, m, "<ctrl+pgup>") // skips Data
	if m.sheet.Name() != "Sheet1" || m.sheet.Value(addr("A1")).Num != 42 {
		t.Errorf("previous: %s, A1 = %v", m.sheet.Name(), m.sheet.Value(addr("A1")))
	}
	press(t, m, "<ctrl+pgdown>")
	if m.sheet.Name() != "Sheet3" {
		t.Errorf("next: %s", m.sheet.Name())
	}
	// Move left goes past the hidden sheet to the visible neighbour.
	run(m, m.runCommand("sheet.move_left"))
	if sheetNames(m) != "Sheet3,Sheet1,Data" {
		t.Errorf("moved left: %s", sheetNames(m))
	}
	run(m, m.runCommand("edit.undo"))

	// The last visible sheet can't be hidden or deleted.
	run(m, m.runCommand("sheet.hide"))
	if m.sheet.Name() != "Sheet1" || commands["sheet.hide"].available(m) || commands["sheet.delete"].available(m) {
		t.Fatalf("one visible sheet left: on %s", m.sheet.Name())
	}
	if !strings.HasPrefix(status(m), " Sheet1   +") {
		t.Errorf("tabs %q", status(m))
	}

	// Going to a cell on a hidden sheet says it's hidden.
	run(m, m.runCommand("goto"))
	m.line.Set("Data!A1")
	press(t, m, "<enter>")
	if m.sheet.Name() != "Sheet1" || m.mode != modeError || m.errMsg != "Data is hidden; View > Hidden sheets shows it again" {
		t.Errorf("goto: on %s, %q", m.sheet.Name(), m.errMsg)
	}
	press(t, m, "<esc>")

	// Undo shows the sheet again; redo hides it and shows another.
	press(t, m, "<ctrl+z>")
	if m.sheet.Name() != "Sheet3" || m.sheet.Hidden() {
		t.Errorf("undo: on %s", m.sheet.Name())
	}
	press(t, m, "<ctrl+y>")
	if m.sheet.Hidden() || m.sheet.Name() != "Sheet1" {
		t.Errorf("redo: on %s (hidden %v)", m.sheet.Name(), m.sheet.Hidden())
	}
}

func TestHiddenSheetsPicker(t *testing.T) {
	m := hiddenBook(t)
	if commands["sheet.unhide"].available(m) {
		t.Error("Hidden sheets available with none hidden")
	}
	run(m, m.runCommand("sheet.hide"))
	run(m, m.runCommand("sheet.unhide"))
	p, ok := m.overlay.(*picker)
	if !ok || p.title != "Hidden sheets" || len(p.items) != 1 || p.items[0].title != "Data" {
		t.Fatalf("overlay %#v", m.overlay)
	}
	if !strings.Contains(screen(m), "Hidden sheets") || !strings.Contains(screen(m), "A1, 1 cell") {
		t.Errorf("picker:\n%s", screen(m))
	}
	press(t, m, "<enter>")
	if m.overlay != nil || m.sheet.Name() != "Data" || m.sheet.Hidden() {
		t.Fatalf("unhid: on %s", m.sheet.Name())
	}
	if !strings.HasPrefix(status(m), " Sheet1   Data   Sheet3") || !strings.Contains(line(m, contextLine), "Unhid Data") {
		t.Errorf("status %q, context %q", status(m), line(m, contextLine))
	}
	press(t, m, "<ctrl+z>")
	if !m.book().Lookup("Data").Hidden() || m.sheet.Name() == "Data" {
		t.Errorf("undo unhide: on %s", m.sheet.Name())
	}
}

// Hiding is recorded as a command, unhiding with the sheet picked as
// its answer, and both replay.
func TestRecordHideAndUnhide(t *testing.T) {
	m := hiddenBook(t)
	send(m, nil)
	run(m, m.runCommand("macro.record"))
	run(m, m.runCommand("sheet.hide"))
	run(m, m.runCommand("sheet.unhide"))
	press(t, m, "<enter>")
	run(m, m.runCommand("macro.stop"))
	m.line.Set("Toggle")
	press(t, m, "<enter>")
	m.line.Set("")
	press(t, m, "<enter>")
	mc, ok := m.book().Macro("Toggle")
	if !ok {
		t.Fatalf("no macro: %q", m.errMsg)
	}
	want := "run(\"sheet.hide\")\nrun(\"sheet.unhide\", answer=\"Data\")"
	if !strings.Contains(mc.Source, want) {
		t.Errorf("script:\n%s\nwant it to contain\n%s", mc.Source, want)
	}

	m = hiddenBook(t)
	script(t, m, `
run("sheet.hide")
set("A2", active_sheet())
run("sheet.unhide", answer="data")
`)
	if m.sheet.Name() != "Data" || m.sheet.Hidden() || m.book().Lookup("Sheet3").Value(addr("A2")).Str != "Sheet3" {
		t.Errorf("replay: on %s, %q", m.sheet.Name(), m.errMsg)
	}
	script(t, m, `run("sheet.hide")
run("sheet.unhide", answer="Nope")`)
	if !strings.Contains(m.warn, `Hidden sheets asks for one of Data: give run("sheet.unhide", answer=...)`) {
		t.Errorf("bad answer: %q", m.warn)
	}
}
