//go:build stress && unix

// Stress benchmark of 012 serve: many sessions at once, each over its own
// SSH connection on loopback, measuring the heap each session holds, the
// time from a key press leaving the client to the frame answering it
// arriving back while every session types at once, and the CPU spent per
// frame. The clients run in the same process, so heap and CPU include
// their side of the connections: upper bounds for the server. Only built
// with -tags stress (see `make stress` and docs/limits.md).
package serve

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/stress"
)

func BenchmarkSessions(b *testing.B) {
	for _, n := range []int{10, 50} {
		for _, shape := range []string{"empty", "dense-1000x26"} {
			b.Run(fmt.Sprintf("sessions=%d/%s", n, shape), func(b *testing.B) { benchSessions(b, n, shape) })
		}
	}
}

func benchSessions(b *testing.B, n int, shape string) {
	key := newKey(b)
	srv, addr, dir := testServer(b, func(o *Options) { o.MaxSessions, o.IdleTimeout = n, 0 }, key)
	if shape != "empty" {
		writeDense(b, filepath.Join(dir, "dense.012"))
	}
	before := liveHeap()
	terms := make([]*term, n)
	for i := range terms {
		c, err := dial(addr, key, srv.HostKey())
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { c.Close() })
		terms[i] = openTerm(b, c, 120, 40)
		terms[i].waitFor(b, "READY")
		if shape != "empty" {
			io.WriteString(terms[i].stdin, "\x0fdense\r") // Ctrl+O dense
			terms[i].waitFor(b, "dense.012")
		}
	}
	perSession := float64(liveHeap()-before) / float64(n) / 1024
	if path := os.Getenv("SERVE_HEAP_PROFILE"); path != "" {
		// What the sessions hold, while they're open.
		f, err := os.Create(path)
		if err != nil {
			b.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}

	// Every session presses a key at once, b.N times; each waits for its
	// frame.
	keys := []string{"\x1b[B", "\x1b[A"} // down, up
	var mu sync.Mutex
	var lat []time.Duration
	cpu := cpuTime()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		for _, tm := range terms {
			wg.Go(func() {
				d := tm.press(b, keys[i%2])
				mu.Lock()
				lat = append(lat, d)
				mu.Unlock()
			})
		}
		wg.Wait()
	}
	b.StopTimer()
	b.ReportMetric(float64((cpuTime()-cpu).Microseconds())/float64(b.N*n), "cpu_us/frame")
	b.ReportMetric(perSession, "KiB/session")
	slices.Sort(lat)
	b.ReportMetric(ms(lat[len(lat)/2]), "p50_ms")
	b.ReportMetric(ms(lat[len(lat)*95/100]), "p95_ms")
	b.ReportMetric(ms(lat[len(lat)-1]), "max_ms")
}

// press sends k and returns how long the first bytes of the answering
// frame took to arrive.
func (tm *term) press(b *testing.B, k string) time.Duration {
	tm.mu.Lock()
	had := tm.out.Len()
	tm.mu.Unlock()
	select { // forget output that arrived before the key
	case <-tm.wrote:
	default:
	}
	start := time.Now()
	io.WriteString(tm.stdin, k)
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-tm.wrote:
			tm.mu.Lock()
			grew := tm.out.Len() > had
			tm.mu.Unlock()
			if grew {
				return time.Since(start)
			}
		case <-timeout:
			b.Fatal("no frame after a key press")
		}
	}
}

func writeDense(b *testing.B, path string) {
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	if err := stress.Dense(1000, 26).Write(f); err != nil {
		b.Fatal(err)
	}
}

// cpuTime is the CPU time the process has used, user and system.
func cpuTime() time.Duration {
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// liveHeap is the heap in use after a full collection.
func liveHeap() int64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.HeapAlloc)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
