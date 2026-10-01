//go:build stress

// Stress benchmarks of formulas and pivot tables over linked sources of
// ten million rows (stress.SalesParquet, stress.SalesSQLite): each
// question answered from scratch by a job, as the screen's host runs it
// in the background, with the peak heap it took. Timed under the bench
// lock.
package paged

import (
	"context"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// bigRows is the rows of the generated sources.
const bigRows = 10_000_000

// bigFormulas are the formulas measured, reading the source as sales:
// A id, B cat, C amount, D day.
var bigFormulas = []struct{ name, formula string }{
	{"SUM", "=SUM(sales[amount])"},
	{"AVERAGE", "=AVERAGE(sales[amount])"},
	{"COUNTIFS", `=COUNTIFS(sales[cat],"north",sales[amount],">5000")`},
	{"SUMIFS", `=SUMIFS(sales[amount],sales[cat],"east")`},
	{"SUMPRODUCT", "=SUMPRODUCT(sales[amount],sales[id])"},
	{"XLOOKUP-last", "=XLOOKUP(9999999,sales[id],sales[amount])"},
	{"MATCH-middle", "=MATCH(5000000,sales[id],0)"},
	{"MEDIAN", "=MEDIAN(sales[amount])"},
	{"PERCENTILE", "=PERCENTILE(sales[amount],0.9)"},
	{"MODE", "=MODE(sales[amount])"},
	{"LARGE", "=LARGE(sales[amount],1000)"},
	{"RANK", "=RANK(5000,sales[amount])"},
	{"STDEV-held", "=STDEV(sales[amount])"},
}

func BenchmarkSourceFormulas(b *testing.B) {
	dir := stress.GeneratedDir(b.TempDir())
	pq, err := stress.SalesParquet(dir, bigRows)
	if err != nil {
		b.Fatal(err)
	}
	db, err := stress.SalesSQLite(dir, bigRows)
	if err != nil {
		b.Fatal(err)
	}
	defer stress.BenchLock(b)()
	for _, src := range []struct{ kind, path string }{{"parquet", pq}, {"sqlite", db}} {
		b.Run(src.kind, func(b *testing.B) {
			for _, f := range bigFormulas {
				b.Run(f.name, func(b *testing.B) { benchFormula(b, src.path, f.formula) })
			}
			b.Run("pivot-cat", func(b *testing.B) { benchPivot(b, src.path) })
		})
	}
}

// bigBook is a workbook linking the source at path as sales, opened by
// a host of its own.
func bigBook(b *testing.B, path string) (*sheet.Workbook, *Host) {
	w := sheet.NewBook()
	if _, err := w.AddSource("sales", sheet.LinkSource{Path: path}, w.Sheet(0)); err != nil {
		b.Fatal(err)
	}
	h := NewHost(sheet.DefaultMaxCells)
	h.Settle(context.Background(), w, Links(w, same, b.TempDir()))
	if info, _ := w.LookupSource("sales"); info.Shape.Rows != bigRows {
		b.Fatalf("the source has %d rows: %s", info.Shape.Rows, info.Err)
	}
	return w, h
}

// benchFormula times the job answering formula over the source.
func benchFormula(b *testing.B, path, formula string) {
	w, h := bigBook(b, path)
	defer h.Close()
	s := w.Sheet(0)
	var v sheet.Value
	measure(b, func() {
		h.forget("sales")
		s.Set(sheet.Addr{}, "")
		s.Set(sheet.Addr{}, formula)
		h.Settle(context.Background(), w, nil)
		v = s.Value(sheet.Addr{})
	})
	if sheet.IsPending(v) {
		b.Fatalf("%s never answered", formula)
	}
	b.ReportMetric(float64(bigRows)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
}

// benchPivot times a pivot table of the amounts by category.
func benchPivot(b *testing.B, path string) {
	w, h := bigBook(b, path)
	defer h.Close()
	info, _ := w.LookupSource("sales")
	r := sheet.Rect{To: sheet.Addr{Col: 3, Row: sheet.MaxRows - 1}}
	p := sheet.NewPivot(info.Sheet, r)
	p.Rows = []sheet.PivotGroup{{Col: 1}}
	p.Values = []sheet.PivotValue{{Col: 2, Summarize: sheet.SumBy}, {Col: 2, Summarize: sheet.CountBy}}
	measure(b, func() {
		h.forget("sales")
		pt, err := w.CreatePivot(info.Sheet, r, "", p)
		if err != nil {
			b.Fatal(err)
		}
		h.Settle(context.Background(), w, nil)
		if v := pt.Value(sheet.Addr{Row: 9}); v.Kind != sheet.Text {
			b.Fatalf("no Grand Total row: %v", v)
		}
		w.DeleteSheet(pt)
	})
}

// measure runs fn in the benchmark's loop, reporting the peak heap it
// took above what was live before.
func measure(b *testing.B, fn func()) {
	before := stress.LiveHeap()
	stop := stress.SamplePeak()
	for b.Loop() {
		fn()
	}
	peak := stop()
	b.ReportMetric(float64(peak-min(peak, before))/(1<<20), "peak-MB")
}
