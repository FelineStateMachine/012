package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// vimModel is a model with vim keys on and a small table: A1:C6 filled
// with their own addresses, row 1 as a header.
func vimModel(t *testing.T) *Model {
	t.Helper()
	m := newModel()
	for _, col := range "ABC" {
		for row := 1; row <= 6; row++ {
			a := string(col) + string(rune('0'+row))
			if err := m.sheet.Set(addr(a), a); err != nil {
				t.Fatal(err)
			}
		}
	}
	m.sheet.ClearHistory()
	m.saved, m.changed = m.sheet.StateID(), false
	m.runCommand("settings.vim")
	return m
}

func indicator(m *Model) string {
	f := strings.Fields(line(m, menuLine))
	return f[len(f)-1]
}

// File > Settings > Vim keys turns NORMAL mode on and off; letters move
// instead of typing, and the setting survives File > New.
func TestVimSetting(t *testing.T) {
	m := newModel()
	press(t, m, "<alt+f>", "<up>", "<up>", "<right>")
	if s := screen(m); !strings.Contains(s, "Vim keys") {
		t.Fatalf("settings submenu:\n%s", s)
	}
	press(t, m, "<down>", "<down>", "<enter>")
	if !m.prefs.vim || indicator(m) != "NORMAL" || !strings.Contains(line(m, contextLine), "Vim keys on") {
		t.Fatalf("vim %v, indicator %q, context %q", m.prefs.vim, indicator(m), line(m, contextLine))
	}
	press(t, m, "j", "l", "x")
	if m.cur != addr("B2") || m.mode != modeReady || m.sheet.Len() != 0 {
		t.Errorf("hjkl: cur %v mode %v cells %d", m.cur, m.mode, m.sheet.Len())
	}
	if !strings.Contains(line(m, m.height-1), ":  command") {
		t.Errorf("status line: %q", line(m, m.height-1))
	}
	m.runCommand("file.new")
	if !m.prefs.vim || indicator(m) != "NORMAL" {
		t.Errorf("File > New dropped vim keys")
	}
	m.runCommand("settings.vim")
	press(t, m, "j")
	if m.prefs.vim || indicator(m) != "ENTER" || bar(m) != "j" {
		t.Errorf("off: indicator %q, bar %q", indicator(m), bar(m))
	}
}

func TestVimMotionsAndCounts(t *testing.T) {
	m := vimModel(t)
	steps := []struct {
		keys []string
		want string
	}{
		{[]string{"3j"}, "A4"},
		{[]string{"2l"}, "C4"},
		{[]string{"k"}, "C3"},
		{[]string{"h"}, "B3"},
		{[]string{"gg"}, "B1"},
		{[]string{"G"}, "B6"},
		{[]string{"3G"}, "B3"},
		{[]string{"5gg"}, "B5"},
		{[]string{"0"}, "A5"},
		{[]string{"$"}, "C5"},
		{[]string{"b"}, "A5"},
		{[]string{"w"}, "C5"},
		{[]string{"<down>"}, "C6"},
		{[]string{"10k"}, "C1"},
		{[]string{"<ctrl+d>"}, "C8"},
		{[]string{"<ctrl+u>"}, "C1"},
		{[]string{"L"}, "C15"},
		{[]string{"M"}, "C8"},
		{[]string{"H"}, "C1"},
		{[]string{"j", "20", "<esc>", "j"}, "C3"},
	}
	for _, s := range steps {
		press(t, m, s.keys...)
		if m.cur.String() != s.want || m.hasRange() {
			t.Fatalf("%v: at %v (range %v), want %s", s.keys, m.cur, m.hasRange(), s.want)
		}
	}
	// A count or a sequence being typed shows on the context line.
	press(t, m, "3d")
	if l := line(m, contextLine); !strings.HasPrefix(l, "3d") || indicator(m) != "NORMAL" {
		t.Errorf("pending: %q", l)
	}
	press(t, m, "<esc>")
	if m.vim.pending() != "" || strings.HasPrefix(line(m, contextLine), "3d") {
		t.Errorf("Esc kept %q", m.vim.pending())
	}
	// A broken sequence does nothing.
	press(t, m, "dz", "j")
	if m.cur != addr("C4") || m.sheet.Len() != 18 {
		t.Errorf("dz: at %v, %d cells", m.cur, m.sheet.Len())
	}
}

func TestVimRowOperators(t *testing.T) {
	m := vimModel(t)
	press(t, m, "j", "2dd")
	if input(m, "A2") != "A4" || m.sheet.Len() != 12 || m.cur != addr("A2") {
		t.Fatalf("2dd: A2 %q, %d cells, at %v", input(m, "A2"), m.sheet.Len(), m.cur)
	}
	if !strings.Contains(line(m, contextLine), "Deleted 2 rows") {
		t.Errorf("context: %q", line(m, contextLine))
	}
	// p puts the rows back below, P above; each is one undo step.
	press(t, m, "p")
	if input(m, "A3") != "A2" || input(m, "A4") != "A3" || input(m, "C4") != "C3" || m.cur != addr("A3") {
		t.Fatalf("p: A3 %q A4 %q C4 %q at %v", input(m, "A3"), input(m, "A4"), input(m, "C4"), m.cur)
	}
	press(t, m, "u")
	if input(m, "A3") != "A5" || m.sheet.Len() != 12 {
		t.Fatalf("u: A3 %q", input(m, "A3"))
	}
	press(t, m, "gg", "P")
	if input(m, "A1") != "A2" || input(m, "A3") != "A1" {
		t.Fatalf("P: A1 %q A3 %q", input(m, "A1"), input(m, "A3"))
	}
	press(t, m, "u", "u")
	if input(m, "A2") != "A2" || m.sheet.Len() != 18 {
		t.Fatalf("uu: A2 %q, %d cells", input(m, "A2"), m.sheet.Len())
	}
	press(t, m, "<ctrl+r>")
	if input(m, "A2") != "A4" {
		t.Fatalf("Ctrl+R: A2 %q", input(m, "A2"))
	}
	press(t, m, "u")
	// yy copies without deleting; 2p pastes twice.
	press(t, m, "G", "yy", "2p")
	if input(m, "B7") != "B6" || input(m, "B8") != "B6" || m.sheet.Len() != 24 {
		t.Fatalf("yy 2p: B7 %q B8 %q, %d cells", input(m, "B7"), input(m, "B8"), m.sheet.Len())
	}
	// x clears cells to the right, keeping rows.
	press(t, m, "gg", "2x")
	if input(m, "A1") != "" || input(m, "B1") != "" || input(m, "C1") != "C1" || m.hasRange() {
		t.Fatalf("2x: %q %q %q", input(m, "A1"), input(m, "B1"), input(m, "C1"))
	}
	// o and O open a row and start typing in it.
	press(t, m, "o", "new", "<enter>")
	if input(m, "A2") != "new" || input(m, "A3") != "A2" {
		t.Fatalf("o: A2 %q A3 %q", input(m, "A2"), input(m, "A3"))
	}
	press(t, m, "gg", "O")
	if m.mode != modeEnter || m.cur != addr("A1") || input(m, "C2") != "C1" {
		t.Fatalf("O: mode %v at %v, C2 %q", m.mode, m.cur, input(m, "C2"))
	}
	press(t, m, "<esc>")
	if indicator(m) != "NORMAL" {
		t.Errorf("after Esc: %q", indicator(m))
	}
}

func TestVimEditKeys(t *testing.T) {
	m := vimModel(t)
	press(t, m, "i", "x", "<enter>")
	if input(m, "A1") != "xA1" || m.cur != addr("A2") {
		t.Fatalf("i: A1 %q at %v", input(m, "A1"), m.cur)
	}
	press(t, m, "a", "y", "<enter>")
	if input(m, "A2") != "A2y" {
		t.Fatalf("a: A2 %q", input(m, "A2"))
	}
	press(t, m, "=", "1+1", "<enter>")
	if m.sheet.Value(addr("A3")).Num != 2 {
		t.Fatalf("=: A3 %q", input(m, "A3"))
	}
	// Sheets keys vim doesn't take still work.
	press(t, m, "<enter>")
	if m.mode != modeEdit {
		t.Errorf("Enter: mode %v", m.mode)
	}
	press(t, m, "<esc>", "<delete>")
	if input(m, "A4") != "" {
		t.Errorf("Del: A4 %q", input(m, "A4"))
	}
}

func TestVimVisual(t *testing.T) {
	m := vimModel(t)
	press(t, m, "v", "j", "l")
	if indicator(m) != "VISUAL" || m.selection().String() != "A1:B2" {
		t.Fatalf("v: %q %v", indicator(m), m.selection())
	}
	if !strings.Contains(line(m, contextLine), "other corner") {
		t.Errorf("context: %q", line(m, contextLine))
	}
	press(t, m, "o")
	if m.cur != addr("B2") || m.selection().String() != "A1:B2" {
		t.Errorf("o: at %v, %v", m.cur, m.selection())
	}
	press(t, m, "y")
	if indicator(m) != "NORMAL" || m.hasRange() || m.copied.clip == nil || m.copied.clip.Src.String() != "A1:B2" {
		t.Fatalf("y: %q range %v", indicator(m), m.hasRange())
	}
	press(t, m, "G", "p")
	if input(m, "B6") != "A1" || input(m, "C7") != "B2" {
		t.Fatalf("p: B6 %q C7 %q", input(m, "B6"), input(m, "C7"))
	}
	press(t, m, "gg", "0", "v", "d")
	if input(m, "A1") != "" || indicator(m) != "NORMAL" {
		t.Fatalf("v d: A1 %q", input(m, "A1"))
	}
	press(t, m, "p")
	if input(m, "A1") != "A1" {
		t.Fatalf("p after d: A1 %q", input(m, "A1"))
	}
	// V selects whole rows; motions stretch them and d deletes them.
	press(t, m, "j", "V", "2j")
	if indicator(m) != "VISUAL" || m.whole != wholeRows || m.selection().From.Row != 1 || m.selection().To.Row != 3 {
		t.Fatalf("V: %v whole %v", m.selection(), m.whole)
	}
	press(t, m, "d")
	if input(m, "A2") != "A5" || !m.rowsCopied() {
		t.Fatalf("V d: A2 %q", input(m, "A2"))
	}
	// Esc leaves VISUAL, keeping the active cell.
	press(t, m, "v", "l", "<esc>")
	if indicator(m) != "NORMAL" || m.hasRange() || m.cur != addr("A2") {
		t.Errorf("Esc: %q %v at %v", indicator(m), m.hasRange(), m.cur)
	}
}

func TestVimFind(t *testing.T) {
	m := vimModel(t)
	press(t, m, "n")
	if !strings.Contains(line(m, contextLine), "No search yet") {
		t.Errorf("n before a search: %q", line(m, contextLine))
	}
	press(t, m, "/", "B", "<enter>")
	if m.overlay != nil || m.cur != addr("B1") || indicator(m) != "NORMAL" {
		t.Fatalf("/B: overlay %v at %v", m.overlay, m.cur)
	}
	press(t, m, "n", "n")
	if m.cur != addr("B3") || !strings.Contains(line(m, contextLine), "Match 3 of 6") {
		t.Fatalf("nn: at %v, %q", m.cur, line(m, contextLine))
	}
	press(t, m, "N")
	if m.cur != addr("B2") {
		t.Errorf("N: at %v", m.cur)
	}
	press(t, m, "gg", "N")
	if m.cur != addr("B6") {
		t.Errorf("N wraps: at %v", m.cur)
	}
}

func TestVimCommandLine(t *testing.T) {
	m := vimModel(t)
	press(t, m, ":")
	if indicator(m) != "COMMAND" || !strings.HasPrefix(line(m, contextLine), ":") {
		t.Fatalf(": %q %q", indicator(m), line(m, contextLine))
	}
	press(t, m, "B12", "<enter>")
	if m.cur != addr("B12") || m.overlay != nil || indicator(m) != "NORMAL" {
		t.Fatalf(":B12 at %v", m.cur)
	}
	press(t, m, ":40", "<enter>")
	if m.cur != addr("B40") {
		t.Fatalf(":40 at %v", m.cur)
	}
	press(t, m, ":Sheet1!C2", "<enter>")
	if m.cur != addr("C2") {
		t.Fatalf(":Sheet1!C2 at %v", m.cur)
	}
	// Completion comes from the command registry.
	press(t, m, ":fill")
	s := screen(m)
	for _, want := range []string{"Commands", "Fill down", "edit.fill_down", "Fill right"} {
		if !strings.Contains(s, want) {
			t.Fatalf(":fill completions lack %q:\n%s", want, s)
		}
	}
	if !strings.Contains(line(m, m.height-1), "Copy the top row") {
		t.Errorf("status: %q", line(m, m.height-1))
	}
	press(t, m, "<tab>")
	if m.line.Text() != "edit.fill_down" {
		t.Errorf("Tab: %q", m.line.Text())
	}
	press(t, m, "<tab>")
	if m.line.Text() != "edit.fill_right" {
		t.Errorf("Tab Tab: %q", m.line.Text())
	}
	press(t, m, "<esc>")
	// A title runs its command; so does the first completion of a prefix.
	press(t, m, "v", "j", ":", "fill down", "<enter>")
	if input(m, "C3") != "C2" {
		t.Errorf(":fill down: C3 %q", input(m, "C3"))
	}
	press(t, m, "<esc>", ":edit.undo", "<enter>")
	if input(m, "C3") != "C3" {
		t.Errorf(":edit.undo: C3 %q", input(m, "C3"))
	}
	press(t, m, ":keyboard sh", "<enter>")
	if _, ok := m.overlay.(*shortcuts); !ok {
		t.Errorf(":keyboard sh opened %T", m.overlay)
	}
	press(t, m, "<esc>", ":nonsense", "<enter>")
	if m.mode != modeError || !strings.Contains(line(m, m.height-1), "Not a command, cell or range: nonsense") {
		t.Errorf(":nonsense: %q", line(m, m.height-1))
	}
	press(t, m, "<esc>", ":", "<backspace>")
	if m.overlay != nil {
		t.Errorf("Backspace on an empty line left %T open", m.overlay)
	}
}

func TestVimFileCommands(t *testing.T) {
	dir := t.TempDir()
	m := vimModel(t)
	name := filepath.Join(dir, "book")
	press(t, m, "x", ":w "+name, "<enter>")
	if _, err := os.Stat(name + ".012"); err != nil || m.filename != name+".012" || m.changed {
		t.Fatalf(":w: %v, filename %q, changed %v", err, m.filename, m.changed)
	}
	press(t, m, ":w "+filepath.Join(dir, "out.csv"), "<enter>")
	if data, err := os.ReadFile(filepath.Join(dir, "out.csv")); err != nil || !strings.Contains(string(data), "B1,C1") {
		t.Fatalf(":w out.csv: %v %q", err, data)
	}
	press(t, m, "j", "x", ":e other", "<enter>")
	if m.mode != modeError || !strings.Contains(line(m, m.height-1), "Unsaved changes") {
		t.Fatalf(":e with changes: %q", line(m, m.height-1))
	}
	press(t, m, "<esc>", ":e! "+name, "<enter>")
	if m.changed || input(m, "A2") != "A2" {
		t.Fatalf(":e!: changed %v, A2 %q", m.changed, input(m, "A2"))
	}
	if _, ok := press(t, m, ":q", "<enter>").(tea.QuitMsg); !ok {
		t.Error(":q didn't quit a saved sheet")
	}
	press(t, m, "x", ":q", "<enter>")
	if !strings.Contains(line(m, contextLine), "unsaved changes") {
		t.Errorf(":q with changes: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if _, ok := press(t, m, ":wq", "<enter>").(tea.QuitMsg); !ok || m.changed {
		t.Errorf(":wq didn't save and quit")
	}
}

// Help lists the vim keys first, and the palette and menus show vim keys
// where Sheets' are taken.
func TestVimHelpAndShortcuts(t *testing.T) {
	m := vimModel(t)
	press(t, m, "<f1>")
	s := screen(m)
	if !strings.Contains(s, "Vim keys: NORMAL") || !strings.Contains(s, "Left, down, up, right") {
		t.Errorf("help:\n%s", s)
	}
	found := false
	for _, r := range helpRows(true) {
		found = found || r.action == "Cut rows" && strings.Join(r.keys, " ") == "dd"
	}
	if !found {
		t.Error("help lacks dd for Cut rows")
	}
	if m.shortcut("row.cut") != "dd" || m.shortcut("edit.fill_down") != "" || m.shortcut("edit.undo") != "Ctrl+Z" {
		t.Errorf("shortcuts: %q %q %q", m.shortcut("row.cut"), m.shortcut("edit.fill_down"), m.shortcut("edit.undo"))
	}
	if shortcut("edit.fill_down") != "Ctrl+D" {
		t.Errorf("Sheets shortcut: %q", shortcut("edit.fill_down"))
	}
	rows := helpRows(true)
	if rows[0].heading != "Vim keys: NORMAL" {
		t.Errorf("first help group %q", rows[0].heading)
	}
	for _, r := range helpRows(false) {
		if strings.HasPrefix(r.heading, "Vim") {
			t.Error("vim keys in help with vim keys off")
		}
	}
}
