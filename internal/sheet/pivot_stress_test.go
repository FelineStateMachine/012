//go:build stress

package sheet_test

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// BenchmarkPivot is a pivot over a full 8192-row table recomputing after
// one edit to its source: the edit, the recalculation and the pivot's
// regrouping and rewriting of its results. The table's columns are an
// id, a category of 8, a number and one of 1000 items, twice.
func BenchmarkPivot(b *testing.B) {
	num := func(col int) sheet.PivotValue { return sheet.PivotValue{Col: col, Summarize: sheet.SumBy} }
	shapes := []struct {
		name string
		make func(src *sheet.Sheet, r sheet.Rect) sheet.Pivot
	}{
		{"rows-8", func(src *sheet.Sheet, r sheet.Rect) sheet.Pivot {
			p := sheet.NewPivot(src, r)
			p.Rows = []sheet.PivotGroup{{Col: 1}}
			p.Values = []sheet.PivotValue{num(2), {Col: 3, Summarize: sheet.CountABy}}
			return p
		}},
		{"rows-8x8-cols-8", func(src *sheet.Sheet, r sheet.Rect) sheet.Pivot {
			p := sheet.NewPivot(src, r)
			p.Rows = []sheet.PivotGroup{{Col: 1}, {Col: 5}}
			p.Columns = []sheet.PivotGroup{{Col: 5}}
			p.Values = []sheet.PivotValue{num(2), {Col: 6, Summarize: sheet.AverageBy}}
			return p
		}},
		{"rows-1000-unique", func(src *sheet.Sheet, r sheet.Rect) sheet.Pivot {
			p := sheet.NewPivot(src, r)
			p.Rows = []sheet.PivotGroup{{Col: 3, Desc: true, SortBy: 1}}
			p.Values = []sheet.PivotValue{num(2), {Col: 7, Summarize: sheet.CountUniqueBy}}
			return p
		}},
		{"frequency-1000", func(src *sheet.Sheet, r sheet.Rect) sheet.Pivot {
			return sheet.FrequencyPivot(src, r, 3)
		}},
	}
	for _, sh := range shapes {
		b.Run(sh.name, func(b *testing.B) {
			src := stress.Table(sheet.MaxRows-1, 8)
			r := sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 7, Row: sheet.MaxRows - 1})
			pv, err := src.Book().CreatePivot(src, r, "", sh.make(src, r))
			if err != nil {
				b.Fatal(err)
			}
			if pv.PivotError() != "" {
				b.Fatal(pv.PivotError())
			}
			edit := sheet.Addr{Col: 2, Row: 4000}
			inputs := [2]string{"1234.5", "4321.5"}
			i := 0
			for b.Loop() {
				if err := src.Set(edit, inputs[i%2]); err != nil {
					b.Fatal(err)
				}
				i++
			}
			out, _ := pv.PivotRange()
			b.ReportMetric(float64(out.To.Row+1), "rows-out")
		})
	}
}
