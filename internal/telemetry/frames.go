package telemetry

import (
	"log/slog"
	"maps"
	"runtime/metrics"
	"slices"
	"sync"
	"time"
)

// Frames come 60 a second, too many to log one by one at Info, so they
// are summarized once a second with the gauges: a "frames" event with
// counts, percentiles and the live heap.

// window is how often the summary is written.
const window = time.Second

// maxSamples bounds the durations kept per window.
const maxSamples = 4096

type frameStats struct {
	mu     sync.Mutex
	since  time.Time
	render []time.Duration // View and everything the frame took
	keys   []time.Duration // from a key press to its frame
	gauges map[string]int64
}

var frames = frameStats{gauges: map[string]int64{}}

func (f *frameStats) reset(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.since, f.render, f.keys = now, f.render[:0], f.keys[:0]
}

// Frame records a frame that took render to draw. key is how long ago
// the key press it answers happened, or 0 when it answers none.
func Frame(render, key time.Duration) {
	if !on.Load() {
		return
	}
	now := time.Now()
	f := &frames
	f.mu.Lock()
	if len(f.render) < maxSamples {
		f.render = append(f.render, render)
	}
	if key > 0 && len(f.keys) < maxSamples {
		f.keys = append(f.keys, key)
	}
	due := now.Sub(f.since) >= window
	var rs, ks []time.Duration
	var gauges map[string]int64
	if due {
		rs, ks, gauges = slices.Clone(f.render), slices.Clone(f.keys), maps.Clone(f.gauges)
		f.since, f.render, f.keys = now, f.render[:0], f.keys[:0]
	}
	f.mu.Unlock()
	if due {
		summarize(rs, ks, gauges)
	}
	Debug("frame", render, slog.Float64("key_ms", ms(key)))
}

// Set records a gauge, such as the number of cells or JEV questions
// waiting, reported with the next frame summary.
func Set(name string, v int64) {
	if !on.Load() {
		return
	}
	frames.mu.Lock()
	frames.gauges[name] = v
	frames.mu.Unlock()
}

func summarize(render, keys []time.Duration, gauges map[string]int64) {
	attrs := []slog.Attr{
		slog.Int("frames", len(render)),
		slog.Float64("render_p50_ms", ms(percentile(render, 50))),
		slog.Float64("render_p95_ms", ms(percentile(render, 95))),
		slog.Float64("render_max_ms", ms(percentile(render, 100))),
		slog.Int("keys", len(keys)),
		slog.Float64("key_p50_ms", ms(percentile(keys, 50))),
		slog.Float64("key_p95_ms", ms(percentile(keys, 95))),
		slog.Float64("key_max_ms", ms(percentile(keys, 100))),
		slog.Int64("heap_bytes", heapBytes()),
	}
	for _, k := range slices.Sorted(maps.Keys(gauges)) {
		attrs = append(attrs, slog.Int64(k, gauges[k]))
	}
	log(slog.LevelInfo, "frames", window, attrs)
}

// percentile is the p-th percentile of ds (nearest rank), 0 when empty.
func percentile(ds []time.Duration, p int) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	slices.Sort(ds)
	i := (len(ds)*p + 99) / 100
	return ds[max(i-1, 0)]
}

// heapBytes is the heap held by live and not yet swept objects, read
// without stopping the world as runtime.ReadMemStats would.
func heapBytes() int64 {
	s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return int64(s[0].Value.Uint64())
}
