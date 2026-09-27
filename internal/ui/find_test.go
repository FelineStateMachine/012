package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// findModel has a small sheet to search.
func findModel(t *testing.T) *Model {
	t.Helper()
	m := tallModel()
	press(t, m, "Rent", "<tab>", "1450", "<enter>")
	press(t, m, "rental car", "<tab>", "=B1*2", "<enter>")
	press(t, m, "Total rent", "<tab>", "=SUM(B1:B2)", "<enter>")
	press(t, m, "<ctrl+home>")
	return m
}

// col is the screen column where sub starts in line l.
func col(l, sub string) int {
	return ansi.StringWidth(l[:strings.Index(l, sub)])
}

func findBarOf(t *testing.T, m *Model) *findBar {
	t.Helper()
	f, ok := m.overlay.(*findBar)
	if !ok {
		t.Fatalf("no find bar (overlay %T)", m.overlay)
	}
	return f
}

func TestFindAsYouType(t *testing.T) {
	m := findModel(t)
	press(t, m, "<ctrl+f>")
	if m.indicator() != "FIND" && !strings.Contains(line(m, menuLine), "FIND") {
		t.Errorf("indicator %q", line(m, menuLine))
	}
	press(t, m, "ren")
	f := findBarOf(t, m)
	if len(f.matches) != 3 || m.cur != addr("A1") || !strings.Contains(line(m, contextLine), "1 of 3") {
		t.Fatalf("matches %v cur %v line %q", f.matches, m.cur, line(m, contextLine))
	}
	// The terminal cursor sits after the query.
	if x, y, ok := m.cursorPos(); !ok || y != contextLine || x != col(line(m, contextLine), "ren")+3 {
		t.Errorf("cursor %d,%d %v in %q", x, y, ok, line(m, contextLine))
	}
	press(t, m, "<enter>", "<enter>")
	if m.cur != addr("A3") || !strings.Contains(line(m, contextLine), "3 of 3") {
		t.Errorf("after two Enters: cur %v line %q", m.cur, line(m, contextLine))
	}
	press(t, m, "<enter>")
	if m.cur != addr("A1") {
		t.Errorf("did not wrap: cur %v", m.cur)
	}
	press(t, m, "<shift+enter>")
	if m.cur != addr("A3") {
		t.Errorf("shift+enter: cur %v", m.cur)
	}
	// Matches are highlighted in the grid while the bar is open.
	if !m.found(addr("A2")) || m.found(addr("B1")) {
		t.Error("wrong cells highlighted")
	}
	press(t, m, "<esc>")
	if m.overlay != nil || m.cur != addr("A3") || m.found(addr("A2")) {
		t.Errorf("Esc: overlay %T cur %v", m.overlay, m.cur)
	}
	// Ctrl+F again reopens the last search.
	press(t, m, "<ctrl+f>")
	if f := findBarOf(t, m); m.line.text() != "ren" || len(f.matches) != 3 {
		t.Errorf("reopened with %q", m.line.text())
	}
}

func TestFindOptions(t *testing.T) {
	m := findModel(t)
	press(t, m, "<ctrl+f>", "Rent")
	f := findBarOf(t, m)
	press(t, m, "<alt+c>")
	if len(f.matches) != 1 {
		t.Errorf("match case: %v", f.matches)
	}
	press(t, m, "<alt+c>", "<alt+w>")
	if len(f.matches) != 1 || f.matches[0].a != addr("A1") {
		t.Errorf("whole cell: %v", f.matches)
	}
	press(t, m, "<alt+w>")
	for range 4 {
		press(t, m, "<backspace>")
	}
	press(t, m, "B1", "<alt+=>")
	if len(f.matches) != 2 {
		t.Errorf("in formulas: %v", f.matches)
	}
	press(t, m, "<alt+=>")
	for range 2 {
		press(t, m, "<backspace>")
	}
	press(t, m, "<alt+r>", "^r.n")
	if len(f.matches) != 2 {
		t.Errorf("regex: %v", f.matches)
	}
	press(t, m, "(")
	if !strings.Contains(line(m, contextLine), "Invalid regular expression") {
		t.Errorf("bad regex line %q", line(m, contextLine))
	}
	press(t, m, "<backspace>")
	press(t, m, "nomatch")
	if !strings.Contains(line(m, contextLine), "No matches") {
		t.Errorf("no-match line %q", line(m, contextLine))
	}
}

func TestFindWithinSelection(t *testing.T) {
	m := findModel(t)
	press(t, m, "<shift+down>", "<ctrl+f>", "ren")
	f := findBarOf(t, m)
	if len(f.matches) != 2 || !strings.Contains(line(m, contextLine), "in A1:A2") {
		t.Fatalf("scoped: %v line %q", f.matches, line(m, contextLine))
	}
	press(t, m, "<alt+s>")
	if len(f.matches) != 3 {
		t.Errorf("unscoped: %v", f.matches)
	}
}

func TestReplace(t *testing.T) {
	m := findModel(t)
	press(t, m, "<ctrl+h>", "rent", "<tab>", "lease")
	f := findBarOf(t, m)
	if !f.replace || f.field != 1 || !strings.Contains(line(m, contextLine), "Replace › lease") {
		t.Fatalf("replace bar %q", line(m, contextLine))
	}
	press(t, m, "<enter>") // replace A1, move on
	if input(m, "A1") != "lease" || m.cur != addr("A2") {
		t.Errorf("replace one: A1 %q cur %v", input(m, "A1"), m.cur)
	}
	press(t, m, "<ctrl+enter>")
	if input(m, "A2") != "leaseal car" || input(m, "A3") != "Total lease" {
		t.Errorf("replace all: %q %q", input(m, "A2"), input(m, "A3"))
	}
	if !strings.Contains(line(m, contextLine), "No matches") {
		t.Errorf("after replace all %q", line(m, contextLine))
	}
	press(t, m, "<esc>", "<ctrl+z>")
	if input(m, "A2") != "rental car" || input(m, "A3") != "Total rent" || input(m, "A1") != "lease" {
		t.Errorf("undo of replace all: %q %q %q", input(m, "A1"), input(m, "A2"), input(m, "A3"))
	}
}

func TestFindMouse(t *testing.T) {
	m := findModel(t)
	press(t, m, "<ctrl+f>", "rent")
	f := findBarOf(t, m)
	l := line(m, contextLine)
	send(m, tea.MouseClickMsg{X: col(l, "Aa"), Y: contextLine, Button: tea.MouseLeft})
	if !f.opts.MatchCase || len(f.matches) != 2 {
		t.Errorf("clicking Aa: case %v matches %v", f.opts.MatchCase, f.matches)
	}
	// Clicking the grid closes the bar and moves there.
	click(m, cellX(1), gridTop+1, 0)
	if m.overlay != nil || m.cur != addr("B2") {
		t.Errorf("grid click: overlay %T cur %v", m.overlay, m.cur)
	}
}

func TestFindInMenusAndPalette(t *testing.T) {
	m := newModel()
	press(t, m, "<alt+e>")
	var got []string
	for _, it := range openMenu(t, m).top().items {
		got = append(got, it.label())
	}
	if s := strings.Join(got, ","); !strings.Contains(s, "Find,Find and replace") {
		t.Errorf("Edit menu %s", s)
	}
}
