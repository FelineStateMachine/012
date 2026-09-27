package choicebar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost records what the bar did.
type fakeHost struct {
	th      theme.Theme
	closed  int
	answers []string
}

func (f *fakeHost) Theme() *theme.Theme { return &f.th }
func (f *fakeHost) Close()              { f.closed++ }

func (f *fakeHost) RecordAnswer(answer string, cancelled bool) {
	if cancelled {
		answer = "(cancelled)"
	}
	f.answers = append(f.answers, answer)
}

// newBar asks about unsaved changes, noting the choice made in *ran.
func newBar(ran *string) (*Bar, *fakeHost) {
	h := &fakeHost{th: theme.New(true)}
	run := func(what string) func() tea.Cmd { return func() tea.Cmd { *ran = what; return nil } }
	return New(h, "You have unsaved changes.", true, "More", []Choice{
		{Key: "enter", Label: "Save and quit", Run: run("save")},
		{Key: "d", Label: "Discard", Run: run("discard")},
		{Key: "esc", Label: "Cancel", Run: run("cancel")},
	}), h
}

func TestKeysChoose(t *testing.T) {
	var ran string
	b, h := newBar(&ran)
	b.Key(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if ran != "" || h.closed != 0 {
		t.Fatalf("a key of no choice acted: %q, closed %d", ran, h.closed)
	}
	b.Key(tea.KeyPressMsg{Code: 'D', Text: "D", Mod: tea.ModShift})
	if ran != "discard" || h.closed != 1 || strings.Join(h.answers, ",") != "d" {
		t.Fatalf("D: ran %q, closed %d, answers %v", ran, h.closed, h.answers)
	}
	b.Key(tea.KeyPressMsg{Code: tea.KeyEscape})
	if ran != "cancel" || h.answers[1] != "(cancelled)" {
		t.Fatalf("Esc: ran %q, answers %v", ran, h.answers)
	}
}

func TestFindByKeyOrLabel(t *testing.T) {
	var ran string
	b, _ := newBar(&ran)
	for answer, want := range map[string]int{"enter": 0, "DISCARD": 1, "Esc": 2, "nope": -1} {
		if got := b.Find(answer); got != want {
			t.Errorf("Find(%q) = %d, want %d", answer, got, want)
		}
	}
}

func TestClickChoosesOrCancels(t *testing.T) {
	var ran string
	b, h := newBar(&ran)
	line, _ := b.ContextLine()
	plain := ansi.Strip(line)
	if !strings.HasPrefix(plain, "You have unsaved changes.   ") || !strings.Contains(plain, "Discard   ") {
		t.Fatalf("context line %q", plain)
	}
	x := strings.Index(plain, "Discard")
	b.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, X: x, Y: overlay.ContextLine})
	if ran != "discard" {
		t.Fatalf("clicking Discard ran %q", ran)
	}
	ran = ""
	b.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, X: 0, Y: overlay.ContextLine + 3})
	if ran != "" || h.closed != 2 {
		t.Fatalf("a click elsewhere should only close: ran %q, closed %d", ran, h.closed)
	}
	if desc, _ := b.Status(); desc != "More" {
		t.Fatalf("status %q", desc)
	}
}
