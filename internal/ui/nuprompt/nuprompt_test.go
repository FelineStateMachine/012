package nuprompt

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

type fakeHost struct {
	th      theme.Theme
	line    lineedit.Line
	closed  bool
	ran     []string
	history []string
	running string
	stopped bool
}

func (h *fakeHost) Theme() *theme.Theme    { return &h.th }
func (h *fakeHost) Size() (int, int)       { return 80, 24 }
func (h *fakeHost) Line() *lineedit.Line   { return &h.line }
func (h *fakeHost) Close()                 { h.closed = true }
func (h *fakeHost) ShellHistory() []string { return h.history }
func (h *fakeHost) Running() string        { return h.running }
func (h *fakeHost) Stop()                  { h.stopped = true }
func (h *fakeHost) Said() string           { return "" }
func (h *fakeHost) Submit(line string) tea.Cmd {
	h.ran = append(h.ran, line)
	return nil
}
func (h *fakeHost) Words() []Word {
	return []Word{{Text: "$r1", Desc: "ls"}, {Text: "$big", Desc: "$r1 | first"}, {Text: "sort-by", Desc: "command"}, {Text: "str join", Desc: "command"}}
}

func typeText(p *Prompt, text string) {
	for _, r := range text {
		p.Key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func key(p *Prompt, code rune) { p.Key(tea.KeyPressMsg{Code: code}) }

func TestCompletes(t *testing.T) {
	h := &fakeHost{th: theme.New(true)}
	p := New(h, "")
	typeText(p, "$")
	if len(p.Shown()) != 2 {
		t.Fatalf("$ completes %+v", p.Shown())
	}
	typeText(p, "b")
	key(p, tea.KeyTab)
	if h.line.Text() != "$big" {
		t.Errorf("tab put %q", h.line.Text())
	}
	typeText(p, " | st")
	if got := p.Shown(); len(got) != 1 || got[0].Text != "str join" {
		t.Errorf("st completes %+v", got)
	}
	key(p, tea.KeyTab)
	if h.line.Text() != "$big | str join" {
		t.Errorf("tab put %q", h.line.Text())
	}
	// A word that isn't a command's place isn't completed.
	typeText(p, " so")
	if len(p.Shown()) != 0 {
		t.Errorf("an argument completes %+v", p.Shown())
	}
	if !strings.Contains(p.FormulaBar(), Mark) {
		t.Errorf("formula bar %q", p.FormulaBar())
	}
}

func TestRunsAndRecalls(t *testing.T) {
	h := &fakeHost{th: theme.New(true), history: []string{"ls", "$r1 | first"}}
	p := New(h, "")
	key(p, tea.KeyUp)
	if h.line.Text() != "$r1 | first" {
		t.Errorf("up %q", h.line.Text())
	}
	key(p, tea.KeyUp)
	key(p, tea.KeyEnter)
	if len(h.ran) != 1 || h.ran[0] != "ls" || h.line.Text() != "" {
		t.Errorf("ran %q, line %q", h.ran, h.line.Text())
	}
	// Esc stops what runs, then goes back to the grid.
	h.running = "r1"
	key(p, tea.KeyEscape)
	if !h.stopped || h.closed {
		t.Errorf("esc while running: stopped %v closed %v", h.stopped, h.closed)
	}
	h.running = ""
	key(p, tea.KeyEscape)
	if !h.closed {
		t.Error("esc didn't close")
	}
}
