package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"
)

// ticked stands for a sixelDrawMsg asked for: tests deliver
// sixelDrawMsg themselves, with the clock where they want it.
type ticked struct{}

// sixelModel is a wide model with a chart, on a terminal that has said
// what a sixel terminal says: sixel in DA1, 16 color registers and its
// cell size. Its clock stands still unless the test moves it.
func sixelModel(t *testing.T) (*Model, *time.Time) {
	t.Helper()
	m := wideModel()
	m.term.tmux = false // wherever the tests run
	clock := time.Unix(1000, 0)
	m.term.six.now = func() time.Time { return clock }
	m.term.six.tick = func() tea.Cmd { return func() tea.Msg { return ticked{} } }
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "<enter>", "<esc>")
	if !strings.Contains(screen(m), "█") {
		t.Fatalf("no bars in the text chart:\n%s", screen(m))
	}
	send(m, uv.UnknownCsiEvent("\x1b[?1;0;16S"))
	if got := sixelOut(m, uv.PrimaryDeviceAttributesEvent{62, 4, 22}); !m.term.sixel || got != "\x1b[16t" {
		t.Fatalf("sixel not detected, or no cell size asked for: %q", got)
	}
	if m.term.registers != 16 {
		t.Errorf("%d registers, want 16", m.term.registers)
	}
	if got := sixelOut(m, uv.CellSizeEvent{Width: 8, Height: 16}); got != "<tick>" {
		t.Fatalf("after the cell size: %q, want images asked for", got)
	}
	return m, &clock
}

// sixelOut updates m with msg and returns what the commands write to the
// terminal, with "<clear>" for a screen cleared and "<tick>" for images
// asked for.
func sixelOut(m *Model, msg tea.Msg) string {
	_, cmd := m.Update(msg)
	m.View()
	var b strings.Builder
	for _, msg := range flatten(cmd) {
		switch msg := msg.(type) {
		case tea.RawMsg:
			b.WriteString(msg.Msg.(string))
		case ticked:
			b.WriteString("<tick>")
		default:
			if fmt.Sprintf("%T", msg) == "tea.clearScreenMsg" {
				b.WriteString("<clear>")
			}
		}
	}
	return b.String()
}

func TestSixelImages(t *testing.T) {
	m, clock := sixelModel(t)
	if len(m.term.six.plan) != 1 || strings.Contains(screen(m), "█") {
		t.Fatalf("plan %v; the plot area isn't blank:\n%s", m.term.six.plan, screen(m))
	}
	if !strings.Contains(screen(m), "1,500") {
		t.Error("the axes aren't text around the image")
	}
	// Not drawn until the frame has been still for sixelSettle.
	if got := sixelOut(m, sixelDrawMsg{}); got != "<tick>" {
		t.Errorf("drawn over a frame still changing: %q", got)
	}
	*clock = clock.Add(sixelSettle)
	p := m.term.six.plan[0]
	got := sixelOut(m, sixelDrawMsg{})
	at := fmt.Sprintf("\x1b7\x1b[%d;%dH\x1bP0;1;0q", p.at.Min.Y+1, p.at.Min.X+1)
	if !strings.HasPrefix(got, at) || !strings.HasSuffix(got, "\x1b\\\x1b8") {
		t.Fatalf("image drawn as %.60q, want it at %q with the cursor saved", got, at)
	}
	if n := strings.Count(got, ";2;"); n == 0 || n > 16 {
		t.Errorf("%d colors defined for 16 registers", n)
	}
	// Drawn once: a frame that doesn't change draws nothing.
	if got := sixelOut(m, sixelDrawMsg{}); got != "" {
		t.Errorf("drawn again over the same frame: %.40q", got)
	}
	// A row the image crosses changes: drawn again.
	press(t, m, "<f5>", fmt.Sprintf("A%d", p.at.Min.Y-gridTop+1), "<enter>")
	m.View() // as Bubble Tea does after every update
	*clock = clock.Add(time.Second)
	if got := sixelOut(m, sixelDrawMsg{}); !strings.HasPrefix(got, at) {
		t.Errorf("not drawn again after its rows changed: %.40q", got)
	}
}

// Scrolling moves the image: the screen is cleared, erasing it, and it is
// drawn where it went. The command palette over it makes it text.
func TestSixelMovesAndHides(t *testing.T) {
	m, clock := sixelModel(t)
	*clock = clock.Add(time.Second)
	sixelOut(m, sixelDrawMsg{})
	was := m.term.six.plan[0].at
	if got := sixelOut(m, tea.MouseWheelMsg{X: 50, Y: gridTop + 2, Button: tea.MouseWheelDown}); !strings.Contains(got, "<clear>") {
		t.Fatalf("scrolled without clearing: %q", got)
	}
	if len(m.term.six.plan) == 1 && m.term.six.plan[0].at == was {
		t.Fatalf("the image didn't move: %v", m.term.six.plan)
	}
	*clock = clock.Add(time.Second)
	if got := sixelOut(m, sixelDrawMsg{}); len(m.term.six.plan) == 1 && !strings.Contains(got, "\x1bP") {
		t.Error("not drawn where it went")
	}
	sixelOut(m, tea.MouseWheelMsg{X: 50, Y: gridTop + 2, Button: tea.MouseWheelUp})
	*clock = clock.Add(time.Second)
	sixelOut(m, sixelDrawMsg{})
	if got := sixelOut(m, tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}); len(m.term.six.plan) != 0 || !strings.Contains(got, "<clear>") {
		t.Errorf("palette open: plan %v, %q", m.term.six.plan, got)
	}
}

// Sixel is used only when kitty graphics aren't, never inside tmux, and
// not with chart-images off.
func TestSixelDetection(t *testing.T) {
	m := wideModel()
	m.term.tmux = false
	m.Update(uv.PrimaryDeviceAttributesEvent{62, 22})
	if m.term.sixel {
		t.Error("sixel without attribute 4")
	}
	m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 31}, Payload: []byte("OK")})
	m.Update(uv.PrimaryDeviceAttributesEvent{62, 4})
	if m.term.sixel {
		t.Error("sixel on a terminal with kitty graphics")
	}
	m = wideModel()
	m.term.tmux = true
	m.Update(uv.PrimaryDeviceAttributesEvent{62, 4})
	if m.term.sixel {
		t.Error("sixel inside tmux")
	}
	m, _ = sixelModel(t)
	m.term.noImages = true
	if got := sixelOut(m, nil); m.term.sixelOn() || len(m.term.six.plan) != 0 || got != "" {
		t.Errorf("sixel images with chart-images = false: %q", got)
	}
	for in, want := range map[string]int{"\x1b[?1;0;256S": 256, "\x1b[?1;0;1024S": 256, "\x1b[?1;3;0S": 0, "\x1b[?2;0;4S": 0} {
		if n, _ := parseRegisters(in); n != want {
			t.Errorf("%q: %d registers, want %d", in, n, want)
		}
	}
}
