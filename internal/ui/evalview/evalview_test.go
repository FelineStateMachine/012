package evalview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

type fakeHost struct {
	th     theme.Theme
	loc    *locale.Locale
	w      int
	closed bool
}

func (h *fakeHost) Theme() *theme.Theme         { return &h.th }
func (h *fakeHost) Size() (int, int)            { return h.w, 24 }
func (h *fakeHost) Close()                      { h.closed = true }
func (h *fakeHost) Locale() *locale.Locale      { return h.loc }
func named(code rune) tea.KeyPressMsg           { return tea.KeyPressMsg{Code: code} }
func text(v *View) string                       { return ansi.Strip(strings.Join(v.Layout()[0].Lines, "\n")) }
func status(v *View) string                     { d, k := v.Status(); return ansi.Strip(d + " | " + k) }
func at(s string) sheet.Addr                    { a, _ := sheet.ParseAddr(s); return a }
func newHost(w int, l *locale.Locale) *fakeHost { return &fakeHost{th: theme.New(true), loc: l, w: w} }

func book(t *testing.T) *sheet.Sheet {
	t.Helper()
	s := sheet.New()
	for a, in := range map[string]string{"A1": "1.5", "A2": "2", "B1": "=A1*2", "C1": "=SUM(B1,A2)"} {
		if err := s.Set(at(a), in); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestEvaluate(t *testing.T) {
	s := book(t)
	h := newHost(60, locale.Canonical)
	v := New(h, s.EvaluateSteps(at("C1")))
	if got := text(v); !strings.Contains(got, "Evaluate C1") || !strings.Contains(got, "=SUM(B1,A2)") ||
		!strings.Contains(got, "Next  B1 = 3") || !strings.Contains(got, "0 of 3") {
		t.Errorf("first:\n%s", got)
	}
	if got := status(v); !strings.Contains(got, "Enter  evaluate") || !strings.Contains(got, "→  step in") {
		t.Errorf("status %q", got)
	}
	// Stepping into B1 shows its formula; stepping out puts its value in.
	v.Key(named(tea.KeyRight))
	if got := text(v); v.Depth() != 2 || !strings.Contains(got, "Evaluate C1 › B1") || !strings.Contains(got, "=A1*2") {
		t.Errorf("into B1:\n%s", got)
	}
	v.Key(named(tea.KeyEnter))
	v.Key(named(tea.KeyEnter))
	if got := text(v); !strings.Contains(got, "Value  3") || !strings.Contains(status(v), "Enter  step out") {
		t.Errorf("B1 done:\n%s", got)
	}
	v.Key(named(tea.KeyEnter))
	if got := text(v); v.Depth() != 1 || !strings.Contains(got, "=SUM(3,A2)") || !strings.Contains(got, "Next  A2 = 2") {
		t.Errorf("back out:\n%s", got)
	}
	v.Key(named(tea.KeyEnter))
	v.Key(named(tea.KeyEnter))
	if got := text(v); !strings.Contains(got, "Value  5") || !strings.Contains(status(v), "Enter  restart") {
		t.Errorf("done:\n%s", got)
	}
	v.Key(named(tea.KeyEnter))
	if got := text(v); !strings.Contains(got, "0 of 3") {
		t.Errorf("restart:\n%s", got)
	}
	// Esc steps out, then closes.
	v.Key(named(tea.KeyRight))
	v.Key(named(tea.KeyEscape))
	if v.Depth() != 1 || h.closed {
		t.Error("Esc didn't step out")
	}
	v.Key(named(tea.KeyEscape))
	if !h.closed {
		t.Error("Esc didn't close")
	}
}

// Formulas are written in the locale's syntax, and long ones wrap.
func TestEvaluateLocaleAndWrap(t *testing.T) {
	s := book(t)
	s.Set(at("D1"), "=A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1+A1")
	de, _ := locale.Lookup("de-DE")
	v := New(newHost(40, de), s.EvaluateSteps(at("C1")))
	v.Key(named(tea.KeyRight))
	v.Key(named(tea.KeyEnter))
	if got := text(v); !strings.Contains(got, "=1,5*2") {
		t.Errorf("de-DE:\n%s", got)
	}
	v = New(newHost(40, locale.Canonical), s.EvaluateSteps(at("D1")))
	lines := v.Layout()[0].Lines
	if len(lines) < 6 {
		t.Errorf("didn't wrap:\n%s", text(v))
	}
	for _, l := range lines {
		if ansi.StringWidth(l) != ansi.StringWidth(lines[0]) {
			t.Errorf("ragged box:\n%s", text(v))
		}
	}
}
