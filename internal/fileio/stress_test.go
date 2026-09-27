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
	"strconv"
	"testing"

	"012/internal/sheet"
	"012/internal/stress"
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
			var res *Result
			for b.Loop() {
				if res, err = Import(context.Background(), path, opt); err != nil {
					b.Fatal(err)
				}
			}
			b.SetBytes(st.Size())
			cells := res.Sheet.Len()
			b.ReportMetric(float64(cells), "cells")
			b.ReportMetric(float64(cells)*float64(b.N)/b.Elapsed().Seconds(), "cells/s")
			res = nil
			before := liveHeap()
			res, _ = Import(context.Background(), path, opt)
			b.ReportMetric(float64(liveHeap()-before)/float64(max(cells, 1)), "B/cell")
			runtime.KeepAlive(res)
		})
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
