package ui

import (
	"flag"
	"fmt"
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// The speed gate (make speed, part of make check): frames and
// recalculation on mid-sized sheets from internal/stress, held to
// testdata/speed.json. See docs/contributing/limits.md#the-speed-gate.
var (
	speedFlag       = flag.Bool("speed", false, "run the speed gate against testdata/speed.json")
	speedUpdateFlag = flag.Bool("speed-update", false, "measure the speed gate and rewrite testdata/speed.json")
)

// speedCase is one measured operation: setup builds its state and
// returns the operation, which must leave the state as it found it
// after an even number of calls.
type speedCase struct {
	name  string
	setup func() func()
}

// frameCase is an arrow key through to its frame at 200 x 60, down and
// up in turn, on the sheet build makes.
func frameCase(name string, build func() *sheet.Sheet) speedCase {
	return speedCase{"frame/" + name, func() func() {
		m := sized(build(), 200, 60)
		t := newFakeTerm(200, 60)
		keys := [2]tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyUp}}
		i := 0
		return func() {
			m.Update(keys[i%2])
			t.frame(m)
			i++
		}
	}}
}

// outputCase is an arrow key through to its frame at 200 x 60 in a
// notebook output's grid of rows rows, entered, down and up in turn,
// halfway down the output.
func outputCase(rows int) speedCase {
	return speedCase{fmt.Sprintf("frame/output-%dx4", rows), func() func() {
		m := outputModel(bigTable(rows), 200, 60)
		m.runCommand("nb.edit")
		g := m.gridIn()
		g.child.cur.Row = rows / 2
		g.child.scrollTo(g.child.cur)
		t := newFakeTerm(200, 60)
		keys := [2]tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyUp}}
		i := 0
		return func() {
			m.Update(keys[i%2])
			t.frame(m)
			i++
		}
	}}
}

// sourceFrameCase is Page Down and Page Up in turn through to their
// frames at 200 x 60, halfway down a linked source's tab, the pages
// they show already read, as scrolling within what's read is.
func sourceFrameCase() speedCase {
	return speedCase{fmt.Sprintf("frame/source-%dx4", speedSourceRows), func() func() {
		m := speedSourceModel()
		m.srcView().MoveTo(speedSourceRows/2, 2)
		keys := [2]tea.KeyPressMsg{{Code: tea.KeyPgDown}, {Code: tea.KeyPgUp}}
		for _, k := range keys { // both windows read
			m.Update(k)
			settleSources(m)
		}
		t := newFakeTerm(200, 60)
		i := 0
		return func() {
			m.Update(keys[i%2])
			t.frame(m)
			i++
		}
	}}
}

// sourceSumCase is a linked source read again and a SUM over one of its
// columns worked out again, as the file changing makes it, run to the
// end rather than in the background.
func sourceSumCase() speedCase {
	return speedCase{fmt.Sprintf("source/sum-%d", speedSourceRows), func() func() {
		m := speedSourceModel()
		info, _ := m.sheet.Source()
		m.showSheet(m.book().Sheet(0))
		m.sheet.Set(sheet.Addr{}, "=SUM("+info.Name+"[amount])")
		settleSources(m)
		return func() {
			m.sources().host.Reload(info.Name)
			if !settleSources(m) || m.sheet.Value(sheet.Addr{}).Kind != sheet.Number {
				panic("the SUM over the source never settled")
			}
		}
	}}
}

// speedSourceModel is a model showing the speed gate's source.
func speedSourceModel() *Model {
	path, err := speedSources()
	if err != nil {
		panic(err)
	}
	return sourceModel(path, 200, 60)
}

// editCase is typing into one cell of a stress shape and the
// incremental recalculation that follows, two entries in turn.
func editCase(sh stress.Shape) speedCase {
	return speedCase{"edit/" + sh.Name, func() func() {
		s := sh.Build()
		inputs := [2]string{sh.Input, sh.Input + "1"}
		i := 0
		return func() {
			if err := s.Set(sh.Edit, inputs[i%2]); err != nil {
				panic(err)
			}
			i++
		}
	}}
}

// recalcCase is a full recalculation of the sheet build makes.
func recalcCase(name string, build func() *sheet.Sheet) speedCase {
	return speedCase{"recalc/" + name, func() func() {
		s := build()
		return s.RecalcAll
	}}
}

func speedCases() []speedCase {
	dense := func() *sheet.Sheet { return stress.Dense(stress.Rows, 26) }
	laidout := func() *sheet.Sheet { return stress.Laidout(stress.Rows, 26) }
	charts := func() *sheet.Sheet {
		s := stress.Table(200, 3)
		stress.Charts(s, 8)
		return s
	}
	shape := func(name string, build func() *sheet.Sheet, a string, input string) stress.Shape {
		at, _ := sheet.ParseAddr(a)
		return stress.Shape{Name: name, Build: build, Edit: at, Input: input}
	}
	fanin := shape("fanin-1000xSUM8192", func() *sheet.Sheet { return stress.FanIn(stress.Rows, 1000) }, "A4001", "7")
	return []speedCase{
		frameCase("dense-8192x26", dense),
		frameCase("laidout-8192x26", laidout),
		frameCase("scale-8192x26", scaled),
		frameCase("charts-8", charts),
		outputCase(100000),
		sourceFrameCase(),
		sourceSumCase(),
		editCase(fanin),
		editCase(shape("chain-8192", func() *sheet.Sheet { return stress.Chain(stress.Rows) }, "A1", "2")),
		editCase(shape("criteria-10xSUMIF8192", func() *sheet.Sheet { return stress.Criteria(stress.Rows, 10) }, "A4001", "7")),
		editCase(shape("lookup-100xVLOOKUP8192", func() *sheet.Sheet { return stress.Lookup(stress.Rows, 100) }, "A4001", "4000")),
		recalcCase("running-8192", func() *sheet.Sheet { return stress.RunningTotals(stress.Rows) }),
		recalcCase("arrays-50xFILTER8192", func() *sheet.Sheet { return stress.Arrays(stress.Rows, 50) }),
	}
}

// TestSpeed measures every case and compares it with the baseline:
// allocations per operation, and time per operation relative to a
// calibration workload timed beside it. A case over its margin is
// measured once more before it fails, so one noisy run doesn't.
func TestSpeed(t *testing.T) {
	if !*speedFlag && !*speedUpdateFlag {
		t.Skip("the speed gate runs with -speed (make speed)")
	}
	unlock := benchLock(t)
	defer unlock()
	base, err := readSpeedBaseline()
	if err != nil && !*speedUpdateFlag {
		t.Fatal(err)
	}
	next := speedBaseline{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Cases: map[string]speedResult{}}
	for _, c := range speedCases() {
		op := c.setup()
		got := measureSpeed(op)
		if !*speedUpdateFlag {
			if verdict := base.judge(c.name, got); verdict != "" {
				got = measureSpeed(op) // a second chance, against noise
				if verdict = base.judge(c.name, got); verdict != "" {
					t.Errorf("%s: %s", c.name, verdict)
				}
			}
		}
		t.Logf("%-36s %8.3f ms %6.2fx calibration %8.0f allocs", c.name, got.NS/1e6, got.Ratio, got.Allocs)
		next.Cases[c.name] = got
	}
	if *speedUpdateFlag {
		if err := next.write(); err != nil {
			t.Fatal(err)
		}
		fmt.Println("wrote", speedBaselinePath)
	}
}
