package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Dialogs recorded as the commands that open them, answered with their
// choices, and replayed from those answers.

// salesModel is a table of sales with a header row, the fixture both a
// recording and its replay start from.
func salesModel() *Model {
	m := newModel()
	for i, row := range [][]string{
		{"Region", "Units", "Price"},
		{"North", "12", "3.5"},
		{"South", "40", "2"},
		{"East", "7", "9"},
		{"West", "25", "4"},
	} {
		for j, v := range row {
			m.sheet.Set(addr(string(rune('A'+j))+string(rune('1'+i))), v)
		}
	}
	return m
}

// recordDo records what do does as a macro named name, from wherever m
// is, and returns its script without the header.
func recordDo(t *testing.T, m *Model, name string, do func()) string {
	t.Helper()
	send(m, nil)
	run(m, m.runCommand("macro.record"))
	do()
	send(m, nil)
	run(m, m.runCommand("macro.stop"))
	m.line.Set(name)
	press(t, m, "<enter>")
	m.line.Set("")
	press(t, m, "<enter>")
	mc, ok := m.book().Macro(name)
	if !ok {
		t.Fatalf("no macro %s: %q", name, m.errMsg)
	}
	return body(mc.Source)
}

// replays runs macro name from the recording model on a fresh fixture
// made by fresh, and checks it leaves the same workbook.
func replays(t *testing.T, recorded *Model, name string, fresh func() *Model) *Model {
	t.Helper()
	r := fresh()
	mc, _ := recorded.book().Macro(name)
	if err := r.book().SaveMacro("", mc, "add"); err != nil {
		t.Fatal(err)
	}
	r.trustHere()
	mc, _ = r.book().Macro(name)
	run(r, r.runMacro(mc))
	if r.note != "Ran "+name {
		t.Fatalf("running %s: note %q warn %q err %q", name, r.note, r.warn, r.errMsg)
	}
	if got, want := fileOf(t, r), fileOf(t, recorded); got != want {
		t.Errorf("%s replayed differently:\n%s\nrecorded:\n%s", name, got, want)
	}
	return r
}

func TestRecordSortBar(t *testing.T) {
	m := salesModel()
	got := recordDo(t, m, "Sort", func() {
		run(m, m.runCommand("data.sort_range"))
		press(t, m, "<right>", "<space>", "<alt+a>", "<enter>")
	})
	want := `run("data.sort_range", answer={"by": [{"column": "B", "order": "desc"}, {"column": "A"}], "header": True})`
	if got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	if input(m, "A2") != "South" {
		t.Fatalf("not sorted: A2 %q", input(m, "A2"))
	}
	replays(t, m, "Sort", salesModel)
}

func TestScriptAnswersSortBar(t *testing.T) {
	m := salesModel()
	script(t, m, `run("data.sort_range", answer={"by": [{"column": "C"}], "header": False})`)
	if input(m, "A1") != "South" || input(m, "A5") != "Region" {
		t.Fatalf("sorted with the header: A1 %q A5 %q warn %q", input(m, "A1"), input(m, "A5"), m.warn)
	}
	if m.overlay != nil || m.mode != modeReady {
		t.Fatalf("left mode %v, overlay %T", m.mode, m.overlay)
	}
	m = salesModel()
	script(t, m, `run("data.sort_range", answer={"by": [{"column": "Z"}]})`)
	if !strings.Contains(m.warn, `Sort range: column "Z" isn't in A1:C5`) || m.overlay != nil {
		t.Fatalf("warn %q overlay %T", m.warn, m.overlay)
	}
}

func TestRecordFilterPicker(t *testing.T) {
	m := salesModel()
	got := recordDo(t, m, "Filter", func() {
		run(m, m.runCommand("data.filter"))
		press(t, m, "<alt+down>", "<down>", "<down>", "<space>", "<enter>")
		// The button of another column, with a condition.
		press(t, m, "<right>")
		click(m, m.filterButtonX(2), headerLine, 0)
		press(t, m, "<tab>")
		for range int(sheet.CondGreater) {
			press(t, m, "<down>")
		}
		press(t, m, "3", "<enter>")
	})
	want := `run("data.filter")
run("data.filter_column", answer={"column": "A", "hidden": ["North"]})
select("B1")
run("data.filter_column", answer={"column": "C", "condition": "gt", "value": "3"})`
	if got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	if m.sheet.HiddenRows() != 2 {
		t.Fatalf("hidden %d", m.sheet.HiddenRows())
	}
	replays(t, m, "Filter", salesModel)
}

func TestScriptAnswersFilterPicker(t *testing.T) {
	m := salesModel()
	script(t, m, `run("data.filter_column", answer={"column": "B", "hidden": ["40"]})`)
	if !strings.Contains(m.warn, "the sheet has no filter") {
		t.Fatalf("warn %q", m.warn)
	}
	script(t, m, `run("data.filter")
run("data.filter_column", answer={"column": "D", "hidden": ["40"]})`)
	if !strings.Contains(m.warn, "column D isn't in the filter's range A1:C5") || m.overlay != nil {
		t.Fatalf("warn %q overlay %T", m.warn, m.overlay)
	}
	script(t, m, `select("B2")
run("data.filter_column", answer={"hidden": ["40", "7"]})`)
	if m.sheet.HiddenRows() != 2 || m.note != "Ran S" {
		t.Fatalf("hidden %d note %q warn %q", m.sheet.HiddenRows(), m.note, m.warn)
	}
}
