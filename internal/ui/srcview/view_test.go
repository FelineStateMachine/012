package srcview

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a source of n rows, row i holding i and "r<i>", whose
// rows from lost on are on their way.
type fakeHost struct {
	th         theme.Theme
	n, lost    int64
	from, to   int64 // what was last wanted
	counted    bool
	wantCalled int
}

func newHost(n int64) *fakeHost {
	return &fakeHost{th: theme.New(true), n: n, lost: n, counted: true}
}

func (h *fakeHost) Theme() *theme.Theme    { return &h.th }
func (h *fakeHost) Locale() *locale.Locale { return locale.Canonical }
func (h *fakeHost) Columns() []Column      { return []Column{{Name: "id"}, {Name: "name"}} }
func (h *fakeHost) Rows() (int64, bool)    { return h.n, h.counted }

func (h *fakeHost) Row(i int64) (int64, []sheet.LiveCell, bool) {
	if i >= h.lost || i >= h.n {
		return 0, nil, false
	}
	return i, []sheet.LiveCell{{V: sheet.Value{Kind: sheet.Number, Num: float64(i)}},
		{V: sheet.Value{Kind: sheet.Text, Str: "r" + strconv.FormatInt(i, 10)}}}, true
}

func (h *fakeHost) Want(from, to int64) { h.from, h.to, h.wantCalled = from, to, h.wantCalled+1 }

func view(n int64, width, height int) (*View, *fakeHost) {
	h := newHost(n)
	v := New(h)
	v.Resize(width, height)
	return v, h
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "ctrl+down":
		return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
	case "ctrl+home":
		return tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func TestDrawNumbersRowsAsTheTab(t *testing.T) {
	v, h := view(10_000_000, 60, 13)
	lines := v.Draw()
	if len(lines) != 13 {
		t.Fatalf("%d lines", len(lines))
	}
	if l := ansi.Strip(lines[namesLine]); !strings.Contains(l, "1  id") && !strings.Contains(l, "id") {
		t.Errorf("header row %q", l)
	}
	if l := ansi.Strip(lines[firstRow]); !strings.HasPrefix(strings.TrimSpace(l), "2 ") || !strings.Contains(l, "r0") {
		t.Errorf("first row %q", l)
	}
	if h.from > 0 || h.to < 10 {
		t.Errorf("wanted %d to %d", h.from, h.to)
	}
	// The last row is the tab's row 10,000,001, and the thumb is at the
	// bottom of the track.
	v.Key(key("ctrl+down"))
	lines = v.Draw()
	last := ansi.Strip(lines[len(lines)-1])
	if !strings.HasPrefix(strings.TrimSpace(last), "10000001 ") || !strings.HasSuffix(last, "┃") {
		t.Errorf("last row %q", last)
	}
	if first := ansi.Strip(lines[firstRow]); !strings.HasSuffix(first, "│") {
		t.Errorf("the track at the top %q", first)
	}
}

func TestRowsOnTheirWay(t *testing.T) {
	v, h := view(100, 60, 10)
	h.lost = 3
	lines := v.Draw()
	if l := ansi.Strip(lines[firstRow+3]); !strings.Contains(l, "…") {
		t.Errorf("a row on its way %q", l)
	}
	if _, _, ok := v.Cell(); !ok {
		t.Error("the first row is here")
	}
	h.counted = false
	v.Refit()
	v.Draw()
	if v.fitted {
		t.Error("fitted before the rows were counted")
	}
}

func TestKeysAndScroll(t *testing.T) {
	v, _ := view(1000, 60, 13)
	v.Draw()
	v.Key(key("down"))
	if row, _ := v.Active(); row != 1 {
		t.Errorf("down: row %d", row)
	}
	v.Key(key("pgdown"))
	if row, _ := v.Active(); row != int64(v.Lines()) || v.Top() != int64(v.Lines()-1) {
		t.Errorf("page down: row %d top %d", row, v.Top())
	}
	v.Key(key("end"))
	if _, col := v.Active(); col != 1 {
		t.Errorf("end: column %d", col)
	}
	v.Key(key("ctrl+home"))
	if row, col := v.Active(); row != 0 || col != 0 || v.Top() != 0 {
		t.Errorf("ctrl+home: %d %d top %d", row, col, v.Top())
	}
	if v.Key(key("x")) {
		t.Error("a letter is the view's")
	}
	v.Scroll(500)
	if row, _ := v.Active(); row < v.Top() || row >= v.Top()+int64(v.Lines()) {
		t.Errorf("the active row %d left the window from %d", row, v.Top())
	}
}

func TestMouse(t *testing.T) {
	v, _ := view(1000, 60, 13)
	v.Draw()
	if hit := v.HitAt(59, firstRow+2); !hit.Bar {
		t.Errorf("the scrollbar isn't hit: %+v", hit)
	}
	v.DragBar(firstRow + v.Lines() - 1)
	if last := int64(1000 - v.Lines()); v.Top() != last {
		t.Errorf("the thumb dragged to the bottom shows from %d, want %d", v.Top(), last)
	}
	x := v.ColX(1)
	if hit := v.HitAt(x, firstRow); !hit.Cell || hit.Col != 1 || hit.Row != v.Top() {
		t.Errorf("a click on column B: %+v", hit)
	}
	if hit := v.HitAt(x, namesLine); hit.Cell || hit.Bar {
		t.Errorf("the header row is hit: %+v", hit)
	}
}
