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
	if r.cur != addr("D8192") {
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

// script saves src as a trusted macro and runs it to the end.
func script(t *testing.T, m *Model, src string) {
	t.Helper()
	old := ""
	if hasMacro(m.book(), "S") {
		old = "S"
	}
	if err := m.book().SaveMacro(old, sheet.Macro{Name: "S", Source: src}, "add"); err != nil {
		t.Fatal(err)
	}
	m.trustHere()
	run(m, m.runMacro(mustMacro(t, m, "S")))
}

func TestScriptsReadWhatTheyWrote(t *testing.T) {
	m := newModel()
	script(t, m, `
set("A1:A3", [[1], [2], [3]])
set_formula("B1:B3", "A1*10")
total = 0
for row in get("B1:B3"):
    total += row[0]
set("C1", total)
set("C2", get_formula("B2"))
number_format("C1", "currency", decimals=0)
set("C3", get_number_format("C1"))
add_sheet("Two")
set("A1", "on " + active_sheet())
activate_sheet("Sheet1")
select("A:B", active="B5")
set("D1", selection() + " " + active_cell())
print("total", total)
`)
	if m.warn != "" {
		t.Fatal(m.warn)
	}
	for a, want := range map[string]string{"C1": "60", "C2": "=A2*10", "C3": "currency", "D1": "A:B B5"} {
		if got := input(m, a); got != want {
			t.Errorf("%s = %q, want %q", a, got, want)
		}
	}
	if two := m.book().Lookup("Two"); two == nil || two.Cell(addr("A1")).Input != "on Two" {
		t.Error("sheet Two")
	}
	if m.whole != wholeCols || m.cur != addr("B5") {
		t.Errorf("selection %v whole %v", m.selection(), m.whole)
	}
	if m.note != "S: total 60" {
		t.Errorf("note %q", m.note)
	}
}

func TestScriptErrorsShowWhere(t *testing.T) {
	m := newModel()
	script(t, m, "set(\"A1\", 5)\nrun(\"format.bold\")\nget(\"nowhere\")\n")
	want := "S:3:4: get: not a cell or range: nowhere   Ctrl+Z undoes what it did"
	if m.warn != want {
		t.Errorf("warn %q\nwant %q", m.warn, want)
	}
	if !strings.Contains(line(m, contextLine), "S:3:4: get: not a cell") {
		t.Errorf("context line %q", line(m, contextLine))
	}
	press(t, m, "<ctrl+z>")
	if m.sheet.Len() != 0 {
		t.Errorf("undo left cells: note %q A1 %q undo %q", m.note, input(m, "A1"), m.book().UndoLabel())
	}

	for src, want := range map[string]string{
		`run("file.save")`:       "S:1:4: run: Save (file.save) can't run in a macro",
		`run("column.width")`:    `S:1:4: run: Column width asks "Column width (1-240)": give run("column.width", answer=...)`,
		`run("no.such")`:         `S:1:4: run: no command "no.such"`,
		`run("data.sort_range")`: "S:1:4: run: Sort range opens a dialog; it can't run in a macro",
		"move(0, -1)":            "S:1:5: move: moving 0, -1 from A1 goes off the sheet",
	} {
		m := newModel()
		script(t, m, src)
		if !strings.HasPrefix(m.warn, want) {
			t.Errorf("%s: %q, want %q", src, m.warn, want)
		}
		if m.mode != modeReady || m.overlay != nil || m.prompt != nil {
			t.Errorf("%s left mode %v", src, m.mode)
		}
	}
}

func TestStepLimitStopsARunaway(t *testing.T) {
	defer func(n uint64) { macroMaxSteps = n }(macroMaxSteps)
	macroMaxSteps = 50_000
	m := newModel()
	script(t, m, "set(\"A1\", 1)\nwhile True:\n    pass\n")
	if !strings.HasPrefix(m.warn, "S:") || !strings.Contains(m.warn, ": stopped after 50") || !strings.HasSuffix(m.warn, "Ctrl+Z undoes what it did") {
		t.Errorf("warn %q", m.warn)
	}
	if m.macros.run != nil || m.indicator() != "READY" {
		t.Errorf("still running: %q", m.indicator())
	}
}

func TestEscStopsARun(t *testing.T) {
	m := newModel()
	m.book().SaveMacro("", sheet.Macro{Name: "Spin", Source: "while True:\n    move(0, 0)\n"}, "add")
	m.trustHere()
	cmd := m.runMacro(mustMacro(t, m, "Spin"))
	if m.indicator() != "CMD" || !strings.Contains(line(m, contextLine), "Running Spin") {
		t.Errorf("indicator %q, context %q", m.indicator(), line(m, contextLine))
	}
	msg := cmd()
	press(t, m, "x") // held back while running
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	for range 1000 {
		if _, done := msg.(macroDoneMsg); done {
			break
		}
		_, next := m.Update(msg)
		if next == nil {
			break
		}
		msg = next()
		if b, ok := msg.(tea.BatchMsg); ok {
			msg = b[len(b)-1]()
		}
	}
	if m.macros.run != nil {
		m.Update(msg)
	}
	if m.macros.run != nil || m.warn != "Stopped Spin with Esc" || m.sheet.Len() != 0 {
		t.Errorf("run %v warn %q cells %d", m.macros.run, m.warn, m.sheet.Len())
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
	if !strings.Contains(line(m, contextLine), "made on another computer") {
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
	if cmd := m.overlay.key(m, tea.KeyPressMsg{Code: tea.KeyF4}); cmd != nil || !strings.Contains(line(m, m.height-1), "No editor in this session") {
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
