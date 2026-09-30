//go:build stress

// Stress benchmarks of sources: a Parquet file and a SQLite table of ten
// million rows, generated once into $STRESS_DIR/generated (or a
// temporary directory), read in place: opening, summing a column as SUM
// does, a page deep in the file, and building and paging a sorted and a
// filtered view, each with the peak heap it took. Timed under the bench
// lock (/tmp/012-bench.lock), as the speed gate is.
package fileio

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// bigRows is the rows of the generated sources (stress.SalesParquet).
const bigRows = 10_000_000

// BenchmarkSource reads ten million rows of each kind of source.
func BenchmarkSource(b *testing.B) {
	dir := stress.GeneratedDir(b.TempDir())
	pq, err := stress.SalesParquet(dir, bigRows)
	if err != nil {
		b.Fatal(err)
	}
	db, err := stress.SalesSQLite(dir, bigRows)
	if err != nil {
		b.Fatal(err)
	}
	files := map[string]SourceSpec{"parquet": {Path: pq}, "sqlite": {Path: db}}
	defer stress.BenchLock(b)()
	for _, kind := range []string{"parquet", "sqlite"} {
		spec := files[kind]
		spec.TempDir = b.TempDir()
		b.Run(kind, func(b *testing.B) { benchSource(b, spec) })
	}
}

// sortByAmount and northOnly are the views measured.
var (
	sortByAmount = SourceOrder{Sort: []SourceSort{{Col: 2, Desc: true}}}
	northOnly    = SourceOrder{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondExactly, Arg: "north"}}}}
)

func benchSource(b *testing.B, spec SourceSpec) {
	ctx := context.Background()
	b.Run("open", func(b *testing.B) {
		measure(b, func() {
			src, err := OpenSource(ctx, spec)
			if err != nil {
				b.Fatal(err)
			}
			if src.Rows() != bigRows {
				b.Fatalf("%d rows", src.Rows())
			}
			src.Close()
		})
	})
	src, err := OpenSource(ctx, spec)
	if err != nil {
		b.Fatal(err)
	}
	defer src.Close()
	b.Run("sum-column", func(b *testing.B) {
		measure(b, func() {
			total := 0.0
			err := src.Scan(ctx, 0, []int{2}, func(_ int64, v []sheet.LiveCell) bool {
				total += v[0].V.Num
				return true
			})
			if err != nil {
				b.Fatal(err)
			}
		})
		b.ReportMetric(float64(bigRows)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
	})
	for _, o := range []struct {
		name  string
		order SourceOrder
	}{{"plain", SourceOrder{}}, {"sorted", sortByAmount}, {"filtered", northOnly}} {
		var v SourceView
		b.Run("view-"+o.name, func(b *testing.B) {
			measure(b, func() {
				if v != nil {
					v.Close()
				}
				if v, err = src.View(ctx, o.order); err != nil {
					b.Fatal(err)
				}
			})
			b.ReportMetric(float64(v.Rows()), "rows")
		})
		b.Run("page-"+o.name, func(b *testing.B) {
			r := rand.New(rand.NewPCG(1, 2))
			measure(b, func() {
				from := v.Rows()/2 + r.Int64N(v.Rows()/2-60)
				nums, _, err := v.Page(ctx, from, 60)
				if err != nil || len(nums) != 60 {
					b.Fatalf("page at %d: %d rows, %v", from, len(nums), err)
				}
			})
		})
		v.Close()
	}
}

// measure runs fn in the benchmark's loop, reporting the peak heap it
// took above what was live before.
func measure(b *testing.B, fn func()) {
	before := liveHeap()
	stop := samplePeak()
	for b.Loop() {
		fn()
	}
	peak := stop()
	b.ReportMetric(float64(peak-min(peak, before))/(1<<20), "peak-MB")
}
