package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a screen of its own with a record of what the picker did.
type fakeHost struct {
	th      theme.Theme
	line    lineedit.Line
	w, h    int
	closed  int
	answers []string
}

func newHost() *fakeHost { return &fakeHost{th: theme.New(true), w: 80, h: 24} }

func (f *fakeHost) Theme() *theme.Theme       { return &f.th }
func (f *fakeHost) Size() (width, height int) { return f.w, f.h }
func (f *fakeHost) Line() *lineedit.Line      { return &f.line }
func (f *fakeHost) Close()                    { f.closed++ }

func (f *fakeHost) RecordAnswer(answer string, cancelled bool) {
	if cancelled {
		answer = "(cancelled)"
	}
	f.answers = append(f.answers, answer)
}

func items(picked *string, titles ...string) []Item {
	out := make([]Item, len(titles))
	for i, t := range titles {
		out[i] = Item{Title: t, Name: len(t), Desc: "Does " + t, Pick: func() tea.Cmd { *picked = t; return nil }}
	}
	return out
}

func typeText(p *Picker, s string) {
	for _, r := range s {
		p.Key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func titles(p *Picker) []string {
	var out []string
	for _, m := range p.Shown() {
		out = append(out, m.Item.Title)
	}
	return out
}

func TestSearchRanksWordPrefixesFirst(t *testing.T) {
	h, picked := newHost(), ""
	p := New(h, "Test", "Type", 60, items(&picked, "Command line", "Column width", "Save"))
	if got := len(p.Shown()); got != 3 {
		t.Fatalf("empty search shows %d", got)
	}
	typeText(p, "col")
	if got := strings.Join(titles(p), ","); got != "Column width,Command line" {
		t.Errorf("col matched %s", got)
	}
	if h.line.Text() != "col" {
		t.Errorf("search typed %q", h.line.Text())
	}
}

func TestEnterPicksAndRecords(t *testing.T) {
	h, picked := newHost(), ""
	p := New(h, "Test", "Type", 60, items(&picked, "Alpha", "Beta"))
	p.Answers = true
	p.Key(tea.KeyPressMsg{Code: tea.KeyDown})
	p.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if picked != "Beta" || strings.Join(h.answers, ",") != "Beta" {
		t.Errorf("picked %q, answers %v", picked, h.answers)
	}
	p.Key(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.closed != 1 || h.answers[len(h.answers)-1] != "(cancelled)" {
		t.Errorf("Esc: closed %d, answers %v", h.closed, h.answers)
	}
}

func TestUnavailableItemsDontPick(t *testing.T) {
	h, picked := newHost(), ""
	its := items(&picked, "Undo")
	its[0].Off = true
	p := New(h, "Test", "Type", 60, its)
	p.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if picked != "" {
		t.Error("an unavailable item was picked")
	}
	if desc, _ := p.Status(); desc != "Does Undo (not available now)" {
		t.Errorf("status %q", desc)
	}
	if _, ok := p.Answer("undo"); ok {
		t.Error("a script answered with an unavailable item")
	}
}

func TestEnterHookAndAnswer(t *testing.T) {
	h, picked := newHost(), ""
	p := New(h, "Test", "Type", 60, items(&picked, "Alpha"))
	var query string
	p.Enter = func(q string) (tea.Cmd, bool) { query = q; return nil, q == "/tmp" }
	typeText(p, "/tmp")
	p.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if query != "/tmp" || picked != "" {
		t.Errorf("Enter hook got %q, picked %q", query, picked)
	}
	if _, ok := p.Answer("ALPHA"); !ok || picked != "Alpha" {
		t.Errorf("answer picked %q", picked)
	}
}

func TestLayoutAndMouse(t *testing.T) {
	h, picked := newHost(), ""
	p := New(h, "Test", "Type a name", 60, items(&picked, "Alpha", "Beta", "Gamma"))
	boxes := p.Layout()
	if len(boxes) != 1 || boxes[0].ID != ID {
		t.Fatalf("layout %+v", boxes)
	}
	b := boxes[0]
	if b.Y != overlay.MenuLine+1 || b.X+b.Width() > h.w || b.Height() != 3+3+1 {
		t.Errorf("box at %d,%d, %dx%d", b.X, b.Y, b.Width(), b.Height())
	}
	out := ansi.Strip(strings.Join(b.Lines, "\n"))
	for _, want := range []string{"Test", "Type a name", "Gamma", "3 of 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("box lacks %q:\n%s", want, out)
		}
	}
	x, y := p.Cursor()
	if y != b.Y+1 || x != b.X+1+ansi.StringWidth(overlay.SearchPrompt) {
		t.Errorf("cursor at %d,%d", x, y)
	}
	p.Mouse(overlay.MouseEvent{Kind: overlay.MouseMotion, Box: ID, Row: FirstRow + 2})
	if p.Selected().Title != "Gamma" {
		t.Errorf("hover selected %q", p.Selected().Title)
	}
	p.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseLeft, Box: ID, Row: FirstRow + 1})
	if picked != "Beta" {
		t.Errorf("click picked %q", picked)
	}
	p.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseLeft})
	if h.closed != 1 {
		t.Error("a click outside didn't close the picker")
	}
}

func TestNoMatches(t *testing.T) {
	h, picked := newHost(), ""
	p := New(h, "Test", "Type", 60, items(&picked, "Alpha"))
	typeText(p, "zzq")
	if p.Selected() != nil {
		t.Error("something is selected")
	}
	if out := ansi.Strip(strings.Join(p.Layout()[0].Lines, "\n")); !strings.Contains(out, "No matches") || !strings.Contains(out, "0 of 1") {
		t.Errorf("no-match box:\n%s", out)
	}
}
