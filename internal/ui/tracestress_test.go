//go:build stress

package ui

import (
	"fmt"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// BenchmarkTrace is an arrow key through to its frame on sheets of a
// million cells, with tracing off and on: a chain of a million formulas,
// a cell read by a million formulas (the pointer moving onto and off
// it), and a million numbers. Tracing costs the links it lists, capped,
// and a question per cell on screen.
func BenchmarkTrace(b *testing.B) {
	const million = 1 << 20
	shapes := []struct {
		name  string
		build func() *sheet.Sheet
	}{
		{"chain-1M", func() *sheet.Sheet { return stress.Chain(million) }},
		{"fanout-1M", func() *sheet.Sheet { return stress.FanOut(million - 1) }},
		{"dense-1M", func() *sheet.Sheet { return stress.Dense(stress.Rows, million/stress.Rows) }},
	}
	for _, sh := range shapes {
		s := lazy(sh.build)
		for _, on := range []bool{false, true} {
			b.Run(fmt.Sprintf("arrow/%s/on=%v/200x60", sh.name, on), func(b *testing.B) {
				m := sized(s(), 200, 60)
				t := newFakeTerm(200, 60)
				m.tview = nil
				if on {
					m.runCommand("view.trace")
				}
				keys := []string{"down", "up"}
				i := 0
				for b.Loop() {
					m.Update(key(keys[i%len(keys)]))
					t.frame(m)
					i++
				}
			})
		}
	}
}
