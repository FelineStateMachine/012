//go:build stress

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/live"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// BenchmarkFollow follows a CSV log while another goroutine appends
// 10,000 rows a second to it for three seconds, polling as the UI does
// (every live.Interval, at once while more is waiting), applying each
// update to the workbook, with formulas over the region, and drawing a
// frame after each. It reports the slowest and the 99th percentile of
// an update applied (writing the cells and recalculating what reads
// them) and of a frame, and the rows a second that came in: an update
// must stay a small part of a frame's budget, however fast the file
// grows. The window cases keep the last rows, rewriting the region on
// every update; "all" keeps every row.
func BenchmarkFollow(b *testing.B) {
	for _, window := range []int{0, 1000, 10000} {
		name := "all"
		if window > 0 {
			name = fmt.Sprintf("window-%d", window)
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				r := followRun(b, window)
				b.ReportMetric(r.applyMax, "apply-max-ms")
				b.ReportMetric(r.applyP99, "apply-p99-ms")
				b.ReportMetric(r.frameMax, "frame-max-ms")
				b.ReportMetric(r.frameP99, "frame-p99-ms")
				b.ReportMetric(r.rate, "rows/s")
			}
		})
	}
}

type followResult struct{ applyMax, applyP99, frameMax, frameP99, rate float64 }

const (
	followRate     = 10000 // rows appended a second
	followSeconds  = 3
	followBatchGap = 10 * time.Millisecond
)

func followRun(b *testing.B, window int) followResult {
	b.StopTimer()
	path := filepath.Join(b.TempDir(), "access.csv")
	os.WriteFile(path, []byte("time,level,path,ms\n"), 0o644)
	m := sized(sheet.New(), 200, 60)
	id, err := m.sheet.AddLinked(sheet.Addr{}, sheet.LinkSource{Path: "access.csv", Window: window})
	if err != nil {
		b.Fatal(err)
	}
	m.sheet.Set(sheet.Addr{Col: 6}, "=SUM(D:D)")
	m.sheet.Set(sheet.Addr{Col: 6, Row: 1}, `=COUNTIF(B:B,"error")`)
	m.sheet.Set(sheet.Addr{Col: 6, Row: 2}, "=AVERAGE(D2:D100000)")
	src := live.NewFile("access.csv", path, fileio.CSV, fileio.Options{})
	defer src.Close()
	t := newFakeTerm(200, 60)
	done := make(chan int)
	go appendLog(path, done)
	b.StartTimer()

	var applies, frames []float64
	start, total := time.Now(), 0
	writing := true
	for {
		select {
		case total = <-done:
			writing = false
		default:
		}
		u, ok := src.Poll(context.Background())
		if ok {
			at := time.Now()
			if _, err := m.book().ApplyLive(u.Op(id)); err != nil {
				b.Fatal(err)
			}
			applies = append(applies, ms(time.Since(at)))
			at = time.Now()
			t.frame(m)
			frames = append(frames, ms(time.Since(at)))
		}
		switch {
		case u.More:
		case writing:
			time.Sleep(live.Interval)
		default:
			if r, _ := m.book().LinkedRegion(id); r.Rows+r.Dropped < total {
				continue
			}
			return followResult{applyMax: slices.Max(applies), applyP99: p99(applies), frameMax: slices.Max(frames),
				frameP99: p99(frames), rate: float64(total) / time.Since(start).Seconds()}
		}
	}
}

// appendLog appends followRate rows a second to path, in batches, for
// followSeconds, then sends how many it wrote.
func appendLog(path string, done chan<- int) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	levels := []string{"info", "info", "info", "warn", "error"}
	per := followRate * int(followBatchGap) / int(time.Second)
	n := 0
	for range followSeconds * int(time.Second/followBatchGap) {
		var batch []byte
		for range per {
			batch = fmt.Appendf(batch, "%d,%s,/api/%d,%d\n", n, levels[n%len(levels)], n%97, n%1300)
			n++
		}
		f.Write(batch)
		time.Sleep(followBatchGap)
	}
	done <- n
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func p99(xs []float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[min(len(s)-1, len(s)*99/100)]
}
