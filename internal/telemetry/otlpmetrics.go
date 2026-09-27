package telemetry

import (
	"maps"
	"slices"
	"time"
)

// The metrics OTLP sends: per-second frame windows as summaries and
// gauges, and cumulative counters and operation histograms.

// durationBounds are the histogram buckets for operation durations, in
// milliseconds.
var durationBounds = []float64{0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000}

type durationHist struct {
	counts        []uint64
	count         uint64
	sum, min, max float64
}

func (h *durationHist) add(v float64) {
	if h.counts == nil {
		h.counts = make([]uint64, len(durationBounds)+1)
		h.min, h.max = v, v
	}
	i, _ := slices.BinarySearch(durationBounds, v)
	h.counts[i]++
	h.count++
	h.sum += v
	h.min, h.max = min(h.min, v), max(h.max, v)
}

// frameWindow is one second of frames, from the frame summary.
type frameWindow struct {
	start, end  time.Time
	render, key stats
	heap        int64
	gauges      map[string]int64
}

type stats struct {
	count          int
	sum            float64 // ms
	p50, p95, pmax float64 // ms
}

// metricsLocked builds the metrics: the queued frame windows as
// summaries and gauges, and the cumulative counters and histograms as
// they stand at now.
func (e *exporter) metricsLocked(windows []frameWindow, now time.Time) []metric {
	out := windowMetrics(windows)
	out = append(out, metric{Name: "o12.frames", Unit: "{frame}", Description: "Frames drawn", Sum: &sum{
		DataPoints:             []numberPoint{intPoint(e.started, now, e.frames)},
		AggregationTemporality: temporalityCumulative, IsMonotonic: true,
	}})
	if len(e.ops) > 0 {
		out = append(out, metric{Name: "o12.operation.duration", Unit: "ms", Description: "Duration of operations such as recalc, import and sort", Histogram: e.opsHistogram(now)})
	}
	dropped := &sum{AggregationTemporality: temporalityCumulative, IsMonotonic: true}
	for i, n := range e.dropped {
		dropped.DataPoints = append(dropped.DataPoints, intPoint(e.started, now, n+e.failed[i], keyValue{Key: "signal", Value: str(signalNames[i])}))
	}
	out = append(out, metric{Name: "o12.telemetry.dropped", Unit: "{item}", Description: "Telemetry items not delivered: queue full or request failed", Sum: dropped})
	return out
}

// windowMetrics are the frame windows as a summary of frame durations,
// one of key latencies, and gauges of the heap and whatever Set
// recorded; none when there are no windows.
func windowMetrics(windows []frameWindow) []metric {
	if len(windows) == 0 {
		return nil
	}
	render := &summary{}
	key := &summary{}
	heap := &gauge{}
	gauges := map[string]*gauge{}
	for _, w := range windows {
		render.DataPoints = append(render.DataPoints, w.render.point(w.start, w.end))
		if w.key.count > 0 {
			key.DataPoints = append(key.DataPoints, w.key.point(w.start, w.end))
		}
		heap.DataPoints = append(heap.DataPoints, intPoint(time.Time{}, w.end, w.heap))
		for k, v := range w.gauges {
			g := gauges[k]
			if g == nil {
				g = &gauge{}
				gauges[k] = g
			}
			g.DataPoints = append(g.DataPoints, intPoint(time.Time{}, w.end, v))
		}
	}
	out := []metric{
		{Name: "o12.frame.duration", Unit: "ms", Description: "Time to draw a frame (View), per second", Summary: render},
		{Name: "o12.heap", Unit: "By", Description: "Heap held by live and unswept objects", Gauge: heap},
	}
	if len(key.DataPoints) > 0 {
		out = append(out, metric{Name: "o12.key.latency", Unit: "ms", Description: "From a key press to the end of its frame, per second", Summary: key})
	}
	for _, k := range slices.Sorted(maps.Keys(gauges)) {
		out = append(out, metric{Name: "o12." + k, Gauge: gauges[k]})
	}
	return out
}

// opsHistogram is the operation durations since the start, one point per
// operation.
func (e *exporter) opsHistogram(now time.Time) *histogram {
	h := &histogram{AggregationTemporality: temporalityCumulative}
	for _, name := range slices.Sorted(maps.Keys(e.ops)) {
		d := e.ops[name]
		counts := make([]u64, len(d.counts))
		for i, c := range d.counts {
			counts[i] = u64(c)
		}
		h.DataPoints = append(h.DataPoints, histogramPoint{
			Attributes:        []keyValue{{Key: "event", Value: str(name)}},
			StartTimeUnixNano: nanos(e.started), TimeUnixNano: nanos(now),
			Count: u64(d.count), Sum: d.sum, BucketCounts: counts, ExplicitBounds: durationBounds,
			Min: d.min, Max: d.max,
		})
	}
	return h
}

func (s stats) point(start, end time.Time) summaryPoint {
	return summaryPoint{
		StartTimeUnixNano: nanos(start), TimeUnixNano: nanos(end),
		Count: u64(s.count), Sum: s.sum,
		QuantileValues: []quantileValue{{0.5, s.p50}, {0.95, s.p95}, {1, s.pmax}},
	}
}
