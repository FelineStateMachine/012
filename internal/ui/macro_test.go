package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// record records keys as a macro named name with the shortcut key, from
// wherever m is, and returns its script.
func record(t *testing.T, m *Model, relative bool, name, key string, keys ...string) string {
	t.Helper()
	id := "macro.record"
	if relative {
		id = "macro.record_relative"
	}
	send(m, nil) // an idle update, as between keys
	run(m, m.runCommand(id))
	if m.rec == nil {
		t.Fatal("not recording")
	}
	if !strings.Contains(line(m, menuLine), "REC") {
		t.Errorf("no REC chip: %q", line(m, menuLine))
	}
	press(t, m, keys...)
	run(m, m.runCommand("macro.stop"))
	if m.mode != modePrompt || !strings.HasPrefix(m.prompt.label, "Save macro as") {
		t.Fatalf("no name prompt: mode %v", m.mode)
	}
	m.line.set(name)
	press(t, m, "<enter>")
	m.line.set(key)
	press(t, m, "<enter>")
	if m.rec != nil {
		t.Fatalf("still recording: %q %q", m.note, m.errMsg)
	}
	mc, ok := m.book().Macro(name)
	if !ok {
		t.Fatalf("no macro %s: %q", name, m.errMsg)
	}
	return mc.Source
}

// body is a script without its header comment.
func body(src string) string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(src), "\n") {
		if !strings.HasPrefix(l, "# Recorded") && !strings.HasPrefix(l, "# Edit it") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// fileOf is the workbook as saved, for comparing two of them.
func fileOf(t *testing.T, m *Model) string {
	t.Helper()
	var b bytes.Buffer
	if err := m.book().Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// withMacro gives a fresh model the macro from another, trusted.
func withMacro(t *testing.T, from *Model, name string) *Model {
	t.Helper()
	m := newModel()
	mc, _ := from.book().Macro(name)
	if err := m.book().SaveMacro("", mc, "add"); err != nil {
		t.Fatal(err)
	}
	m.trustHere()
	return m
}

func TestRecordAbsoluteAndReplay(t *testing.T) {
	m := newModel()
	src := record(t, m, false, "Header", "1",
		"<down>", "Rent", "<tab>", "1450", "<enter>",
		"<right>", "<right>", "=B2*12", "<enter>",
		"<up>", "<shift+left>", "<ctrl+b>",
		"<ctrl+home>")
	want := `select("A2")
enter("Rent")
select("B2")
enter("1450")
select("C3")
enter("=B2*12")
select("B3:C3", active="C3")
run("format.bold")
select("A1")`
	// The selection is recorded when something acts on it, so Enter's
	// move to A3 and the arrows to C3 are one select.
	if got := body(src); got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	if !strings.HasPrefix(src, "# Recorded with absolute references") {
		t.Errorf("header: %q", src)
	}

	// Replaying on a new sheet from anywhere does the same.
	r := withMacro(t, m, "Header")
	press(t, r, "<down>", "<down>", "<right>")
	press(t, r, "<ctrl+alt+shift+1>")
	if r.note != "Ran Header" {
		t.Fatalf("note %q warn %q err %q", r.note, r.warn, r.errMsg)
	}
	if fileOf(t, r) != fileOf(t, m) {
		t.Errorf("replay differs:\n%s\nrecorded:\n%s", fileOf(t, r), fileOf(t, m))
	}
	if r.cur != m.cur || r.selection() != m.selection() {
		t.Errorf("replay left %v, recording %v", r.selection(), m.selection())
	}
}

func TestRecordRelativeReplaysFromTheActiveCell(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "<down>") // B2
	src := record(t, m, true, "Pair", "",
		"Name", "<tab>", "=LEN(B2)", "<enter>",
		"<shift+right>", "<ctrl+i>", "<ctrl+down>")
	want := `enter("Name")
move(1, 0)
enter("=LEN(B2)", origin="C2")
move(-1, 1)
extend(1, 0)
run("format.italic")
jump("down")`
	if got := body(src); got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}

	r := withMacro(t, m, "Pair")
	press(t, r, "<down>", "<down>", "<down>", "<right>", "<right>", "<right>") // D4
	run(r, r.runMacro(mustMacro(t, r, "Pair")))
	if got := input(r, "D4") + "|" + input(r, "E4"); got != "Name|=LEN(D4)" {
		t.Errorf("replayed at D4: %q", got)
	}
	if c := r.sheet.Cell(addr("E5")); c == nil || !c.Style.Italic {
		t.Error("E5 isn't italic")
	}
	if r.cur != addr("D1048576") {
		t.Errorf("jump down ended at %v", r.cur)
	}
}

func mustMacro(t *testing.T, m *Model, name string) sheet.Macro {
	t.Helper()
	mc, ok := m.book().Macro(name)
	if !ok {
		t.Fatalf("no macro %s", name)
	}
	return mc
}

func TestReplayIsOneUndoStep(t *testing.T) {
	m := newModel()
	record(t, m, false, "Three", "2", "1", "<enter>", "2", "<enter>", "=A1+A2", "<enter>", "<shift+up>", "<ctrl+i>")
	r := withMacro(t, m, "Three")
	before := r.book().UndoLabel()
	press(t, r, "<ctrl+alt+shift+2>")
	if input(r, "A3") != "=A1+A2" || r.sheet.Value(addr("A3")).Num != 3 {
		t.Fatalf("A3 %q = %v", input(r, "A3"), r.sheet.Value(addr("A3")))
	}
	if got := r.book().UndoLabel(); got != "run macro Three" {
		t.Errorf("undo step %q (before %q)", got, before)
	}
	press(t, r, "<ctrl+z>")
	if r.sheet.Len() != 0 || r.note != "Undid: run macro Three" {
		t.Errorf("after undo: %d cells, note %q", r.sheet.Len(), r.note)
	}
	if r.book().UndoLabel() != before {
		t.Errorf("more than one step: %q", r.book().UndoLabel())
	}
	press(t, r, "<ctrl+y>")
	if input(r, "A2") != "2" {
		t.Error("redo")
	}
}

func TestRecordingAnswersAndSkips(t *testing.T) {
	m := newModel()
	press(t, m, "x", "<enter>")
	src := record(t, m, false, "Widths", "",
		"<f10>", // a menu opened and closed records nothing
		"<esc>",
		"<ctrl+z>", // undo isn't recorded; the macro says so
	)
	run(m, nil)
	if !strings.Contains(src, "# Not recorded: undid: enter x in a1") && !strings.Contains(src, "# Not recorded: undid") {
		t.Errorf("recorded:\n%s", src)
	}

	m = newModel()
	run(m, m.runCommand("macro.record"))
	run(m, m.runCommand("column.width"))
	m.line.set("15")
	press(t, m, "<enter>")
	run(m, m.runCommand("macro.stop"))
	press(t, m, "<enter>", "<enter>")
	if got := body(mustMacro(t, m, "Macro 1").Source); got != `run("column.width", answer="15")` {
		t.Errorf("recorded %q", got)
	}
	r := withMacro(t, m, "Macro 1")
	run(r, r.runMacro(mustMacro(t, r, "Macro 1")))
	if r.sheet.ColWidth(0) != 15 || r.mode != modeReady {
		t.Errorf("width %d mode %v %q", r.sheet.ColWidth(0), r.mode, r.warn)
	}
}

func TestRecordingTheMouse(t *testing.T) {
	for _, relative := range []bool{false, true} {
		m := newModel()
		press(t, m, "1", "<enter>", "2", "<enter>", "<up>", "<up>", "<shift+down>")
		run(m, m.runCommand(map[bool]string{false: "macro.record", true: "macro.record_relative"}[relative]))
		m.startFill() // drag the fill handle from A1:A2 down to A5
		m.dragFillTo(addr("A5"))
		m.finishFill()
		m.autofit(1) // double-click column B's border
		run(m, m.runCommand("macro.stop"))
		press(t, m, "<enter>", "<enter>")
		want := `fill(to="A1:A5")` + "\n" + `set_width("B", 3)`
		if relative {
			want = `fill(rows=3)` + "\n" + `set_width("B", 3)`
		}
		if got := body(mustMacro(t, m, "Macro 1").Source); got != want {
			t.Errorf("relative %v: recorded\n%s\nwant\n%s", relative, got, want)
		}
		r := withMacro(t, m, "Macro 1")
		press(t, r, "1", "<enter>", "2", "<enter>", "<up>", "<up>", "<shift+down>")
		run(r, r.runMacro(mustMacro(t, r, "Macro 1")))
		if input(r, "A5") != "5" || r.sheet.ColWidth(1) != 3 {
			t.Errorf("relative %v: A5 %q, width %d, %q", relative, input(r, "A5"), r.sheet.ColWidth(1), r.warn)
		}
	}
}

func TestStopWithNothingRecorded(t *testing.T) {
	m := newModel()
	run(m, m.runCommand("macro.record"))
	run(m, m.runCommand("macro.stop"))
	if m.rec != nil || !strings.Contains(m.note, "Nothing was recorded") {
		t.Errorf("rec %v note %q", m.rec, m.note)
	}
	// Esc on the name keeps recording.
	run(m, m.runCommand("macro.record"))
	press(t, m, "a", "<enter>")
	run(m, m.runCommand("macro.stop"))
	press(t, m, "<esc>")
	if m.rec == nil || m.note != "Still recording" {
		t.Errorf("rec %v note %q", m.rec, m.note)
	}
}

func TestMacrosFromElsewhereAskOnce(t *testing.T) {
	w := sheet.NewBook()
	w.SaveMacro("", sheet.Macro{Name: "Hi", Key: "3", Source: `set("A1", "hi")`}, "add")
	w.SetMacroOrigin("someone-else")
	var b bytes.Buffer
	w.Write(&b)
	s, err := sheet.Read(&b)
	if err != nil {
		t.Fatal(err)
	}
	m := New(s, "hi.012")
	m.SetMachine("this-one")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if m.sheet.Len() != 0 {
		t.Fatal("opening ran a macro")
	}
	press(t, m, "<ctrl+alt+#>") // Ctrl+Alt+Shift+3 without the kitty protocol
	if !strings.Contains(line(m, contextLine), "Trust this file's macros?") || !strings.Contains(line(m, m.height-1), "made on another computer") {
		t.Fatalf("no trust question: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if m.sheet.Len() != 0 {
		t.Fatal("ran after cancel")
	}
	press(t, m, "<ctrl+alt+shift+3>", "<enter>")
	if input(m, "A1") != "hi" || m.book().MacroOrigin() != "this-one" {
		t.Fatalf("A1 %q origin %q", input(m, "A1"), m.book().MacroOrigin())
	}
	press(t, m, "<delete>", "<ctrl+alt+shift+3>")
	if input(m, "A1") != "hi" {
		t.Error("asked twice")
	}
}

func TestPaletteAndPickerRunMacros(t *testing.T) {
	m := newModel()
	record(t, m, false, "Stamp", "5", "stamp", "<enter>")
	press(t, m, "<ctrl+k>", "stamp")
	if !strings.Contains(screen(m), "Ctrl+Alt+Shift+5") {
		t.Errorf("palette:\n%s", screen(m))
	}
	press(t, m, "<enter>")
	if m.note != "Ran Stamp" {
		t.Errorf("note %q", m.note)
	}
	run(m, m.runCommand("macro.run"))
	press(t, m, "<enter>")
	if m.note != "Ran Stamp" || m.overlay != nil {
		t.Errorf("picker: note %q", m.note)
	}
}

func TestManageMacros(t *testing.T) {
	m := newModel()
	m.AllowEditor()
	record(t, m, false, "One", "1", "1", "<enter>")
	run(m, m.runCommand("macro.manage"))
	if !strings.Contains(screen(m), "+ Write a macro") || !strings.Contains(screen(m), "One") {
		t.Fatalf("manager:\n%s", screen(m))
	}
	press(t, m, "<down>", "<f2>")
	m.line.set("Uno")
	press(t, m, "<enter>")
	press(t, m, "<f3>")
	m.line.set("7")
	press(t, m, "<enter>")
	mc := mustMacro(t, m, "Uno")
	if mc.Key != "7" {
		t.Errorf("key %q", mc.Key)
	}
	press(t, m, "<ctrl+d>")
	if len(m.book().Macros()) != 0 || !strings.Contains(line(m, m.height-1), "Deleted Uno") {
		t.Errorf("after delete: %v %q", m.book().Macros(), line(m, m.height-1))
	}
	press(t, m, "<esc>", "<ctrl+z>")
	if _, ok := m.book().Macro("Uno"); !ok {
		t.Error("undo didn't bring it back")
	}
}

func TestWithoutAnEditorScriptsAreReadOnly(t *testing.T) {
	m := newModel()
	record(t, m, false, "One", "", "1", "<enter>")
	if commands["macro.new"].available(m) {
		t.Error("Write a macro is available without an editor")
	}
	run(m, m.runCommand("macro.manage"))
	if strings.Contains(screen(m), "+ Write a macro") {
		t.Error("the manager offers to write a macro")
	}
	if cmd := m.overlay.Key(tea.KeyPressMsg{Code: tea.KeyF4}); cmd != nil || !strings.Contains(line(m, m.height-1), "No editor in this session") {
		t.Errorf("F4: %v %q", cmd, line(m, m.height-1))
	}
}

func TestEditedScriptsAreSavedAndChecked(t *testing.T) {
	m := newModel()
	m.AllowEditor()
	record(t, m, false, "Ed", "", "a", "<enter>")
	path := filepath.Join(t.TempDir(), "ed.star")
	os.WriteFile(path, []byte("set(\"B1\", 2)\nset(\"B2\",\n"), 0o600)
	m.Update(macroEditedMsg{name: "Ed", path: path, orig: "x"})
	if !strings.Contains(m.warn, "Saved Ed, but it has a mistake at Ed:3:1:") {
		t.Errorf("warn %q", m.warn)
	}
	if mustMacro(t, m, "Ed").Source != "set(\"B1\", 2)\nset(\"B2\",\n" {
		t.Error("not saved")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("temp file left")
	}

	// A new script is trusted, and runs.
	path = filepath.Join(t.TempDir(), "new.star")
	os.WriteFile(path, []byte("set(\"C1\", 3)\n"), 0o600)
	m.Update(macroEditedMsg{name: "New", path: path, orig: newMacroTemplate, isNew: true})
	run(m, m.runMacro(mustMacro(t, m, "New")))
	if input(m, "C1") != "3" {
		t.Errorf("C1 %q warn %q", input(m, "C1"), m.warn)
	}
}
