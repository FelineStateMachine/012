package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// . repeats the last change with its count, or the count typed before
// it; an edit begun with i, a, o or = repeats with what was typed.
func TestVimRepeat(t *testing.T) {
	m := vimModel(t)
	press(t, m, ".")
	if !strings.Contains(line(m, contextLine), "No change to repeat") {
		t.Errorf(". first: %q", line(m, contextLine))
	}
	press(t, m, "x", "j", ".")
	if input(m, "A1") != "" || input(m, "A2") != "" || input(m, "A3") != "A3" {
		t.Fatalf("x j .: A1 %q A2 %q", input(m, "A1"), input(m, "A2"))
	}
	// A count before . replaces the change's own.
	press(t, m, "l", "2x", "j", ".")
	if input(m, "B3") != "" || input(m, "C3") != "" {
		t.Fatalf("2x .: B3 %q C3 %q", input(m, "B3"), input(m, "C3"))
	}
	press(t, m, "j", "h", "3.")
	if input(m, "A4") != "" || input(m, "B4") != "" || input(m, "C4") != "" {
		t.Fatalf("3.: %q %q %q", input(m, "A4"), input(m, "B4"), input(m, "C4"))
	}
	// An edit repeats with its text and the key that accepted it.
	press(t, m, "gg", "0", "a", "!", "<enter>")
	if input(m, "A1") != "!" || m.cur != addr("A2") {
		t.Fatalf("a!: A1 %q at %v", input(m, "A1"), m.cur)
	}
	press(t, m, "j", "j", "j", ".")
	if input(m, "A5") != "A5!" || m.cur != addr("A6") {
		t.Fatalf("a . : A5 %q at %v", input(m, "A5"), m.cur)
	}
	// o repeats the row it opened and what was typed in it.
	press(t, m, "G", "o", "new", "<enter>")
	press(t, m, ".")
	if input(m, "A7") != "new" || input(m, "A8") != "" || input(m, "A9") != "new" {
		t.Fatalf("o .: A7 %q A9 %q", input(m, "A7"), input(m, "A9"))
	}
	// dd repeats with its count; moves and copies don't replace the change.
	press(t, m, "gg", "2dd", "yy", "j", ".")
	if input(m, "A1") != "A3" || input(m, "A2") != "A6" {
		t.Fatalf("2dd yy .: A1 %q A2 %q", input(m, "A1"), input(m, "A2"))
	}
	// Each change undoes on its own.
	press(t, m, "u")
	if input(m, "A3") != "A5!" {
		t.Errorf("u after .: A3 %q", input(m, "A3"))
	}
}

// Registers: "a keeps its own copy, "0 the last copy, "1 to "9 deleted
// rows and "- deleted cells, while the clipboard follows every copy.
func TestVimRegisters(t *testing.T) {
	m := vimModel(t)
	press(t, m, `"ayy`)
	if r := m.vim.regs['a']; r.clip == nil || !r.rows {
		t.Fatalf(`"ayy: %+v`, r)
	}
	press(t, m, "j", "yy", "j", "dd") // "0 row 2, "1 row 3; A3 is now A4
	press(t, m, "l", "x")             // "- is B3, which now holds B4
	for name, want := range map[rune]string{'a': "A1", '0': "A2", '1': "A3", '-': "B4"} {
		r := m.vim.regs[name]
		if r.clip == nil {
			t.Fatalf(`"%c is empty`, name)
		}
		if got := formatTSV(r.clip.TextIn(m.locale()))[:2]; got != want {
			t.Errorf(`"%c holds %q, want %q`, name, got, want)
		}
	}
	// "ap pastes row 1 as a new row below; the clipboard stays "-.
	press(t, m, "G", `"ap`)
	if input(m, "A6") != "A1" || input(m, "C6") != "C1" {
		t.Fatalf(`"ap: A6 %q C6 %q`, input(m, "A6"), input(m, "C6"))
	}
	press(t, m, "0", "p")
	if input(m, "A6") != "B4" {
		t.Errorf("p after \"ap pastes the clipboard: A6 %q", input(m, "A6"))
	}
	// A second delete of rows moves "1 to "2.
	press(t, m, "gg", "dd")
	if formatTSV(m.vim.regs['2'].clip.TextIn(m.locale()))[:2] != "A3" {
		t.Errorf(`"2 after dd: %q`, formatTSV(m.vim.regs['2'].clip.TextIn(m.locale())))
	}
	press(t, m, `"bp`)
	if !strings.Contains(line(m, contextLine), `Register "b is empty`) {
		t.Errorf(`"bp: %q`, line(m, contextLine))
	}
	press(t, m, `"A`)
	if !strings.Contains(line(m, contextLine), `No register "A`) || m.vim.pending() != "" {
		t.Errorf(`"A: %q, pending %q`, line(m, contextLine), m.vim.pending())
	}
	// The register shows while the sequence is typed.
	press(t, m, `"a2`)
	if l := line(m, contextLine); !strings.HasPrefix(l, `"a2`) {
		t.Errorf("pending register: %q", l)
	}
	press(t, m, "<esc>")
	// VISUAL y into a register, and p from one over a selection.
	press(t, m, "gg", "0", "v", "l", `"cy`)
	if r := m.vim.regs['c']; r.clip == nil || r.rows || r.clip.Src.String() != "A1:B1" {
		t.Fatalf(`v "cy: %+v`, r)
	}
	press(t, m, "G", "v", `"cp`)
	if at := m.cur.String(); input(m, at) != input(m, "A1") {
		t.Errorf(`v "cp: %s %q`, at, input(m, at))
	}
}

// "+p asks the terminal for the system clipboard and pastes its answer.
func TestVimSystemClipboard(t *testing.T) {
	m := vimModel(t)
	press(t, m, "G", "j", `"+p`)
	if !m.vim.clipPaste {
		t.Fatal(`"+p didn't ask for the clipboard`)
	}
	send(m, tea.ClipboardMsg{Content: "x\ty\n1\t2"})
	if input(m, "A7") != "x" || input(m, "B8") != "2" || m.vim.clipPaste {
		t.Fatalf(`"+p block: A7 %q B8 %q`, input(m, "A7"), input(m, "B8"))
	}
	press(t, m, "<esc>", "gg", `"+p`)
	send(m, tea.ClipboardMsg{Content: "one"})
	if input(m, "A1") != "one" || m.mode != modeReady {
		t.Fatalf(`"+p value: A1 %q mode %v`, input(m, "A1"), m.mode)
	}
	// An answer nobody asked for is ignored.
	send(m, tea.ClipboardMsg{Content: "two"})
	if input(m, "A1") != "one" {
		t.Errorf("unasked clipboard pasted: %q", input(m, "A1"))
	}
}

// cc and S clear the row and start typing; s clears the cell. Both keep
// what they cleared to paste.
func TestVimChange(t *testing.T) {
	m := vimModel(t)
	press(t, m, "j", "l", "cc")
	if m.mode != modeEnter || m.cur != addr("B2") || input(m, "A2") != "" || input(m, "C2") != "" {
		t.Fatalf("cc: mode %v at %v, A2 %q", m.mode, m.cur, input(m, "A2"))
	}
	press(t, m, "new", "<enter>")
	if input(m, "B2") != "new" || input(m, "A3") != "A3" || m.sheet.Len() != 16 {
		t.Fatalf("cc new: B2 %q, %d cells", input(m, "B2"), m.sheet.Len())
	}
	if r := m.vim.regs['1']; r.clip == nil || !r.rows {
		t.Errorf(`cc didn't keep the row in "1`)
	}
	press(t, m, "2s")
	if m.mode != modeEnter || input(m, "B3") != "" || input(m, "C3") != "" || input(m, "A3") != "A3" {
		t.Fatalf("2s: mode %v B3 %q C3 %q", m.mode, input(m, "B3"), input(m, "C3"))
	}
	press(t, m, "<esc>", "p")
	if input(m, "B3") != "B3" || input(m, "C3") != "C3" {
		t.Errorf("p after s: B3 %q C3 %q", input(m, "B3"), input(m, "C3"))
	}
	press(t, m, "gg", "2S", "top", "<enter>")
	if input(m, "B1") != "top" || input(m, "A2") != "" || input(m, "A3") != "A3" {
		t.Errorf("2S: B1 %q A2 %q", input(m, "B1"), input(m, "A2"))
	}
	press(t, m, "j", ".")
	if input(m, "B3") != "top" || input(m, "A3") != "" {
		t.Errorf("S .: B3 %q A3 %q", input(m, "B3"), input(m, "A3"))
	}
}

// Marks: m sets one, ` goes back to the cell and ' to its row, across
// sheets; '' goes back from a jump.
func TestVimMarks(t *testing.T) {
	m := vimModel(t)
	press(t, m, "j", "l", "ma")
	if !strings.Contains(line(m, contextLine), "Mark a at B2") {
		t.Errorf("ma: %q", line(m, contextLine))
	}
	press(t, m, "G", "$", "`a")
	if m.cur != addr("B2") {
		t.Fatalf("`a: at %v", m.cur)
	}
	press(t, m, "l", "'a")
	if m.cur != addr("A2") {
		t.Fatalf("'a: at %v", m.cur)
	}
	press(t, m, "``")
	if m.cur != addr("C2") {
		t.Fatalf("``: at %v", m.cur)
	}
	press(t, m, "G", "``")
	if m.cur != addr("C2") {
		t.Fatalf("G ``: at %v", m.cur)
	}
	press(t, m, ":B12", "<enter>", "''")
	if m.cur != addr("A2") {
		t.Fatalf(":B12 '' goes to the row: at %v", m.cur)
	}
	press(t, m, "'b")
	if !strings.Contains(line(m, contextLine), "Mark b isn't set") {
		t.Errorf("'b: %q", line(m, contextLine))
	}
	// A mark on another sheet shows that sheet.
	first := m.sheet
	m.runCommand("sheet.new")
	press(t, m, "`a")
	if m.sheet != first || m.cur != addr("B2") {
		t.Errorf("`a from another sheet: %s at %v", m.sheet.Name(), m.cur)
	}
	// In VISUAL mode a mark moves the selection's corner.
	press(t, m, "gg", "0", "v", "`a")
	if indicator(m) != "VISUAL" || m.selection().String() != "A1:B2" {
		t.Errorf("v `a: %q %v", indicator(m), m.selection())
	}
}

// Up and Down on the : line go through the lines run before, those
// starting with what's typed; the history outlasts the file.
func TestVimCommandHistory(t *testing.T) {
	m := vimModel(t)
	press(t, m, ":B5", "<enter>", ":fill down", "<enter>", ":B7", "<enter>")
	press(t, m, ":", "<up>")
	if m.line.Text() != "B7" {
		t.Fatalf("Up: %q", m.line.Text())
	}
	press(t, m, "<up>", "<up>")
	if m.line.Text() != "B5" {
		t.Fatalf("Up Up Up: %q", m.line.Text())
	}
	press(t, m, "<up>")
	if m.line.Text() != "B5" {
		t.Errorf("Up past the oldest: %q", m.line.Text())
	}
	press(t, m, "<down>", "<down>", "<down>")
	if m.line.Text() != "" {
		t.Errorf("Down back to what was typed: %q", m.line.Text())
	}
	press(t, m, "<esc>", ":B", "<up>")
	if m.line.Text() != "B7" {
		t.Errorf("B Up: %q", m.line.Text())
	}
	press(t, m, "<up>", "<enter>")
	if m.cur != addr("B5") {
		t.Errorf("recalled :B5 at %v", m.cur)
	}
	// Ctrl+N moves the completions.
	press(t, m, ":fill", "<ctrl+n>")
	if s := screen(m); !strings.Contains(line(m, m.height-1), "Copy the") || !strings.Contains(s, "Fill right") {
		t.Errorf("Ctrl+N: %q", line(m, m.height-1))
	}
	press(t, m, "<esc>")
	m.runCommand("file.new")
	press(t, m, ":", "<up>")
	if m.line.Text() != "B5" {
		t.Errorf("history after File > New: %q", m.line.Text())
	}
}

// :w! writes over a file changed on disk since it was opened, and over a
// file of the name given, without asking.
func TestVimWriteForced(t *testing.T) {
	dir := t.TempDir()
	m := vimModel(t)
	name := filepath.Join(dir, "book.012")
	press(t, m, ":w "+name, "<enter>")
	if m.filename != name {
		t.Fatalf(":w: %q", m.filename)
	}
	// Another program writes the file.
	later := time.Now().Add(time.Minute)
	if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(name, later, later)
	press(t, m, "x", ":w", "<enter>")
	if !strings.Contains(line(m, contextLine), "changed on disk") {
		t.Fatalf(":w after a change on disk: %q", line(m, contextLine))
	}
	press(t, m, "<esc>", ":w!", "<enter>")
	if data, _ := os.ReadFile(name); string(data) == "{}" || m.changed {
		t.Fatalf(":w! didn't write: changed %v", m.changed)
	}
	other := filepath.Join(dir, "other.012")
	os.WriteFile(other, []byte("{}"), 0o644)
	press(t, m, ":w "+other, "<enter>")
	if !strings.Contains(line(m, contextLine), "exists") {
		t.Fatalf(":w other: %q", line(m, contextLine))
	}
	press(t, m, "<esc>", ":w! "+other, "<enter>")
	if data, _ := os.ReadFile(other); string(data) == "{}" || m.filename != other {
		t.Errorf(":w! other: %q, filename %q", data, m.filename)
	}
	csv := filepath.Join(dir, "out.csv")
	os.WriteFile(csv, []byte("old"), 0o644)
	press(t, m, ":w! "+csv, "<enter>")
	if data, _ := os.ReadFile(csv); !strings.Contains(string(data), "B1") {
		t.Errorf(":w! out.csv: %q", data)
	}
}
