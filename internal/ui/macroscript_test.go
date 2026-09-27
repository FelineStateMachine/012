package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Scripts run against the model: the API, errors, limits and Esc.

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

func TestScriptsAnswerQuestions(t *testing.T) {
	m := newModel()
	script(t, m, `
add_sheet("Two")
set("A1", 1)
run("sheet.delete", answer="enter")
select("A1:A3")
run("data.define_name", answer="Sales")
run("sheet.rename", answer="Budget")
`)
	if m.warn != "" {
		t.Fatal(m.warn)
	}
	if m.book().Len() != 1 || m.sheet.Name() != "Budget" {
		t.Errorf("sheets %d, %q", m.book().Len(), m.sheet.Name())
	}
	if n, ok := m.sheet.LookupName("Sales"); !ok || n.Range.String() != "A1:A3" {
		t.Errorf("name %+v %v", n, ok)
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

