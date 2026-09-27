package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Dialogs recorded as the commands that open them, answered with their
// choices, and replayed from those answers.

// salesModel is a table of sales with a header row, the fixture both a
// recording and its replay start from.
func salesModel() *Model { return withSales(newModel()) }

// wideSales is salesModel on a screen wide enough for a chart beside the
// table.
func wideSales() *Model { return withSales(wideModel()) }

func withSales(m *Model) *Model {
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

func TestNamesPickerChangesAreNoted(t *testing.T) {
	m := salesModel()
	m.sheet.DefineName("Units", sheet.Rect{From: addr("B2"), To: addr("B5")})
	got := recordDo(t, m, "Names", func() {
		run(m, m.runCommand("data.named_ranges"))
		press(t, m, "<down>", "<ctrl+d>", "<esc>")
	})
	if !strings.HasPrefix(got, "# Not recorded: ") || !strings.Contains(got, "Units") {
		t.Fatalf("recorded:\n%s", got)
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

func TestRecordChartEditorAndDrags(t *testing.T) {
	m := wideSales()
	got := recordDo(t, m, "Chart", func() {
		run(m, m.runCommand("insert.chart"))
		press(t, m, "3", "t", "Units", "<enter>", "<enter>")
		// Moved a step at a time, then dragged by the corner: one call.
		press(t, m, "<down>", "<right>", "<right>")
		c := m.sheet.Charts()[0]
		x, y := m.chartScreen(c)
		send(m, tea.MouseClickMsg{X: x + c.W - 1, Y: y + c.H - 1, Button: tea.MouseLeft})
		send(m, tea.MouseMotionMsg{X: x + 29, Y: y + 11, Button: tea.MouseLeft})
		send(m, tea.MouseReleaseMsg{X: x + 29, Y: y + 11, Button: tea.MouseLeft})
		// Edited again: the header row off.
		press(t, m, "<enter>", "h", "<enter>", "<esc>")
	})
	want := `run("insert.chart", answer={"type": "line", "data": "A1:C5", "at": "D1", "width": 48, "height": 16, "header": True, "labels": True, "title": "Units"})
run("chart.edit", answer={"chart": 1, "at": "F2", "width": 30, "height": 12})
run("chart.edit", answer={"chart": 1, "header": False})`
	if got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	replays(t, m, "Chart", wideSales)
}

func TestScriptAnswersChartCommands(t *testing.T) {
	m := wideSales()
	script(t, m, `select("A1:B5")
run("insert.chart", answer={"type": "pie", "title": "Units"})
run("chart.edit", answer={"chart": 1, "at": "H2", "legend": "none"})`)
	cs := m.sheet.Charts()
	if len(cs) != 1 || cs[0].Type != sheet.ChartPie || cs[0].Title != "Units" || cs[0].At.String() != "H2" || cs[0].Legend != sheet.LegendNone || cs[0].Data.String() != "A1:B5" {
		t.Fatalf("charts %+v, warn %q", cs, m.warn)
	}
	if m.overlay != nil || m.mode != modeReady {
		t.Fatalf("left mode %v overlay %T", m.mode, m.overlay)
	}
	script(t, m, `run("chart.edit", answer={"chart": 2, "at": "A1"})`)
	if !strings.Contains(m.warn, "there's no chart 2 on Sheet1: it has 1") {
		t.Fatalf("warn %q", m.warn)
	}
	script(t, m, `run("chart.edit", answer={"type": "blimp"})`)
	if !strings.Contains(m.warn, `unknown chart type "blimp"`) || m.overlay != nil {
		t.Fatalf("warn %q overlay %T", m.warn, m.overlay)
	}
}

func TestRecordFindAndReplace(t *testing.T) {
	m := salesModel()
	got := recordDo(t, m, "Replace", func() {
		press(t, m, "<ctrl+h>", "th", "<tab>", "TH", "<enter>")
		press(t, m, "<alt+c>", "<ctrl+enter>", "<esc>")
		press(t, m, "X", "<enter>")
	})
	want := `select("A2")
run("edit.replace", answer={"find": "th", "replace": "TH", "cell": "A2"})
run("edit.replace", answer={"find": "th", "replace": "TH", "matchCase": True})
enter("X")
select("A4")`
	if got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	if input(m, "A2") != "NorTH" || input(m, "A3") != "X" {
		t.Fatalf("A2 %q A3 %q", input(m, "A2"), input(m, "A3"))
	}
	replays(t, m, "Replace", salesModel)
}

// salesRules is salesModel with two conditional formats on B2:B5 and a
// dropdown on A2:A5.
func salesRules() *Model {
	m := salesModel()
	for _, line := range []string{`{"ranges":"B2:B5","condition":"gt","values":["20"],"fill":"green"}`, `{"ranges":"B2:B5","condition":"lt","values":["10"],"fill":"red"}`} {
		f, _ := sheet.ParseCondFormat(line)
		m.sheet.AddCondFormat(f)
	}
	v, _ := sheet.ParseValidation(`{"ranges":"A2:A5","criteria":"list","items":["North","South","East","West"]}`)
	m.sheet.AddValidation(v)
	return m
}

func TestRecordRulesPanelEdits(t *testing.T) {
	m := salesRules()
	got := recordDo(t, m, "Rules", func() {
		run(m, m.runCommand("format.conditional"))
		press(t, m, "<down>", "<shift+down>") // green after red
		press(t, m, "<enter>", "<down>", "<down>", "<down>", "<backspace>", "<backspace>", "30", "<enter>", "<esc>")
		run(m, m.runCommand("data.validation"))
		press(t, m, "<down>", "<delete>", "<esc>")
	})
	want := `run("format.conditional_move", answer={"rule": 1, "to": 2})
run("format.conditional_set", answer={"rule": 2, "ranges": "B2:B5", "condition": "gt", "values": ["30"], "fill": "green"})
run("data.validation_remove", answer=1)`
	if got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	replays(t, m, "Rules", salesRules)
}

func TestScriptsChangeRules(t *testing.T) {
	m := salesRules()
	script(t, m, `run("format.conditional_set", answer={"rule": 1, "ranges": "C2:C5", "condition": "empty", "fill": "yellow"})
run("data.validation_set", answer={"rule": 1, "ranges": "A2:A9", "criteria": "list", "items": ["N", "S"]})
run("format.conditional_move", answer={"rule": 2, "to": 1})`)
	fs, vs := m.sheet.CondFormats(), m.sheet.Validations()
	if len(fs) != 2 || fs[1].Ranges[0].String() != "C2:C5" || vs[0].Ranges[0].String() != "A2:A9" || m.note != "Ran S" {
		t.Fatalf("formats %+v validations %+v warn %q", fs, vs, m.warn)
	}
	for src, want := range map[string]string{
		`run("format.conditional_remove", answer=3)`:                       "there's no rule 3: the sheet has 2",
		`run("format.conditional_move", answer={"rule": 1})`:               `give "to", a place from 1 to 2`,
		`run("data.validation_set", answer={"ranges": "A1"})`:              "give the rule's number",
		`run("format.conditional_set", answer={"rule": 1, "ranges": "?"})`: `invalid ranges "?"`,
	} {
		script(t, m, src)
		if !strings.Contains(m.warn, want) {
			t.Errorf("%s: warn %q, want %q", src, m.warn, want)
		}
	}
}

func TestRecordPivotEditor(t *testing.T) {
	m := wideSales()
	got := recordDo(t, m, "Pivot", func() {
		run(m, m.runCommand("data.pivot"))
		press(t, m, "<space>", "reg", "<enter>")
		press(t, m, "<down>", "<down>", "<space>", "units", "<enter>", "<enter>")
		// Edited again: the grand total row off. Unchanged, it records
		// nothing.
		run(m, m.runCommand("data.pivot_edit"))
		press(t, m, "<enter>")
		run(m, m.runCommand("data.pivot_edit"))
		press(t, m, "<end>", "<up>", "<space>", "<enter>")
	})
	want := `run("data.pivot", answer={"source": "Sheet1!A1:C5", "rows": [{"column": "A"}], "values": [{"column": "B", "summarize": "sum"}], "rowTotals": True, "columnTotals": True})
run("data.pivot_edit", answer={"source": "Sheet1!A1:C5", "rows": [{"column": "A"}], "values": [{"column": "B", "summarize": "sum"}], "rowTotals": False, "columnTotals": True})`
	if got != want {
		t.Fatalf("recorded:\n%s\nwant:\n%s", got, want)
	}
	replays(t, m, "Pivot", wideSales)
}

func TestScriptAnswersPivotCommands(t *testing.T) {
	m := wideSales()
	script(t, m, `run("data.pivot_edit", answer={"source": "Sheet1!A1:C5"})`)
	if !strings.Contains(m.warn, "Sheet1 has no pivot table") {
		t.Fatalf("warn %q", m.warn)
	}
	script(t, m, `run("data.pivot", answer={"source": "Sheet1!A1:C5", "rows": [{"column": "A"}], "values": [{"column": "C", "summarize": "max"}]})`)
	if m.sheet.Name() != "Pivot Table 1" || shows(m, "B1") != "MAX of Price" || m.overlay != nil {
		t.Fatalf("on %s, B1 %q, overlay %T, warn %q", m.sheet.Name(), shows(m, "B1"), m.overlay, m.warn)
	}
}
