//go:build stress

package sheet_test

import (
	"runtime"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// BenchmarkMemoryDerived is the heap a derived cell holds: the cells a
// formula spills (SEQUENCE over 8192 x 64, the array the anchor keeps
// to compare with the next one reported apart) and a pivot's results
// (a row per id of a 8191-row table, with a SUM and a COUNTA).
func BenchmarkMemoryDerived(b *testing.B) {
	b.Run("spilled-8192x64", func(b *testing.B) {
		for b.Loop() {
			before := heap()
			s := sheet.New()
			if err := s.Set(sheet.Addr{}, "=SEQUENCE(8192, 64)"); err != nil {
				b.Fatal(err)
			}
			after := heap()
			n := float64(s.Len())
			const valueBytes = 32 // a Value in the array the anchor keeps
			b.ReportMetric(float64(after-before)/n, "B/cell")
			b.ReportMetric(float64(after-before)/n-valueBytes, "B/cell-stored")
			runtime.KeepAlive(s)
		}
	})
	b.Run("pivot-8191", func(b *testing.B) {
		for b.Loop() {
			src := stress.Table(stress.Rows-1, 8)
			r := sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 7, Row: stress.Rows - 1})
			p := sheet.NewPivot(src, r)
			p.Rows = []sheet.PivotGroup{{Col: 0}}
			p.Values = []sheet.PivotValue{{Col: 2, Summarize: sheet.SumBy}, {Col: 3, Summarize: sheet.CountABy}}
			before := heap()
			pv, err := src.Book().CreatePivot(src, r, "", p)
			if err != nil {
				b.Fatal(err)
			}
			after := heap()
			b.ReportMetric(float64(after-before)/float64(pv.Len()), "B/cell")
			runtime.KeepAlive(pv)
		}
	})
}
