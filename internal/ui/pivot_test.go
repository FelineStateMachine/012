package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// salesTable types a table of sales and leaves A1 active.
func salesTable(t *testing.T, m *Model) {
	t.Helper()
	press(t, m, "Region", "<tab>", "Product", "<tab>", "Units", "<enter>")
	press(t, m, "East", "<tab>", "Pens", "<tab>", "10", "<enter>")
	press(t, m, "West", "<tab>", "Ink", "<tab>", "4", "<enter>")
	press(t, m, "East", "<tab>", "Ink", "<tab>", "6", "<enter>")
	press(t, m, "<ctrl+home>")
}

// shows is what the cell at a shows on the sheet shown.
func shows(m *Model, a string) string { return m.sheet.ShownText(addr(a)) }

func TestPivotCreateAndEdit(t *testing.T) {
	m := wideModel()
	salesTable(t, m)
	m.runCommand("data.pivot")
	if m.sheet.Name() != "Pivot Table 1" || m.indicator() != "PIVOT" {
		t.Fatalf("on %q in %s", m.sheet.Name(), m.indicator())
	}
	if s := screen(m); !strings.Contains(s, "Pivot table") || !strings.Contains(s, "Data  Sheet1!A1:C4") {
		t.Fatalf("editor not shown:\n%s", s)
	}
	// Space on Rows picks a field to add.
	press(t, m, "<space>")
	if !strings.Contains(screen(m), "Add to Rows") {
		t.Fatalf("no field picker:\n%s", screen(m))
	}
	press(t, m, "reg", "<enter>")
	if m.indicator() != "PIVOT" || shows(m, "A1") != "Region" || shows(m, "A2") != "East" {
		t.Fatalf("after adding Region: %s, A1 %q A2 %q", m.indicator(), shows(m, "A1"), shows(m, "A2"))
	}
	// Down to Values, add Units: summed, as it holds numbers.
	press(t, m, "<down>", "<down>", "<space>", "units", "<enter>")
	if shows(m, "B1") != "SUM of Units" || shows(m, "B2") != "16" || shows(m, "B4") != "20" {
		t.Fatalf("values: %q %q %q", shows(m, "B1"), shows(m, "B2"), shows(m, "B4"))
	}
	if !strings.Contains(screen(m), "‹ SUM ›") || !strings.Contains(line(m, m.height-1), "summarize by") {
		t.Errorf("value line or hints missing:\n%s", screen(m))
	}
	press(t, m, "<left>")
	if shows(m, "B1") != "MIN of Units" || shows(m, "B2") != "6" {
		t.Errorf("after Left: %q %q", shows(m, "B1"), shows(m, "B2"))
	}
	press(t, m, "<right>", "s", "s", "s")
	if shows(m, "B2") != "80.00%" {
		t.Errorf("show as %% of grand total: %q", shows(m, "B2"))
	}
	// Enter keeps it all; each change is a step of its own.
	press(t, m, "<enter>")
	if m.mode != modeReady {
		t.Fatalf("mode %v after Enter", m.mode)
	}
	press(t, m, "<ctrl+z>", "<ctrl+z>", "<ctrl+z>")
	if shows(m, "B2") != "16" || !strings.Contains(line(m, contextLine), "Undid: show as") {
		t.Errorf("undo: %q, %q", shows(m, "B2"), line(m, contextLine))
	}
	// Data > Edit pivot table opens it again.
	m.runCommand("data.pivot_edit")
	if m.indicator() != "PIVOT" {
		t.Errorf("edit reopened %s", m.indicator())
	}
}

func TestPivotEditorEscCancelsNew(t *testing.T) {
	m := wideModel()
	salesTable(t, m)
	m.runCommand("data.pivot")
	press(t, m, "<space>", "prod", "<enter>")
	press(t, m, "<esc>")
	if m.book().Len() != 1 || m.sheet.Name() != "Sheet1" || m.mode != modeReady {
		t.Errorf("after Esc: %d sheets, on %s, mode %v", m.book().Len(), m.sheet.Name(), m.mode)
	}
	// Esc in the field picker goes back to the editor.
	m.runCommand("data.pivot")
	press(t, m, "<space>", "<esc>")
	if m.indicator() != "PIVOT" {
		t.Errorf("Esc in the picker left %s", m.indicator())
	}
	// Nothing to summarize far from the data.
	press(t, m, "<esc>", "<f5>", "H20", "<enter>")
	m.runCommand("data.pivot")
	if m.book().Len() != 1 || !strings.Contains(line(m, contextLine), "Select the data") {
		t.Errorf("pivoted nothing: %q", line(m, contextLine))
	}
}

func TestPivotRefusesEditing(t *testing.T) {
	m := wideModel()
	salesTable(t, m)
	m.runCommand("data.pivot")
	press(t, m, "<space>", "reg", "<enter>", "<enter>")
	press(t, m, "<down>")
	for _, keys := range [][]string{{"x"}, {"<delete>"}, {"<enter>"}, {"<ctrl+alt+=>"}, {"<ctrl+b>"}, {"<ctrl+d>"}} {
		press(t, m, keys...)
		if m.mode != modeReady || !strings.Contains(line(m, contextLine), "Pivot table results can't be edited") {
			t.Errorf("%v: mode %v, %q", keys, m.mode, line(m, contextLine))
		}
		press(t, m, "<esc>")
	}
	if shows(m, "A2") != "East" || m.sheet.Cell(addr("A2")).Style.Bold {
		t.Errorf("A2 changed: %q", shows(m, "A2"))
	}
	// Copying works, and pastes values; beside the results is editable.
	press(t, m, "<ctrl+c>", "<right>", "<right>", "<ctrl+v>")
	if input(m, "C2") != "East" {
		t.Errorf("pasted %q", input(m, "C2"))
	}
	press(t, m, "note", "<enter>")
	if input(m, "C2") != "note" {
		t.Errorf("typed beside: %q", input(m, "C2"))
	}
}

func TestFrequencyTable(t *testing.T) {
	m := wideModel()
	salesTable(t, m)
	press(t, m, "<right>", "<down>", "<alt+F>")
	if m.sheet.Name() != "Frequency of Product" {
		t.Fatalf("on %q, %q", m.sheet.Name(), line(m, contextLine))
	}
	want := [][]string{{"Product", "Count", "Percent"}, {"Ink", "2", "66.67%"}, {"Pens", "1", "33.33%"}, {"Grand Total", "3", "100.00%"}}
	for r, row := range want {
		for c, w := range row {
			if got := m.sheet.ShownText(sheet.Addr{Col: c, Row: r}); got != w {
				t.Errorf("%s = %q, want %q", sheet.Addr{Col: c, Row: r}, got, w)
			}
		}
	}
	if !strings.Contains(line(m, contextLine), "Counted Product: 2 distinct values") {
		t.Errorf("note %q", line(m, contextLine))
	}
	// Live: a change to the data recounts.
	src := m.book().Sheet(0)
	src.Set(addr("B4"), "Pens")
	if got := shows(m, "A2"); got != "Pens" {
		t.Errorf("after an edit, A2 = %q", got)
	}
}

func TestPivotFilterAndSource(t *testing.T) {
	m := wideModel()
	salesTable(t, m)
	m.runCommand("data.pivot")
	press(t, m, "<space>", "prod", "<enter>")
	press(t, m, "<down>", "<down>", "<space>", "units", "<enter>")
	// Filters: add Region, then uncheck West in its values.
	press(t, m, "<down>", "<space>", "reg", "<enter>", "<space>")
	if _, ok := m.overlay.(*filterPicker); !ok || !strings.Contains(screen(m), "Filter Region") {
		t.Fatalf("no values picker:\n%s", screen(m))
	}
	press(t, m, "<down>", "<down>", "<space>", "<enter>")
	if m.indicator() != "PIVOT" || shows(m, "B2") != "6" || shows(m, "A4") != "Grand Total" {
		t.Errorf("filtered: %s, B2 %q, A4 %q", m.indicator(), shows(m, "B2"), shows(m, "A4"))
	}
	if !strings.Contains(screen(m), "1 hidden") {
		t.Errorf("filter line:\n%s", screen(m))
	}
	// The data range is pointed at on its sheet.
	press(t, m, "<home>", "<space>")
	if m.mode != modePrompt || m.sheet.Name() != "Sheet1" {
		t.Fatalf("mode %v on %s", m.mode, m.sheet.Name())
	}
	press(t, m, "<shift+up>", "<enter>")
	p, _ := m.sheet.Pivot()
	if m.sheet.Name() != "Pivot Table 1" || m.indicator() != "PIVOT" || p.Range.String() != "A1:C3" {
		t.Errorf("after pointing: %s %s %s", m.sheet.Name(), m.indicator(), p.Range)
	}
}

func TestPivotRenameValueAndColumnSubtotals(t *testing.T) {
	m := wideModel()
	salesTable(t, m)
	m.runCommand("data.pivot")
	// Columns: Region, then Product; Values: Units.
	press(t, m, "<down>", "<space>", "reg", "<enter>")
	press(t, m, "a", "prod", "<enter>")
	press(t, m, "<down>", "<space>", "units", "<enter>")
	if shows(m, "D1") != "East Total" || shows(m, "F1") != "West Total" || shows(m, "G1") != "Grand Total" {
		t.Fatalf("headers %q %q %q\n%s", shows(m, "D1"), shows(m, "F1"), shows(m, "G1"), screen(m))
	}
	// R renames the value, on the context line.
	press(t, m, "r")
	if m.mode != modePrompt || !strings.Contains(line(m, contextLine), "SUM of Units") {
		t.Fatalf("mode %v, %q", m.mode, line(m, contextLine))
	}
	press(t, m, "Units sold", "<enter>")
	if m.indicator() != "PIVOT" || shows(m, "B3") != "Units sold" || !strings.Contains(screen(m), "Units sold") {
		t.Fatalf("renamed: %s, B3 %q\n%s", m.indicator(), shows(m, "B3"), screen(m))
	}
	// A new summary keeps the name; Esc on the prompt changes nothing.
	press(t, m, "<left>", "r", "<esc>")
	if m.indicator() != "PIVOT" || shows(m, "B3") != "Units sold" {
		t.Errorf("after Left and Esc: %s, %q", m.indicator(), shows(m, "B3"))
	}
	// An empty name goes back to Sheets' own.
	press(t, m, "r", " ")
	press(t, m, "<enter>")
	if shows(m, "B3") != "MIN of Units" {
		t.Errorf("cleared name: %q", shows(m, "B3"))
	}
	press(t, m, "<enter>", "<ctrl+z>")
	if shows(m, "B3") != "Units sold" {
		t.Errorf("undo: %q", shows(m, "B3"))
	}
}

func TestPivotMenus(t *testing.T) {
	m := wideModel()
	press(t, m, "<alt+d>")
	s := screen(m)
	for _, want := range []string{"Pivot table", "Edit pivot table", "Frequency table (column stats)"} {
		if !strings.Contains(s, want) {
			t.Errorf("Data menu lacks %q:\n%s", want, s)
		}
	}
}
