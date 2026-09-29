package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// bigTable is an output of n rows of four columns, as nu prints it.
func bigTable(n int) string {
	var b strings.Builder
	b.WriteString("[[name, size, when, ok]; ")
	for i := range n {
		fmt.Fprintf(&b, "[file%d.txt, %dkb, 2026-09-%02dT10:00:00Z, true], ", i, i%900, i%28+1)
	}
	b.WriteString("[last, 1b, 2026-09-01T00:00:00Z, false]]")
	return b.String()
}

// outputModel is a notebook of one cell named files whose output is
// out, w by h, the output selected.
func outputModel(out string, w, h int) *Model {
	m := sized(sheet.New(), w, h)
	m.OpenNotebook()
	id := m.book().NewCellID()
	m.sheet.SetNotebookCells("cells", []notebook.Cell{{ID: id, Source: "files = ls"}})
	m.book().SetOutput(id, &notebook.Output{NUON: []byte(out), Count: 1})
	m.nbView().Select(0, true)
	return m
}

// entered is a notebook with lsOut's output, its grid entered.
func entered(t *testing.T) (*Model, *outGrid) {
	t.Helper()
	m := outputModel(lsOut, 80, 24)
	press(t, m, "<enter>")
	g := m.gridIn()
	if g == nil {
		t.Fatalf("Enter didn't enter the output:\n%s", screen(m))
	}
	return m, g
}

func TestOutputGridLooks(t *testing.T) {
	m := outputModel(lsOut, 80, 24)
	s := screen(m)
	for _, want := range []string{"Out[1]:            name      size", "1  a.txt       2.0 kB", "3  c.txt       5.0 kB"} {
		if !strings.Contains(s, want) {
			t.Errorf("lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "  A  ") || strings.Contains(s, " C ") {
		t.Errorf("the columns are lettered, or drawn past the table:\n%s", s)
	}
	rec := outputModel("{name: a.txt, size: 2kb}", 80, 24)
	if s := screen(rec); !strings.Contains(s, "field     value") || !strings.Contains(s, "2  size        2.0 kB") {
		t.Errorf("a record's grid:\n%s", s)
	}
	if strings.Contains(line(m, menuLine), "OUTPUT") {
		t.Error("selecting the output entered it")
	}
}

func TestOutputGridSelectsAndCopies(t *testing.T) {
	m, g := entered(t)
	if !strings.Contains(line(m, menuLine), "OUTPUT") {
		t.Errorf("mode: %q", line(m, menuLine))
	}
	press(t, m, "<down>", "<shift+right>", "<shift+down>")
	if got := g.child.selection(); got.String() != "A3:B4" {
		t.Errorf("selected %s", got)
	}
	if st := line(m, m.height-1); !strings.Contains(st, "name to size, rows 2 to 3") || !strings.Contains(st, "Count 4") {
		t.Errorf("status: %q", st)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if got := fmt.Sprint(cmd()); !strings.Contains(got, "b.txt\t10\nc.txt\t5000") {
		t.Errorf("the system clipboard: %q", got)
	}
	// Ctrl+V on a sheet pastes what was copied, formats and all.
	press(t, m, "<esc>", "<esc>")
	run(m, m.runCommand("sheet.new"))
	press(t, m, "<ctrl+v>")
	if got := m.sheet.ShownText(addr("B2")); got != "5.0 kB" {
		t.Errorf("pasted %q", got)
	}
}

func TestOutputGridSortsFiltersFinds(t *testing.T) {
	m, g := entered(t)
	press(t, m, "<right>")
	run(m, m.runCommand("data.sort_sheet_za"))
	if got := line(m, 7); !strings.Contains(got, "1  c.txt") {
		t.Errorf("sorted by size, Z to A:\n%s", screen(m))
	}
	if !strings.Contains(string(m.book().Output(m.sheet.NotebookCells()[0].ID).NUON), "a.txt, 2kb") {
		t.Error("sorting changed the output")
	}
	press(t, m, "<ctrl+z>")
	if got := line(m, 7); !strings.Contains(got, "1  a.txt") {
		t.Errorf("undo takes back the sort:\n%s", screen(m))
	}
	run(m, m.runCommand("data.filter"))
	var hide []string
	for _, v := range g.child.sheet.FilterValues(1) {
		if v.Label != "10 B" {
			hide = append(hide, v.Text)
		}
	}
	g.child.sheet.FilterColumn(1, sheet.Criteria{Hidden: hide})
	if s := screen(m); !strings.Contains(s, "2  b.txt") || strings.Contains(s, "a.txt") {
		t.Errorf("filtered to 10 B:\n%s", s)
	}
	run(m, m.runCommand("data.filter_remove"))
	press(t, m, "<ctrl+f>", "c.t")
	if !strings.Contains(line(m, menuLine), "FIND") || g.child.cur != addr("A4") {
		t.Errorf("find: %s at %s\n%s", line(m, menuLine), g.child.cur, screen(m))
	}
}

func TestOutputGridRefusesEdits(t *testing.T) {
	m, g := entered(t)
	press(t, m, "x")
	if !strings.Contains(line(m, contextLine), "can't be changed") || g.child.mode != modeReady {
		t.Errorf("typing: %q", line(m, contextLine))
	}
	press(t, m, "<delete>")
	run(m, m.runCommand("format.bold"))
	if got := g.child.sheet.ShownText(addr("A2")); got != "a.txt" || g.child.sheet.CellStyle(addr("A2")).Bold {
		t.Errorf("the output changed: %q", got)
	}
	if commands["clear"].available(m) || !commands["data.sort_sheet_az"].available(m) {
		t.Error("the menus offer what the grid refuses")
	}
}

func TestOutputGridFullScreenAndBack(t *testing.T) {
	m, g := entered(t)
	press(t, m, "<enter>")
	if !m.nbView().FullOpen() || !strings.HasPrefix(strings.TrimSpace(line(m, nbBody)), "name") {
		t.Fatalf("Enter didn't show the grid full-screen:\n%s", screen(m))
	}
	press(t, m, "<esc>")
	if m.nbView().FullOpen() || m.gridIn() != g {
		t.Error("Esc from full-screen left the grid")
	}
	press(t, m, "<esc>")
	if m.gridIn() != nil || !strings.Contains(line(m, menuLine), "NOTEBOOK") {
		t.Error("Esc didn't leave the grid")
	}
}

func TestOutputGridMouse(t *testing.T) {
	m := outputModel(lsOut, 80, 24)
	y := strings.Count(screen(m)[:strings.Index(screen(m), "b.txt")], "\n")
	x := screenX(t, m, y, "b.txt")
	send(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	send(m, tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	g := m.gridIn()
	if g == nil || g.child.cur != addr("A3") {
		t.Fatalf("a click didn't enter the grid at b.txt:\n%s", screen(m))
	}
	// Drag the border right of name to widen it.
	w := g.child.sheet.ColWidth(0)
	bx := x + w - 2
	send(m, tea.MouseClickMsg{X: bx, Y: y - 2, Button: tea.MouseLeft})
	send(m, tea.MouseMotionMsg{X: bx + 4, Y: y - 2, Button: tea.MouseLeft})
	send(m, tea.MouseReleaseMsg{X: bx + 4, Y: y - 2, Button: tea.MouseLeft})
	if got := g.child.sheet.ColWidth(0); got != w+4 {
		t.Errorf("the column is %d wide, was %d", got, w)
	}
}

func TestOutputGridChartsOnASheet(t *testing.T) {
	m, _ := entered(t)
	run(m, m.runCommand("insert.chart"))
	if m.sheet.IsNotebook() || m.sheet.Name() != "files" || !strings.Contains(line(m, menuLine), "CHART") {
		t.Fatalf("the chart isn't on the output's sheet:\n%s", screen(m))
	}
	if _, r, ok := m.book().Region("files"); !ok || !r.Output {
		t.Error("the output wasn't sent to the sheet")
	}
}

// A long output scrolls in its window and jumps to its end.
func TestOutputGridLong(t *testing.T) {
	if testing.Short() {
		t.Skip("reads 100,000 rows")
	}
	m := outputModel(bigTable(100000), 120, 40)
	press(t, m, "<enter>", "<ctrl+down>")
	if s := screen(m); !strings.Contains(s, "100001  last") || !strings.Contains(s, "rows 99,992 to 100,001 of 100,001") {
		t.Errorf("the end:\n%s", s)
	}
}
