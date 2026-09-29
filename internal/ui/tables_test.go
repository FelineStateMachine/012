package ui

import (
	"strings"
	"testing"
)

// unitsModel is a model whose sheet holds Region, Units and Amount in
// A1:C4, the cursor on B2.
func unitsModel(t *testing.T) *Model {
	t.Helper()
	m := tallModel()
	m.Update(teaSize(100, 30))
	for a, v := range map[string]string{
		"A1": "Region", "B1": "Units", "C1": "Amount",
		"A2": "North", "B2": "2", "C2": "10",
		"A3": "South", "B3": "3", "C3": "20",
		"A4": "East", "B4": "5", "C4": "30",
	} {
		m.sheet.Set(addr(a), v)
	}
	m.selectRect(rectOf("B2"))
	return m
}

func TestConvertToTable(t *testing.T) {
	m := unitsModel(t)
	press(t, m, "<ctrl+alt+t>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Name for the table of A1:C4: Table1") {
		t.Fatalf("prompt %q", l)
	}
	press(t, m, "Sales", "<enter>")
	tb, ok := m.sheet.TableAt(addr("B2"))
	if !ok || tb.Name != "Sales" || tb.Range != rectOf("A1:C4") {
		t.Fatalf("table %+v", tb)
	}
	if l := line(m, contextLine); !strings.Contains(l, "=SUM(Sales[Amount])") {
		t.Errorf("note %q", l)
	}
	if l := line(m, formulaLine); !strings.HasPrefix(l, " Sales") {
		t.Errorf("name box %q", l)
	}
	if m.available("table.create") {
		t.Error("Convert to table offered inside a table")
	}
	// On a table's cell the context line names the table and column.
	press(t, m, "<esc>", "<right>")
	if l := line(m, contextLine); !strings.Contains(l, "Table Sales, column Units: Sales[Units] in formulas") {
		t.Errorf("context %q", l)
	}
	// Typing below it grows it; undo takes the row back out.
	press(t, m, "<ctrl+down>", "<down>", "40", "<enter>")
	if tb, _ := m.sheet.TableAt(addr("C5")); tb.Range != rectOf("A1:C5") {
		t.Errorf("grew to %v", tb.Range)
	}
	press(t, m, "<ctrl+z>")
	if _, ok := m.sheet.TableAt(addr("C5")); ok {
		t.Error("undo kept the row")
	}
}

func TestTableCommands(t *testing.T) {
	m := unitsModel(t)
	m.sheet.CreateTable("Sales", rectOf("A1:C4"))
	m.sheet.Set(addr("E1"), "=SUM(Sales[Amount])")
	m.runCommand("table.rename")
	press(t, m, "Revenue", "<enter>")
	if input(m, "E1") != "=SUM(Revenue[Amount])" {
		t.Errorf("renamed formula %q", input(m, "E1"))
	}
	m.runCommand("table.resize")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Range for Revenue, header row first: A1:C4") {
		t.Fatalf("resize prompt %q", l)
	}
	press(t, m, "<shift+left>", "<enter>")
	if tb, _ := m.sheet.TableAt(addr("A1")); tb.Range != rectOf("A1:B4") {
		t.Errorf("resized to %v", tb.Range)
	}
	wantShown(t, m, "E1", "#REF!")
	press(t, m, "<ctrl+z>")
	for id, want := range map[string]bool{"table.banded": false, "table.header": true} {
		if c := commands[id]; !c.checked(m) != !want {
			t.Errorf("%s checked %v", id, c.checked(m))
		}
		m.runCommand(id)
		if commands[id].checked(m) == want {
			t.Errorf("%s didn't toggle", id)
		}
	}
	m.runCommand("table.remove")
	if m.sheet.HasTables() || input(m, "E1") != "=SUM($C$2:$C$4)" {
		t.Errorf("removed: %v %q", m.sheet.HasTables(), input(m, "E1"))
	}
	if m.available("table.rename") {
		t.Error("table commands offered outside a table")
	}
}

// Filters and sorts started in a table take its range, header kept.
func TestTableScopesFilterAndSort(t *testing.T) {
	m := unitsModel(t)
	m.sheet.Set(addr("A6"), "apart")
	m.sheet.CreateTable("Sales", rectOf("A1:C5"))
	m.selectRect(rectOf("C3"))
	m.runCommand("data.sort_range_za")
	if input(m, "C2") != "30" || input(m, "C1") != "Amount" {
		t.Errorf("sorted: C1 %q C2 %q", input(m, "C1"), input(m, "C2"))
	}
	m.runCommand("data.filter")
	if r, _ := m.sheet.FilterRange(); r != rectOf("A1:C5") {
		t.Errorf("filter on %v", r)
	}
}

func TestTablesPicker(t *testing.T) {
	m := unitsModel(t)
	m.sheet.CreateTable("Sales", rectOf("A1:C4"))
	m.sheet.Set(addr("F1"), "x")
	m.sheet.CreateTable("Other", rectOf("F1:G3"))
	m.runCommand("data.tables")
	scr := screen(m)
	for _, want := range []string{"Tables", "Sales", "A1:C4", "Other", "F1:G3"} {
		if !strings.Contains(scr, want) {
			t.Errorf("missing %q in\n%s", want, scr)
		}
	}
	press(t, m, "oth", "<enter>")
	if m.selection() != rectOf("F1:G3") {
		t.Errorf("went to %v", m.selection())
	}
	press(t, m, "<esc>", "<f5>", "sales", "<enter>")
	if m.selection() != rectOf("A1:C4") {
		t.Errorf("Go to a table: %v", m.selection())
	}
	m.runCommand("data.tables")
	press(t, m, "sales", "<ctrl+d>")
	if _, _, ok := m.book().Table("Sales"); ok {
		t.Error("not removed")
	}
	if st := line(m, m.height-1); !strings.Contains(st, "Removed the table Sales") {
		t.Errorf("status %q", st)
	}
	press(t, m, "<esc>")
}

func TestTableSuggestions(t *testing.T) {
	m := unitsModel(t)
	m.sheet.CreateTable("Sales", rectOf("A1:C4"))
	m.selectRect(rectOf("E1"))
	press(t, m, "=SUM(sal")
	list, _ := m.entry.assist.Shown(m.host())
	if len(list) == 0 || list[0].Name != "Sales" || !strings.Contains(list[0].Detail, "A1:C4") {
		t.Fatalf("suggestions %v", list)
	}
	press(t, m, "<tab>", "[am")
	if list, _ = m.entry.assist.Shown(m.host()); len(list) != 1 || list[0].Name != "Amount" {
		t.Fatalf("columns %v", list)
	}
	if ctx := line(m, contextLine); !strings.HasPrefix(ctx, "Sales[Region, Units, Amount]") {
		t.Errorf("context %q", ctx)
	}
	press(t, m, "<tab>")
	if m.line.Text() != "=SUM(Sales[Amount]" {
		t.Fatalf("accepted %q", m.line.Text())
	}
	press(t, m, ")", "<enter>")
	wantShown(t, m, "E1", "60")
	// Items after #, and a column after @.
	m.selectRect(rectOf("D2"))
	press(t, m, "=Sales[#h")
	if list, _ = m.entry.assist.Shown(m.host()); len(list) != 1 || list[0].Name != "#Headers" {
		t.Fatalf("items %v", list)
	}
	press(t, m, "<esc>", "<esc>", "=Sales[@u", "<tab>", "<enter>")
	if input(m, "D2") != "=Sales[@Units]" {
		t.Errorf("this row %q", input(m, "D2"))
	}
	wantShown(t, m, "D2", "2")
}

// Scripts answer the table commands' questions, as a recording writes
// them.
func TestTableCommandsInScripts(t *testing.T) {
	m := unitsModel(t)
	script(t, m, `
select("A1:C4")
run("table.create", answer="Sales")
select("B2")
run("table.rename", answer="Revenue")
run("table.resize", answer="A1:B4")
run("table.banded")
`)
	if m.warn != "" {
		t.Fatal(m.warn)
	}
	tb, ok := m.sheet.TableAt(addr("A2"))
	if !ok || tb.Name != "Revenue" || tb.Range != rectOf("A1:B4") || !tb.Banded {
		t.Errorf("table %+v", tb)
	}
}

// wantShown checks the value the cell at a shows.
func wantShown(t *testing.T, m *Model, a, want string) {
	t.Helper()
	if got := m.sheet.Value(addr(a)).String(); got != want {
		t.Errorf("%s = %q, want %q", a, got, want)
	}
}
