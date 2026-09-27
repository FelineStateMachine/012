package filterpick

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a screen and an edit line in a locale.
type fakeHost struct {
	th     theme.Theme
	line   lineedit.Line
	loc    *locale.Locale
	closed int
}

func (h *fakeHost) Theme() *theme.Theme    { return &h.th }
func (h *fakeHost) Size() (int, int)       { return 100, 30 }
func (h *fakeHost) Line() *lineedit.Line   { return &h.line }
func (h *fakeHost) Close()                 { h.closed++ }
func (h *fakeHost) Locale() *locale.Locale { return h.loc }

var values = []sheet.FilterValue{{Text: "East", Count: 2, Shown: true}, {Text: "North", Count: 1, Shown: true}, {Text: "", Count: 1, Shown: false}}

// open opens a picker as the model does, noting the criteria applied
// and whether it was cancelled.
func open(h *fakeHost, got *sheet.Criteria, cancelled *bool) *Picker {
	p := New(h, "Filter A  Region", 3, values, sheet.Condition{}, func(cr sheet.Criteria) { *got = cr })
	p.OnCancel = func() { *cancelled = true }
	p.Start()
	return p
}

func newHost() *fakeHost {
	loc, _ := locale.Lookup("en-US")
	return &fakeHost{th: theme.New(true), loc: loc}
}

func keys(p *Picker, ks ...tea.KeyPressMsg) {
	for _, k := range ks {
		p.Key(k)
	}
}

func text(p *Picker, s string) {
	for _, r := range s {
		p.Key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

var (
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	space = tea.KeyPressMsg{Code: tea.KeySpace}
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func TestCheckAndApply(t *testing.T) {
	h := newHost()
	var cr sheet.Criteria
	var cancelled bool
	p := open(h, &cr, &cancelled)
	out := ansi.Strip(strings.Join(p.Layout()[0].Lines, "\n"))
	for _, want := range []string{"Filter A  Region", "[ ] Select all", "[x] East", "[ ] (Blanks)", "3 of 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("box lacks %q:\n%s", want, out)
		}
	}
	keys(p, down, space) // uncheck East
	text(p, "nor")
	if out := ansi.Strip(strings.Join(p.Layout()[0].Lines, "\n")); !strings.Contains(out, "1 of 3") {
		t.Errorf("search didn't narrow:\n%s", out)
	}
	keys(p, enter)
	if h.closed != 1 || strings.Join(cr.Hidden, ",") != "East," || cr.Cond.Op != sheet.CondNone {
		t.Fatalf("applied %+v, closed %d", cr, h.closed)
	}
}

func TestConditionAndCancel(t *testing.T) {
	h := newHost()
	var cr sheet.Criteria
	var cancelled bool
	p := open(h, &cr, &cancelled)
	keys(p, tab, down, down, down) // the condition: Text contains
	text(p, "or")
	if x, y := p.Cursor(); y != overlay.GridTop+1 || x < 3 {
		t.Errorf("cursor %d, %d", x, y)
	}
	keys(p, enter)
	if cr.Cond.Op != sheet.CondContains || cr.Cond.Arg != "or" {
		t.Fatalf("condition %+v", cr.Cond)
	}
	p = open(h, &cr, &cancelled)
	keys(p, esc)
	if !cancelled {
		t.Fatal("Esc didn't cancel")
	}
	cancelled = false
	p = open(h, &cr, &cancelled)
	p.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, X: 90, Y: 20})
	if !cancelled {
		t.Fatal("a click outside didn't cancel")
	}
}

func TestAnswerInTheLocale(t *testing.T) {
	h := newHost()
	h.loc, _ = locale.Lookup("de-DE")
	var cr sheet.Criteria
	var cancelled bool
	p := open(h, &cr, &cancelled)
	if err := p.Answer(`{"hidden":["North"],"condition":"gt","value":"1.5"}`); err != nil {
		t.Fatal(err)
	}
	if strings.Join(cr.Hidden, ",") != "North" || cr.Cond.Op != sheet.CondGreater || cr.Cond.Arg != "1.5" {
		t.Fatalf("answered %+v", cr)
	}
	if err := open(h, &cr, &cancelled).Answer(`{"condition":"bigger"}`); err == nil || !strings.Contains(err.Error(), `"bigger"`) {
		t.Fatalf("a bad condition: %v", err)
	}
}
