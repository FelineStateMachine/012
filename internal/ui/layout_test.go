package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// rect parses a range for tests.
func rect(s string) sheet.Rect {
	r, ok := sheet.ParseRange(s)
	if !ok {
		panic(s)
	}
	return r
}

// gridLines returns the grid's screen lines, right-trimmed.
func gridLines(m *Model) []string {
	lines := strings.Split(screen(m), "\n")
	out := lines[gridTop : gridTop+m.visibleRows()]
	for i := range out {
		out[i] = strings.TrimRight(out[i], " ")
	}
	return out
}

func TestWrapGrowsTheRow(t *testing.T) {
	m := newModel()
	press(t, m, "one two three four five", "<enter>", "<up>")
	if got := gridLines(m)[0]; !strings.Contains(got, "one two three four five") {
		t.Fatalf("overflow: %q", got)
	}
	run(m, m.runCommand("format.wrap"))
	g := gridLines(m)
	want := []string{"one two", "three", "four", "five"}
	for i, w := range want {
		if !strings.Contains(g[i], w) {
			t.Errorf("line %d = %q, want %q", i, g[i], w)
		}
	}
	// The row number sits on the row's last line, beside its values.
	if strings.Contains(g[0], " 1 ") || !strings.HasPrefix(strings.TrimSpace(g[3]), "1 ") {
		t.Errorf("row number: %q", g[:4])
	}
	if !strings.HasPrefix(strings.TrimSpace(g[4]), "2") {
		t.Errorf("row 2 after: %q", g[4])
	}
	if !commands["format.wrap"].checked(m) || commands["format.wrap_clip"].checked(m) {
		t.Error("Wrap not checked")
	}
	// Clip keeps the text in its column.
	run(m, m.runCommand("format.wrap_clip"))
	if got := gridLines(m)[0]; !strings.Contains(got, "one two t") || strings.Contains(got, "three") {
		t.Errorf("clip: %q", got)
	}
	press(t, m, "<ctrl+z>")
	if gridLines(m)[1] == "" {
		t.Error("undo clip: not wrapped")
	}
}

func TestRowHeightPromptAndFit(t *testing.T) {
	m := newModel()
	press(t, m, "x", "<enter>", "<up>")
	run(m, m.runCommand("row.height"))
	if m.mode != modePrompt || m.prompt.indicator != "HEIGHT" {
		t.Fatalf("mode %v", m.mode)
	}
	press(t, m, "<right>", "<right>")
	if h, _ := m.sheet.RowHeight(0); h != 3 {
		t.Errorf("preview height %d", h)
	}
	press(t, m, "<enter>")
	if m.shape(0).Lines != 3 || !strings.Contains(gridLines(m)[2], "x") {
		t.Errorf("rows: %q", gridLines(m)[:4])
	}
	press(t, m, "<ctrl+z>")
	if _, ok := m.sheet.RowHeight(0); ok {
		t.Error("undo kept the height")
	}
	press(t, m, "<ctrl+y>")
	run(m, m.runCommand("row.fit"))
	if _, ok := m.sheet.RowHeight(0); ok || m.shape(0).Lines != 1 {
		t.Error("fit to data")
	}
	run(m, m.runCommand("row.height"))
	press(t, m, "99", "<enter>")
	if m.mode != modeError {
		t.Errorf("height 99 accepted: %v", m.mode)
	}
}

func TestResizeRowByDragging(t *testing.T) {
	m := newModel()
	handle := m.hdrW() - 1
	send(m, tea.MouseMotionMsg{X: handle, Y: gridTop + 1})
	if m.mouse.hover.kind != hitRowBorder || !strings.Contains(line(m, gridTop+1), "▄") {
		t.Fatalf("hover %v line %q", m.mouse.hover.kind, line(m, gridTop+1))
	}
	send(m, tea.MouseClickMsg(leftAt(handle, gridTop+1)))
	send(m, tea.MouseMotionMsg(leftAt(handle, gridTop+3)))
	send(m, tea.MouseReleaseMsg(leftAt(handle, gridTop+3)))
	if h, _ := m.sheet.RowHeight(1); h != 3 {
		t.Errorf("dragged height %d", h)
	}
	// Row 2's handle is now on its third line; a double click fits it.
	click(m, handle, gridTop+3, 0)
	click(m, handle, gridTop+3, 0)
	if _, ok := m.sheet.RowHeight(1); ok {
		t.Error("double-click didn't fit the row")
	}
}

func TestBordersDraw(t *testing.T) {
	m := newModel()
	press(t, m, "a", "<tab>", "b", "<enter>", "c", "<tab>", "d", "<enter>")
	m.cur = addr("A1")
	m.selectRect(rect("A1:B2"))
	run(m, m.runCommand("format.borders_all"))
	g := gridLines(m)
	for i, want := range []string{"┌─────────┬─────────┐", "│a        │b        │", "├─────────┼─────────┤", "│c        │d        │", "└─────────┴─────────┘"} {
		if !strings.Contains(g[i], want) {
			t.Errorf("line %d = %q, want %q", i, g[i], want)
		}
	}
	run(m, m.runCommand("format.border_double"))
	run(m, m.runCommand("format.borders_outer"))
	if g := gridLines(m); !strings.Contains(g[0], "╔═════════╤═════════╗") || !strings.Contains(g[2], "╟─────────┼─────────╢") {
		t.Errorf("double outer: %q", g[:3])
	}
	run(m, m.runCommand("format.borders_clear"))
	if g := gridLines(m); strings.ContainsAny(strings.Join(g, ""), "─│═║") {
		t.Errorf("cleared: %q", g[:3])
	}
	// Text doesn't run on across a border.
	m.clearSelection()
	m.cur = addr("D5")
	press(t, m, "a long label", "<enter>")
	m.cur = addr("E5")
	run(m, m.runCommand("format.border_left"))
	if got := gridLines(m)[4]; strings.Contains(got, "a long label") || !strings.Contains(got, "a long la║") {
		t.Errorf("overflow over a border: %q", got)
	}
}

func TestMergeCells(t *testing.T) {
	m := newModel()
	press(t, m, "Title", "<tab>", "lost", "<enter>")
	m.cur = addr("A3")
	press(t, m, "b", "<enter>", "a", "<enter>")
	m.cur = addr("A1")
	m.selectRect(rect("A1:C1"))
	run(m, m.runCommand("format.merge_all"))
	if _, ok := m.overlay.(*choiceBar); !ok || !strings.Contains(line(m, contextLine), "keeps only the top-left value") {
		t.Fatalf("no warning: %q", line(m, contextLine))
	}
	press(t, m, "<enter>")
	if mg, ok := m.sheet.MergeAt(addr("C1")); !ok || mg != rect("A1:C1") || !m.sheet.Cell(addr("B1")).Blank() {
		t.Fatalf("merge %v %v", mg, ok)
	}
	// Centered across the merge.
	if got := gridLines(m)[0]; !strings.Contains(got, strings.Repeat(" ", 12)+"Title") {
		t.Errorf("centered: %q", got)
	}
	// One cell for moving, selecting and clicking.
	if m.cur != addr("A1") || m.selection() != rect("A1:C1") || m.hasRange() {
		t.Errorf("cur %v selection %v", m.cur, m.selection())
	}
	press(t, m, "<right>")
	if m.cur != addr("D1") {
		t.Errorf("right from the merge: %v", m.cur)
	}
	press(t, m, "<left>")
	if m.cur != addr("A1") {
		t.Errorf("left into the merge: %v", m.cur)
	}
	click(m, cellX(2), gridTop, 0)
	if m.cur != addr("A1") {
		t.Errorf("click in the merge: %v", m.cur)
	}
	press(t, m, "New", "<enter>")
	if m.sheet.Value(addr("A1")).Str != "New" || m.cur != addr("A2") {
		t.Errorf("edit: %v at %v", m.sheet.Value(addr("A1")), m.cur)
	}
	m.cur = addr("B4")
	press(t, m, "<shift+up>", "<shift+up>", "<shift+up>")
	if m.selection() != rect("A1:C4") {
		t.Errorf("selection grows over the merge: %v", m.selection())
	}
	// Sorting over merged cells is refused.
	run(m, m.runCommand("data.sort_range_az"))
	if m.mode != modeError || !strings.Contains(m.errMsg, "merged") {
		t.Errorf("sort: %v %q", m.mode, m.errMsg)
	}
	press(t, m, "<esc>")
	m.cur = addr("B1")
	m.clearSelection()
	m.cur = m.snap(m.cur)
	run(m, m.runCommand("format.unmerge"))
	if len(m.sheet.Merges()) != 0 {
		t.Errorf("unmerge left %v", m.sheet.Merges())
	}
	press(t, m, "<ctrl+z>")
	if len(m.sheet.Merges()) != 1 {
		t.Error("undo unmerge")
	}
}

func TestMergeVertically(t *testing.T) {
	m := newModel()
	m.selectRect(rect("A1:B3"))
	run(m, m.runCommand("format.merge_vertical"))
	m.selectRect(rect("D2:E2"))
	run(m, m.runCommand("format.merge_all"))
	if len(m.sheet.Merges()) != 3 {
		t.Fatalf("merges %v", m.sheet.Merges())
	}
	m.cur = addr("A1")
	m.clearSelection()
	press(t, m, "<down>")
	if m.cur != addr("A4") {
		t.Errorf("down out of a vertical merge: %v", m.cur)
	}
	press(t, m, "<up>")
	if m.cur != addr("A1") {
		t.Errorf("up into a vertical merge: %v", m.cur)
	}
	// A spill can't write over merged cells, and cells an array spills
	// into can't be merged.
	m.cur = addr("D1")
	press(t, m, "=SEQUENCE(3)", "<enter>")
	if m.sheet.Value(addr("D1")).Kind != sheet.Error {
		t.Errorf("spill over merged cells: %v", m.sheet.Value(addr("D1")))
	}
	m.cur = addr("F1")
	press(t, m, "=SEQUENCE(3)", "<enter>")
	m.cur = addr("F2")
	m.selectRect(rect("F2:G2"))
	run(m, m.runCommand("format.merge_all"))
	if _, merged := m.sheet.MergeAt(addr("F2")); merged || !strings.Contains(m.note, "array") {
		t.Errorf("merged over a spill: %q", m.note)
	}
}

func TestTallRowsScroll(t *testing.T) {
	m := newModel()
	for r := range 30 {
		m.sheet.SetRowHeight(r, r, 3)
	}
	m.sheet.Set(addr("A30"), "end")
	// Going down keeps the active cell on screen, a whole row at a time.
	for range 29 {
		press(t, m, "<down>")
	}
	b, ok := m.bandOf(29)
	if !ok || b.shown != 3 {
		t.Fatalf("row 30 shown %v %+v", ok, b)
	}
	if m.scrollRows() != m.visibleRows()/3 {
		t.Errorf("a page is %d rows", m.scrollRows())
	}
	top := m.top
	press(t, m, "<pgup>")
	if m.top != top-m.scrollRows() {
		t.Errorf("page up from %d to %d", top, m.top)
	}
	// Frozen rows keep their height.
	m.sheet.SetFrozen(1, 0)
	if m.frozenLines(1) != 3 || m.screenRows()[1] != divider {
		t.Errorf("frozen lines %d rows %v", m.frozenLines(1), m.screenRows())
	}
	// A click on any line of a tall row picks it.
	m.top = 5
	y, _ := m.rowY(5)
	click(m, cellX(0), y+2, 0)
	if m.cur.Row != 5 {
		t.Errorf("clicked row %d", m.cur.Row+1)
	}
}

func TestRecordRowHeightDrag(t *testing.T) {
	m := newModel()
	run(m, m.runCommand("macro.record"))
	handle := m.hdrW() - 1
	send(m, tea.MouseClickMsg(leftAt(handle, gridTop)))
	send(m, tea.MouseMotionMsg(leftAt(handle, gridTop+1)))
	send(m, tea.MouseReleaseMsg(leftAt(handle, gridTop+1)))
	run(m, m.runCommand("macro.stop"))
	press(t, m, "<enter>", "<enter>")
	if got := body(mustMacro(t, m, "Macro 1").Source); got != `set_height("1", 2)` {
		t.Errorf("recorded %q", got)
	}
	r := withMacro(t, m, "Macro 1")
	run(r, r.runMacro(mustMacro(t, r, "Macro 1")))
	if h, _ := r.sheet.RowHeight(0); h != 2 {
		t.Errorf("replayed height %d (%q)", h, r.warn)
	}
}
