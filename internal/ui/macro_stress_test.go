//go:build stress

package ui

import (
	"fmt"
	"testing"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// replayActions is a recording of n actions as a user makes them: entries
// of numbers, text and formulas, moves between them, a selection and a
// command now and then.
func replayActions(n int, relative bool) []macro.Action {
	out := make([]macro.Action, 0, n)
	for i := 0; len(out) < n; i++ {
		row := i/4 + 1
		switch i % 8 {
		case 0:
			out = append(out, macro.Call("enter", fmt.Sprint(i)))
		case 1:
			out = append(out, macro.Call("enter", "item "+fmt.Sprint(i)))
		case 2:
			a := macro.Call("enter", fmt.Sprintf("=A%d*2", row))
			if relative {
				a = a.With("origin", fmt.Sprintf("C%d", row))
			}
			out = append(out, a)
		case 3:
			out = append(out, macro.Call("extend", 2, 0))
		case 4:
			out = append(out, macro.Call("run", "format.bold"))
		case 5, 6:
			if relative {
				out = append(out, macro.Call("move", 1, 0))
			} else {
				out = append(out, macro.Call("select", fmt.Sprintf("%s%d", sheet.ColName(i%8-4), row)))
			}
		case 7:
			if relative {
				out = append(out, macro.Call("move", -2, 1))
			} else {
				out = append(out, macro.Call("select", fmt.Sprintf("A%d", row+1)))
			}
		}
	}
	return out
}

// BenchmarkMacroReplay replays a 1000-action recorded macro through the
// real path: the script on its own goroutine, each call served on the
// model through Bubble Tea messages, all of it one undo step.
func BenchmarkMacroReplay(b *testing.B) {
	for _, relative := range []bool{false, true} {
		name := "absolute"
		if relative {
			name = "relative"
		}
		src := macro.Source(recordingHeader(relative), replayActions(1000, relative))
		b.Run(name+"/actions=1000", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				m := newModel()
				m.book().SaveMacro("", sheet.Macro{Name: "Bench", Source: src}, "add")
				m.trustHere()
				mc, _ := m.book().Macro("Bench")
				b.StartTimer()
				run(m, m.runMacro(mc))
				if m.warn != "" {
					b.Fatal(m.warn)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1000, "ns/action")
		})
	}
}

// BenchmarkMacroScript is a script's own loop: 10,000 cells set and read
// back, to show what a call to the spreadsheet costs next to Starlark
// computing on its own.
func BenchmarkMacroScript(b *testing.B) {
	src := "for i in range(1, 5001):\n    set(\"A\" + str(i), i)\n    x = get(\"A\" + str(i))\n"
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		m := newModel()
		m.book().SaveMacro("", sheet.Macro{Name: "Loop", Source: src}, "add")
		m.trustHere()
		mc, _ := m.book().Macro("Loop")
		b.StartTimer()
		run(m, m.runMacro(mc))
		if m.warn != "" {
			b.Fatal(m.warn)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/10000, "ns/call")
}
