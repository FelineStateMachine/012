package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// table fills n rows below a header "Item, Qty": Row 1..n and the row
// number mod 4.
func table(t *testing.T, m *Model, n int) {
	t.Helper()
	s := m.sheet
	s.Set(addr("A1"), "Item")
	s.Set(addr("B1"), "Qty")
	for i := range n {
		s.Set(sheet.Addr{Row: i + 1}, fmt.Sprintf("Row %d", i+1))
		s.Set(sheet.Addr{Col: 1, Row: i + 1}, fmt.Sprint((i+1)%4))
	}
	s.ClearHistory()
}

func TestFrozenPanesStayWhileScrolling(t *testing.T) {
	m := newModel() // 80x20: 15 grid lines
	table(t, m, 40)
	m.runCommand("view.freeze_rows1")
	m.runCommand("view.freeze_cols1")
	if m.note != "Froze 1 column" || !m.changed {
		t.Errorf("note %q changed %v", m.note, m.changed)
	}
	press(t, m, "<ctrl+end>")
	if m.cur != addr("B41") {
		t.Fatalf("cur %v", m.cur)
	}
	// Row 1 and column A stay; a divider line and column separate them.
	if l := line(m, gridTop); !strings.HasPrefix(l, "    1  Item     │ Qty") {
		t.Errorf("frozen row %q", l)
	}
	if l := line(m, gridTop+1); !strings.HasPrefix(l, "────────────────┼───") {
		t.Errorf("divider %q", l)
	}
	if l := line(m, gridTop+14); !strings.HasPrefix(l, "   41  Row 40   │        0") {
		t.Errorf("last row %q", l)
	}
	if m.top != 28 {
		t.Errorf("top %d, want 28 (13 scrolling rows ending at 41)", m.top)
	}
	// Moving up into the frozen row scrolls back to the top, as in Sheets.
	press(t, m, "<ctrl+up>")
	if m.cur != addr("B1") || m.top != 1 {
		t.Errorf("ctrl+up: cur %v top %d", m.cur, m.top)
	}
	press(t, m, "<down>")
	if m.cur != addr("B2") || m.top != 1 {
		t.Errorf("down from the frozen row: cur %v top %d", m.cur, m.top)
	}
	// Horizontal scrolling keeps column A.
	for range 10 {
		press(t, m, "<right>")
	}
	if l := line(m, headerLine); !strings.HasPrefix(l, "          A     │") || strings.Contains(l, " B ") {
		t.Errorf("header %q", l)
	}
	// Undo unfreezes.
	press(t, m, "<ctrl+z>")
	if _, c := m.sheet.Frozen(); c != 0 || m.note != "Undid: freeze 1 column" {
		t.Errorf("undo: %d frozen columns, note %q", c, m.note)
	}
}

func TestFrozenPanesShrinkToFit(t *testing.T) {
	m := newModel()
	m.cur = addr("A30")
	m.runCommand("view.freeze_rows_cur")
	if r, _ := m.frozen(); r != 13 || !strings.Contains(m.note, "13 fit in this window") {
		t.Errorf("frozen %d, note %q", r, m.note)
	}
	if got := len(m.screenRows()); got != m.visibleRows() {
		t.Errorf("%d screen rows", got)
	}
}

func TestMouseWithFrozenPanes(t *testing.T) {
	m := newModel()
	table(t, m, 40)
	m.sheet.SetFrozen(2, 1)
	m.top, m.left = 20, 3
	divX := rowHdrW + sheet.DefaultWidth // the column divider
	tests := []struct {
		x, y int
		want hit
	}{
		{rowHdrW + 1, gridTop, hit{kind: hitCell, addr: addr("A1")}},
		{rowHdrW + 1, gridTop + 1, hit{kind: hitCell, addr: addr("A2")}},
		{rowHdrW + 1, gridTop + 2, hit{}}, // the divider line
		{rowHdrW + 1, gridTop + 3, hit{kind: hitCell, addr: addr("A21")}},
		{divX, gridTop + 3, hit{}},
		{divX + 1, gridTop + 3, hit{kind: hitCell, addr: addr("D21")}},
		{divX + 1, gridTop, hit{kind: hitCell, addr: addr("D1")}},
		{2, gridTop + 3, hit{kind: hitRowHeader, addr: sheet.Addr{Col: 3, Row: 20}}},
		{divX + sheet.DefaultWidth, headerLine, hit{kind: hitColBorder, addr: sheet.Addr{Col: 3, Row: 20}}},
		{rowHdrW + sheet.DefaultWidth - 1, headerLine, hit{kind: hitColBorder, addr: sheet.Addr{Col: 0, Row: 20}}},
	}
	for _, tt := range tests {
		if got := m.hitTest(tt.x, tt.y); got != tt.want {
			t.Errorf("hitTest(%d, %d) = %+v, want %+v", tt.x, tt.y, got, tt.want)
		}
	}
	// Resizing a scrolling column measures from where it's drawn.
	send(m, tea.MouseClickMsg(leftAt(divX+sheet.DefaultWidth, headerLine)))
	send(m, tea.MouseMotionMsg(leftAt(divX+14, headerLine)))
	send(m, tea.MouseReleaseMsg(leftAt(divX+14, headerLine)))
	if w := m.sheet.ColWidth(3); w != 14 {
		t.Errorf("D width %d", w)
	}
	// The context menu opens under the clicked cell.
	x, y := m.cellPos(addr("D22"))
	if x != divX+1 || y != gridTop+4 {
		t.Errorf("cellPos D22 = %d, %d", x, y)
	}
}

func TestDragIntoFrozenRowsScrollsBack(t *testing.T) {
	m := newModel()
	table(t, m, 40)
	m.sheet.SetFrozen(1, 0)
	m.top = 6
	send(m, tea.MouseClickMsg(leftAt(cellX(1), gridTop+4)))
	if m.cur != addr("B9") {
		t.Fatalf("cur %v", m.cur)
	}
	// Dragging onto the frozen header row scrolls up instead of jumping
	// to row 1.
	send(m, tea.MouseMotionMsg(leftAt(cellX(1), gridTop)))
	if m.ext != addr("B7") || !m.mouse.autoscrolling {
		t.Fatalf("ext %v autoscrolling %v", m.ext, m.mouse.autoscrolling)
	}
	for i := 0; i < 10 && m.mouse.autoscrolling; i++ {
		send(m, autoscrollMsg{})
	}
	if m.top != 1 || m.ext.Row > 1 {
		t.Errorf("after autoscroll: top %d ext %v", m.top, m.ext)
	}
	send(m, tea.MouseReleaseMsg(leftAt(cellX(1), gridTop)))
	if r := m.selection(); r.From.Row > 1 || r.To != addr("B9") {
		t.Errorf("selection %v", r)
	}
}

func TestNavigationSkipsFilteredRows(t *testing.T) {
	m := newModel()
	table(t, m, 40)
	m.sheet.CreateFilter(sheet.Rect{To: addr("B41")})
	m.sheet.FilterColumn(1, sheet.Criteria{Hidden: []string{"1", "2", "3"}}) // rows 5, 9, ..., 41 show
	press(t, m, "<ctrl+home>", "<down>")
	if m.cur != addr("A5") {
		t.Errorf("down: %v", m.cur)
	}
	press(t, m, "<down>", "<up>", "<up>")
	if m.cur != addr("A1") {
		t.Errorf("down, up, up: %v", m.cur)
	}
	press(t, m, "<ctrl+down>")
	if m.cur != addr("A41") {
		t.Errorf("ctrl+down: %v", m.cur)
	}
	// Row numbers show the gaps; the header marks the filtered column.
	press(t, m, "<ctrl+home>")
	var nums []string
	for i := range 4 {
		nums = append(nums, strings.Fields(line(m, gridTop+i))[0])
	}
	if strings.Join(nums, " ") != "1 5 9 13" {
		t.Errorf("row numbers %v", nums)
	}
	if l := line(m, headerLine); !strings.Contains(l, "A ▾") || !strings.Contains(l, "B ▼") {
		t.Errorf("header %q", l)
	}
	if !strings.Contains(line(m, m.height-1), "Filter hides 30 rows") {
		t.Errorf("status %q", line(m, m.height-1))
	}
	// Page down moves a screenful (15) of visible rows: ten in the
	// filter, then five below it.
	press(t, m, "<pgdown>")
	if m.cur != addr("A46") {
		t.Errorf("pgdown: %v", m.cur)
	}
	// Clicking a row maps through the gaps.
	press(t, m, "<ctrl+home>")
	click(m, cellX(0), gridTop+2, 0)
	if m.cur != addr("A9") {
		t.Errorf("click: %v", m.cur)
	}
}

func TestFilteredNavigationIsFast(t *testing.T) {
	m := newModel()
	table(t, m, sheet.MaxRows-2)
	m.sheet.SetFrozen(1, 1)
	m.sheet.CreateFilter(sheet.Rect{To: sheet.Addr{Col: 1, Row: sheet.MaxRows - 1}})
	m.sheet.FilterColumn(1, sheet.Criteria{Hidden: []string{"1", "2", "3"}})
	m.View()
	start := time.Now()
	for _, k := range []string{"<down>", "<pgdown>", "<ctrl+down>", "<ctrl+up>", "<pgup>", "<up>"} {
		press(t, m, k)
		m.View()
	}
	if d := time.Since(start) / 6; d > 20*time.Millisecond {
		t.Errorf("a keystroke took %v", d)
	}
}
