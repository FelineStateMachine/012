package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
	"github.com/FelineStateMachine/012/internal/ui/rules"
)

func mustValidate(t *testing.T, m *Model, line string) {
	t.Helper()
	v, err := sheet.ParseValidation(line)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.sheet.AddValidation(v); err != nil {
		t.Fatal(err)
	}
}

func TestConditionalFormattingPanel(t *testing.T) {
	m := newModel()
	press(t, m, "5", "<enter>", "150", "<enter>", "<up>", "<up>", "<shift+down>")
	run(m, m.runCommand("format.conditional"))
	if _, ok := m.overlay.(*rules.Editor); !ok || !strings.Contains(screen(m), "Conditional format rules") {
		t.Fatalf("overlay %T:\n%s", m.overlay, screen(m))
	}
	if !strings.Contains(line(m, menuLine), "RULES") {
		t.Errorf("indicator: %q", line(m, menuLine))
	}
	press(t, m, "<enter>", "<down>", "<down>")
	for range 9 {
		press(t, m, "<right>") // Greater than
	}
	press(t, m, "<down>", "100", "<enter>")
	fs := m.sheet.CondFormats()
	if len(fs) != 1 || fs[0].Summary() != "Greater than 100" || sheet.RangesText(fs[0].Ranges) != "A1:A2" || !m.changed {
		t.Fatalf("rules %+v", fs)
	}
	press(t, m, "<esc>")
	if m.overlay != nil || m.mode != modeReady {
		t.Fatalf("still open: %T", m.overlay)
	}
	// A2 is drawn on the green fill, A1 isn't.
	sp := rowtext.Span{Text: "150", Owner: 0}
	base, colored, _ := m.ruleSpan(addr("A2"), &sp, m.th.Cell, false, m.slotColor)
	if !colored || base.GetBackground() != m.th.RuleFill[sheet.ColorGreen].GetBackground() {
		t.Errorf("A2 base %v colored %v", base.GetBackground(), colored)
	}
	if _, colored, _ := m.ruleSpan(addr("A1"), &sp, m.th.Cell, false, m.slotColor); colored {
		t.Error("A1 formatted")
	}
	press(t, m, "<ctrl+z>")
	if len(m.sheet.CondFormats()) != 0 {
		t.Error("undo kept the rule")
	}
}

func TestCheckboxes(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "<shift+down>", "<shift+down>")
	run(m, m.runCommand("insert.checkbox"))
	if vs := m.sheet.Validations(); len(vs) != 1 || vs[0].Kind != sheet.ValidCheckbox {
		t.Fatalf("validations %+v", vs)
	}
	press(t, m, "<esc>", "<space>")
	if input(m, "B1") != "TRUE" || m.mode != modeReady {
		t.Fatalf("B1 %q mode %v", input(m, "B1"), m.mode)
	}
	out := screen(m)
	if strings.Count(out, "[✓]") != 1 || strings.Count(out, "[ ]") != 2 {
		t.Errorf("glyphs:\n%s", out)
	}
	if !strings.Contains(line(m, contextLine), "Space") {
		t.Errorf("context line %q", line(m, contextLine))
	}
	// A selection toggles together, as the active cell goes.
	press(t, m, "<shift+down>", "<shift+down>", "<space>")
	if input(m, "B1") != "FALSE" || input(m, "B3") != "" { // blank is unchecked already
		t.Errorf("B1 %q B3 %q", input(m, "B1"), input(m, "B3"))
	}
	press(t, m, "<space>")
	if input(m, "B1") != "TRUE" || input(m, "B3") != "TRUE" || m.sheet.Book().UndoLabel() != "check B1:B3" {
		t.Errorf("check all: B1 %q B3 %q %q", input(m, "B1"), input(m, "B3"), m.sheet.Book().UndoLabel())
	}
	press(t, m, "<space>")
	if input(m, "B2") != "FALSE" {
		t.Errorf("uncheck all: B2 %q", input(m, "B2"))
	}
	press(t, m, "<ctrl+z>", "<ctrl+z>")
	// A click on the glyph toggles; Space elsewhere starts an entry.
	press(t, m, "<esc>")
	click(m, minRowHdrW+sheet.DefaultWidth+4, gridTop+1, 0)
	if m.cur != addr("B2") || input(m, "B2") != "TRUE" {
		t.Errorf("click: cur %v B2 %q", m.cur, input(m, "B2"))
	}
	press(t, m, "<left>", " ")
	if m.mode != modeEnter {
		t.Errorf("space on a plain cell: mode %v", m.mode)
	}
}

func TestDropdown(t *testing.T) {
	m := newModel()
	mustValidate(t, m, `{"ranges":"A1:A5","criteria":"list","items":["Yes","No","Maybe"]}`)
	if !strings.Contains(line(m, gridTop), "▾") {
		t.Errorf("no dropdown mark: %q", line(m, gridTop))
	}
	press(t, m, "<alt+down>")
	p, ok := m.overlay.(*picker.Picker)
	if !ok || len(p.Items) != 3 || p.At == nil {
		t.Fatalf("overlay %T", m.overlay)
	}
	press(t, m, "no", "<enter>")
	if input(m, "A1") != "No" || m.overlay != nil {
		t.Fatalf("A1 %q overlay %T", input(m, "A1"), m.overlay)
	}
	// Clicking the mark of another cell opens its list there.
	click(m, minRowHdrW+sheet.DefaultWidth-1, gridTop+2, 0)
	if _, ok := m.overlay.(*picker.Picker); !ok || m.cur != addr("A3") {
		t.Fatalf("click: cur %v overlay %T", m.cur, m.overlay)
	}
	press(t, m, "<esc>")
	if input(m, "A3") != "" {
		t.Error("Esc entered something")
	}
}

func TestValidationRejectsAndWarns(t *testing.T) {
	m := newModel()
	mustValidate(t, m, `{"ranges":"A1:A9","criteria":"number","condition":"between","values":["1","10"],"reject":true}`)
	mustValidate(t, m, `{"ranges":"B1:B9","criteria":"number","condition":"between","values":["1","10"]}`)
	press(t, m, "42", "<enter>")
	if m.mode != modeEdit || input(m, "A1") != "" || !strings.Contains(line(m, contextLine), "Invalid entry in A1: Input must be a number between 1 and 10") {
		t.Fatalf("reject: mode %v A1 %q %q", m.mode, input(m, "A1"), line(m, contextLine))
	}
	press(t, m, "<esc>", "<right>", "42", "<enter>")
	if input(m, "B1") != "42" || !strings.Contains(line(m, contextLine), "Invalid entry in B1") {
		t.Fatalf("warn: B1 %q %q", input(m, "B1"), line(m, contextLine))
	}
	press(t, m, "<up>")
	if !strings.Contains(line(m, contextLine), "Invalid: Input must be a number between 1 and 10") || !m.sheet.Look(addr("B1")).Invalid {
		t.Errorf("on the invalid cell: %q", line(m, contextLine))
	}
	// Scripts are refused too.
	script(t, m, `select("A2")
enter("0")`)
	if input(m, "A2") != "" || !strings.Contains(m.warn, "Input must be a number between 1 and 10") {
		t.Errorf("script: A2 %q warn %q", input(m, "A2"), m.warn)
	}
}

func TestRuleMacros(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+down>")
	src := recordRules(t, m)
	for _, want := range []string{`run("format.conditional_add", answer="{\"ranges\":\"A1:A2\",\"condition\":\"not_empty\",\"fill\":\"green\"}")`,
		`run("insert.checkbox")`, `run("data.validation_add", answer="{\"ranges\":\"C1:C2\",\"criteria\":\"list\",\"items\":[\"a\",\"b\"]}")`} {
		if !strings.Contains(src, want) {
			t.Errorf("script lacks %s:\n%s", want, src)
		}
	}
	fresh := newModel()
	script(t, fresh, src)
	if len(fresh.sheet.CondFormats()) != 1 || len(fresh.sheet.Validations()) != 2 {
		t.Errorf("replay: %+v %+v %q", fresh.sheet.CondFormats(), fresh.sheet.Validations(), fresh.warn)
	}
}

// recordRules records adding a conditional format from the panel, a
// checkbox and a dropdown from the Add command.
func recordRules(t *testing.T, m *Model) string {
	t.Helper()
	send(m, nil)
	run(m, m.runCommand("macro.record"))
	run(m, m.runCommand("format.conditional"))
	press(t, m, "<enter>", "<enter>", "<esc>", "<right>")
	run(m, m.runCommand("insert.checkbox"))
	press(t, m, "<right>")
	run(m, m.runCommand("data.validation_add"))
	m.line.Set(`{"ranges":"C1:C2","criteria":"list","items":["a","b"]}`)
	press(t, m, "<enter>")
	run(m, m.runCommand("macro.stop"))
	m.line.Set("Rules")
	press(t, m, "<enter>", "<enter>")
	mc, ok := m.book().Macro("Rules")
	if !ok {
		t.Fatalf("no macro: %q", m.errMsg)
	}
	return mc.Source
}

func TestRulesRespectPivots(t *testing.T) {
	m := wideModel()
	salesTable(t, m)
	m.runCommand("data.pivot")
	press(t, m, "<space>", "reg", "<enter>", "<enter>")
	press(t, m, "<down>")
	run(m, m.runCommand("insert.checkbox"))
	if len(m.sheet.Validations()) != 0 || !strings.Contains(line(m, contextLine), "Pivot table results can't be edited") {
		t.Errorf("checkbox on a pivot: %+v %q", m.sheet.Validations(), line(m, contextLine))
	}
	if err := m.host().SaveValidation(-1, sheet.Validation{Ranges: []sheet.Rect{rectOf("A1:B3")}, Kind: sheet.ValidCheckbox}); err == nil {
		t.Error("validation saved over a pivot")
	}
	// Conditional formats only draw, so they may color a pivot.
	if err := m.host().SaveFormat(-1, sheet.CondFormat{Ranges: []sheet.Rect{rectOf("B2:B3")}, Op: sheet.RuleGreater, Args: [2]string{"5"}, Style: sheet.RuleStyle{Bold: true}}); err != nil {
		t.Error(err)
	}
}
