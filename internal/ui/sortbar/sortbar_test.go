package sortbar

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a sheet with a header row, recording the sort asked for.
type fakeHost struct {
	th     theme.Theme
	s      *sheet.Sheet
	w      int
	closed int
	data   sheet.Rect
	keys   []sheet.SortKey
	answer string
}

func newHost() *fakeHost {
	s := sheet.New()
	for i, row := range [][]string{{"Item", "Amount", "Qty"}, {"Rent", "1450", "1"}, {"Food", "300", "4"}} {
		for j, v := range row {
			s.Set(sheet.Addr{Col: j, Row: i}, v)
		}
	}
	return &fakeHost{th: theme.New(true), s: s, w: 120}
}

func (h *fakeHost) Theme() *theme.Theme { return &h.th }
func (h *fakeHost) Size() (int, int)    { return h.w, 30 }
func (h *fakeHost) Sheet() *sheet.Sheet { return h.s }
func (h *fakeHost) Close()              { h.closed++ }
func (h *fakeHost) Sort(data sheet.Rect, keys []sheet.SortKey, answer string) tea.Cmd {
	h.data, h.keys, h.answer = data, keys, answer
	return nil
}

var rng = sheet.Rect{To: sheet.Addr{Col: 2, Row: 2}}

func press(b *Bar, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		b.Key(k)
	}
}

var (
	right  = tea.KeyPressMsg{Code: tea.KeyRight}
	space  = tea.KeyPressMsg{Code: tea.KeySpace}
	enter  = tea.KeyPressMsg{Code: tea.KeyEnter}
	altA   = tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt}
	altH   = tea.KeyPressMsg{Code: 'h', Mod: tea.ModAlt}
	letter = func(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }
)

func TestKeysPickColumnsAndOrder(t *testing.T) {
	h := newHost()
	b := New(h, rng, 1, 0)
	if b.Data().String() != "A2:C3" || b.Active() != (sheet.Addr{Row: 1}) {
		t.Fatalf("data %s, active %s", b.Data(), b.Active())
	}
	press(b, right, space, altA, letter('c'))
	line, _ := b.ContextLine()
	if plain := ansi.Strip(line); !strings.Contains(plain, "Sort A2:C3 by  B Amount  Z→A  then  C Qty  A→Z") {
		t.Fatalf("bar %q", plain)
	}
	press(b, enter)
	if h.closed != 1 || h.data.String() != "A2:C3" || len(h.keys) != 2 || h.keys[0] != (sheet.SortKey{Col: 1, Desc: true}) || h.keys[1].Col != 2 {
		t.Fatalf("sorted %s by %v, closed %d", h.data, h.keys, h.closed)
	}
	if want := `{"by":[{"column":"B","order":"desc"},{"column":"C"}],"header":true}`; h.answer != want {
		t.Fatalf("answer %s\nwant   %s", h.answer, want)
	}
}

func TestHeaderToggle(t *testing.T) {
	h := newHost()
	b := New(h, rng, 1, 0)
	press(b, altH)
	if b.Data().String() != "A1:C3" {
		t.Fatalf("without the header row the data is %s", b.Data())
	}
	b.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, X: 0, Y: overlay.ContextLine + 1})
	if h.closed != 1 {
		t.Fatalf("a click off the bar should close it")
	}
}

func TestAnswerSortsAsPicked(t *testing.T) {
	h := newHost()
	b := New(h, rng, 1, 0)
	if _, err := b.Answer(`{"by":[{"column":"c","order":"desc"}],"header":false}`); err != nil {
		t.Fatal(err)
	}
	if h.data.String() != "A1:C3" || len(h.keys) != 1 || h.keys[0] != (sheet.SortKey{Col: 2, Desc: true}) {
		t.Fatalf("sorted %s by %v", h.data, h.keys)
	}
	for text, want := range map[string]string{
		`{"by":[]}`:                            "names no columns",
		`{"by":[{"column":"F"}]}`:              `column "F" isn't in A1:C3`,
		`{"by":[{"column":"A","order":"up"}]}`: `the order of A is "up"`,
		`["A"]`:                                "the sort's answer",
	} {
		if _, err := New(h, rng, 1, 0).Answer(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Answer(%s) = %v, want %q", text, err, want)
		}
	}
}

func TestStatusFitsNarrowScreens(t *testing.T) {
	h := newHost()
	b := New(h, rng, 1, 0)
	desc, keys := b.Status()
	if !strings.Contains(ansi.Strip(desc), "Alt+A add") || !strings.Contains(ansi.Strip(keys), "Esc") {
		t.Fatalf("wide status %q %q", desc, keys)
	}
	h.w = 30
	desc, keys = b.Status()
	if ansi.Strip(desc) != "" || ansi.StringWidth(keys) > 30 {
		t.Fatalf("narrow status %q %q", desc, keys)
	}
}
