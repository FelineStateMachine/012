package nbview

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// bigNotebook is 50 code cells, each with a 10,000-row output of four
// columns, expanded.
func bigNotebook() (*fakeHost, *View) {
	var b strings.Builder
	b.WriteString("[[name, size, when, ok]; ")
	for i := range 10000 {
		fmt.Fprintf(&b, "[file%d.txt, %dkb, 2026-09-%02dT10:00:00Z, true], ", i, i%900, i%28+1)
	}
	b.WriteString("[last, 1b, 2026-09-01T00:00:00Z, false]]")
	out := []byte(b.String())
	h := newHost()
	for i := range 50 {
		h.cells = append(h.cells, notebook.Cell{ID: i + 1, Source: fmt.Sprintf("t%d = ls **/* | where size > %dkb | sort-by size", i, i)})
		h.outs[i+1] = &notebook.Output{NUON: out, Count: i + 1}
	}
	v := newView(h, 120, 40)
	for _, c := range h.cells {
		v.expand[c.ID] = true
	}
	return h, v
}

// Scrolling a notebook of 50 cells with 10,000-row outputs draws a
// frame in well under a frame's time, once the outputs are parsed.
func TestScrollsAtFrameSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip("parses 50 large outputs")
	}
	_, v := bigNotebook()
	v.Lines() // parses what shows
	for range 50 {
		v.Wheel(10000) // passes every output, parsing it
	}
	v.Wheel(-1 << 30)
	start := time.Now()
	const frames = 200
	for range frames {
		v.Wheel(97)
		v.Lines()
	}
	if per := time.Since(start) / frames; per > 8*time.Millisecond {
		t.Errorf("a frame takes %s", per)
	}
}

func BenchmarkScroll(b *testing.B) {
	_, v := bigNotebook()
	for range 60 {
		v.Wheel(10000)
	}
	v.Wheel(-1 << 30)
	b.ResetTimer()
	for b.Loop() {
		v.Wheel(97)
		v.Lines()
	}
}
