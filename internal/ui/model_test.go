package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"one23/internal/sheet"
)

var named = map[string]tea.Key{
	"enter": {Code: tea.KeyEnter}, "esc": {Code: tea.KeyEscape}, "backspace": {Code: tea.KeyBackspace},
	"tab": {Code: tea.KeyTab}, "delete": {Code: tea.KeyDelete}, "home": {Code: tea.KeyHome},
	"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft}, "right": {Code: tea.KeyRight},
	"pgdown": {Code: tea.KeyPgDown}, "f1": {Code: tea.KeyF1}, "f2": {Code: tea.KeyF2}, "f5": {Code: tea.KeyF5},
	"f10": {Code: tea.KeyF10}, "space": {Code: tea.KeySpace},
}

// press sends keys to m. Named keys go in angle brackets with optional
// modifiers, e.g. "<shift+down>" or "<ctrl+s>"; other text is typed a
// character at a time. Commands run and their messages are fed back,
// except tea.Quit, which is returned.
func press(t *testing.T, m *Model, keys ...string) tea.Msg {
	t.Helper()
	var last tea.Msg
	for _, k := range keys {
		name, ok := strings.CutPrefix(k, "<")
		if !ok {
			for _, r := range k {
				if msg := send(m, tea.KeyPressMsg{Code: r, Text: string(r)}); msg != nil {
					last = msg
				}
			}
			continue
		}
		parts := strings.Split(strings.TrimSuffix(name, ">"), "+")
		var mod tea.KeyMod
		for _, p := range parts[:len(parts)-1] {
			switch p {
			case "shift":
				mod |= tea.ModShift
			case "ctrl":
				mod |= tea.ModCtrl
			case "alt":
				mod |= tea.ModAlt
			}
		}
		base := parts[len(parts)-1]
		key, ok := named[base]
		if !ok && len(base) == 1 {
			key, ok = tea.Key{Code: rune(base[0])}, true
		}
		if !ok {
			t.Fatalf("unknown key %q", k)
		}
		key.Mod = mod
		if msg := send(m, tea.KeyPressMsg(key)); msg != nil {
			last = msg
		}
	}
	return last
}

// send delivers msg and runs resulting commands, returning a QuitMsg if
// one is produced.
func send(m *Model, msg tea.Msg) tea.Msg {
	_, cmd := m.Update(msg)
	for cmd != nil {
		out := cmd()
		if _, ok := out.(tea.QuitMsg); ok {
			return out
		}
		_, cmd = m.Update(out)
	}
	return nil
}

func newModel() *Model {
	m := New(sheet.New(), "")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	return m
}

func screen(m *Model) string {
	return ansi.Strip(m.View().Content)
}

func line(m *Model, i int) string {
	return strings.TrimRight(strings.Split(screen(m), "\n")[i], " ")
}

func addr(s string) sheet.Addr {
	a, _ := sheet.ParseAddr(s)
	return a
}

func input(m *Model, a string) string {
	if c := m.sheet.Cell(addr(a)); c != nil {
		return c.Input
	}
	return ""
}

func TestEnterCommitsAndMovesDown(t *testing.T) {
	m := newModel()
	press(t, m, "12", "<enter>", "30", "<enter>", "=SUM(A1:A2)", "<enter>")
	if m.mode != modeReady || m.cur != addr("A4") {
		t.Fatalf("mode %v at %v", m.mode, m.cur)
	}
	if got := m.sheet.Value(addr("A3")).Num; got != 42 {
		t.Errorf("A3 = %v", got)
	}
	press(t, m, "<up>")
	if !strings.HasPrefix(line(m, 0), " A3   =SUM(A1:A2)") {
		t.Errorf("formula bar %q", line(m, 0))
	}
}

func TestTabThenEnterReturnsToStartColumn(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "Name", "<tab>", "Qty", "<tab>", "Price", "<enter>")
	if m.cur != addr("B2") {
		t.Errorf("cur %v, want B2", m.cur)
	}
	if input(m, "D1") != "Price" {
		t.Errorf("D1 = %q", input(m, "D1"))
	}
}

func TestArrowsAcceptAndMove(t *testing.T) {
	m := newModel()
	press(t, m, "hello", "<right>")
	if input(m, "A1") != "hello" || m.cur != addr("B1") {
		t.Errorf("A1 %q cur %v", input(m, "A1"), m.cur)
	}
	press(t, m, "x", "<esc>")
	if input(m, "B1") != "" || m.mode != modeReady {
		t.Errorf("Esc kept %q", input(m, "B1"))
	}
}

func TestTextOverflow(t *testing.T) {
	m := newModel()
	press(t, m, "Quarterly revenue report", "<enter>")
	if row := line(m, gridTop); !strings.Contains(row, " Quarterly revenue report") {
		t.Errorf("text did not overflow: %q", row)
	}
	press(t, m, "<up>", "<right>", "5", "<enter>")
	if row := line(m, gridTop); !strings.Contains(row, " Quarterly        5") {
		t.Errorf("overflow not cut off: %q", row)
	}
}

func TestPointMode(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>", "2", "<enter>")
	press(t, m, "=SUM(", "<up>", "<up>")
	if m.mode != modePoint || line(m, 1) != "=SUM(A1" {
		t.Fatalf("mode %v, edit line %q", m.mode, line(m, 1))
	}
	press(t, m, "<shift+down>")
	if line(m, 1) != "=SUM(A1:A2" {
		t.Fatalf("edit line %q", line(m, 1))
	}
	press(t, m, ")", "<enter>")
	if input(m, "A3") != "=SUM(A1:A2)" || m.sheet.Value(addr("A3")).Num != 3 || m.cur != addr("A4") {
		t.Errorf("A3 %q = %v, cur %v", input(m, "A3"), m.sheet.Value(addr("A3")), m.cur)
	}
}

func TestInvalidFormulaStaysInEdit(t *testing.T) {
	m := newModel()
	press(t, m, "=B1+", "<enter>")
	if m.mode != modeEdit || m.hint != "Formula is incomplete" || m.sheet.Cell(addr("A1")) != nil {
		t.Fatalf("mode %v hint %q", m.mode, m.hint)
	}
	press(t, m, "1", "<enter>")
	if m.mode != modeReady || m.sheet.Value(addr("A1")).Num != 1 || m.cur != addr("A2") {
		t.Errorf("mode %v A1 %+v cur %v", m.mode, m.sheet.Value(addr("A1")), m.cur)
	}
}

func TestEditExisting(t *testing.T) {
	m := newModel()
	press(t, m, "=2*3", "<enter>", "<up>", "<f2>", "<left>", "<backspace>", "4", "<enter>")
	// Caret lands before "3", Backspace removes "*", then "4" is inserted.
	if got := input(m, "A1"); got != "=243" {
		t.Errorf("input %q, want =243", got)
	}
	press(t, m, "<up>", "<enter>")
	if m.mode != modeEdit || string(m.buf) != "=243" {
		t.Errorf("Enter should edit: mode %v buf %q", m.mode, string(m.buf))
	}
}

func TestShiftSelectStatsAndClear(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>", "2", "<enter>", "3", "<enter>", "<ctrl+home>")
	press(t, m, "<shift+down>", "<shift+down>")
	if got := m.selection().String(); got != "A1:A3" || m.cur != addr("A1") {
		t.Fatalf("selection %s cur %v", got, m.cur)
	}
	status := line(m, m.height-1)
	for _, want := range []string{"A1:A3", "Sum 6", "Avg 2", "Count 3"} {
		if !strings.Contains(status, want) {
			t.Errorf("status %q missing %q", status, want)
		}
	}
	press(t, m, "<delete>")
	if m.sheet.Len() != 0 {
		t.Errorf("%d cells left after clear", m.sheet.Len())
	}
	press(t, m, "<down>")
	if m.hasRange() {
		t.Error("arrow did not collapse the selection")
	}
}

func TestCtrlArrowJumps(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>", "2", "<enter>", "3", "<enter>", "<ctrl+home>")
	press(t, m, "<ctrl+down>")
	if m.cur != addr("A3") {
		t.Errorf("ctrl+down to %v", m.cur)
	}
	press(t, m, "<ctrl+home>", "<ctrl+shift+down>")
	if got := m.selection().String(); got != "A1:A3" {
		t.Errorf("ctrl+shift+down selected %s", got)
	}
}

func TestSelectAllColumnsRows(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "<down>", "x", "<enter>", "<ctrl+a>")
	if got := m.selection().String(); got != "A1:B2" {
		t.Errorf("ctrl+a selected %s", got)
	}
	press(t, m, "<ctrl+a>")
	if m.whole != wholeAll {
		t.Error("second ctrl+a did not select everything")
	}
	press(t, m, "<esc>", "<ctrl+space>")
	if got := m.selection(); got.From.Row != 0 || got.To.Row != sheet.MaxRows-1 {
		t.Errorf("ctrl+space selected %s", got)
	}
	press(t, m, "<esc>", "<shift+space>")
	if got := m.selection(); got.From.Col != 0 || got.To.Col != sheet.MaxCols-1 {
		t.Errorf("shift+space selected %s", got)
	}
}

func click(m *Model, x, y int, mod tea.KeyMod) {
	send(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: mod})
	send(m, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// cellX returns a screen x inside column c when scrolled to column A.
func cellX(c int) int { return rowHdrW + c*sheet.DefaultWidth + 2 }

func TestMouseSelection(t *testing.T) {
	m := newModel()
	click(m, cellX(1), gridTop+2, 0)
	if m.cur != addr("B3") || m.hasRange() {
		t.Fatalf("click: cur %v", m.cur)
	}
	click(m, cellX(2), gridTop+4, tea.ModShift)
	if got := m.selection().String(); got != "B3:C5" {
		t.Errorf("shift+click selected %s", got)
	}

	send(m, tea.MouseClickMsg{X: cellX(0), Y: gridTop, Button: tea.MouseLeft})
	send(m, tea.MouseMotionMsg{X: cellX(1), Y: gridTop + 1, Button: tea.MouseLeft})
	send(m, tea.MouseReleaseMsg{X: cellX(1), Y: gridTop + 1, Button: tea.MouseLeft})
	if got := m.selection().String(); got != "A1:B2" || m.cur != addr("A1") {
		t.Errorf("drag selected %s, cur %v", got, m.cur)
	}

	click(m, cellX(3), headerLine, 0)
	if got := m.selection(); got.From != addr("D1") || got.To != addr("D8192") {
		t.Errorf("column header selected %s", got)
	}
	click(m, 1, gridTop+3, 0)
	if got := m.selection(); got.From != addr("A4") || got.To.Col != sheet.MaxCols-1 {
		t.Errorf("row header selected %s", got)
	}
}

func TestDoubleClickEdits(t *testing.T) {
	m := newModel()
	press(t, m, "hi", "<enter>")
	click(m, cellX(0), gridTop, 0)
	click(m, cellX(0), gridTop, 0)
	if m.mode != modeEdit || string(m.buf) != "hi" {
		t.Errorf("mode %v buf %q", m.mode, string(m.buf))
	}
	press(t, m, "<esc>")
	m.lastClick = time.Time{}
	click(m, cellX(0), gridTop, 0)
	if m.mode != modeReady {
		t.Error("a single click edited")
	}
}

func TestWheelDoesNotSnapBack(t *testing.T) {
	m := newModel()
	for range 5 {
		send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	}
	if m.top != 15 || m.cur != addr("A1") {
		t.Fatalf("top %d cur %v", m.top, m.cur)
	}
	press(t, m, "<down>")
	if m.top != 1 {
		t.Errorf("moving the cursor should bring it back into view, top %d", m.top)
	}
}

func TestMenuColumnWidthOnSelection(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+right>", "<f10>")
	if line(m, 1) != "File  Edit  Format" {
		t.Fatalf("menu line %q", line(m, 1))
	}
	press(t, m, "f") // two items start with F: cycles to Format
	press(t, m, "<enter>", "c", "15", "<enter>")
	if m.mode != modeReady || m.sheet.ColWidth(0) != 15 || m.sheet.ColWidth(1) != 15 {
		t.Errorf("mode %v widths %d %d", m.mode, m.sheet.ColWidth(0), m.sheet.ColWidth(1))
	}
	// Arrows preview live; Esc restores.
	press(t, m, "<f10>", "<left>", "<enter>", "c", "<right>", "<right>")
	if m.sheet.ColWidth(0) != 17 {
		t.Errorf("preview width %d", m.sheet.ColWidth(0))
	}
	press(t, m, "<esc>")
	if m.sheet.ColWidth(0) != 15 {
		t.Errorf("width after esc %d", m.sheet.ColWidth(0))
	}
}

func TestGoto(t *testing.T) {
	m := newModel()
	press(t, m, "<ctrl+g>", "Z100", "<enter>")
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

func TestSaveAndOpen(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	press(t, m, "Hello", "<enter>", "42", "<enter>")
	press(t, m, "<ctrl+s>", "budget", "<enter>")
	if m.filename != "budget.o23" || m.changed {
		t.Fatalf("filename %q changed %v err %q", m.filename, m.changed, m.errMsg)
	}
	if _, err := os.Stat("budget.o23"); err != nil {
		t.Fatal(err)
	}
	// Ctrl+S again saves without asking.
	press(t, m, "7", "<enter>", "<ctrl+s>")
	if m.mode != modeReady || m.changed {
		t.Errorf("second save: mode %v changed %v", m.mode, m.changed)
	}

	m2 := newModel()
	press(t, m2, "<ctrl+o>")
	if !strings.Contains(line(m2, 2), "budget.o23") {
		t.Errorf("file list %q", line(m2, 2))
	}
	press(t, m2, "budget", "<enter>")
	if m2.sheet.Value(addr("A2")).Num != 42 || m2.filename != "budget.o23" {
		t.Errorf("opened A2 %+v file %q", m2.sheet.Value(addr("A2")), m2.filename)
	}
	press(t, m2, "<ctrl+o>", "missing", "<enter>")
	if m2.mode != modeError {
		t.Errorf("mode %v", m2.mode)
	}
}

func TestQuitConfirmsUnsavedChanges(t *testing.T) {
	m := newModel()
	if _, ok := press(t, m, "<ctrl+q>").(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+Q with no changes did not quit")
	}
	m = newModel()
	press(t, m, "1", "<enter>")
	if msg := press(t, m, "<ctrl+q>"); msg != nil || m.mode != modeMenu {
		t.Fatalf("Ctrl+Q with changes: %v mode %v", msg, m.mode)
	}
	press(t, m, "<right>")
	if !strings.Contains(line(m, 2), "unsaved changes") {
		t.Errorf("no unsaved warning: %q", line(m, 2))
	}
	if _, ok := press(t, m, "<enter>").(tea.QuitMsg); !ok {
		t.Error("Quit without saving did not quit")
	}
}

func TestScrollFollowsCursor(t *testing.T) {
	m := newModel()
	for range 40 {
		press(t, m, "<down>")
	}
	if m.top == 0 || m.cur.Row != 40 || m.cur.Row >= m.top+m.visibleRows() {
		t.Errorf("top %d cur %v rows %d", m.top, m.cur, m.visibleRows())
	}
	press(t, m, "<ctrl+home>")
	if m.top != 0 || m.cur != (sheet.Addr{}) {
		t.Errorf("ctrl+home: top %d cur %v", m.top, m.cur)
	}
}

func TestHelpListsShortcuts(t *testing.T) {
	m := newModel()
	press(t, m, "<f1>")
	s := screen(m)
	for _, want := range []string{"Ctrl+S", "Save the sheet", "Backspace / Del", "SUM"} {
		if !strings.Contains(s, want) {
			t.Errorf("help missing %q", want)
		}
	}
}
