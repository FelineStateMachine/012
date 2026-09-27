//go:build stress

// Stress benchmarks of import and export: the real datasets fetched by
// scripts/stress-data.sh into .deps/stress (skipped when missing), and
// synthetic sheets written in every export format. Only built with
// -tags stress (see `make stress` and docs/limits.md).
package fileio

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"strconv"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// stressDir is where scripts/stress-data.sh puts the datasets.
func stressDir() string {
	if d := os.Getenv("STRESS_DIR"); d != "" {
		return d
	}
	return filepath.Join("..", "..", ".deps", "stress")
}

// datasets reads the manifest's file names.
func datasets(b *testing.B) []string {
	f, err := os.Open(filepath.Join("..", "..", "scripts", "stress-data.tsv"))
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma, r.LazyQuotes, r.FieldsPerRecord = '\t', true, -1
	rows, err := r.ReadAll()
	if err != nil {
		b.Fatal(err)
	}
	var names []string
	for _, row := range rows[1:] {
		names = append(names, row[0])
	}
	return names
}

// BenchmarkImport imports each dataset, reporting throughput, cells
// kept, and the live heap of the imported sheet per cell.
func BenchmarkImport(b *testing.B) {
	for _, name := range datasets(b) {
		path := filepath.Join(stressDir(), name)
		b.Run(name, func(b *testing.B) {
			st, err := os.Stat(path)
			if err != nil {
				b.Skip("run scripts/stress-data.sh first")
			}
			opt := Options{}
			if k, _ := KindOf(name); k == SQLite {
				opt.Table = "PlaylistTrack" // the biggest table
			}
			benchImport(b, path, st.Size(), opt)
		})
	}
}

// BenchmarkImportXLSX imports synthetic workbooks written by 012's
// exporter: numbers, text and a column of formulas, each a full 8192
// rows.
func BenchmarkImportXLSX(b *testing.B) {
	for _, tc := range []struct {
		name string
		s    *sheet.Sheet
	}{
		{"numbers-8192x26", stress.Dense(sheet.MaxRows, 26)},
		{"table-8192x26", stress.Table(sheet.MaxRows-1, 26)},
		{"formulas-8192", stress.Chain(sheet.MaxRows)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), tc.name+".xlsx")
			if _, err := Export(context.Background(), path, XLSX, Snap(tc.s, sheet.Rect{}, "data"), ExportOptions{}); err != nil {
				b.Fatal(err)
			}
			st, err := os.Stat(path)
			if err != nil {
				b.Fatal(err)
			}
			benchImport(b, path, st.Size(), Options{})
		})
	}
}

// benchImport imports path in a loop and reports throughput, cells kept,
// the live heap of the imported sheet per cell (B/cell) and the peak
// heap while importing, per cell (peak-B/cell), sampled.
func benchImport(b *testing.B, path string, size int64, opt Options) {
	var res *Result
	var err error
	for b.Loop() {
		if res, err = Import(context.Background(), path, opt); err != nil {
			b.Fatal(err)
		}
	}
	b.SetBytes(size)
	cells := res.Sheet.Len()
	b.ReportMetric(float64(cells), "cells")
	b.ReportMetric(float64(cells)*float64(b.N)/b.Elapsed().Seconds(), "cells/s")
	res = nil
	before := liveHeap()
	stop := samplePeak()
	res, _ = Import(context.Background(), path, opt)
	peak := stop()
	b.ReportMetric(float64(liveHeap()-before)/float64(max(cells, 1)), "B/cell")
	b.ReportMetric(float64(peak-min(peak, before))/float64(max(cells, 1)), "peak-B/cell")
	runtime.KeepAlive(res)
}

// samplePeak samples the heap every 100 microseconds until the returned
// function is called, which returns the most seen: live objects and
// garbage not yet collected, what the process holds.
func samplePeak() func() uint64 {
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	read := func() uint64 {
		metrics.Read(sample)
		return sample[0].Value.Uint64()
	}
	done := make(chan struct{})
	peak := make(chan uint64)
	go func() {
		hi := read()
		t := time.NewTicker(100 * time.Microsecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				peak <- max(hi, read())
				return
			case <-t.C:
				hi = max(hi, read())
			}
		}
	}()
	return func() uint64 {
		close(done)
		return <-peak
	}
}

// BenchmarkExport writes a dense numeric sheet in each export format and
// reports the file size.
func BenchmarkExport(b *testing.B) {
	for _, size := range []struct{ rows, cols int }{{sheet.MaxRows, 26}, {sheet.MaxRows, 256}} {
		s := stress.Dense(size.rows, size.cols)
		snap := Snap(s, sheet.Rect{}, "data")
		for _, k := range []Kind{CSV, TSV, XLSX, SQLite} {
			b.Run(k.String()+"/"+strconv.Itoa(size.rows)+"x"+strconv.Itoa(size.cols), func(b *testing.B) {
				path := filepath.Join(b.TempDir(), "out"+k.Ext())
				for b.Loop() {
					os.Remove(path)
					if _, err := Export(context.Background(), path, k, snap, ExportOptions{Table: "data"}); err != nil {
						b.Fatal(err)
					}
				}
				if st, err := os.Stat(path); err == nil {
					b.ReportMetric(float64(st.Size())/(1<<20), "MB-file")
				}
			})
		}
	}
}

func liveHeap() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}
