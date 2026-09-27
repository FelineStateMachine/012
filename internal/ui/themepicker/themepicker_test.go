package themepicker

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost records what the picker drew and kept.
type fakeHost struct {
	th       theme.Theme
	line     lineedit.Line
	current  string
	previews []string
	kept     string
	closed   int
}

func (f *fakeHost) Theme() *theme.Theme       { return &f.th }
func (f *fakeHost) Size() (width, height int) { return 100, 30 }
func (f *fakeHost) Line() *lineedit.Line      { return &f.line }
func (f *fakeHost) Close()                    { f.closed++ }
func (f *fakeHost) RecordAnswer(string, bool) {}
func (f *fakeHost) Current() string           { return f.current }
func (f *fakeHost) ThemesDir() string         { return "" }
func (f *fakeHost) Preview(name string)       { f.previews = append(f.previews, name) }
func (f *fakeHost) Keep(name string)          { f.kept = name }
func (f *fakeHost) last() string              { return f.previews[len(f.previews)-1] }
func newHost(current string) *fakeHost        { return &fakeHost{th: theme.New(true), current: current} }
func down() tea.KeyPressMsg                   { return tea.KeyPressMsg{Code: tea.KeyDown} }
func key(code rune) tea.KeyPressMsg           { return tea.KeyPressMsg{Code: code} }
func typed(s string) tea.KeyPressMsg          { return tea.KeyPressMsg{Code: []rune(s)[0], Text: s} }
func press(p *Picker, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		p.Key(k)
	}
}

func selected(p *Picker) string   { return p.Selected().Title }
func outside() overlay.MouseEvent { return overlay.MouseEvent{Kind: overlay.MousePress} }
func hover(row int) overlay.MouseEvent {
	return overlay.MouseEvent{Kind: overlay.MouseMotion, Box: "picker", Row: row}
}

func entries() []theme.Entry { return theme.List("") }
func name(i int) string      { return entries()[i].Name }
func index(n string) int {
	for i, e := range entries() {
		if e.Name == n {
			return i
		}
	}
	return -1
}

func TestOpensOnTheCurrentTheme(t *testing.T) {
	want := name(3)
	h := newHost(want)
	p := New(h)
	if selected(p) != want || h.last() != want {
		t.Errorf("opened on %q, drew %v", selected(p), h.previews)
	}
}

func TestPreviewsAsItMovesAndRestoresOnEsc(t *testing.T) {
	h := newHost(name(0))
	p := New(h)
	press(p, down())
	if h.last() != name(1) {
		t.Errorf("Down drew %v", h.previews)
	}
	press(p, key(tea.KeyEscape))
	if h.last() != "" || h.closed != 1 {
		t.Errorf("Esc: drew %v, closed %d", h.previews, h.closed)
	}
	n := len(h.previews)
	p.Mouse(hover(4))
	if len(h.previews) != n {
		t.Error("a closed picker drew a theme")
	}
}

func TestEnterKeeps(t *testing.T) {
	h := newHost(name(0))
	p := New(h)
	press(p, down(), down(), key(tea.KeyEnter))
	if h.kept != name(2) {
		t.Errorf("kept %q", h.kept)
	}
}

func TestClickOutsideRestores(t *testing.T) {
	h := newHost(name(0))
	p := New(h)
	press(p, down())
	p.Mouse(outside())
	if h.last() != "" || h.closed != 1 {
		t.Errorf("click outside: drew %v, closed %d", h.previews, h.closed)
	}
}

func TestSearchPreviewsTheBestMatch(t *testing.T) {
	h := newHost(name(0))
	p := New(h)
	target := name(len(entries()) - 1)
	for _, r := range target {
		press(p, typed(string(r)))
	}
	if index(selected(p)) < 0 || h.last() != selected(p) {
		t.Errorf("searching %q selected %q, drew %v", target, selected(p), h.previews)
	}
}

// search types s into the picker.
func search(p *Picker, s string) {
	for _, r := range s {
		press(p, typed(string(r)))
	}
}

// shownNames are the titles the picker lists.
func shownNames(p *Picker) []string {
	var out []string
	for _, m := range p.Shown() {
		out = append(out, m.Item.Title)
	}
	return out
}

func TestDarkOrLightFiltersByKind(t *testing.T) {
	dark := map[string]bool{}
	for _, e := range entries() {
		dark[e.Name] = e.Dark
	}
	for _, q := range []string{"light", "Dark", "light sol", "dark cat"} {
		p := New(newHost(name(0)))
		search(p, q)
		want := strings.EqualFold(q[:4], "dark")
		got := shownNames(p)
		if len(got) == 0 {
			t.Errorf("%q lists nothing", q)
		}
		for _, n := range got {
			if n == theme.Terminal || dark[n] != want {
				t.Errorf("%q lists %q (dark %v)", q, n, dark[n])
			}
		}
	}
	p := New(newHost(name(0)))
	search(p, "light")
	if slices.Contains(shownNames(p), "Bright Lights") {
		t.Error(`"light" lists the dark "Bright Lights"`)
	}
	if n := len(shownNames(p)); n >= len(entries())/2 {
		t.Errorf(`"light" lists %d of %d themes`, n, len(entries()))
	}
	p = New(newHost(name(0)))
	search(p, "light sol")
	if !strings.Contains(selected(p), "Solarized Light") {
		t.Errorf(`"light sol" selected %q`, selected(p))
	}
	// Other searches still match names fuzzily, whatever their kind.
	p = New(newHost(name(0)))
	search(p, "brightli")
	if selected(p) != "Bright Lights" {
		t.Errorf(`"brightli" selected %q`, selected(p))
	}
}
