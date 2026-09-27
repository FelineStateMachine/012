package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// status is the status line, where the tabs are.
func status(m *Model) string { return line(m, m.height-1) }

// tabX is the screen x of the middle of a sheet's tab, or of the + for
// name "+".
func tabX(t *testing.T, m *Model, name string) int {
	t.Helper()
	_, spans := m.statusLayout()
	for _, sp := range spans {
		if name == "+" && sp.kind == hitTabAdd || sp.kind == hitTab && m.tabSheet(sp.index).Name() == name {
			return sp.x + sp.w/2
		}
	}
	t.Fatalf("no tab %q in %q", name, status(m))
	return 0
}

// sheetNames lists the workbook's sheets in tab order.
func sheetNames(m *Model) string {
	var names []string
	for _, s := range m.book().Sheets() {
		names = append(names, s.Name())
	}
	return strings.Join(names, ",")
}

func TestTabKeys(t *testing.T) {
	m := newModel()
	if s := status(m); !strings.HasPrefix(s, " Sheet1   +  │ untitled") {
		t.Fatalf("status %q", s)
	}
	press(t, m, "<down>", "<down>", "<f11>") // plain F11 does nothing
	press(t, m, "<shift+f11>")
	if m.sheet.Name() != "Sheet2" || m.cur != addr("A1") || !strings.HasPrefix(status(m), " Sheet1   Sheet2   +") {
		t.Fatalf("new sheet: %s at %v, status %q", m.sheet.Name(), m.cur, status(m))
	}
	press(t, m, "x", "<enter>", "<right>")
	// Each sheet remembers where its cursor was.
	press(t, m, "<ctrl+pgup>")
	if m.sheet.Name() != "Sheet1" || m.cur != addr("A3") || input(m, "A1") != "" {
		t.Errorf("previous: %s at %v", m.sheet.Name(), m.cur)
	}
	press(t, m, "<ctrl+pgup>") // no wrapping
	if m.sheet.Name() != "Sheet1" {
		t.Errorf("wrapped to %s", m.sheet.Name())
	}
	press(t, m, "<alt+right>")
	if m.sheet.Name() != "Sheet2" || m.cur != addr("B2") || input(m, "A1") != "x" {
		t.Errorf("next: %s at %v", m.sheet.Name(), m.cur)
	}
	press(t, m, "<alt+left>", "<ctrl+pgdown>")
	if m.sheet.Name() != "Sheet2" {
		t.Errorf("ctrl+pgdown: %s", m.sheet.Name())
	}
	// Undo is workbook-wide and shows the sheet it changes.
	press(t, m, "<ctrl+pgup>", "<ctrl+z>")
	if m.sheet.Name() != "Sheet2" || input(m, "A1") != "" || !strings.Contains(line(m, contextLine), "Undid: edit A1") {
		t.Errorf("undo: %s, %q", m.sheet.Name(), line(m, contextLine))
	}
	press(t, m, "<ctrl+z>")
	if m.book().Len() != 1 || m.sheet.Name() != "Sheet1" || m.cur != addr("A3") {
		t.Errorf("undo the new sheet: %d sheets, on %s at %v", m.book().Len(), m.sheet.Name(), m.cur)
	}
	press(t, m, "<ctrl+y>")
	if m.book().Len() != 2 || m.sheet.Name() != "Sheet2" {
		t.Errorf("redo: %d sheets, on %s", m.book().Len(), m.sheet.Name())
	}
}

func TestTabMouse(t *testing.T) {
	m := newModel()
	leftClick(m, tabX(t, m, "+"), m.height-1)
	leftClick(m, tabX(t, m, "+"), m.height-1)
	if sheetNames(m) != "Sheet1,Sheet2,Sheet3" || m.sheet.Name() != "Sheet3" {
		t.Fatalf("+ twice: %s, on %s", sheetNames(m), m.sheet.Name())
	}
	leftClick(m, tabX(t, m, "Sheet1"), m.height-1)
	if m.sheet.Name() != "Sheet1" {
		t.Errorf("click: on %s", m.sheet.Name())
	}
	// Double-click renames, on the context line.
	x := tabX(t, m, "Sheet2")
	leftClick(m, x, m.height-1)
	leftClick(m, x, m.height-1)
	if m.mode != modePrompt || !strings.HasPrefix(line(m, contextLine), "Rename sheet: Sheet2") {
		t.Fatalf("double-click: mode %v, %q", m.mode, line(m, contextLine))
	}
	press(t, m, "Q3 plan", "<enter>")
	if sheetNames(m) != "Sheet1,Q3 plan,Sheet3" || !strings.Contains(line(m, contextLine), "Renamed Sheet2 to Q3 plan") {
		t.Errorf("renamed: %s, %q", sheetNames(m), line(m, contextLine))
	}
	// Dragging a tab onto another moves it there.
	x = tabX(t, m, "Sheet3")
	send(m, tea.MouseClickMsg{X: x, Y: m.height - 1, Button: tea.MouseLeft})
	to := tabX(t, m, "Sheet1")
	send(m, tea.MouseMotionMsg{X: to, Y: m.height - 1, Button: tea.MouseLeft})
	send(m, tea.MouseReleaseMsg{X: to, Y: m.height - 1, Button: tea.MouseLeft})
	if sheetNames(m) != "Sheet3,Sheet1,Q3 plan" || m.sheet.Name() != "Sheet3" {
		t.Errorf("drag: %s, on %s", sheetNames(m), m.sheet.Name())
	}
	// Right-click opens the tab's menu over the status line.
	send(m, tea.MouseClickMsg{X: tabX(t, m, "Q3 plan"), Y: m.height - 1, Button: tea.MouseRight})
	o := openMenu(t, m)
	if m.sheet.Name() != "Q3 plan" || o.levels[0].items[0].label() != "Rename" {
		t.Fatalf("right-click: on %s, menu %v", m.sheet.Name(), o.levels[0].items)
	}
	press(t, m, "<down>", "<enter>") // Duplicate
	if sheetNames(m) != "Sheet3,Sheet1,Q3 plan,Copy of Q3 plan" || m.sheet.Name() != "Copy of Q3 plan" {
		t.Errorf("duplicate: %s, on %s", sheetNames(m), m.sheet.Name())
	}
}

func TestRenameErrors(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+f11>")
	m.runCommand("sheet.rename")
	press(t, m, "sheet1", "<enter>")
	if m.mode != modeError || !strings.Contains(status(m), "There's already a sheet named Sheet1") {
		t.Errorf("duplicate name: mode %v, %q", m.mode, status(m))
	}
	press(t, m, "x")
	m.runCommand("sheet.rename")
	press(t, m, "<esc>")
	if m.mode != modeReady || m.sheet.Name() != "Sheet2" {
		t.Errorf("esc: mode %v, %s", m.mode, m.sheet.Name())
	}
}

func TestDeleteSheetAsks(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+f11>", "1", "<enter>", "2", "<enter>")
	if !commands["sheet.delete"].available(m) {
		t.Fatal("delete unavailable with two sheets")
	}
	m.runCommand("sheet.delete")
	if l := line(m, contextLine); !strings.Contains(l, "Delete Sheet2 and its 2 cells?") {
		t.Fatalf("question %q", l)
	}
	press(t, m, "<enter>")
	if sheetNames(m) != "Sheet1" || m.sheet.Name() != "Sheet1" || commands["sheet.delete"].available(m) {
		t.Errorf("deleted: %s, on %s", sheetNames(m), m.sheet.Name())
	}
	press(t, m, "<ctrl+z>")
	if sheetNames(m) != "Sheet1,Sheet2" || m.sheet.Name() != "Sheet2" || input(m, "A2") != "2" {
		t.Errorf("undo: %s, on %s", sheetNames(m), m.sheet.Name())
	}
	// An empty sheet goes without asking.
	press(t, m, "<shift+f11>")
	m.runCommand("sheet.delete")
	if m.mode != modeReady || sheetNames(m) != "Sheet1,Sheet2" {
		t.Errorf("empty sheet: mode %v, %s", m.mode, sheetNames(m))
	}
}

// Typing a formula and switching sheets points into the other sheet, as
// clicking a tab while editing does in Sheets.
func TestPointIntoAnotherSheet(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+f11>", "<down>", "5", "<enter>", "<ctrl+pgup>")
	press(t, m, "<right>", "=")
	press(t, m, "<ctrl+pgdown>")
	if m.mode != modePoint || m.sheet.Name() != "Sheet2" || bar(m) != "=Sheet2!A3" {
		t.Fatalf("pointing: mode %v on %s, bar %q", m.mode, m.sheet.Name(), bar(m))
	}
	if name := strings.TrimSpace(ansi.Cut(line(m, formulaLine), 0, nameBoxW)); name != "Sheet1!B1" {
		t.Errorf("name box %q", name)
	}
	press(t, m, "<up>", "*2")
	if m.mode != modeEnter || bar(m) != "=Sheet2!A2*2" || m.sheet.Name() != "Sheet2" {
		t.Fatalf("typing on: mode %v on %s, bar %q", m.mode, m.sheet.Name(), bar(m))
	}
	// A click on the other sheet points too.
	press(t, m, "+")
	click(m, cellX(0), gridTop+1, 0)
	if bar(m) != "=Sheet2!A2*2+Sheet2!A2" {
		t.Errorf("click: bar %q", bar(m))
	}
	// Back on the entry's sheet, references have no sheet.
	press(t, m, "<ctrl+pgup>")
	if m.sheet.Name() != "Sheet1" || bar(m) != "=Sheet2!A2*2+B1" {
		t.Errorf("back home: on %s, bar %q", m.sheet.Name(), bar(m))
	}
	press(t, m, "<left>", "<enter>")
	if m.sheet.Name() != "Sheet1" || m.cur != addr("B2") || input(m, "B1") != "=Sheet2!A2*2+A1" || m.sheet.Value(addr("B1")).Num != 10 {
		t.Errorf("stored: on %s at %v, B1 %q = %v", m.sheet.Name(), m.cur, input(m, "B1"), m.sheet.Value(addr("B1")))
	}
	// Enter while pointing on another sheet stores the formula and
	// returns to its cell's sheet.
	press(t, m, "=", "<alt+right>", "<enter>")
	if m.sheet.Name() != "Sheet1" || input(m, "B2") != "=Sheet2!A2" || m.cur != addr("B3") {
		t.Errorf("enter away: on %s, B2 %q, at %v", m.sheet.Name(), input(m, "B2"), m.cur)
	}
	// Esc while typing on another sheet cancels and returns.
	press(t, m, "=", "<alt+right>", "<esc>", "<esc>")
	if m.sheet.Name() != "Sheet1" || m.mode != modeReady || input(m, "B3") != "" {
		t.Errorf("esc away: on %s, mode %v", m.sheet.Name(), m.mode)
	}
	// Text isn't a formula: switching stores it first.
	press(t, m, "hello", "<alt+right>")
	if m.sheet.Name() != "Sheet2" || m.mode != modeReady || m.book().Sheet(0).Cell(addr("B3")).Input != "hello" {
		t.Errorf("text: on %s, mode %v", m.sheet.Name(), m.mode)
	}
}

func TestGotoOtherSheet(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+f11>")
	m.runCommand("sheet.rename")
	press(t, m, "Q3 plan", "<enter>", "<ctrl+pgup>")
	m.runCommand("goto")
	press(t, m, "'Q3 plan'!C4", "<enter>")
	if m.sheet.Name() != "Q3 plan" || m.cur != addr("C4") {
		t.Errorf("goto: %s %v", m.sheet.Name(), m.cur)
	}
	m.runCommand("goto")
	press(t, m, "Sheet1!B2:C3", "<enter>")
	if m.sheet.Name() != "Sheet1" || m.selection() != sheet.NewRect(addr("B2"), addr("C3")) {
		t.Errorf("goto range: %s %v", m.sheet.Name(), m.selection())
	}
	m.runCommand("goto")
	press(t, m, "Nope!A1", "<enter>")
	if m.mode != modeError || !strings.Contains(status(m), "There's no sheet named Nope") {
		t.Errorf("unknown sheet: %q", status(m))
	}
}

func TestSheetPicker(t *testing.T) {
	m := newModel()
	for range 3 {
		press(t, m, "<shift+f11>")
	}
	press(t, m, "<alt+shift+k>")
	p := openPicker(t, m)
	if len(p.shown) != 4 || p.Sel != 3 {
		t.Fatalf("picker: %d shown, %d selected", len(p.shown), p.Sel)
	}
	press(t, m, "sheet2", "<enter>")
	if m.overlay != nil || m.sheet.Name() != "Sheet2" {
		t.Errorf("picked %s", m.sheet.Name())
	}
}

func TestManyTabsScroll(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	for range 11 {
		press(t, m, "<shift+f11>")
	}
	s := status(m)
	if !strings.HasPrefix(s, "‹  Sheet") || !strings.Contains(s, " Sheet12 ") || strings.Contains(s, " Sheet1 ") {
		t.Errorf("last sheet shown: %q", s)
	}
	for range 11 {
		press(t, m, "<ctrl+pgup>")
	}
	s = status(m)
	if !strings.HasPrefix(s, " Sheet1 ") || !strings.Contains(s, "›") {
		t.Errorf("first sheet shown: %q", s)
	}
	// The arrows step through the sheets.
	leftClick(m, strings.Index(s, "›"), m.height-1)
	if m.sheet.Name() != "Sheet2" {
		t.Errorf("› went to %s", m.sheet.Name())
	}
}

func TestCopyAndCutBetweenSheets(t *testing.T) {
	m := newModel()
	press(t, m, "5", "<tab>", "=A1*2", "<enter>")
	press(t, m, "<up>", "<shift+right>", "<ctrl+c>", "<shift+f11>", "<ctrl+v>")
	if input(m, "B1") != "=A1*2" || m.sheet.Value(addr("B1")).Num != 10 {
		t.Errorf("paste: B1 %q = %v", input(m, "B1"), m.sheet.Value(addr("B1")))
	}
	press(t, m, "<ctrl+pgup>", "<ctrl+home>", "<ctrl+x>", "<ctrl+pgdown>", "<down>", "<down>")
	if !strings.Contains(line(m, contextLine), "Cut Sheet1!A1") {
		t.Errorf("cut on another sheet: %q", line(m, contextLine))
	}
	press(t, m, "<ctrl+v>")
	one := m.book().Sheet(0)
	if input(m, "A3") != "5" || one.Cell(addr("B1")).Input != "=Sheet2!A3*2" || one.Value(addr("B1")).Num != 10 {
		t.Errorf("move: A3 %q, Sheet1!B1 %q", input(m, "A3"), one.Cell(addr("B1")).Input)
	}
}

func TestFindAllSheets(t *testing.T) {
	m := newModel()
	press(t, m, "rent", "<enter>", "<shift+f11>", "<down>", "rental", "<enter>", "<ctrl+pgup>")
	press(t, m, "<ctrl+f>", "rent")
	f := findBarOf(t, m)
	if len(f.matches) != 1 || !strings.Contains(line(m, contextLine), "in Sheet1") {
		t.Fatalf("this sheet: %d matches, %q", len(f.matches), line(m, contextLine))
	}
	press(t, m, "<alt+s>")
	if len(f.matches) != 2 || !strings.Contains(line(m, contextLine), "in all sheets") {
		t.Fatalf("all sheets: %d matches, %q", len(f.matches), line(m, contextLine))
	}
	press(t, m, "<enter>")
	if m.sheet.Name() != "Sheet2" || m.cur != addr("A2") || !strings.Contains(line(m, contextLine), "2 of 2 on Sheet2") {
		t.Errorf("next: on %s at %v, %q", m.sheet.Name(), m.cur, line(m, contextLine))
	}
	if !m.found(addr("A2")) || m.found(addr("A1")) {
		t.Error("highlights not per sheet")
	}
	press(t, m, "<ctrl+h>", "lease", "<ctrl+enter>")
	if input(m, "A2") != "leaseal" || m.book().Sheet(0).Cell(addr("A1")).Input != "lease" {
		t.Errorf("replace all: %q", input(m, "A2"))
	}
	press(t, m, "<esc>", "<ctrl+z>")
	if input(m, "A2") != "rental" || m.book().Sheet(0).Cell(addr("A1")).Input != "rent" {
		t.Errorf("one undo: %q", input(m, "A2"))
	}
}

func TestNamedRangeOnAnotherSheet(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+f11>", "3", "<enter>", "4", "<enter>")
	m.sheet.DefineName("Costs", sheet.NewRect(addr("A1"), addr("A2")))
	press(t, m, "<ctrl+pgup>", "=SUM(Costs)", "<enter>")
	if v := m.sheet.Value(addr("A1")); v.Num != 7 {
		t.Errorf("SUM(Costs) = %v", v)
	}
	m.runCommand("data.named_ranges")
	if !strings.Contains(screen(m), "Sheet2!A1:A2") {
		t.Errorf("names list doesn't say the sheet:\n%s", screen(m))
	}
	press(t, m, "cos", "<enter>")
	if m.sheet.Name() != "Sheet2" || m.selection() != sheet.NewRect(addr("A1"), addr("A2")) {
		t.Errorf("go to name: on %s, %v", m.sheet.Name(), m.selection())
	}
	if name := strings.TrimSpace(ansi.Cut(line(m, formulaLine), 0, nameBoxW)); name != "Costs" {
		t.Errorf("name box %q", name)
	}
	press(t, m, "<ctrl+pgup>", "<ctrl+home>", "<shift+down>")
	if name := strings.TrimSpace(ansi.Cut(line(m, formulaLine), 0, nameBoxW)); name != "A1:A2" {
		t.Errorf("name box on Sheet1 %q", name)
	}
}

func TestTraceIntoOtherSheet(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+f11>", "7", "<enter>", "<ctrl+pgup>", "=Sheet2!A1*2", "<enter>", "<up>")
	press(t, m, "<alt+,>")
	if m.sheet.Name() != "Sheet2" || m.cur != addr("A1") || !strings.Contains(line(m, contextLine), "1 precedent of Sheet1!A1: Sheet2!A1") {
		t.Errorf("trace: on %s at %v, %q", m.sheet.Name(), m.cur, line(m, contextLine))
	}
	press(t, m, "<esc>")
	if m.sheet.Name() != "Sheet1" || m.cur != addr("A1") {
		t.Errorf("esc: on %s at %v", m.sheet.Name(), m.cur)
	}
}
