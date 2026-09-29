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
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// bigRows is the rows of the generated sources.
const bigRows = 10_000_000

// bigRow is a row of them: an id, one of eight categories, an amount
// and a date.
type bigRow struct {
	ID     int64   `parquet:"id"`
	Cat    string  `parquet:"cat,dict"`
	Amount float64 `parquet:"amount"`
	Day    int32   `parquet:"day,date"`
}

var bigCats = []string{"north", "south", "east", "west", "central", "coast", "hills", "islands"}

func bigRowAt(i int) bigRow {
	r := rand.New(rand.NewPCG(uint64(i), 12))
	return bigRow{ID: int64(i), Cat: bigCats[r.IntN(len(bigCats))], Amount: float64(r.IntN(1_000_000)) / 100,
		Day: int32(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()/86400) + int32(r.IntN(2000))}
}

// generated is the directory the sources are kept in between runs.
func generated(b *testing.B) string {
	if d := os.Getenv("STRESS_DIR"); d != "" {
		dir := filepath.Join(d, "generated")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir
		}
	}
	return b.TempDir()
}

// bigParquet writes the Parquet source unless it's there.
func bigParquet(b *testing.B, dir string) string {
	name := filepath.Join(dir, fmt.Sprintf("sales-%d.parquet", bigRows))
	if _, err := os.Stat(name); err == nil {
		return name
	}
	tmp := name + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		b.Fatal(err)
	}
	w := parquet.NewGenericWriter[bigRow](f, parquet.MaxRowsPerRowGroup(1<<20))
	batch := make([]bigRow, 1<<16)
	for i := 0; i < bigRows; i += len(batch) {
		n := min(len(batch), bigRows-i)
		for j := range n {
			batch[j] = bigRowAt(i + j)
		}
		if _, err := w.Write(batch[:n]); err != nil {
			b.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
	f.Close()
	if err := os.Rename(tmp, name); err != nil {
		b.Fatal(err)
	}
	return name
}

// bigSQLite writes the SQLite source unless it's there.
func bigSQLite(b *testing.B, dir string) string {
	name := filepath.Join(dir, fmt.Sprintf("sales-%d.sqlite", bigRows))
	if _, err := os.Stat(name); err == nil {
		return name
	}
	tmp := name + ".part"
	os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp+"?_pragma=journal_mode(OFF)&_pragma=synchronous(OFF)")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	if _, err := tx.Exec("CREATE TABLE sales (id INTEGER, cat TEXT, amount REAL, day DATE)"); err != nil {
		b.Fatal(err)
	}
	ins, err := tx.Prepare("INSERT INTO sales VALUES (?, ?, ?, ?)")
	if err != nil {
		b.Fatal(err)
	}
	for i := range bigRows {
		r := bigRowAt(i)
		day := time.Unix(int64(r.Day)*86400, 0).UTC().Format(time.DateOnly)
		if _, err := ins.Exec(r.ID, r.Cat, r.Amount, day); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	db.Close()
	if err := os.Rename(tmp, name); err != nil {
		b.Fatal(err)
	}
	return name
}

// BenchmarkSource reads ten million rows of each kind of source.
func BenchmarkSource(b *testing.B) {
	dir := generated(b)
	files := map[string]SourceSpec{
		"parquet": {Path: bigParquet(b, dir)},
		"sqlite":  {Path: bigSQLite(b, dir)},
	}
	defer benchLock(b)()
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
