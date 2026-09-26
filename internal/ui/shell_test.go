package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// fakeCommand replaces (or adds) a command for the length of a test and
// returns a run counter. The real command, if any, is restored afterwards.
func fakeCommand(t *testing.T, id, title string, enabled func(*Model) bool) *int {
	t.Helper()
	runs := new(int)
	prev, had := commands[id]
	commands[id] = &command{id: id, title: title, desc: title + " (test)", enabled: enabled,
		run: func(*Model) tea.Cmd { *runs++; return nil }}
	t.Cleanup(func() {
		if had {
			commands[id] = prev
		} else {
			delete(commands, id)
		}
	})
	return runs
}

func mouseAt(m *Model, msg tea.MouseMsg) { send(m, msg) }

func leftClick(m *Model, x, y int) {
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	mouseAt(m, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// menu returns the open menu overlay, failing if there is none.
func openMenu(t *testing.T, m *Model) *menuOverlay {
	t.Helper()
	o, ok := m.overlay.(*menuOverlay)
	if !ok {
		t.Fatalf("no menu open (mode %v, overlay %T)", m.mode, m.overlay)
	}
	return o
}

func highlighted(t *testing.T, m *Model) string {
	t.Helper()
	l := openMenu(t, m).top()
	if l.sel < 0 {
		return ""
	}
	return l.items[l.sel].label()
}

func TestControlPanelLayout(t *testing.T) {
	m := newModel()
	if l := line(m, menuLine); !strings.HasPrefix(l, " File  Edit") || !strings.HasSuffix(l, "READY") {
		t.Errorf("menu bar %q", l)
	}
	press(t, m, "<shift+down>", "<shift+right>")
	if l := line(m, formulaLine); strings.TrimSpace(l) != "A1:B2" {
		t.Errorf("name box should show the selection: %q", l)
	}
	press(t, m, "<esc>", "=1+")
	if x, y, _ := m.cursorPos(); y != formulaLine || x != formulaBarTextX()+3 {
		t.Errorf("entry cursor at %d,%d", x, y)
	}
	if !strings.Contains(line(m, contextLine), "Enter  accept") {
		t.Errorf("context line %q", line(m, contextLine))
	}
	press(t, m, "<esc>", "<ctrl+g>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Go to: A1") || !strings.HasSuffix(l, "Esc  cancel") {
		t.Errorf("prompt line %q", l)
	}
	if x, y, _ := m.cursorPos(); y != contextLine || x != len("Go to: A1") {
		t.Errorf("prompt cursor at %d,%d", x, y)
	}
}

func TestVisibleItemsHidesMissingCommands(t *testing.T) {
	items := []menuItem{
		sep, {cmd: "nope.one"}, {cmd: "clear"}, sep, sep, {cmd: "nope.two"}, sep,
		{title: "Empty", items: []menuItem{{cmd: "nope.three"}}},
		{cmd: "goto"}, sep,
	}
	var got []string
	for _, it := range visibleItems(items) {
		if it.sep {
			got = append(got, "-")
		} else {
			got = append(got, it.label())
		}
	}
	if s := strings.Join(got, ","); s != "Clear,-,Go to" {
		t.Errorf("visible items %s", s)
	}
	// Data has no registered commands yet, so the bar hides it.
	for _, bm := range barMenus() {
		if bm.def.title == "Data" {
			t.Error("empty Data menu shown")
		}
	}
}

func TestMenuBarKeyboard(t *testing.T) {
	m := newModel()
	press(t, m, "<alt+f>")
	if o := openMenu(t, m); barMenus()[o.bar].def.title != "File" || highlighted(t, m) != "New" {
		t.Fatalf("Alt+F opened %d at %q", o.bar, highlighted(t, m))
	}
	if !strings.HasSuffix(line(m, menuLine), "MENU") {
		t.Errorf("indicator %q", line(m, menuLine))
	}
	// Up wraps to the last item, skipping the separator.
	press(t, m, "<up>")
	if highlighted(t, m) != "Quit" {
		t.Errorf("up highlighted %q", highlighted(t, m))
	}
	if !strings.Contains(line(m, m.height-1), "Close one23") {
		t.Errorf("status line should describe the item: %q", line(m, m.height-1))
	}
	press(t, m, "<right>")
	if barMenus()[openMenu(t, m).bar].def.title != "Edit" {
		t.Error("Right did not move to Edit")
	}
	press(t, m, "<left>", "<left>")
	if barMenus()[openMenu(t, m).bar].def.title != "Help" {
		t.Error("Left from File did not wrap to Help")
	}
	press(t, m, "<esc>")
	if m.mode != modeReady || m.overlay != nil {
		t.Errorf("Esc left mode %v", m.mode)
	}
	press(t, m, "<f10>")
	if barMenus()[openMenu(t, m).bar].def.title != "File" {
		t.Error("F10 did not open File")
	}
	press(t, m, "<f10>")
	if m.overlay != nil {
		t.Error("F10 did not close the menu")
	}
}

func TestMenuLetterRunsCommand(t *testing.T) {
	m := newModel()
	// Format has two items starting with C: the first press highlights
	// Column width, Enter runs it.
	press(t, m, "<shift+right>", "<alt+o>", "c")
	if highlighted(t, m) != "Column width" {
		t.Fatalf("c highlighted %q", highlighted(t, m))
	}
	press(t, m, "<enter>", "15", "<enter>")
	if m.mode != modeReady || m.sheet.ColWidth(0) != 15 || m.sheet.ColWidth(1) != 15 {
		t.Errorf("mode %v widths %d %d", m.mode, m.sheet.ColWidth(0), m.sheet.ColWidth(1))
	}
	// With several items starting with the letter, it cycles instead.
	press(t, m, "<alt+f>", "s")
	if highlighted(t, m) != "Save" {
		t.Errorf("s highlighted %q", highlighted(t, m))
	}
	press(t, m, "s")
	if highlighted(t, m) != "Save as" {
		t.Errorf("second s highlighted %q", highlighted(t, m))
	}
	press(t, m, "<enter>")
	if m.mode != modePrompt || m.prompt.label != "Save as:" {
		t.Errorf("Save as did not prompt: mode %v", m.mode)
	}
}

func TestSubmenuAndDisabledItems(t *testing.T) {
	percent := fakeCommand(t, "format.percent", "Percent", nil)
	off := func(*Model) bool { return false }
	fakeCommand(t, "format.automatic", "Automatic", off)
	fakeCommand(t, "format.plain_text", "Plain text", off)
	fakeCommand(t, "format.number", "Number", off)
	m := newModel()
	press(t, m, "<alt+o>")
	if l := line(m, 2); highlighted(t, m) != "Number" || !strings.Contains(l, "│ Number") || !strings.Contains(l, "› │") {
		t.Fatalf("Format opened at %q:\n%s", highlighted(t, m), screen(m))
	}
	press(t, m, "<right>")
	// The unavailable formats before Percent are skipped.
	if o := openMenu(t, m); len(o.levels) != 2 || highlighted(t, m) != "Percent" {
		t.Fatalf("submenu at %q", highlighted(t, m))
	}
	press(t, m, "<down>", "<up>")
	if highlighted(t, m) != "Percent" {
		t.Errorf("down and up moved onto %q", highlighted(t, m))
	}
	press(t, m, "<left>")
	if len(openMenu(t, m).levels) != 1 {
		t.Error("Left did not close the submenu")
	}
	press(t, m, "<enter>", "<esc>")
	if len(openMenu(t, m).levels) != 1 {
		t.Error("Esc closed more than the submenu")
	}
	press(t, m, "<enter>", "<enter>")
	if *percent != 1 || m.overlay != nil || m.mode != modeReady {
		t.Errorf("percent ran %d times, overlay %T", *percent, m.overlay)
	}
}

func TestMenuMouse(t *testing.T) {
	m := newModel()
	before := strings.Split(screen(m), "\n")
	leftClick(m, 2, menuLine) // "File"
	o := openMenu(t, m)
	if barMenus()[o.bar].def.title != "File" {
		t.Fatalf("clicked File, opened %d", o.bar)
	}
	if !strings.Contains(line(m, menuLine+1), "┌") || m.View().MouseMode != tea.MouseModeAllMotion {
		t.Errorf("dropdown not drawn under the title: %q", line(m, menuLine+1))
	}
	// The dropdown covers the grid without moving it.
	after := strings.Split(screen(m), "\n")
	w := o.layout(m)[0].width()
	for y := gridTop; y < m.height-1; y++ {
		a, b := ansi.Cut(after[y], w, m.width), ansi.Cut(before[y], w, m.width)
		if strings.TrimRight(a, " ") != strings.TrimRight(b, " ") {
			t.Errorf("row %d moved:\n%q\n%q", y, a, b)
		}
	}
	box := o.layout(m)[0]
	// Hovering highlights; the Save item is the third row.
	mouseAt(m, tea.MouseMotionMsg{X: box.x + 3, Y: box.y + 3})
	if highlighted(t, m) != "Save" {
		t.Errorf("hover highlighted %q", highlighted(t, m))
	}
	// Hovering another title switches menus.
	mouseAt(m, tea.MouseMotionMsg{X: barMenus()[1].x + 1, Y: menuLine})
	if openMenu(t, m).bar != 1 {
		t.Error("hovering Edit did not switch menus")
	}
	// Clicking the open title closes it; clicking outside closes too.
	leftClick(m, barMenus()[1].x+1, menuLine)
	if m.overlay != nil {
		t.Error("clicking the open title did not close it")
	}
	leftClick(m, 2, menuLine)
	leftClick(m, cellX(5), gridTop+10)
	if m.overlay != nil || m.cur != addr("A1") {
		t.Errorf("outside click: overlay %T, cur %v (the click should only close)", m.overlay, m.cur)
	}
	// Clicking an item runs it.
	t.Chdir(t.TempDir())
	leftClick(m, 2, menuLine)
	box = openMenu(t, m).layout(m)[0]
	leftClick(m, box.x+3, box.y+4) // Save as
	if m.mode != modePrompt || m.prompt.label != "Save as:" {
		t.Errorf("click on Save as: mode %v", m.mode)
	}
}

func TestQuitAsksWithChoiceBar(t *testing.T) {
	m := newModel()
	if _, ok := press(t, m, "<ctrl+q>").(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+Q with no changes did not quit")
	}
	m = newModel()
	press(t, m, "1", "<enter>")
	if msg := press(t, m, "<ctrl+q>"); msg != nil || m.mode != modeMenu {
		t.Fatalf("Ctrl+Q with changes: %v mode %v", msg, m.mode)
	}
	if l := line(m, contextLine); !strings.HasPrefix(l, "You have unsaved changes.") || !strings.Contains(l, "D  Discard") {
		t.Errorf("choice bar %q", l)
	}
	press(t, m, "<esc>")
	if m.mode != modeReady {
		t.Errorf("Esc: mode %v", m.mode)
	}
	press(t, m, "<ctrl+q>")
	if _, ok := press(t, m, "d").(tea.QuitMsg); !ok {
		t.Error("D did not discard and quit")
	}
}

func TestSaveAndQuit(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	press(t, m, "1", "<enter>", "<ctrl+q>", "<enter>")
	// Untitled: Save and quit asks for a name first; Esc cancels both.
	if m.mode != modePrompt {
		t.Fatalf("mode %v", m.mode)
	}
	press(t, m, "<esc>")
	if m.quitAfterSave {
		t.Error("cancelling Save as left Save and quit pending")
	}
	press(t, m, "<ctrl+q>", "<enter>")
	if _, ok := press(t, m, "sheet", "<enter>").(tea.QuitMsg); !ok || m.changed {
		t.Errorf("did not save and quit: changed %v err %q", m.changed, m.errMsg)
	}
	// Clicking a choice on the context line works too.
	m = newModel()
	press(t, m, "1", "<enter>", "<ctrl+q>")
	x := strings.Index(line(m, contextLine), "Discard")
	if msg, _ := m.shellMouse(tea.MouseClickMsg{X: x, Y: contextLine, Button: tea.MouseLeft}); msg == nil {
		t.Error("clicking Discard did not quit")
	}
}
