package shortcuts

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a screen with forty rows of shortcuts in four groups.
type fakeHost struct {
	th     theme.Theme
	w, h   int
	closed int
}

func (f *fakeHost) Theme() *theme.Theme { return &f.th }
func (f *fakeHost) Size() (int, int)    { return f.w, f.h }
func (f *fakeHost) Close()              { f.closed++ }
func (f *fakeHost) Shortcuts() []Row {
	var rows []Row
	for g := range 4 {
		rows = append(rows, Row{Heading: "Group " + strconv.Itoa(g)})
		for i := range 9 {
			rows = append(rows, Row{Keys: []string{"Ctrl+" + strconv.Itoa(i)}, Action: "Action " + strconv.Itoa(g*10+i) + " does something at some length"})
		}
	}
	return rows
}

func text(v *View) string {
	return ansi.Strip(strings.Join(v.Layout()[0].Lines, "\n"))
}

func TestScrollsAndCloses(t *testing.T) {
	h := &fakeHost{th: theme.New(true), w: 80, h: 24}
	v := New(h)
	out := text(v)
	for _, want := range []string{"Keyboard shortcuts", "Group 0", "Action 0", "Ctrl+0", "1-19 of 44"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
	v.Key(tea.KeyPressMsg{Code: tea.KeyDown})
	v.Key(tea.KeyPressMsg{Code: tea.KeyDown})
	if v.Top() != 2 {
		t.Errorf("down twice scrolled to %d", v.Top())
	}
	v.Key(tea.KeyPressMsg{Code: tea.KeyEnd})
	if v.Top() != 44-19 {
		t.Errorf("End scrolled to %d", v.Top())
	}
	v.Mouse(overlay.MouseEvent{Kind: overlay.MouseWheel, Button: tea.MouseWheelUp})
	if v.Top() != 44-19-3 {
		t.Errorf("the wheel scrolled to %d", v.Top())
	}
	v.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Box: ID})
	if h.closed != 0 {
		t.Error("a click in the box closed it")
	}
	v.Mouse(overlay.MouseEvent{Kind: overlay.MousePress})
	v.Key(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.closed != 2 {
		t.Errorf("a click outside and Esc closed it %d times", h.closed)
	}
}

func TestTwoColumnsWhenWide(t *testing.T) {
	h := &fakeHost{th: theme.New(true), w: 200, h: 50}
	lines, w := New(h).Lines()
	if len(lines) >= 40 || w < 40 {
		t.Fatalf("%d lines %d wide", len(lines), w)
	}
	if first := ansi.Strip(lines[0]); !strings.Contains(first, "Group 0") || !strings.Contains(first, "Group 2") {
		t.Errorf("the columns split elsewhere: %q", first)
	}
}
