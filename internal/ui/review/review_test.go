package review

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost holds suggestions and settles them as the model would.
type fakeHost struct {
	th     theme.Theme
	items  []Item
	closed int
	shown  []string
}

func (f *fakeHost) Theme() *theme.Theme       { return &f.th }
func (f *fakeHost) Size() (width, height int) { return 80, 24 }
func (f *fakeHost) Close()                    { f.closed++ }
func (f *fakeHost) Suggestions() []Item       { return slices.Clone(f.items) }
func (f *fakeHost) Reveal(s string, a sheet.Addr) {
	f.shown = append(f.shown, sheet.Qualified(s, sheet.Rect{From: a, To: a}))
}

func (f *fakeHost) settle(id int, cells []int) {
	for i, it := range f.items {
		if it.ID != id {
			continue
		}
		if cells == nil {
			f.items = slices.Delete(f.items, i, i+1)
			return
		}
		it.Cells = slices.DeleteFunc(slices.Clone(it.Cells), func(c Cell) bool { return slices.Contains(cells, c.Index) })
		f.items[i] = it
		if len(it.Cells) == 0 {
			f.items = slices.Delete(f.items, i, i+1)
		}
		return
	}
}

func (f *fakeHost) Accept(id int, cells []int) { f.settle(id, cells) }
func (f *fakeHost) Reject(id int, cells []int) { f.settle(id, cells) }

func twoSuggestions() *fakeHost {
	return &fakeHost{th: theme.New(true), items: []Item{
		{ID: 1, Agent: "claude", Label: "set", Message: "fix totals", Total: 2, Cells: []Cell{
			{Index: 0, Sheet: "Sheet1", At: sheet.Addr{Row: 1}, Was: "40", Now: "42"},
			{Index: 1, Sheet: "Sheet1", At: sheet.Addr{Row: 2}, Was: "", Now: "=A1+A2"}}},
		{ID: 2, Agent: "claude", Label: "insert 1 row", Whole: true, Other: []string{"inserts or deletes rows or columns on Sheet1"}},
	}}
}

func text(p *Panel) string {
	var b strings.Builder
	for _, l := range p.Layout()[0].Lines {
		b.WriteString(ansi.Strip(l) + "\n")
	}
	return b.String()
}

func TestPanelLists(t *testing.T) {
	h := twoSuggestions()
	p := New(h)
	got := text(p)
	for _, want := range []string{"Suggestions", "◆ claude: fix totals (2 cells)", "◇ A2  40 → 42", "◇ A3  blank → =A1+A2",
		"claude: insert 1 row (whole)", "also inserts or deletes rows", "2 waiting"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if len(h.shown) != 1 || h.shown[0] != "Sheet1!A2" {
		t.Errorf("shown %v", h.shown)
	}
}

func TestPanelSettlesByCellAndWhole(t *testing.T) {
	h := twoSuggestions()
	p := New(h)
	p.Key(tea.KeyPressMsg{Code: tea.KeyDown})
	p.Key(tea.KeyPressMsg{Code: 'r', Text: "r"}) // A2 rejected
	if len(h.items[0].Cells) != 1 || h.items[0].Cells[0].Index != 1 {
		t.Fatalf("items %+v", h.items)
	}
	p.Key(tea.KeyPressMsg{Code: tea.KeyUp})
	p.Key(tea.KeyPressMsg{Code: tea.KeyEnter}) // the rest of the first
	if len(h.items) != 1 || h.items[0].ID != 2 {
		t.Fatalf("items %+v", h.items)
	}
	p.Key(tea.KeyPressMsg{Code: 'A', Text: "A"})
	if len(h.items) != 0 || h.closed != 1 {
		t.Fatalf("accepting all left %+v, closed %d", h.items, h.closed)
	}
}

func TestPanelChipsClick(t *testing.T) {
	h := twoSuggestions()
	p := New(h)
	p.Layout()
	inner := p.box.Width() - 2
	p.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Box: ID, Col: inner - 2, Row: 1}) // ✗ on the first heading
	if len(h.items) != 1 || h.items[0].ID != 2 {
		t.Fatalf("clicking ✗: %+v", h.items)
	}
	p.Layout()
	p.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Box: ID, Col: inner - 6, Row: 1}) // ✓
	if len(h.items) != 0 {
		t.Fatalf("clicking ✓: %+v", h.items)
	}
	desc, keys := p.Status()
	if desc != "" || !strings.Contains(ansi.Strip(keys), "accept") {
		t.Errorf("status %q %q", desc, keys)
	}
}
