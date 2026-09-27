package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeMenuHost is a menu's host without a model: every command is
// available but "off", and what the menu asks for is recorded.
type fakeMenuHost struct {
	th     theme.Theme
	ran    []string
	closed int
	bar    int
	right  [2]int
}

func (f *fakeMenuHost) styles() *theme.Theme      { return &f.th }
func (f *fakeMenuHost) size() (width, height int) { return 80, 24 }
func (f *fakeMenuHost) available(id string) bool  { return id != "off" }
func (f *fakeMenuHost) shortcut(id string) string { return map[string]string{"a": "Ctrl+A"}[id] }
func (f *fakeMenuHost) isChecked(id string) bool  { return id == "b" }
func (f *fakeMenuHost) showBarMenu(i int)         { f.bar = i }
func (f *fakeMenuHost) rightClick(x, y int)       { f.right = [2]int{x, y} }
func (f *fakeMenuHost) closeOverlay()             { f.closed++ }

func (f *fakeMenuHost) runFromOverlay(id string) tea.Cmd {
	f.ran = append(f.ran, id)
	return nil
}

func fakeMenu(h menuHost) *menuOverlay {
	items := []menuItem{
		{title: "Alpha", cmd: "a"}, {title: "Beta", cmd: "b"}, sep,
		{title: "Off", cmd: "off"},
		{title: "More", items: []menuItem{{title: "Gamma", cmd: "c"}}},
	}
	return &menuOverlay{m: h, bar: -1, x: 10, y: 5, levels: []*menuLevel{newLevel(h, items, true)}}
}

func TestMenuAlone(t *testing.T) {
	h := &fakeMenuHost{th: theme.New(true), bar: -1}
	o := fakeMenu(h)
	b := o.Layout()[0]
	out := ansi.Strip(strings.Join(b.Lines, "\n"))
	for _, want := range []string{"Alpha", "Ctrl+A", "Beta", "✓", "Off", "›"} {
		if !strings.Contains(out, want) {
			t.Errorf("menu lacks %q:\n%s", want, out)
		}
	}
	if b.X != 10 || b.Y != 5 {
		t.Errorf("menu at %d,%d", b.X, b.Y)
	}
	// Down skips the separator and the unavailable item.
	o.Key(tea.KeyPressMsg{Code: tea.KeyDown})
	o.Key(tea.KeyPressMsg{Code: tea.KeyDown})
	if it := o.top().items[o.top().sel]; it.title != "More" {
		t.Fatalf("highlighted %q", it.title)
	}
	o.Key(tea.KeyPressMsg{Code: tea.KeyRight})
	o.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(o.levels) != 2 || strings.Join(h.ran, ",") != "c" {
		t.Errorf("levels %d, ran %v", len(o.levels), h.ran)
	}
	// A letter runs the only item starting with it.
	o.Key(tea.KeyPressMsg{Code: tea.KeyEscape})
	o.Key(tea.KeyPressMsg{Code: 'b', Text: "b"})
	if strings.Join(h.ran, ",") != "c,b" {
		t.Errorf("ran %v", h.ran)
	}
	// Right-clicking outside closes the menu and opens one there.
	o.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseRight, X: 50, Y: 20})
	if h.closed != 1 || h.right != [2]int{50, 20} {
		t.Errorf("closed %d, right-click at %v", h.closed, h.right)
	}
}
