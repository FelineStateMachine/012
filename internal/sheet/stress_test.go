//go:build stress

// Stress benchmarks of the engine: build and full recalc, single-edit
// latency per dependency topology, undo, and whole-sheet operations. They
// only build with -tags stress (see `make stress` and docs/limits.md).
package sheet_test

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

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
	s := stress.Dense(sheet.MaxRows, 26)
	all := sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 25, Row: sheet.MaxRows - 1})
	for b.Loop() {
		s.EraseRange(all)
		s.Undo()
	}
}

// BenchmarkHistoryFull makes MaxUndo whole-column edits, the most the
// history keeps, and reports the heap they hold.
func BenchmarkHistoryFull(b *testing.B) {
	for b.Loop() {
		s := stress.Dense(sheet.MaxRows, 4)
		before := heap()
		for i := range sheet.MaxUndo {
			col := sheet.NewRect(sheet.Addr{Col: i % 4}, sheet.Addr{Col: i % 4, Row: sheet.MaxRows - 1})
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
	all := sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 25, Row: sheet.MaxRows - 1})
	for b.Loop() {
		s := stress.Dense(sheet.MaxRows, 26)
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

func BenchmarkNested(b *testing.B) {
	for _, depth := range []int{100, 1000, 10000} {
		for name, f := range map[string]func(int) string{"paren": stress.Nested, "if": stress.NestedIF} {
			b.Run(fmt.Sprintf("%s-%d", name, depth), func(b *testing.B) {
				s := sheet.New()
				src := f(depth)
				for b.Loop() {
					if err := s.Set(sheet.Addr{Col: 1}, src); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkSort(b *testing.B) {
	s := stress.Table(sheet.MaxRows-1, 8)
	r := sheet.NewRect(sheet.Addr{Row: 1}, sheet.Addr{Col: 7, Row: sheet.MaxRows - 1})
	desc := false
	for b.Loop() {
		s.SortRange(r, []sheet.SortKey{{Col: 2, Desc: desc}})
		desc = !desc
	}
}

func BenchmarkFilter(b *testing.B) {
	s := stress.Table(sheet.MaxRows-1, 8)
	s.CreateFilter(sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 7, Row: sheet.MaxRows - 1}))
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
	s := stress.Dense(sheet.MaxRows, 26)
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
	dst := sheet.NewRect(sheet.Addr{}, sheet.Addr{Row: sheet.MaxRows - 1})
	for b.Loop() {
		if _, err := s.FillSeries(src, dst); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSave writes the native .012 file and reports its size.
func BenchmarkSave(b *testing.B) {
	for _, sh := range saveShapes() {
		b.Run(sh.name, func(b *testing.B) {
			s := sh.build()
			var buf bytes.Buffer
			for b.Loop() {
				buf.Reset()
				if err := s.Write(&buf); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(buf.Len())/(1<<20), "MB-file")
		})
	}
}

// BenchmarkOpen reads the native .012 file back.
func BenchmarkOpen(b *testing.B) {
	for _, sh := range saveShapes() {
		b.Run(sh.name, func(b *testing.B) {
			var buf bytes.Buffer
			if err := sh.build().Write(&buf); err != nil {
				b.Fatal(err)
			}
			for b.Loop() {
				if _, err := sheet.Read(bytes.NewReader(buf.Bytes())); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type namedBuild struct {
	name  string
	build func() *sheet.Sheet
}

func saveShapes() []namedBuild {
	return []namedBuild{
		{"dense-8192x26", func() *sheet.Sheet { return stress.Dense(sheet.MaxRows, 26) }},
		{"dense-8192x256", func() *sheet.Sheet { return stress.Dense(sheet.MaxRows, sheet.MaxCols) }},
		{"chain-8192", func() *sheet.Sheet { return stress.Chain(sheet.MaxRows) }},
	}
}

// BenchmarkMemory reports the live heap per non-blank cell after a load.
func BenchmarkMemory(b *testing.B) {
	for _, sh := range []namedBuild{
		{"numbers", func() *sheet.Sheet { return stress.Dense(sheet.MaxRows, 64) }},
		{"formulas", func() *sheet.Sheet { return stress.Chain(sheet.MaxRows) }},
		{"text-200", func() *sheet.Sheet { return stress.LongText(sheet.MaxRows, 200) }},
	} {
		b.Run(sh.name, func(b *testing.B) {
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

// heap is the live heap after a full collection.
func heap() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}
