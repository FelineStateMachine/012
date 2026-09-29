//go:build stress

// Stress benchmarks of the front end: drawing a frame, and a keystroke
// through to the next frame, on big sheets at typical and large terminal
// sizes. A frame is View plus what Bubble Tea does with it: parse the
// string into a cell buffer and diff it onto the terminal. Only built
// with -tags stress (see `make stress` and docs/contributing/limits.md).
package ui

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	typesafe "github.com/FelineStateMachine/typesafe-go"

	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

var termSizes = []struct{ w, h int }{{80, 24}, {200, 60}, {400, 120}}

type uiShape struct {
	name  string
	build func() *sheet.Sheet
}

func uiShapes() []uiShape {
	return []uiShape{
		{"empty", sheet.New},
		{"dense-8192x26", func() *sheet.Sheet { return stress.Dense(stress.Rows, 26) }},
		{"dense-8192x256", func() *sheet.Sheet { return stress.Dense(stress.Rows, stress.Cols) }},
		{"longtext-8192x500", func() *sheet.Sheet { return stress.LongText(stress.Rows, 500) }},
		{"names-1000", func() *sheet.Sheet { return stress.Names(1000) }},
		{"charts-20", func() *sheet.Sheet {
			s := stress.Table(200, 3)
			stress.Charts(s, 20)
			return s
		}},
		{"sparse-1M", func() *sheet.Sheet { return stress.Sparse(10000, 100, 1000) }},
		{"scale-8192x26", scaled},
		{"laidout-8192x26", func() *sheet.Sheet { return stress.Laidout(stress.Rows, 26) }},
		{"bars-8192x26", barred},
	}
}

// barred is dense-8192x26 with data bars over half its columns and
// arrows over the rest, every visible cell drawn with one: what bars
// and icons cost a frame.
func barred() *sheet.Sheet {
	s := stress.Dense(stress.Rows, 26)
	var fs []sheet.CondFormat
	for _, line := range []string{
		`{"ranges":"A1:M8192","dataBar":{"color":"blue","min":{"type":"min"},"max":{"type":"max"}}}`,
		`{"ranges":"N1:Z8192","iconSet":{"icons":"arrows","points":[{"type":"percent","value":"33"},{"type":"percent","value":"67"}]}}`,
	} {
		f, err := sheet.ParseCondFormat(line)
		if err != nil {
			panic(err)
		}
		fs = append(fs, f)
	}
	if s.LoadCondFormats(fs) > 0 {
		panic("rules left out")
	}
	return s
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
	case "ctrl+shift+down":
		return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl | tea.ModShift}
	case "ctrl+shift+up":
		return tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl | tea.ModShift}
	case "ctrl+shift+right":
		return tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl | tea.ModShift}
	case "ctrl+a":
		return tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
	case "ctrl+down":
		return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
	case "ctrl+up":
		return tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}
	case "ctrl+space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModCtrl}
	case "shift+right":
		return tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}
	case "shift+left":
		return tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}
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
		{"extend-edge/dense-8192x256", uiShapes()[2], []string{"ctrl+shift+right"}, []string{"ctrl+shift+down", "ctrl+shift+up"}},
		{"extend/dense-8192x26", uiShapes()[1], []string{"shift+down"}, []string{"shift+down", "shift+up"}},
		{"type/fanin-1000", uiShape{"fanin", func() *sheet.Sheet { return stress.FanIn(stress.Rows, 1000) }},
			nil, []string{"7", "enter", "up"}},
		{"type/dense-8192x26", uiShapes()[1], nil, []string{"7", "enter", "up"}},
		{"arrow/scale-8192x26", uiShapes()[7], nil, []string{"down", "up"}},
		{"arrow/laidout-8192x26", uiShapes()[8], nil, []string{"down", "up"}},
		{"pgdown/laidout-8192x26", uiShapes()[8], nil, []string{"pgdown", "pgup"}},
		{"type/laidout-8192x26", uiShapes()[8], nil, []string{"7", "enter", "up"}},
		{"type/scale-8192x26", uiShapes()[7], nil, []string{"7", "enter", "up"}},
		{"jump/sparse-1M", uiShapes()[6], nil, []string{"ctrl+down", "ctrl+down", "ctrl+up", "ctrl+up"}},
		{"select-sheet/sparse-1M", uiShapes()[6], []string{"ctrl+a", "ctrl+a"}, nil},
		{"extend-column/sparse-1M", uiShapes()[6], []string{"ctrl+space"}, []string{"shift+right", "shift+left"}},
		{"type/sparse-1M", uiShapes()[6], nil, []string{"7", "enter", "up"}},
		{"arrow/corner", uiShape{"corner", sheet.New}, []string{"@XFD1048576"}, []string{"up", "left", "down", "right"}},
	}
	for _, c := range cases {
		s := lazy(c.shape.build)
		for _, sz := range termSizes[:2] {
			b.Run(fmt.Sprintf("%s/%dx%d", c.name, sz.w, sz.h), func(b *testing.B) {
				m := sized(s(), sz.w, sz.h)
				t := newFakeTerm(sz.w, sz.h)
				for _, k := range c.setup {
					if a, ok := strings.CutPrefix(k, "@"); ok { // go to a cell
						cell, _ := sheet.ParseAddr(a)
						m.cur = cell
						m.scrollTo(cell)
						continue
					}
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

// BenchmarkTelemetry is an arrow key through to its frame and an edit
// that recalculates, with telemetry off and on (a JSON log in a temp
// file): what the instrumentation costs.
func BenchmarkTelemetry(b *testing.B) {
	s := lazy(func() *sheet.Sheet { return stress.Dense(stress.Rows, 26) })
	for _, on := range []bool{false, true} {
		name := "off"
		if on {
			name = "on"
		}
		b.Run("arrow/"+name, func(b *testing.B) {
			stop := telemetryFor(b, on)
			defer stop()
			m := sized(s(), 200, 60)
			t := newFakeTerm(200, 60)
			keys := []string{"down", "up"}
			i := 0
			for b.Loop() {
				m.Update(key(keys[i%2]))
				t.frame(m)
				i++
			}
		})
		b.Run("edit/"+name, func(b *testing.B) {
			stop := telemetryFor(b, on)
			defer stop()
			m := sized(s(), 200, 60)
			keys := []string{"7", "enter", "up"}
			i := 0
			for b.Loop() {
				m.Update(key(keys[i%3]))
				i++
			}
		})
	}
}

// telemetryFor turns telemetry on, logging to a temp file, when on is
// set, the way cmd/012 does.
func telemetryFor(b *testing.B, on bool) func() error {
	if !on {
		return func() error { return nil }
	}
	stop, err := telemetry.Setup(telemetry.Config{LogPath: filepath.Join(b.TempDir(), "log.jsonl")})
	if err != nil {
		b.Fatal(err)
	}
	// As cmd/012 does: recalculations nest in the model's trace.
	sheet.OnBegin = func(trace any, op string) {
		t, _ := trace.(*telemetry.Trace)
		t.Begin(op)
	}
	sheet.OnRecalc = func(trace any, i sheet.RecalcInfo) {
		t, _ := trace.(*telemetry.Trace)
		t.End(slog.Int("evaluated", i.Evaluated), slog.Int("cells", i.Cells))
	}
	return func() error {
		sheet.OnBegin, sheet.OnRecalc = nil, nil
		return stop()
	}
}

// fakeJEV answers every question at once, never touching the network.
type stressJEV struct{}

func (stressJEV) SystemOne(context.Context, typesafe.SystemOneRequest) (*typesafe.SystemOneResponse, error) {
	return &typesafe.SystemOneResponse{Answers: map[string]typesafe.Answer{"answer": typesafe.NoulAnswer{Noul: 0.7}}}, nil
}

// BenchmarkJEV loads n JEV.TEST formulas with distinct questions and
// drains every answer through Update, as the running program would.
// Answers that are queued together (up to jevParallel) recalculate once.
func BenchmarkJEV(b *testing.B) {
	for _, n := range []int{100, 1000, 4000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for b.Loop() {
				cache := jev.NewCache()
				s := sheet.New()
				for i := range n {
					s.Load(sheet.Addr{Row: i}, fmt.Sprintf("row %d", i), sheet.Format{}, sheet.Style{})
					s.Load(sheet.Addr{Col: 1, Row: i}, fmt.Sprintf(`=JEV.TEST(A%d, "Is this row %d?")`, i+1, i), sheet.Format{}, sheet.Style{})
				}
				s.RecalcAll()
				m := sized(s, 200, 60)
				m.EnableJEV(stressJEV{}, cache)
				m.jev.gather = 0 // answers already queued still gather; don't wait a frame
				start := time.Now()
				drain(m, m.jev.send(m.spans.Parent()))
				if in, q := cache.Busy(); in+q > 0 {
					b.Fatalf("%d in flight, %d queued", in, q)
				}
				b.ReportMetric(float64(time.Since(start).Microseconds())/float64(n), "us/answer")
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
