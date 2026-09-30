package ui

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// watched is a program's model that notes when a frame first shows a
// text: a session watching another's edit arrive.
type watched struct {
	*Shared
	mu   sync.Mutex
	want string
	seen chan time.Time
}

func (w *watched) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := w.Shared.Update(msg)
	return w, cmd
}

func (w *watched) View() tea.View {
	v := w.Shared.View()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.want != "" && strings.Contains(ansi.Strip(v.Content), w.want) {
		w.want = ""
		w.seen <- time.Now()
	}
	return v
}

// expect has the next frame showing text noted.
func (w *watched) expect(text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.want = text
}

// program runs a served session in a room in a real Bubble Tea program,
// with no terminal, until the test ends.
func program(t *testing.T, r *servedRooms, user, name string) (*watched, *tea.Program) {
	m := New(sheet.New(), "")
	m.Serve(r.root, []string{"TERM=xterm-256color"})
	m.ShareRooms(r.reg, user)
	m.OpenOnStart(name)
	w := &watched{Shared: InRooms(Guard(m)), seen: make(chan time.Time, 1)}
	p := tea.NewProgram(w, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignals(), tea.WithWindowSize(80, 24), tea.WithFPS(FrameRate))
	ctx, cancel := context.WithCancel(context.Background())
	w.Attach(ctx, p)
	done := make(chan struct{})
	go func() {
		p.Run()
		close(done)
	}()
	t.Cleanup(func() {
		p.Quit()
		<-done
		cancel()
	})
	return w, p
}

// An edit in one session reaches another's screen within a frame: the
// other's model has taken it and drawn a frame showing it before a
// frame's time has passed, so the next frame its terminal gets holds it.
func TestSharedEditWithinAFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("times frames")
	}
	unlock := benchLock(t)
	defer unlock()
	r := newRooms(t, room.Edit)
	ann := r.open("ann", "@fast")
	bob, _ := program(t, r, "bob", "@fast")
	deadline := time.Now().Add(5 * time.Second)
	for shared := false; !shared && time.Now().Before(deadline); {
		ann.sh.turn(func() { shared = ann.m.shared() })
		time.Sleep(time.Millisecond)
	}
	var took []time.Duration
	for i := range 30 {
		text := "v" + strings.Repeat("x", i%5) + string(rune('a'+i%26))
		ann.press(text)
		bob.expect(text)
		start := time.Now()
		ann.press("<enter>")
		select {
		case at := <-bob.seen:
			took = append(took, at.Sub(start))
		case <-time.After(2 * time.Second):
			t.Fatalf("bob never showed %q", text)
		}
		ann.press("<up>") // the same cell again, on bob's screen
	}
	slices.Sort(took)
	median := took[len(took)/2]
	t.Logf("an edit reached the other session in %v (median), %v at worst", median, took[len(took)-1])
	if median > FrameInterval {
		t.Errorf("median %v, more than a frame (%v)", median, FrameInterval)
	}
}

// Sessions typing at once in one room, each in its own program: every
// entry lands, and the race detector finds nothing shared unguarded.
func TestSharedSessionsTypeAtOnce(t *testing.T) {
	r := newRooms(t, room.Edit)
	var ps []*tea.Program
	for _, user := range []string{"ann", "bob", "cy"} {
		_, p := program(t, r, user, "@busy")
		ps = append(ps, p)
	}
	time.Sleep(50 * time.Millisecond) // everyone's in
	var wg sync.WaitGroup
	for i, p := range ps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range i + 1 { // each to its own column
				p.Send(tea.KeyPressMsg{Code: tea.KeyRight})
				_ = k
			}
			for row := range 20 {
				for _, c := range "n" + string(rune('0'+row%10)) {
					p.Send(tea.KeyPressMsg{Code: c, Text: string(c)})
				}
				p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
		}()
	}
	wg.Wait()
	// Look as one more participant would, in a turn of its own.
	look, _ := r.reg.Join("@busy", &peerLink{name: "look", kick: make(chan struct{}, 1)}, nil)
	deadline := time.Now().Add(5 * time.Second)
	for {
		n := 0
		look.Do(func(w *sheet.Workbook) { n = w.Sheets()[0].Len() })
		if n == 60 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of 60 entries landed", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
