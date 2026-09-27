//go:build stress

// Stress benchmarks of the front end: drawing a frame, and a keystroke
// through to the next frame, on big sheets at typical and large terminal
// sizes. A frame is View plus what Bubble Tea does with it: parse the
// string into a cell buffer and diff it onto the terminal. Only built
// with -tags stress (see `make stress` and docs/limits.md).
package ui

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	typesafe "github.com/FelineStateMachine/typesafe-go"
	uv "github.com/charmbracelet/ultraviolet"

	"012/internal/jev"
	"012/internal/sheet"
	"012/internal/stress"
)

var termSizes = []struct{ w, h int }{{80, 24}, {200, 60}, {400, 120}}

type uiShape struct {
	name  string
	build func() *sheet.Sheet
}

func uiShapes() []uiShape {
	return []uiShape{
		{"empty", sheet.New},
		{"dense-8192x26", func() *sheet.Sheet { return stress.Dense(sheet.MaxRows, 26) }},
		{"dense-8192x256", func() *sheet.Sheet { return stress.Dense(sheet.MaxRows, sheet.MaxCols) }},
		{"longtext-8192x500", func() *sheet.Sheet { return stress.LongText(sheet.MaxRows, 500) }},
		{"names-1000", func() *sheet.Sheet { return stress.Names(1000) }},
		{"charts-20", func() *sheet.Sheet {
			s := stress.Table(200, 3)
			stress.Charts(s, 20)
			return s
		}},
	}
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

// BenchmarkView is View alone: 012's share of a frame.
func BenchmarkView(b *testing.B) {
	for _, sh := range uiShapes() {
		s := lazy(sh.build)
		for _, sz := range termSizes {
			b.Run(fmt.Sprintf("%s/%dx%d", sh.name, sz.w, sz.h), func(b *testing.B) {
				m := sized(s(), sz.w, sz.h)
				for b.Loop() {
					m.View()
				}
			})
		}
	}
}

// BenchmarkFrame is a whole frame: View, parse and diff.
func BenchmarkFrame(b *testing.B) {
	for _, sh := range uiShapes() {
		s := lazy(sh.build)
		for _, sz := range termSizes {
			b.Run(fmt.Sprintf("%s/%dx%d", sh.name, sz.w, sz.h), func(b *testing.B) {
				m := sized(s(), sz.w, sz.h)
				t := newFakeTerm(sz.w, sz.h)
				for b.Loop() {
					t.frame(m)
				}
			})
		}
	}
}

func key(k string) tea.KeyPressMsg {
	switch k {
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "shift+down":
		return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
	case "shift+up":
		return tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}
	case "ctrl+a":
		return tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// BenchmarkKeystroke is a key press through to its frame, alternating
// keys that undo each other so the state stays put. Cases without keys
// draw frames over a standing selection (whose statistics the status
// line shows).
func BenchmarkKeystroke(b *testing.B) {
	cases := []struct {
		name  string
		shape uiShape
		setup []string
		keys  []string
	}{
		{"arrow/dense-8192x26", uiShapes()[1], nil, []string{"down", "up"}},
		{"arrow/dense-8192x256", uiShapes()[2], nil, []string{"right", "left"}},
		{"pgdown/dense-8192x256", uiShapes()[2], nil, []string{"pgdown", "pgup"}},
		{"arrow/longtext", uiShapes()[3], nil, []string{"down", "up"}},
		{"arrow/names-1000", uiShapes()[4], nil, []string{"down", "up"}},
		{"arrow/charts-20", uiShapes()[5], nil, []string{"down", "up"}},
		{"select-data/dense-8192x256", uiShapes()[2], []string{"ctrl+a"}, nil},
		{"select-sheet/dense-8192x256", uiShapes()[2], []string{"ctrl+a", "ctrl+a"}, nil},
		{"extend-data/dense-8192x256", uiShapes()[2], []string{"ctrl+a"}, []string{"shift+up", "shift+down"}},
		{"extend/dense-8192x26", uiShapes()[1], []string{"shift+down"}, []string{"shift+down", "shift+up"}},
		{"type/fanin-1000", uiShape{"fanin", func() *sheet.Sheet { return stress.FanIn(sheet.MaxRows, 1000) }},
			nil, []string{"7", "enter", "up"}},
		{"type/dense-8192x26", uiShapes()[1], nil, []string{"7", "enter", "up"}},
	}
	for _, c := range cases {
		s := lazy(c.shape.build)
		for _, sz := range termSizes[:2] {
			b.Run(fmt.Sprintf("%s/%dx%d", c.name, sz.w, sz.h), func(b *testing.B) {
				m := sized(s(), sz.w, sz.h)
				t := newFakeTerm(sz.w, sz.h)
				for _, k := range c.setup {
					m.Update(key(k))
				}
				i := 0
				for b.Loop() {
					if len(c.keys) > 0 {
						m.Update(key(c.keys[i%len(c.keys)]))
					}
					t.frame(m)
					i++
				}
			})
		}
	}
}

// fakeJEV answers every question at once, never touching the network.
type stressJEV struct{}

func (stressJEV) SystemOne(context.Context, typesafe.SystemOneRequest) (*typesafe.SystemOneResponse, error) {
	return &typesafe.SystemOneResponse{Answers: map[string]typesafe.Answer{"answer": typesafe.NoulAnswer{Noul: 0.7}}}, nil
}

// BenchmarkJEV loads n JEV.TEST formulas with distinct questions and
// drains every answer through Update, as the running program would,
// counting frames drawn along the way at 60 per second.
func BenchmarkJEV(b *testing.B) {
	for _, n := range []int{100, 1000, 4000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for b.Loop() {
				cache := jev.NewCache()
				prev := sheet.Remote
				sheet.Remote = cache
				s := sheet.New()
				for i := range n {
					s.Load(sheet.Addr{Row: i}, fmt.Sprintf("row %d", i), sheet.Format{}, sheet.Style{})
					s.Load(sheet.Addr{Col: 1, Row: i}, fmt.Sprintf(`=JEV.TEST(A%d, "Is this row %d?")`, i+1, i), sheet.Format{}, sheet.Style{})
				}
				s.RecalcAll()
				m := sized(s, 200, 60)
				m.EnableJEV(stressJEV{}, cache)
				start := time.Now()
				drain(m, m.sendJEV())
				if in, q := cache.Busy(); in+q > 0 {
					b.Fatalf("%d in flight, %d queued", in, q)
				}
				b.ReportMetric(float64(time.Since(start).Microseconds())/float64(n), "us/answer")
				sheet.Remote = prev
			}
		})
	}
}

// lazy builds a sheet on first use, so running one sub-benchmark doesn't
// build every shape.
func lazy(build func() *sheet.Sheet) func() *sheet.Sheet {
	var s *sheet.Sheet
	return func() *sheet.Sheet {
		if s == nil {
			s = build()
		}
		return s
	}
}

// drain runs cmd and every command its messages lead to, breadth first,
// like Bubble Tea's event loop with instant commands.
func drain(m *Model, cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
}
