//go:build stress

package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/stress"
)

// BenchmarkSourceScroll scrolls a linked source of ten million rows
// (stress.SalesParquet) through to its frames: Page Down and Up within
// the rows read, halfway down, as scrolling is between pages; and
// jumping to rows all over the source, as dragging the scrollbar does,
// reading the pages each jump shows (run to the end here, in the
// background on the screen) before the frame that shows them. The row
// numbers and the scrollbar are over every row, whatever the grid's.
func BenchmarkSourceScroll(b *testing.B) {
	path, err := stress.SalesParquet(stress.GeneratedDir(b.TempDir()), 10_000_000)
	if err != nil {
		b.Fatal(err)
	}
	defer stress.BenchLock(b)()
	cases := []struct {
		name string
		keys []tea.KeyPressMsg
		read bool
	}{
		{"pgdown", []tea.KeyPressMsg{{Code: tea.KeyPgDown}, {Code: tea.KeyPgUp}}, false},
		{"jump", nil, true},
	}
	for _, c := range cases {
		for _, sz := range termSizes[:2] {
			b.Run(fmt.Sprintf("%s/%dx%d", c.name, sz.w, sz.h), func(b *testing.B) {
				m := sourceModel(path, sz.w, sz.h)
				m.srcView().MoveTo(5_000_000, 2)
				for _, k := range c.keys {
					m.Update(k)
					settleSources(m)
				}
				t := newFakeTerm(sz.w, sz.h)
				i := 0
				for b.Loop() {
					if c.read {
						m.srcView().MoveTo(int64(i)*1_234_567%10_000_000, 2)
						settleSources(m)
					} else {
						m.Update(c.keys[i%2])
					}
					t.frame(m)
					i++
				}
			})
		}
	}
}
