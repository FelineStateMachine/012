package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// handleX is the screen x of the fill handle in column c (scrolled to A).
func handleX(c int) int { return rowHdrW + (c+1)*sheet.DefaultWidth - 1 }

func TestFillHandleDrag(t *testing.T) {
	tests := []struct {
		name  string
		cells []string // A1 down
		to    sheet.Addr
		want  map[string]string
		sel   string
	}{
		{"down continues a series", []string{"1", "2"}, addr("A5"), map[string]string{"A3": "3", "A5": "5"}, "A1:A5"},
		{"right, further out than down", []string{"Mon"}, addr("C2"), map[string]string{"B1": "Tue", "C1": "Wed", "A2": ""}, "A1:C1"},
		{"back inside fills nothing", []string{"x"}, addr("A1"), map[string]string{"A2": ""}, "A1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := fillModel(tt.cells)
			y := gridTop + len(tt.cells) - 1
			hoverFillHandle(t, m, y)
			send(m, tea.MouseClickMsg(leftAt(handleX(0), y)))
			to := leftAt(cellX(tt.to.Col), gridTop+tt.to.Row)
			send(m, tea.MouseMotionMsg(to))
			if tt.sel != "A1" && !strings.Contains(line(m, contextLine), "Fill "+tt.sel) {
				t.Errorf("context line %q", line(m, contextLine))
			}
			send(m, tea.MouseReleaseMsg(to))
			for a, in := range tt.want {
				if got := input(m, a); got != in {
					t.Errorf("%s = %q, want %q", a, got, in)
				}
			}
			if got := m.selection().String(); got != tt.sel {
				t.Errorf("selection %s, want %s", got, tt.sel)
			}
			if tt.sel != "A1" {
				press(t, m, "<ctrl+z>")
				if m.sheet.CanUndo() || m.sheet.Len() != len(tt.cells) {
					t.Error("fill isn't one undo step")
				}
			}
		})
	}
}

// fillModel has cells entered from A1 down and selected.
func fillModel(cells []string) *Model {
	m := newModel()
	for i, in := range cells {
		m.sheet.Set(sheet.Addr{Row: i}, in)
	}
	m.sheet.ClearHistory()
	m.cur, m.ext = addr("A1"), sheet.Addr{Row: len(cells) - 1}
	m.selecting = len(cells) > 1
	return m
}

// hoverFillHandle hovers the corner cell on line y and checks that the
// fill handle shows there.
func hoverFillHandle(t *testing.T, m *Model, y int) {
	t.Helper()
	send(m, tea.MouseMotionMsg{X: cellX(0), Y: y})
	if !strings.Contains(line(m, y), "▟") {
		t.Fatalf("no handle: %q", line(m, y))
	}
	if h := m.hitTest(handleX(0), y); h.kind != hitFillHandle {
		t.Fatalf("hit %+v", h)
	}
}

func TestFillHandleEscCancels(t *testing.T) {
	m := newModel()
	press(t, m, "1", "<enter>", "<up>")
	send(m, tea.MouseClickMsg(leftAt(handleX(0), gridTop)))
	send(m, tea.MouseMotionMsg(leftAt(cellX(0), gridTop+4)))
	press(t, m, "<esc>")
	send(m, tea.MouseReleaseMsg(leftAt(cellX(0), gridTop+4)))
	if input(m, "A2") != "" || m.mouse.drag != dragNone {
		t.Errorf("A2 %q drag %v", input(m, "A2"), m.mouse.drag)
	}
}

func TestFillDownKeyContinuesSeries(t *testing.T) {
	m := newModel()
	press(t, m, "Item 1", "<enter>", "Item 2", "<enter>", "<up>", "<up>")
	press(t, m, "<shift+down>", "<shift+down>", "<shift+down>", "<ctrl+d>")
	if input(m, "A4") != "Item 4" {
		t.Errorf("A4 = %q", input(m, "A4"))
	}
}

func TestSortBar(t *testing.T) {
	m := newModel()
	table(t, m, 6) // Qty: 1 2 3 0 1 2
	press(t, m, "<ctrl+home>", "<right>")
	m.runCommand("data.sort_range")
	if m.mode != modeMenu || line(m, menuLine)[len(line(m, menuLine))-4:] != "SORT" {
		t.Fatalf("mode %v, %q", m.mode, line(m, menuLine))
	}
	// The header row is found, and the range excludes it.
	if l := line(m, contextLine); !strings.HasPrefix(l, "Sort A2:B7 by  B Qty  A→Z") || !strings.Contains(l, "Header row") {
		t.Errorf("bar %q", l)
	}
	press(t, m, "<space>", "<alt+a>")
	if l := line(m, contextLine); !strings.Contains(l, "B Qty  Z→A  then  A Item  A→Z") {
		t.Errorf("bar %q", l)
	}
	press(t, m, "<left>") // stays in the range
	press(t, m, "<enter>")
	want := []string{"3", "2", "2", "1", "1", "0"}
	wantA := []string{"Row 3", "Row 2", "Row 6", "Row 1", "Row 5", "Row 4"}
	for i := range want {
		a, b := sheet.Addr{Row: i + 1}, sheet.Addr{Col: 1, Row: i + 1}
		if input(m, b.String()) != want[i] || input(m, a.String()) != wantA[i] {
			t.Errorf("row %d: %q %q", i+2, input(m, a.String()), input(m, b.String()))
		}
	}
	if m.note != "Sorted A2:B7 by B Z→A, then A A→Z" || m.mode != modeReady {
		t.Errorf("note %q mode %v", m.note, m.mode)
	}
	// Esc cancels without sorting; Alt+H drops the header row.
	m.runCommand("data.sort_range")
	press(t, m, "<alt+h>")
	if !strings.HasPrefix(line(m, contextLine), "Sort A1:B7 by  B ") {
		t.Errorf("without header %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if m.mode != modeReady || input(m, "A1") != "Item" {
		t.Error("Esc didn't cancel")
	}
}

func TestSortSheetKeepsFrozenRows(t *testing.T) {
	m := newModel()
	table(t, m, 4) // Qty: 1 2 3 0
	m.sheet.SetFrozen(1, 0)
	press(t, m, "<right>", "<down>")
	m.runCommand("data.sort_sheet_za")
	if input(m, "B1") != "Qty" || input(m, "B2") != "3" || input(m, "B5") != "0" {
		t.Errorf("B = %q %q %q", input(m, "B1"), input(m, "B2"), input(m, "B5"))
	}
}

func TestFilterPicker(t *testing.T) {
	m := newModel()
	table(t, m, 8)         // Qty: 1 2 3 0 1 2 3 0
	press(t, m, "<right>") // B1, inside the data
	if commands["data.filter_column"].available(m) {
		t.Error("filter by column available without a filter")
	}
	m.runCommand("data.filter")
	if r, ok := m.sheet.FilterRange(); !ok || r.String() != "A1:B9" {
		t.Fatalf("filter %v %v", r, ok)
	}
	press(t, m, "<alt+down>")
	p, ok := m.overlay.(*filterPicker)
	if !ok {
		t.Fatalf("overlay %T", m.overlay)
	}
	out := screen(m)
	for _, want := range []string{"Filter B  Qty", "If ‹ None ›", "[x] Select all", "[x] 0", "4 of 4"} {
		if !strings.Contains(out, want) {
			t.Errorf("picker lacks %q:\n%s", want, out)
		}
	}
	// Uncheck 0 and 1; the search narrows the list.
	press(t, m, "<down>", "<space>", "<down>", "<space>", "3")
	if len(p.shown) != 1 || !strings.Contains(screen(m), "1 of 4") {
		t.Errorf("search shows %d", len(p.shown))
	}
	press(t, m, "<enter>")
	if m.sheet.HiddenRows() != 4 || m.note != "Filtered column B: 4 rows hidden" {
		t.Errorf("hidden %d note %q", m.sheet.HiddenRows(), m.note)
	}
	// A condition on another column combines with it.
	press(t, m, "<left>", "<alt+down>", "<tab>", "<down>", "<down>", "<down>")
	if !strings.Contains(screen(m), "‹ Text contains ›") {
		t.Fatalf("condition:\n%s", screen(m))
	}
	press(t, m, "Row 2", "<enter>")
	var shown []int
	for r := range 10 {
		if !m.sheet.RowHidden(r) {
			shown = append(shown, r+1)
		}
	}
	if len(shown) != 3 || shown[1] != 3 || shown[2] != 10 {
		t.Errorf("shown rows %v", shown)
	}
	// Esc leaves the criteria alone; removing the filter shows all.
	press(t, m, "<alt+down>", "<space>", "<esc>")
	if m.sheet.HiddenRows() != 7 || m.mode != modeReady {
		t.Errorf("after Esc: %d hidden", m.sheet.HiddenRows())
	}
	m.runCommand("data.filter_remove")
	if m.sheet.HiddenRows() != 0 {
		t.Error("remove kept rows hidden")
	}
}

func TestFilterButtonClick(t *testing.T) {
	m := newModel()
	table(t, m, 4)
	m.runCommand("data.filter")
	x := m.filterButtonX(1)
	if ansi.Cut(line(m, headerLine), x, x+1) != "▾" {
		t.Fatalf("button at %d in %q", x, line(m, headerLine))
	}
	click(m, x, headerLine, 0)
	if p, ok := m.overlay.(*filterPicker); !ok || !strings.HasPrefix(p.title, "Filter B") {
		t.Fatalf("overlay %T", m.overlay)
	}
	// Clicking a value toggles it; clicking outside cancels.
	click(m, x+4, gridTop+filterFirstRow+1, 0)
	if p := m.overlay.(*filterPicker); p.checked["0"] {
		t.Error("click didn't uncheck 0")
	}
	click(m, 70, 18, 0)
	if m.overlay != nil || m.sheet.HiddenRows() != 0 {
		t.Error("click outside applied or stayed open")
	}
}
