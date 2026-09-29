package ui

import (
	"io"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// The shapes and the fake terminal below are shared by the stress
// benchmarks (stress_test.go) and the speed gate (speed_test.go), so
// both draw the same frames.

// scaled is dense-8192x26 under a 3-point color scale over all of it,
// every visible cell a shade: what conditional formatting costs a frame.
func scaled() *sheet.Sheet {
	s := stress.Dense(stress.Rows, 26)
	f, err := sheet.ParseCondFormat(`{"ranges":"A1:Z8192","scale":[{"type":"min","color":"red"},{"type":"percentile","value":"50","color":"yellow"},{"type":"max","color":"green"}]}`)
	if err != nil || s.LoadCondFormats([]sheet.CondFormat{f}) > 0 {
		panic(err)
	}
	return s
}

// fakeTerm stands in for Bubble Tea's renderer: a cell buffer the frame
// is drawn into and a terminal renderer diffing it to io.Discard.
type fakeTerm struct {
	buf uv.ScreenBuffer
	scr *uv.TerminalRenderer
}

func newFakeTerm(w, h int) *fakeTerm {
	t := &fakeTerm{buf: uv.NewScreenBuffer(w, h), scr: uv.NewTerminalRenderer(io.Discard, nil)}
	t.scr.Resize(w, h)
	return t
}

func (t *fakeTerm) frame(m *Model) {
	v := m.View()
	t.buf.Clear()
	uv.NewStyledString(v.Content).Draw(t.buf, t.buf.Bounds())
	t.scr.Render(t.buf.RenderBuffer)
	t.scr.Flush()
}

func sized(s *sheet.Sheet, w, h int) *Model {
	m := New(s, "")
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}
