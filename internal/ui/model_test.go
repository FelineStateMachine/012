package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"one23/internal/sheet"
)

var named = map[string]rune{
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, "backspace": tea.KeyBackspace,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"f2": tea.KeyF2, "f5": tea.KeyF5, "delete": tea.KeyDelete, "home": tea.KeyHome,
	"pgdown": tea.KeyPgDown, "tab": tea.KeyTab,
}

// press sends keys to m. Each argument is a named key in <angle brackets>
// or literal text typed one character at a time. Commands are run and
// their messages fed back, except tea.Quit, which is returned.
func press(t *testing.T, m *Model, keys ...string) tea.Msg {
	t.Helper()
	var last tea.Msg
	send := func(msg tea.Msg) {
		_, cmd := m.Update(msg)
		for cmd != nil {
			out := cmd()
			if _, ok := out.(tea.QuitMsg); ok {
				last = out
				return
			}
			_, cmd = m.Update(out)
		}
	}
	for _, k := range keys {
		if name, ok := strings.CutPrefix(k, "<"); ok {
			name = strings.TrimSuffix(name, ">")
			code, ok := named[name]
			if !ok {
				t.Fatalf("unknown key %q", name)
			}
			send(tea.KeyPressMsg{Code: code})
			continue
		}
		for _, r := range k {
			send(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	return last
}

func newModel() *Model {
	m := New(sheet.New(), "")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	return m
}

func screen(m *Model) string {
	return ansi.Strip(m.View().Content)
}

func addr(s string) sheet.Addr {
	a, _ := sheet.ParseAddr(s)
	return a
}

func TestEntryAndMove(t *testing.T) {
	m := newModel()
	press(t, m, "12", "<down>", "30", "<down>", "@SUM(A1..A2)", "<enter>")
	if m.mode != modeReady || m.cur != addr("A3") {
		t.Fatalf("mode %v at %v", m.mode, m.cur)
	}
	if got := m.sheet.Value(addr("A3")).Num; got != 42 {
		t.Errorf("A3 = %v", got)
	}
	if s := screen(m); !strings.Contains(s, "A3: @SUM(A1..A2)") || !strings.Contains(s, "      42 ") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestLabelSpill(t *testing.T) {
	m := newModel()
	press(t, m, "Quarterly revenue report", "<enter>")
	row := strings.Split(screen(m), "\n")[gridTop]
	if !strings.Contains(row, "Quarterly revenue report") {
		t.Errorf("label did not spill: %q", row)
	}
	// A filled neighbor cuts the spill off.
	press(t, m, "<right>", "5", "<enter>")
	row = strings.Split(screen(m), "\n")[gridTop]
	if !strings.Contains(row, "Quarterly       5 ") {
		t.Errorf("spill not truncated: %q", row)
	}
}

func TestPointMode(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<down>", "2", "<down>", "<down>")
	press(t, m, "@SUM(", "<up>", "<up>", "<up>")
	if m.mode != modePoint || panelText(m, 1) != "@SUM(A1" {
		t.Fatalf("mode %v, line 2 %q", m.mode, panelText(m, 1))
	}
	press(t, m, ".", "<down>")
	if panelText(m, 1) != "@SUM(A1..A2" {
		t.Fatalf("line 2 %q", panelText(m, 1))
	}
	press(t, m, ")", "<enter>")
	c := m.sheet.Cell(addr("A4"))
	if c == nil || c.Input != "@SUM(A1..A2)" || c.Value.Num != 3 {
		t.Errorf("A4 = %+v", c)
	}
}

func TestInvalidFormulaGoesToEdit(t *testing.T) {
	m := newModel()
	press(t, m, "+B1+", "<enter>")
	if m.mode != modeEdit || m.hint == "" || m.sheet.Cell(addr("A1")) != nil {
		t.Fatalf("mode %v hint %q", m.mode, m.hint)
	}
	press(t, m, "1", "<enter>")
	if m.mode != modeReady || m.sheet.Value(addr("A1")).Num != 1 {
		t.Errorf("mode %v A1 %+v", m.mode, m.sheet.Value(addr("A1")))
	}
}

func TestEditExisting(t *testing.T) {
	m := newModel()
	press(t, m, "+2*3", "<enter>", "<f2>", "<left>", "<backspace>", "4", "<enter>")
	// Cursor lands before "3", Backspace removes "*", then "4" is inserted.
	if got := m.sheet.Cell(addr("A1")).Input; got != "+243" {
		t.Errorf("input %q, want +243", got)
	}
}

func TestMenuColumnWidth(t *testing.T) {
	m := newModel()
	press(t, m, "/")
	if !strings.Contains(panelText(m, 1), "Worksheet  Range  File  Quit") {
		t.Fatalf("menu line %q", panelText(m, 1))
	}
	press(t, m, "wcs", "15", "<enter>")
	if m.mode != modeReady || m.sheet.ColWidth(0) != 15 {
		t.Errorf("mode %v width %d", m.mode, m.sheet.ColWidth(0))
	}
	// Arrows preview live; Esc restores.
	press(t, m, "/wcs", "<right>", "<right>")
	if m.sheet.ColWidth(0) != 17 {
		t.Errorf("preview width %d", m.sheet.ColWidth(0))
	}
	press(t, m, "<esc>")
	if m.sheet.ColWidth(0) != 15 {
		t.Errorf("width after esc %d", m.sheet.ColWidth(0))
	}
}

func TestRangeErase(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<right>", "2", "<right>", "3", "<enter>", "<home>")
	press(t, m, "/re", "<right>", "<enter>")
	if m.sheet.Cell(addr("A1")) != nil || m.sheet.Cell(addr("B1")) != nil || m.sheet.Cell(addr("C1")) == nil {
		t.Errorf("cells after erase: %d", m.sheet.Len())
	}
	press(t, m, "/re", "<backspace>", "c1", "<enter>")
	if m.sheet.Len() != 0 {
		t.Errorf("typed range not erased")
	}
}

func TestGoto(t *testing.T) {
	m := newModel()
	press(t, m, "<f5>", "Z100", "<enter>")
	if m.cur != addr("Z100") || m.top == 0 || m.left == 0 {
		t.Errorf("cur %v top %d left %d", m.cur, m.top, m.left)
	}
	press(t, m, "<f5>", "nope", "<enter>")
	if m.mode != modeError {
		t.Errorf("mode %v", m.mode)
	}
	press(t, m, "<esc>")
	if m.mode != modeReady {
		t.Errorf("mode %v after dismiss", m.mode)
	}
}

func TestSaveAndRetrieve(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	press(t, m, "Hello", "<down>", "42", "<enter>")
	press(t, m, "/fs", "budget", "<enter>")
	if m.filename != "budget.o23" || m.changed {
		t.Fatalf("filename %q changed %v err %q", m.filename, m.changed, m.errMsg)
	}
	if _, err := os.Stat(filepath.Join(".", "budget.o23")); err != nil {
		t.Fatal(err)
	}

	m2 := newModel()
	press(t, m2, "/fr")
	if !strings.Contains(panelText(m2, 2), "budget.o23") {
		t.Errorf("file list %q", panelText(m2, 2))
	}
	press(t, m2, "budget", "<enter>")
	if m2.sheet.Value(addr("A2")).Num != 42 || m2.filename != "budget.o23" {
		t.Errorf("retrieved A2 %+v file %q", m2.sheet.Value(addr("A2")), m2.filename)
	}

	press(t, m2, "/fr", "missing", "<enter>")
	if m2.mode != modeError {
		t.Errorf("mode %v", m2.mode)
	}
}

func TestQuit(t *testing.T) {
	m := newModel()
	if msg := press(t, m, "/qn"); msg != nil || m.mode != modeReady {
		t.Fatalf("Quit No: %v mode %v", msg, m.mode)
	}
	press(t, m, "1", "<enter>", "/q", "<right>")
	if !strings.Contains(panelText(m, 2), "NOT SAVED") {
		t.Errorf("no unsaved warning: %q", panelText(m, 2))
	}
	if _, ok := press(t, m, "<enter>").(tea.QuitMsg); !ok {
		t.Error("Quit Yes did not quit")
	}
}

func TestScrollFollowsPointer(t *testing.T) {
	m := newModel()
	for range 40 {
		press(t, m, "<down>")
	}
	if m.top == 0 || m.cur.Row != 40 || m.cur.Row >= m.top+m.visibleRows() {
		t.Errorf("top %d cur %v rows %d", m.top, m.cur, m.visibleRows())
	}
	press(t, m, "<home>")
	if m.top != 0 || m.cur != (sheet.Addr{}) {
		t.Errorf("home: top %d cur %v", m.top, m.cur)
	}
}

func panelText(m *Model, line int) string {
	return strings.TrimRight(strings.Split(screen(m), "\n")[line], " ")
}
