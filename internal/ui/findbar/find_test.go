package findbar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a workbook with an active cell and a record of what the
// bar did.
type fakeHost struct {
	th      theme.Theme
	line    lineedit.Line
	s       *sheet.Sheet
	cur     sheet.Addr
	note    string
	edited  bool
	left    int
	pressed []int
}

func newHost(t *testing.T, cells ...string) *fakeHost {
	t.Helper()
	s := sheet.New()
	for i := 0; i < len(cells); i += 2 {
		if err := s.Set(addr(cells[i]), cells[i+1]); err != nil {
			t.Fatal(err)
		}
	}
	return &fakeHost{th: theme.New(true), s: s}
}

func addr(s string) sheet.Addr {
	a, _ := sheet.ParseAddr(s)
	return a
}

func (f *fakeHost) Theme() *theme.Theme               { return &f.th }
func (f *fakeHost) Size() (width, height int)         { return 120, 24 }
func (f *fakeHost) Line() *lineedit.Line              { return &f.line }
func (f *fakeHost) Book() *sheet.Workbook             { return f.s.Book() }
func (f *fakeHost) At() (*sheet.Sheet, sheet.Addr)    { return f.s, f.cur }
func (f *fakeHost) Show(s *sheet.Sheet, a sheet.Addr) { f.s, f.cur = s, a }
func (f *fakeHost) Note(msg string)                   { f.note = msg }
func (f *fakeHost) Edited()                           { f.edited = true }
func (f *fakeHost) Leave()                            { f.left++ }

func (f *fakeHost) Press(x, y int, _ tea.MouseButton) tea.Cmd {
	f.pressed = []int{x, y}
	return nil
}

func typeText(b *Bar, s string) {
	for _, r := range s {
		b.Key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func open(h *fakeHost, replace bool) *Bar {
	b := New(h)
	b.Reset(nil)
	b.Open(replace)
	return b
}

func TestFindAsYouType(t *testing.T) {
	h := newHost(t, "A1", "Rent", "A2", "rental car", "A3", "Total rent", "B1", "1450")
	b := open(h, false)
	typeText(b, "ren")
	if len(b.Matches()) != 3 || h.cur != addr("A1") {
		t.Fatalf("matches %v, at %v", b.Matches(), h.cur)
	}
	if left, right := b.ContextLine(); !strings.Contains(ansi.Strip(left), "Find › ren") || ansi.Strip(right) != "1 of 3" {
		t.Errorf("context line %q %q", left, right)
	}
	b.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	b.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	b.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.cur != addr("A1") {
		t.Errorf("didn't wrap: at %v", h.cur)
	}
	if !b.Found(h.s, addr("A2")) || b.Found(h.s, addr("B1")) {
		t.Error("wrong cells found")
	}
	b.Key(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.left != 1 {
		t.Error("Esc didn't leave")
	}
}

func TestAgainFromTheActiveCell(t *testing.T) {
	h := newHost(t, "A1", "x", "A5", "x", "A9", "x")
	b := open(h, false)
	typeText(b, "x")
	h.cur = addr("A5")
	b.Again(1)
	if h.cur != addr("A9") || h.note != "Match 3 of 3 for x" {
		t.Errorf("next: at %v, %q", h.cur, h.note)
	}
	b.Again(-1)
	if h.cur != addr("A5") {
		t.Errorf("previous: at %v", h.cur)
	}
}

func TestOptionsAndBadRegex(t *testing.T) {
	h := newHost(t, "A1", "Rent", "A2", "rent")
	b := open(h, false)
	typeText(b, "Rent")
	b.Key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	if !b.Options().MatchCase || len(b.Matches()) != 1 {
		t.Errorf("match case: %v", b.Matches())
	}
	b.Key(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	typeText(b, "(")
	if _, right := b.ContextLine(); ansi.Strip(right) != "Invalid regular expression" {
		t.Errorf("bad regex: %q", right)
	}
}

func TestReplace(t *testing.T) {
	h := newHost(t, "A1", "rent", "A2", "rental car", "A3", "total rent")
	b := open(h, true)
	typeText(b, "rent")
	b.Key(tea.KeyPressMsg{Code: tea.KeyTab})
	typeText(b, "lease")
	if !b.Replacing() || b.Field() != 1 {
		t.Fatalf("replace field %d", b.Field())
	}
	b.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := h.s.Cell(addr("A1")).Input; got != "lease" || !h.edited || h.cur != addr("A2") {
		t.Errorf("replace one: A1 %q, at %v", got, h.cur)
	}
	b.Key(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	if got := h.s.Cell(addr("A3")).Input; got != "total lease" || h.note != "Replaced 2 cells" {
		t.Errorf("replace all: A3 %q, note %q", got, h.note)
	}
}

func TestMouse(t *testing.T) {
	h := newHost(t, "A1", "rent", "A2", "Rent")
	b := open(h, false)
	typeText(b, "rent")
	left, _ := b.ContextLine()
	plain := ansi.Strip(left)
	x := ansi.StringWidth(plain[:strings.Index(plain, "Aa")])
	b.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, X: x, Y: overlay.ContextLine})
	if !b.Options().MatchCase || len(b.Matches()) != 1 {
		t.Errorf("clicking Aa: %v", b.Matches())
	}
	b.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, X: 3, Y: 10})
	if h.left != 1 || len(h.pressed) != 2 || h.pressed[1] != 10 {
		t.Errorf("a click on the grid: left %d, pressed %v", h.left, h.pressed)
	}
}
