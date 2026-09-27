package suggest

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

// fakeHost is an entry being typed on Sheet1 of a book that also has
// Summary and a range named Sales.
type fakeHost struct {
	th     theme.Theme
	line   lineedit.Line
	s      *sheet.Sheet
	typing bool
}

func newHost(t *testing.T) *fakeHost {
	t.Helper()
	s := sheet.New()
	if _, err := s.Book().AddSheet("Summary", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.DefineName("Sales", sheet.Rect{To: sheet.Addr{Row: 4}}); err != nil {
		t.Fatal(err)
	}
	return &fakeHost{th: theme.New(true), s: s, typing: true}
}

func (h *fakeHost) Theme() *theme.Theme      { return &h.th }
func (h *fakeHost) Size() (int, int)         { return 80, 24 }
func (h *fakeHost) Line() *lineedit.Line     { return &h.line }
func (h *fakeHost) Typing() bool             { return h.typing }
func (h *fakeHost) EntrySheet() *sheet.Sheet { return h.s }
func (h *fakeHost) Stored(buf []rune) []rune { return buf }
func (h *fakeHost) TextX() int               { return 12 }

func names(list []Suggestion) []string {
	var out []string
	for _, s := range list {
		out = append(out, s.Name)
	}
	return out
}

func TestOffersNamesSheetsThenFunctions(t *testing.T) {
	h := newHost(t)
	got := names(For(h.s, "s"))
	if len(got) < 3 || got[0] != "Sales" || got[1] != "Summary!" || got[2] != "SEARCH" && !strings.HasPrefix(got[2], "S") {
		t.Fatalf("for s: %v", got[:min(len(got), 5)])
	}
	if got := names(For(h.s, "'su")); len(got) != 1 || got[0] != "Summary!" {
		t.Fatalf("a quoted word offers only sheets: %v", got)
	}
	if got := names(For(h.s, "ifs")); !strings.Contains(strings.Join(got, " "), "COUNTIFS") {
		t.Fatalf("for ifs: %v", got)
	}
}

func TestKeysMoveAndAccept(t *testing.T) {
	h := newHost(t)
	var l List
	h.line.Set("=su")
	if list, _ := l.Shown(h); list != nil {
		t.Fatal("the list shows before a key typed text")
	}
	l.Active = true
	list, start := l.Shown(h)
	if len(list) == 0 || start != 1 {
		t.Fatalf("list %v from %d", names(list), start)
	}
	for list[l.Sel()].Name != "SUM" {
		l.Key(h, "down")
	}
	if !l.Key(h, "tab") || h.line.Text() != "=SUM(" || h.line.Pos != 5 || l.Active {
		t.Fatalf("accepted %q at %d, active %v", h.line.Text(), h.line.Pos, l.Active)
	}
	if l.Key(h, "down") {
		t.Fatal("a hidden list took a key")
	}
	h.line.Set("=Summ!A1")
	h.line.Pos = len("=Summ")
	l.Active = true
	l.Key(h, "enter")
	if h.line.Text() != "=Summary!A1" {
		t.Fatalf("a sheet replaced its name as %q", h.line.Text())
	}
	h.typing, l.Active = false, true
	h.line.Set("=su")
	if list, _ := l.Shown(h); list != nil {
		t.Fatal("the list shows while something is open over the entry")
	}
}

func TestBoxAndMouse(t *testing.T) {
	h := newHost(t)
	l := List{Active: true}
	h.line.Set("=co")
	b, ok := l.Box(h)
	if !ok || b.Y != overlay.ContextLine+1 || b.X != 12+1-2 || b.Height() != Rows+2 {
		t.Fatalf("box at %d,%d, %d high", b.X, b.Y, b.Height())
	}
	if !strings.Contains(ansi.Strip(strings.Join(b.Lines, "\n")), "of ") {
		t.Error("a long list says where it is")
	}
	if l.Mouse(h, tea.MouseClickMsg{X: 70, Y: 20, Button: tea.MouseLeft}) {
		t.Error("a click outside the list was taken")
	}
	l.Mouse(h, tea.MouseMotionMsg{X: b.X + 2, Y: b.Y + 3})
	if l.Sel() != 2 {
		t.Errorf("hovering the third row highlighted %d", l.Sel())
	}
	list, _ := l.Shown(h)
	want := list[2].Name
	l.Mouse(h, tea.MouseClickMsg{X: b.X + 2, Y: b.Y + 3, Button: tea.MouseLeft})
	if !strings.HasPrefix(h.line.Text(), "="+want) {
		t.Errorf("clicking inserted %q, want %s", h.line.Text(), want)
	}
	if desc, _, ok := (&List{Active: true}).Status(h); ok || desc != "" {
		t.Error("status with the list hidden")
	}
}

func TestSignatureMarksTheArgument(t *testing.T) {
	th := theme.New(true)
	buf := []rune("=SUM(1,2")
	sig, desc := Signature(&th, buf, len(buf), ';')
	if !strings.Contains(sig, th.Argument.Render("[value2; ...]")) || desc == "" {
		t.Fatalf("signature %q, %q", sig, desc)
	}
	if left, _, ok := SignatureLine(&th, 20, []rune("=1+2"), 4, ',', ""); ok || left != "" {
		t.Fatalf("outside a function: %q", left)
	}
}
