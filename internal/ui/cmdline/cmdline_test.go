package cmdline

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost knows a few commands and runs lines it recognises.
type fakeHost struct {
	th     theme.Theme
	line   lineedit.Line
	closed int
	ran    []string
	failed string
}

func newHost() *fakeHost { return &fakeHost{th: theme.New(true)} }

func (f *fakeHost) Theme() *theme.Theme       { return &f.th }
func (f *fakeHost) Size() (width, height int) { return 80, 24 }
func (f *fakeHost) Line() *lineedit.Line      { return &f.line }
func (f *fakeHost) Close()                    { f.closed++ }
func (f *fakeHost) Fail(msg string)           { f.failed = msg }

func (f *fakeHost) Commands() []Item {
	return []Item{
		{Word: "edit.fill_down", Title: "Fill down", Desc: "Copy the top cell down", Key: "Ctrl+D"},
		{Word: "format.bold", Title: "Bold", Desc: "Bold the selection", Key: "Ctrl+B"},
		{Word: "edit.undo", Title: "Undo", Desc: "Undo", Off: true},
	}
}

func (f *fakeHost) Run(text string) (tea.Cmd, bool) {
	if text == "edit.fill_down" || text == "w" || text == "B12" {
		f.ran = append(f.ran, text)
		return nil, true
	}
	return nil, false
}

func typeText(c *Line, s string) {
	for _, r := range s {
		c.Key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func words(c *Line) string {
	var out []string
	for _, it := range c.Shown() {
		out = append(out, it.Word)
	}
	return strings.Join(out, ",")
}

func TestCompletes(t *testing.T) {
	h := newHost()
	c := New(h)
	if n := len(c.Shown()); n != len(FileWords)+3 {
		t.Fatalf("empty line completes %d", n)
	}
	typeText(c, "fill")
	if got := words(c); got != "edit.fill_down" {
		t.Errorf("fill completes %s", got)
	}
	typeText(c, " d")
	if got := words(c); got != "edit.fill_down" {
		t.Errorf("fill d completes %s", got)
	}
	h.line.Clear()
	typeText(c, "w")
	if got := words(c); !strings.HasPrefix(got, "w,wq,") {
		t.Errorf("w completes %s", got)
	}
	typeText(c, " out.csv")
	if got := words(c); got != "" {
		t.Errorf("a file command's argument completes %s", got)
	}
}

func TestTabCompletesThenCycles(t *testing.T) {
	h := newHost()
	c := New(h)
	typeText(c, "o")
	c.Key(tea.KeyPressMsg{Code: tea.KeyTab})
	first := h.line.Text()
	c.Key(tea.KeyPressMsg{Code: tea.KeyTab})
	if first != c.Shown()[0].Word || h.line.Text() != c.Shown()[1].Word {
		t.Errorf("Tab put %q then %q", first, h.line.Text())
	}
}

func TestEnterRunsOrFallsBack(t *testing.T) {
	h := newHost()
	c := New(h)
	typeText(c, ":B12")
	c.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.closed != 1 || strings.Join(h.ran, ",") != "B12" {
		t.Errorf("closed %d, ran %v", h.closed, h.ran)
	}
	// Text that isn't a command runs the highlighted completion.
	c = New(h)
	typeText(c, "fill d")
	c.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.failed != "" || strings.Join(h.ran, ",") != "B12,edit.fill_down" {
		t.Errorf("ran %v, failed %q", h.ran, h.failed)
	}
	c = New(h)
	typeText(c, "nonsense")
	c.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.failed != "Not a command, cell or range: nonsense" {
		t.Errorf("failed %q", h.failed)
	}
}

func TestBackspaceOnEmptyCloses(t *testing.T) {
	h := newHost()
	c := New(h)
	c.Key(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if h.closed != 1 {
		t.Error("Backspace on an empty line didn't close it")
	}
}

func TestLayoutStatusAndMouse(t *testing.T) {
	h := newHost()
	c := New(h)
	typeText(c, "o")
	left, _ := c.ContextLine()
	if ansi.Strip(left) != ":o" {
		t.Errorf("context line %q", left)
	}
	if x, y := c.Cursor(); x != 2 || y != overlay.ContextLine {
		t.Errorf("cursor %d,%d", x, y)
	}
	b := c.Layout()[0]
	if b.ID != ID || b.Y != overlay.ContextLine+1 || !strings.Contains(ansi.Strip(strings.Join(b.Lines, "\n")), "Commands") {
		t.Fatalf("box %+v", b)
	}
	for i, it := range c.Shown() {
		if it.Word == "edit.undo" {
			c.Mouse(overlay.MouseEvent{Kind: overlay.MouseMotion, Box: ID, Row: 1 + i})
		}
	}
	if desc, _ := c.Status(); desc != "Undo (not available now)" {
		t.Errorf("status %q", desc)
	}
	c.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseLeft})
	if h.closed != 1 {
		t.Error("a click outside didn't close the line")
	}
}
