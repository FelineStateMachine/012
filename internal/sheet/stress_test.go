//go:build stress

// Stress benchmarks of the engine: build and full recalc, single-edit
// latency per dependency topology, undo, and whole-sheet operations. They
// only build with -tags stress (see `make stress` and docs/contributing/limits.md).
package sheet_test

import (
	"bytes"
	"fmt"
	"runtime"
	"runtime/metrics"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

func BenchmarkBuild(b *testing.B) {
	for _, sh := range stress.Shapes() {
		b.Run(sh.Name, func(b *testing.B) {
			for b.Loop() {
				sh.Build()
			}
		})
	}
}

func BenchmarkRecalcAll(b *testing.B) {
	for _, sh := range stress.Shapes() {
		b.Run(sh.Name, func(b *testing.B) {
			s := sh.Build()
			for b.Loop() {
				s.RecalcAll()
			}
		})
	}
}

// BenchmarkEdit is the engine's share of typing one entry: storing it and
// the incremental recalculation it triggers.
func BenchmarkEdit(b *testing.B) {
	for _, sh := range stress.Shapes() {
		b.Run(sh.Name, func(b *testing.B) {
			s := sh.Build()
			inputs := [2]string{sh.Input, sh.Input + "1"}
			i := 0
			for b.Loop() {
				if err := s.Set(sh.Edit, inputs[i%2]); err != nil {
					b.Fatal(err)
				}
				i++
			}
		})
	}
}

// BenchmarkUndo is one undo and one redo of a single-cell edit.
func BenchmarkUndo(b *testing.B) {
	for _, sh := range stress.Shapes() {
		b.Run(sh.Name, func(b *testing.B) {
			s := sh.Build()
			if err := s.Set(sh.Edit, sh.Input+"1"); err != nil {
				b.Fatal(err)
			}
			for b.Loop() {
				s.Undo()
				s.Redo()
			}
		})
	}
}

// BenchmarkBigUndo clears a dense 8192x26 block and undoes it: the
// largest single step a user makes by hand.
func BenchmarkBigUndo(b *testing.B) {
	s := stress.Dense(stress.Rows, 26)
	all := sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 25, Row: stress.Rows - 1})
	for b.Loop() {
		s.EraseRange(all)
		s.Undo()
	}
}

// BenchmarkClearMax clears all of a full max-cells sheet of numbers (1M
// x 10) as one step and undoes it, then reports the heap such a step
// holds (what forgetting it frees) and the budget's estimate of it.
func BenchmarkClearMax(b *testing.B) {
	s := stress.Dense(stress.MaxRows, stress.MaxCols)
	all := sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: stress.MaxCols - 1, Row: stress.MaxRows - 1})
	s.ClearHistory()
	for b.Loop() {
		s.EraseRange(all)
		s.Undo()
		s.ClearHistory()
	}
	s.EraseRange(all)
	est := s.Book().HistoryBytes()
	held := heap()
	s.ClearHistory()
	b.ReportMetric(float64(held-heap())/(1<<20), "MB-held")
	b.ReportMetric(float64(est)/(1<<20), "MB-estimate")
}

// BenchmarkHistoryFull makes MaxUndo whole-column edits, the most the
// history keeps, and reports the heap they hold.
func BenchmarkHistoryFull(b *testing.B) {
	for b.Loop() {
		s := stress.Dense(stress.Rows, 4)
		before := heap()
		for i := range sheet.MaxUndo {
			col := sheet.NewRect(sheet.Addr{Col: i % 4}, sheet.Addr{Col: i % 4, Row: stress.Rows - 1})
			if err := s.FillEntry(col, col.From, fmt.Sprint(i)); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(heap()-before)/(1<<20), "MB-history")
		b.ReportMetric(float64(s.Book().HistoryBytes())/(1<<20), "MB-estimate")
		runtime.KeepAlive(s)
	}
}

// BenchmarkHistoryWide makes 12 edits that each rewrite a whole
// 8192x26 block, more than the history's byte budget holds, and reports
// the heap the history keeps.
func BenchmarkHistoryWide(b *testing.B) {
	all := sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 25, Row: stress.Rows - 1})
	for b.Loop() {
		s := stress.Dense(stress.Rows, 26)
		before := heap()
		for i := range 12 {
			if err := s.FillEntry(all, all.From, fmt.Sprint(i)); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(heap()-before)/(1<<20), "MB-history")
		b.ReportMetric(float64(s.Book().HistoryBytes())/(1<<20), "MB-estimate")
		runtime.KeepAlive(s)
	}
}

// BenchmarkNested enters deeply nested formulas: within the parser's cap
// (formula.MaxDepth, 1024) they're parsed and evaluated, past it refused.
func BenchmarkNested(b *testing.B) {
	for _, depth := range []int{100, 1000, 10000} {
		for name, f := range map[string]func(int) string{"paren": stress.Nested, "if": stress.NestedIF} {
			b.Run(fmt.Sprintf("%s-%d", name, depth), func(b *testing.B) {
				s := sheet.New()
				src := f(depth)
				for b.Loop() {
					if err := s.Set(sheet.Addr{Col: 1}, src); (err != nil) != (depth > 1000) {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkSort(b *testing.B) {
	s := stress.Table(stress.Rows-1, 8)
	r := sheet.NewRect(sheet.Addr{Row: 1}, sheet.Addr{Col: 7, Row: stress.Rows - 1})
	desc := false
	for b.Loop() {
		s.SortRange(r, []sheet.SortKey{{Col: 2, Desc: desc}})
		desc = !desc
	}
}

func BenchmarkFilter(b *testing.B) {
	s := stress.Table(stress.Rows-1, 8)
	s.CreateFilter(sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 7, Row: stress.Rows - 1}))
	b.Run("apply", func(b *testing.B) {
		arg := [2]string{"5000", "2500"}
		i := 0
		for b.Loop() {
			s.FilterColumn(2, sheet.Criteria{Cond: sheet.Condition{Op: sheet.CondGreater, Arg: arg[i%2]}})
			s.HiddenRows()
			i++
		}
	})
	b.Run("values", func(b *testing.B) {
		for b.Loop() {
			s.FilterValues(3)
		}
	})
}

func BenchmarkFind(b *testing.B) {
	s := stress.Dense(stress.Rows, 26)
	b.Run("find", func(b *testing.B) {
		for b.Loop() {
			if _, err := s.Find("99.5", sheet.FindOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("replace-all", func(b *testing.B) {
		q := [2]string{".", ","}
		i := 0
		for b.Loop() {
			if _, err := s.ReplaceAll(q[i%2], q[(i+1)%2], sheet.FindOptions{}); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

func BenchmarkFillSeries(b *testing.B) {
	s := sheet.New()
	s.Set(sheet.Addr{}, "1")
	s.Set(sheet.Addr{Row: 1}, "2")
	src := sheet.NewRect(sheet.Addr{}, sheet.Addr{Row: 1})
	dst := sheet.NewRect(sheet.Addr{}, sheet.Addr{Row: stress.Rows - 1})
	for b.Loop() {
		if _, err := s.FillSeries(src, dst); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSave writes the native .012 file and reports its size and
// the most heap in use while writing it, above what the sheet holds.
func BenchmarkSave(b *testing.B) {
	for _, sh := range saveShapes() {
		b.Run(sh.name, func(b *testing.B) {
			s := sh.build()
			var out counter
			peak := peakHeap(func() {
				for b.Loop() {
					out = 0
					if err := s.Write(&out); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.ReportMetric(float64(out)/(1<<20), "MB-file")
			b.ReportMetric(float64(peak)/(1<<20), "MB-peak")
		})
	}
}

// BenchmarkOpen reads the native .012 file back, and reports the most
// heap in use while reading it, the sheet read included.
func BenchmarkOpen(b *testing.B) {
	for _, sh := range saveShapes() {
		b.Run(sh.name, func(b *testing.B) {
			var buf bytes.Buffer
			if err := sh.build().Write(&buf); err != nil {
				b.Fatal(err)
			}
			peak := peakHeap(func() {
				for b.Loop() {
					if _, err := sheet.Read(bytes.NewReader(buf.Bytes())); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.ReportMetric(float64(peak)/(1<<20), "MB-peak")
		})
	}
}

// counter is a writer that keeps only the count of bytes written.
type counter int64

func (c *counter) Write(p []byte) (int, error) {
	*c += counter(len(p))
	return len(p), nil
}

// peakHeap runs fn and returns the most heap in use meanwhile above what
// was live before, sampled every millisecond.
func peakHeap(fn func()) uint64 {
	base := heap()
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	var peak atomic.Uint64
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		t := time.NewTicker(time.Millisecond)
		defer t.Stop()
		for {
			metrics.Read(sample)
			if v := sample[0].Value.Uint64(); v > peak.Load() {
				peak.Store(v)
			}
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	})
	fn()
	close(done)
	wg.Wait()
	return peak.Load() - min(base, peak.Load())
}

type namedBuild struct {
	name  string
	build func() *sheet.Sheet
}

func saveShapes() []namedBuild {
	return []namedBuild{
		{"dense-8192x26", func() *sheet.Sheet { return stress.Dense(stress.Rows, 26) }},
		{"dense-8192x256", func() *sheet.Sheet { return stress.Dense(stress.Rows, stress.Cols) }},
		{"chain-8192", func() *sheet.Sheet { return stress.Chain(stress.Rows) }},
		{"dense-1Mx10", func() *sheet.Sheet { return stress.Dense(stress.MaxRows, stress.MaxCols) }},
	}
}

// BenchmarkMemory reports the live heap per non-blank cell after a load:
// built through Load, or opened from a .012 file written beforehand.
func BenchmarkMemory(b *testing.B) {
	for _, sh := range []struct {
		name  string
		prep  func()
		build func() *sheet.Sheet
	}{
		{"numbers", nil, func() *sheet.Sheet { return stress.Dense(stress.Rows, 64) }},
		{"numbers-1Mx10", nil, func() *sheet.Sheet { return stress.Dense(stress.MaxRows, stress.MaxCols) }},
		{"formulas", nil, func() *sheet.Sheet { return stress.Chain(stress.Rows) }},
		{"text-200", nil, func() *sheet.Sheet { return stress.LongText(stress.Rows, 200) }},
		{"open-8192x256", func() { denseFile() }, func() *sheet.Sheet {
			s, err := sheet.Read(bytes.NewReader(denseFile()))
			if err != nil {
				b.Fatal(err)
			}
			return s
		}},
	} {
		b.Run(sh.name, func(b *testing.B) {
			if sh.prep != nil {
				sh.prep()
			}
			for b.Loop() {
				before := heap()
				s := sh.build()
				after := heap()
				b.ReportMetric(float64(after-before)/float64(s.Len()), "B/cell")
				runtime.KeepAlive(s)
			}
		})
	}
}

// denseFile is the .012 file of 8192x256 numbers.
var denseFile = sync.OnceValue(func() []byte {
	var buf bytes.Buffer
	if err := stress.Dense(stress.Rows, stress.Cols).Write(&buf); err != nil {
		panic(err)
	}
	return buf.Bytes()
})

// BenchmarkRead is the cost of reading a cell's value, per cell of 8192
// x 26 numbers: through Sheet.Value, and as formulas read cells in a
// recalculation (a SUM of each column, read directly).
func BenchmarkRead(b *testing.B) {
	s := stress.Dense(stress.Rows, 26)
	cells := float64(stress.Rows * 26)
	b.Run("value", func(b *testing.B) {
		sum := 0.0
		for b.Loop() {
			for row := range stress.Rows {
				for col := range 26 {
					sum += s.Value(sheet.Addr{Col: col, Row: row}).Num
				}
			}
		}
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/cells, "ns/cell")
	})
	b.Run("recalc", func(b *testing.B) {
		for col := range 26 {
			name := sheet.ColName(col)
			if err := s.Set(sheet.Addr{Col: col, Row: stress.Rows}, fmt.Sprintf("=SUM(%s1:%s%d)", name, name, stress.Rows)); err != nil {
				b.Fatal(err)
			}
		}
		for b.Loop() {
			s.RecalcAll()
		}
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/cells, "ns/cell")
	})
}

// heap is the live heap after a full collection.
func heap() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}
