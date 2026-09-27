package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func init() {
	named["f4"] = tea.Key{Code: tea.KeyF4}
	named["end"] = tea.Key{Code: tea.KeyEnd}
}

func TestUndoRedoKeys(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	press(t, m, "1", "<enter>", "=A1*2", "<enter>", "<ctrl+s>", "undo", "<enter>")
	if m.changed {
		t.Fatal("changed after save")
	}
	press(t, m, "<up>", "<delete>")
	if !m.changed || input(m, "A2") != "" {
		t.Fatalf("clear: changed %v A2 %q", m.changed, input(m, "A2"))
	}
	press(t, m, "<ctrl+home>", "<ctrl+z>")
	if input(m, "A2") != "=A1*2" || m.cur != addr("A2") || m.changed {
		t.Errorf("undo: A2 %q cur %v changed %v", input(m, "A2"), m.cur, m.changed)
	}
	if got := line(m, 2); got != "Undid: clear A2" {
		t.Errorf("context line %q", got)
	}
	press(t, m, "<ctrl+z>", "<ctrl+z>")
	if m.sheet.Len() != 0 || line(m, 2) != "Undid: edit A1" || !m.changed {
		t.Errorf("undo all: %d cells, line %q, changed %v", m.sheet.Len(), line(m, 2), m.changed)
	}
	press(t, m, "<ctrl+z>")
	if line(m, 2) != "Nothing to undo" {
		t.Errorf("context line %q", line(m, 2))
	}
	press(t, m, "<ctrl+y>", "<ctrl+shift+z>")
	if m.sheet.Value(addr("A2")).Num != 2 || line(m, 2) != "Redid: edit A2" {
		t.Errorf("redo: A2 %+v line %q", m.sheet.Value(addr("A2")), line(m, 2))
	}
	// The note goes away with the next key.
	press(t, m, "<down>")
	if line(m, 2) != "" {
		t.Errorf("note stayed: %q", line(m, 2))
	}
}

func TestUndoColumnWidthPrompt(t *testing.T) {
	m := newModel()
	run(m, m.runCommand("column.width"))
	press(t, m, "<right>", "<right>", "<enter>")
	if m.sheet.ColWidth(0) != 12 {
		t.Fatalf("width %d", m.sheet.ColWidth(0))
	}
	press(t, m, "<ctrl+z>")
	if m.sheet.ColWidth(0) != 10 || m.sheet.CanUndo() {
		t.Errorf("width preview and result should be one step: width %d", m.sheet.ColWidth(0))
	}
	run(m, m.runCommand("column.width"))
	press(t, m, "<right>", "<esc>")
	if m.sheet.CanUndo() || m.changed {
		t.Errorf("cancelled width prompt left an undo step: %v %v %d %v", m.sheet.CanUndo(), m.changed, m.sheet.ColWidth(0), m.mode)
	}
}

// clipboardText runs the copy command and returns what it sends to the
// system clipboard.
func clipboardText(t *testing.T, m *Model, id string) string {
	t.Helper()
	cmd := m.runCommand(id)
	if cmd == nil {
		t.Fatal("no clipboard command")
	}
	return fmt.Sprint(cmd())
}

func TestCopyPaste(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<tab>", "=A1*2", "<enter>", "2", "<enter>", "<ctrl+home>", "<shift+right>")
	if got := clipboardText(t, m, "edit.copy"); got != "1\t2" {
		t.Errorf("system clipboard %q", got)
	}
	if l := line(m, 2); !m.copyMarked(addr("B1")) || !strings.HasPrefix(l, "Copied A1:B1") || !strings.Contains(l, "Ctrl+V") {
		t.Fatalf("marker %v, line %q", m.copyMarked(addr("B1")), line(m, 2))
	}
	press(t, m, "<down>", "<ctrl+v>")
	if input(m, "B2") != "=A2*2" || m.sheet.Value(addr("B2")).Num != 2 || m.selection().String() != "A2:B2" {
		t.Errorf("paste: B2 %q = %+v, selection %v", input(m, "B2"), m.sheet.Value(addr("B2")), m.selection())
	}
	if line(m, 2) != "Pasted 2 cells at A2:B2" || !m.copied.marked {
		t.Errorf("after paste: line %q marked %v", line(m, 2), m.copied.marked)
	}
	// Pasting into a larger multiple tiles.
	press(t, m, "<down>", "<shift+down>", "<shift+down>", "<shift+right>", "<ctrl+v>")
	if input(m, "B5") != "=A5*2" || m.selection().String() != "A3:B5" {
		t.Errorf("tiled paste: B5 %q selection %v", input(m, "B5"), m.selection())
	}
	press(t, m, "<esc>")
	if m.copied.marked || line(m, 2) != "" {
		t.Errorf("Esc kept the marker: line %q", line(m, 2))
	}
	// Ctrl+V still pastes after the marker is gone.
	press(t, m, "<right>", "<right>", "<ctrl+shift+v>")
	if input(m, "C3") != "1" || input(m, "D3") != "2" {
		t.Errorf("paste values: %q %q", input(m, "C3"), input(m, "D3"))
	}
}

func TestEditClearsCopyMarker(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>", "<up>", "<ctrl+c>", "<down>", "x", "<enter>")
	if m.copied.marked {
		t.Error("an edit kept the copy marker")
	}
	press(t, m, "<ctrl+v>")
	if input(m, "A3") != "1" {
		t.Errorf("paste after edit: %q", input(m, "A3"))
	}
}

func TestCutPasteMoves(t *testing.T) {
	m := newModel()
	press(t, m, "5", "<enter>", "=A1+1", "<enter>", "<ctrl+home>", "<ctrl+x>")
	if l := line(m, 2); !strings.HasPrefix(l, "Cut A1") || !strings.Contains(l, "move here") {
		t.Errorf("context line %q", line(m, 2))
	}
	press(t, m, "<right>", "<right>", "<ctrl+v>")
	if input(m, "A1") != "" || input(m, "C1") != "5" || input(m, "A2") != "=C1+1" {
		t.Errorf("move: A1 %q C1 %q A2 %q", input(m, "A1"), input(m, "C1"), input(m, "A2"))
	}
	if m.copied.clip != nil || line(m, 2) != "Moved A1 to C1" {
		t.Errorf("cut should paste once: line %q", line(m, 2))
	}
	press(t, m, "<ctrl+z>")
	if input(m, "A1") != "5" || input(m, "A2") != "=A1+1" {
		t.Errorf("undo move: A1 %q A2 %q", input(m, "A1"), input(m, "A2"))
	}
}

func TestCopyMarkerDrawn(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>")
	before := m.gridRow(0)
	press(t, m, "<up>", "<ctrl+c>", "<down>")
	if m.gridRow(0) == before || !strings.Contains(m.gridRow(0), "\x1b[") {
		t.Errorf("copy marker not drawn: %q", m.gridRow(0))
	}
}

func TestPasteTSVFromTerminal(t *testing.T) {
	m := newModel()
	press(t, m, "<right>")
	send(m, tea.PasteMsg{Content: "Qty\tPrice\r\n2\t3\r\n\t=B2*C2\r\n"})
	want := map[string]string{"B1": "Qty", "C1": "Price", "B2": "2", "C2": "3", "C3": "=B2*C2"}
	for a, in := range want {
		if input(m, a) != in {
			t.Errorf("%s = %q, want %q", a, input(m, a), in)
		}
	}
	if m.sheet.Value(addr("C3")).Num != 6 || m.selection().String() != "B1:C3" || line(m, 2) != "Pasted 6 cells at B1:C3" {
		t.Errorf("C3 %+v selection %v line %q", m.sheet.Value(addr("C3")), m.selection(), line(m, 2))
	}
	press(t, m, "<ctrl+z>")
	if m.sheet.Len() != 0 {
		t.Errorf("paste should undo in one step, %d cells left", m.sheet.Len())
	}
	// A single line still starts an entry.
	send(m, tea.PasteMsg{Content: "hello\n"})
	if m.mode != modeEnter || m.line.text() != "hello" {
		t.Errorf("mode %v buf %q", m.mode, m.line.text())
	}
}

func TestTSVRoundTrip(t *testing.T) {
	rows := [][]string{{"a", "tab\there", ""}, {`"quoted"`, "line\nbreak", `5" pipe`}, {"", "", ""}}
	text := formatTSV(rows)
	got := parseTSV(text)
	if fmt.Sprint(got) != fmt.Sprint(rows) {
		t.Errorf("round trip:\n%q\n%q", got, rows)
	}
	if got := parseTSV("a\t\tb\n\nc"); fmt.Sprint(got) != fmt.Sprint([][]string{{"a", "", "b"}, {""}, {"c"}}) {
		t.Errorf("blank fields and lines: %q", got)
	}
}

func TestFillKeys(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>", "2", "<enter>", "<ctrl+home>", "<right>", "=A1*10", "<enter>")
	press(t, m, "<up>", "<shift+down>", "<ctrl+d>")
	if input(m, "B2") != "=A2*10" || m.sheet.Value(addr("B2")).Num != 20 {
		t.Errorf("fill down: B2 %q", input(m, "B2"))
	}
	press(t, m, "<shift+right>", "<ctrl+r>")
	if input(m, "C2") != "=B2*10" {
		t.Errorf("fill right: C2 %q", input(m, "C2"))
	}
	// Ctrl+Enter fills the entry into the selection, keeping it selected.
	press(t, m, "<esc>", "<down>", "<down>", "<shift+down>", "<shift+right>", "=A1+$A$1", "<ctrl+enter>")
	if input(m, "C4") != "=B2+$A$1" || m.mode != modeReady || m.selection().String() != "B3:C4" {
		t.Errorf("ctrl+enter: C4 %q mode %v selection %v", input(m, "C4"), m.mode, m.selection())
	}
	press(t, m, "<ctrl+z>")
	if input(m, "B3") != "" || input(m, "C4") != "" {
		t.Error("ctrl+enter fill is not one undo step")
	}
}

func TestInsertDeleteRowsAndColumns(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>", "2", "<enter>", "=SUM(A1:A2)", "<enter>")
	press(t, m, "<up>", "<up>")
	m.runCommand("insert.row_above")
	if input(m, "A4") != "=SUM(A1:A3)" || input(m, "A2") != "" {
		t.Fatalf("insert above: A4 %q", input(m, "A4"))
	}
	m.runCommand("insert.row_below")
	if input(m, "A5") != "=SUM(A1:A4)" {
		t.Errorf("insert below: A5 %q", input(m, "A5"))
	}
	press(t, m, "<ctrl+home>")
	m.runCommand("delete.row")
	if input(m, "A4") != "=SUM(A1:A3)" || m.sheet.Value(addr("A4")).Num != 2 {
		t.Errorf("delete: A4 %q = %+v", input(m, "A4"), m.sheet.Value(addr("A4")))
	}
	m.runCommand("insert.col_left")
	if input(m, "B4") != "=SUM(B1:B3)" {
		t.Errorf("insert col: B4 %q", input(m, "B4"))
	}
	m.runCommand("insert.col_right")
	press(t, m, "<right>")
	m.runCommand("delete.col")
	if input(m, "B4") != "=SUM(B1:B3)" {
		t.Errorf("delete col: B4 %q", input(m, "B4"))
	}
	press(t, m, "<ctrl+z>")
	if line(m, 2) != "Undid: delete 1 column" || m.whole != wholeCols {
		t.Errorf("undo: line %q whole %v", line(m, 2), m.whole)
	}
	// Inserting can't push data off the sheet.
	press(t, m, "<esc>", "<ctrl+end>", "<ctrl+space>")
	m.runCommand("insert.row_above")
	if m.mode != modeError {
		t.Errorf("mode %v", m.mode)
	}
}

func TestF4CyclesReferences(t *testing.T) {
	m := newModel()
	press(t, m, "=A1+B2")
	for _, want := range []string{"=A1+$B$2", "=A1+B$2", "=A1+$B2", "=A1+B2"} {
		press(t, m, "<f4>")
		if m.line.text() != want {
			t.Errorf("F4 gave %q, want %q", m.line.text(), want)
		}
	}
	// F4 while pointing puts the reference in first.
	press(t, m, "*", "<up>", "<f4>")
	if m.line.text() != "=A1+B2*$A$1" || m.mode != modeEnter {
		t.Errorf("F4 in POINT: %q mode %v", m.line.text(), m.mode)
	}
}

func TestCycleRef(t *testing.T) {
	tests := []struct {
		in    string
		caret int
		want  string
		ok    bool
	}{
		{"=a1", 3, "=$A$1", true},
		{"=A1+1", 1, "=$A$1+1", true},
		{"=SUM(A1:B2)", 10, "=SUM($A$1:$B$2)", true},
		{"=SUM(A1:B2)", 7, "=SUM($A$1:$B$2)", true},
		{"=SUM($A$1..B2)", 12, "=SUM(A$1..B$2)", true},
		{`="A1"`, 3, "", false},
		{"=LOG10(2)", 6, "", false},
		{"=1+2", 2, "", false},
	}
	for _, tt := range tests {
		out, pos, ok := cycleRef([]rune(tt.in), tt.caret)
		if ok != tt.ok || string(out) != tt.want {
			t.Errorf("cycleRef(%q, %d) = %q, %v; want %q", tt.in, tt.caret, string(out), ok, tt.want)
		}
		if ok && pos > len(out) {
			t.Errorf("caret %d past end of %q", pos, string(out))
		}
	}
}

func TestInsertDeleteKeysFollowSelection(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<tab>", "2", "<enter>", "=A1+B1", "<enter>", "<ctrl+home>")
	press(t, m, "<ctrl+alt+=>")
	if input(m, "A3") != "=A2+B2" {
		t.Errorf("Ctrl+Alt+= on a cell inserts a row: A3 %q", input(m, "A3"))
	}
	press(t, m, "<ctrl+alt+->")
	press(t, m, "<ctrl+space>", "<ctrl+alt+=>")
	if input(m, "B2") != "=B1+C1" {
		t.Errorf("Ctrl+Alt+= on columns inserts a column: B2 %q", input(m, "B2"))
	}
	press(t, m, "<right>", "<right>", "<ctrl+space>", "<ctrl+alt+->")
	if input(m, "B2") != "=B1+#REF!" {
		t.Errorf("Ctrl+Alt+- on columns deletes them: B2 %q", input(m, "B2"))
	}
}
