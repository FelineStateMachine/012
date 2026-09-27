package telemetry

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OTLP sends events to an OpenTelemetry Collector (or any OTLP/HTTP
// endpoint) with the JSON encoding, using only the standard library:
// slog events as logs, spans as traces, and the frame summaries and
// operation durations as metrics. A background goroutine batches them
// from bounded queues; when a queue is full the newest items are dropped
// and counted, so the UI never waits on the network.

// OTLPConfig says where OTLP goes. ConfigFromEnv reads it from the
// standard OTEL_* variables. Nothing is sent unless an endpoint is set.
type OTLPConfig struct {
	// Endpoint is the base URL, like http://localhost:4318, to which
	// /v1/logs, /v1/traces and /v1/metrics are appended
	// (OTEL_EXPORTER_OTLP_ENDPOINT, or 012 --otlp).
	Endpoint string
	// LogsEndpoint, TracesEndpoint and MetricsEndpoint are full URLs for
	// one signal each, used as they are
	// (OTEL_EXPORTER_OTLP_{LOGS,TRACES,METRICS}_ENDPOINT).
	LogsEndpoint, TracesEndpoint, MetricsEndpoint string
	// NoLogs, NoTraces and NoMetrics leave a signal out
	// (OTEL_{LOGS,TRACES,METRICS}_EXPORTER=none).
	NoLogs, NoTraces, NoMetrics bool
	// Headers go with every request, e.g. for authentication
	// (OTEL_EXPORTER_OTLP_HEADERS). They are never logged.
	Headers map[string]string
	// Resource adds resource attributes (OTEL_RESOURCE_ATTRIBUTES);
	// service.name stays 012.
	Resource map[string]string
	// Timeout bounds each request (OTEL_EXPORTER_OTLP_TIMEOUT, in
	// milliseconds); 3 s when zero.
	Timeout time.Duration
	// NoCompression sends plain JSON instead of gzip
	// (OTEL_EXPORTER_OTLP_COMPRESSION=none).
	NoCompression bool
	// Disabled turns OTLP off whatever else is set (OTEL_SDK_DISABLED).
	Disabled bool
}

const defaultTimeout = 3 * time.Second

// otlpFromEnv reads the OTEL_* variables.
func otlpFromEnv() OTLPConfig {
	env := func(signal string) string {
		return os.Getenv("OTEL_EXPORTER_OTLP_" + signal + "ENDPOINT")
	}
	none := func(signal string) bool {
		return strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_"+signal+"_EXPORTER")), "none")
	}
	c := OTLPConfig{
		Endpoint:        env(""),
		LogsEndpoint:    env("LOGS_"),
		TracesEndpoint:  env("TRACES_"),
		MetricsEndpoint: env("METRICS_"),
		NoLogs:          none("LOGS"),
		NoTraces:        none("TRACES"),
		NoMetrics:       none("METRICS"),
		Headers:         keyValues(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")),
		Resource:        keyValues(os.Getenv("OTEL_RESOURCE_ATTRIBUTES")),
		NoCompression:   strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_COMPRESSION"), "none"),
		Disabled:        strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true"),
	}
	if ms, err := strconv.Atoi(os.Getenv("OTEL_EXPORTER_OTLP_TIMEOUT")); err == nil && ms > 0 {
		c.Timeout = time.Duration(ms) * time.Millisecond
	}
	return c
}

// keyValues parses the "k1=v1,k2=v2" lists of OTEL_EXPORTER_OTLP_HEADERS
// and OTEL_RESOURCE_ATTRIBUTES, whose values are percent-encoded.
func keyValues(s string) map[string]string {
	var m map[string]string
	for kv := range strings.SplitSeq(s, ",") {
		k, v, ok := strings.Cut(kv, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		if d, err := url.PathUnescape(strings.TrimSpace(v)); err == nil {
			v = d
		}
		if m == nil {
			m = map[string]string{}
		}
		m[k] = strings.TrimSpace(v)
	}
	return m
}

// urls are where each signal goes; empty leaves it out.
type urls struct{ logs, traces, metrics string }

func (u urls) any() bool { return u.logs != "" || u.traces != "" || u.metrics != "" }

func (c OTLPConfig) urls() urls {
	if c.Disabled {
		return urls{}
	}
	base := c.Endpoint
	if base != "" && !strings.Contains(base, "://") {
		base = "http://" + base
	}
	pick := func(own, path string, off bool) string {
		switch {
		case off:
			return ""
		case own != "":
			return own
		case base != "":
			return strings.TrimRight(base, "/") + path
		}
		return ""
	}
	return urls{
		logs:    pick(c.LogsEndpoint, "/v1/logs", c.NoLogs),
		traces:  pick(c.TracesEndpoint, "/v1/traces", c.NoTraces),
		metrics: pick(c.MetricsEndpoint, "/v1/metrics", c.NoMetrics),
	}
}

const (
	sigLogs = iota
	sigTraces
	sigMetrics
)

var signalNames = [3]string{"logs", "traces", "metrics"}

// Tuning; variables so tests can shrink them.
var (
	// queueLimit bounds each signal's queue (metric windows count as
	// one item each).
	queueLimit = 2048
	// batchSize wakes the sender before the next tick.
	batchSize = 512
	// flushEvery is how often queued items are sent.
	flushEvery = 2 * time.Second
)

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

type exporter struct {
	urls     urls
	headers  map[string]string
	gzip     bool
	timeout  time.Duration
	client   *http.Client
	resource resource
	scope    scope
	started  time.Time

	mu      sync.Mutex
	logs    []logRecord
	spans   []spanData
	windows []frameWindow
	ops     map[string]*durationHist
	opsNew  bool // ops changed since they were last sent
	frames  int64
	sent    [3]int64 // items accepted by the endpoint
	dropped [3]int64 // items dropped because the queue was full
	failed  [3]int64 // items in requests that failed

	wake    chan struct{}
	done    chan struct{}
	stopped chan struct{}
}

func newExporter(c OTLPConfig, version string) *exporter {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	var id [16]byte
	randomBytes(id[:])
	res := map[string]string{}
	maps.Copy(res, c.Resource)
	maps.Copy(res, map[string]string{
		"service.name":            "012",
		"service.instance.id":     hex.EncodeToString(id[:]),
		"os.type":                 runtime.GOOS,
		"host.arch":               runtime.GOARCH,
		"process.runtime.name":    "go",
		"process.runtime.version": runtime.Version(),
	})
	if version != "" {
		res["service.version"] = version
	}
	var attrs []keyValue
	for _, k := range slices.Sorted(maps.Keys(res)) {
		attrs = append(attrs, keyValue{Key: k, Value: str(res[k])})
	}
	e := &exporter{
		urls:     c.urls(),
		headers:  c.Headers,
		gzip:     !c.NoCompression,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
		resource: resource{Attributes: attrs},
		scope:    scope{Name: "github.com/FelineStateMachine/012/internal/telemetry", Version: version},
		started:  time.Now(),
		ops:      map[string]*durationHist{},
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go e.run()
	return e
}

func randomBytes(b []byte) {
	for i := 0; i < len(b); i += 8 {
		var w [8]byte
		v := rand.Uint64()
		for j := range w {
			w[j] = byte(v >> (8 * j))
		}
		copy(b[i:], w[:])
	}
}

func newIDs() (traceID, spanID) {
	var t traceID
	var s spanID
	for t.IsZero() {
		randomBytes(t[:])
	}
	for s.IsZero() {
		randomBytes(s[:])
	}
	return t, s
}

// span records a finished operation: a span when traces are on, and its
// duration in the operation histogram when metrics are. It returns the
// span's ids for the log record that goes with it. attrs is not kept.
func (e *exporter) span(name string, start, end time.Time, failed bool, attrs []slog.Attr) (traceID, spanID) {
	var t traceID
	var s spanID
	if e.urls.metrics != "" {
		e.mu.Lock()
		h := e.ops[name]
		if h == nil {
			h = &durationHist{}
			e.ops[name] = h
		}
		h.add(ms(end.Sub(start)))
		e.opsNew = true
		e.mu.Unlock()
	}
	if e.urls.traces == "" {
		return t, s
	}
	t, s = newIDs()
	sp := spanData{
		TraceID: t, SpanID: s, Name: name, Kind: spanKindInternal,
		StartTimeUnixNano: nanos(start), EndTimeUnixNano: nanos(end),
		Attributes: attrsOf(attrs),
		Status:     status{Code: statusOK},
	}
	if failed {
		sp.Status = status{Code: statusError}
		for _, a := range attrs {
			if a.Key == "error" {
				sp.Status.Message = a.Value.String()
			}
		}
	}
	e.mu.Lock()
	ok := len(e.spans) < queueLimit
	if ok {
		e.spans = append(e.spans, sp)
	} else {
		e.dropped[sigTraces]++
	}
	n := len(e.spans)
	e.mu.Unlock()
	e.nudge(n)
	return t, s
}

func (e *exporter) log(r logRecord) {
	e.mu.Lock()
	ok := len(e.logs) < queueLimit
	if ok {
		e.logs = append(e.logs, r)
	} else {
		e.dropped[sigLogs]++
	}
	n := len(e.logs)
	e.mu.Unlock()
	e.nudge(n)
}

func (e *exporter) window(w frameWindow) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frames += int64(w.render.count)
	if len(e.windows) >= queueLimit {
		e.dropped[sigMetrics]++
		return
	}
	e.windows = append(e.windows, w)
}

// nudge wakes the sender once a queue holds a batch.
func (e *exporter) nudge(n int) {
	if n < batchSize {
		return
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *exporter) run() {
	defer close(e.stopped)
	t := time.NewTicker(flushEvery)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-e.wake:
		case <-e.done:
			// The last flush shares one timeout, so quitting 012 waits
			// at most that long for a collector that doesn't answer.
			ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
			e.flush(ctx, true)
			cancel()
			return
		}
		e.flush(context.Background(), false)
	}
}

// shutdown sends what is queued and stops the sender, waiting at most a
// little over one request timeout.
func (e *exporter) shutdown() {
	close(e.done)
	select {
	case <-e.stopped:
	case <-time.After(e.timeout + time.Second):
	}
}

func (e *exporter) flush(ctx context.Context, final bool) {
	e.mu.Lock()
	logs, spans, windows := e.logs, e.spans, e.windows
	e.logs, e.spans, e.windows = nil, nil, nil
	var ms []metric
	if len(windows) > 0 || e.opsNew || final {
		ms = e.metricsLocked(windows, time.Now())
		e.opsNew = false
	}
	e.mu.Unlock()

	if len(logs) > 0 {
		e.send(ctx, sigLogs, len(logs), logsRequest{ResourceLogs: []resourceLogs{{
			Resource: e.resource, ScopeLogs: []scopeLogs{{Scope: e.scope, LogRecords: logs}},
		}}})
	}
	if len(spans) > 0 {
		e.send(ctx, sigTraces, len(spans), tracesRequest{ResourceSpans: []resourceSpans{{
			Resource: e.resource, ScopeSpans: []scopeSpans{{Scope: e.scope, Spans: spans}},
		}}})
	}
	if len(ms) > 0 && e.urls.metrics != "" {
		e.send(ctx, sigMetrics, max(len(windows), 1), metricsRequest{ResourceMetrics: []resourceMetrics{{
			Resource: e.resource, ScopeMetrics: []scopeMetrics{{Scope: e.scope, Metrics: ms}},
		}}})
	}
}

// metricsLocked builds the metrics: the queued frame windows as
// summaries and gauges, and the cumulative counters and histograms as
// they stand at now.
func (e *exporter) metricsLocked(windows []frameWindow, now time.Time) []metric {
	var out []metric
	if len(windows) > 0 {
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
		out = append(out,
			metric{Name: "o12.frame.duration", Unit: "ms", Description: "Time to draw a frame (View), per second", Summary: render},
			metric{Name: "o12.heap", Unit: "By", Description: "Heap held by live and unswept objects", Gauge: heap})
		if len(key.DataPoints) > 0 {
			out = append(out, metric{Name: "o12.key.latency", Unit: "ms", Description: "From a key press to the end of its frame, per second", Summary: key})
		}
		for _, k := range slices.Sorted(maps.Keys(gauges)) {
			out = append(out, metric{Name: "o12." + k, Gauge: gauges[k]})
		}
	}
	out = append(out, metric{Name: "o12.frames", Unit: "{frame}", Description: "Frames drawn", Sum: &sum{
		DataPoints:             []numberPoint{intPoint(e.started, now, e.frames)},
		AggregationTemporality: temporalityCumulative, IsMonotonic: true,
	}})
	if len(e.ops) > 0 {
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
		out = append(out, metric{Name: "o12.operation.duration", Unit: "ms", Description: "Duration of operations such as recalc, import and sort", Histogram: h})
	}
	dropped := &sum{AggregationTemporality: temporalityCumulative, IsMonotonic: true}
	for i, n := range e.dropped {
		dropped.DataPoints = append(dropped.DataPoints, intPoint(e.started, now, n+e.failed[i], keyValue{Key: "signal", Value: str(signalNames[i])}))
	}
	out = append(out, metric{Name: "o12.telemetry.dropped", Unit: "{item}", Description: "Telemetry items not delivered: queue full or request failed", Sum: dropped})
	return out
}

func (s stats) point(start, end time.Time) summaryPoint {
	return summaryPoint{
		StartTimeUnixNano: nanos(start), TimeUnixNano: nanos(end),
		Count: u64(s.count), Sum: s.sum,
		QuantileValues: []quantileValue{{0.5, s.p50}, {0.95, s.p95}, {1, s.pmax}},
	}
}

// send posts one request; failures are counted, never retried.
func (e *exporter) send(ctx context.Context, sig, items int, body any) {
	url := [3]string{e.urls.logs, e.urls.traces, e.urls.metrics}[sig]
	err := e.post(ctx, url, body)
	e.mu.Lock()
	if err != nil {
		e.failed[sig] += int64(items)
	} else {
		e.sent[sig] += int64(items)
	}
	e.mu.Unlock()
}

func (e *exporter) post(ctx context.Context, url string, body any) error {
	var buf bytes.Buffer
	var w io.Writer = &buf
	var zw *gzip.Writer
	if e.gzip {
		zw = gzip.NewWriter(&buf)
		w = zw
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return err
	}
	if zw != nil {
		if err := zw.Close(); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.gzip {
		req.Header.Set("Content-Encoding", "gzip")
	}
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("OTLP %s: %s", url, resp.Status)
	}
	return nil
}

// counts reports items sent, dropped and failed per signal.
func (e *exporter) counts() (sent, dropped, failed [3]int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sent, e.dropped, e.failed
}

// spanKey carries a span's ids from log to the OTLP log handler, so the
// log record of an operation links to its span.
type spanKey struct{}

type spanIDs struct {
	trace traceID
	span  spanID
}

// otlpHandler is a slog.Handler that queues OTLP log records.
type otlpHandler struct {
	e      *exporter
	level  slog.Leveler
	attrs  []keyValue
	prefix string
}

func (h *otlpHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *otlpHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := logRecord{
		TimeUnixNano:         nanos(r.Time),
		ObservedTimeUnixNano: nanos(time.Now()),
		SeverityNumber:       severity(r.Level),
		SeverityText:         r.Level.String(),
		Body:                 str(r.Message),
		Attributes:           slices.Clip(h.attrs),
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.Attributes = appendAttr(rec.Attributes, h.prefix, a)
		return true
	})
	if ids, ok := ctx.Value(spanKey{}).(spanIDs); ok {
		rec.TraceID, rec.SpanID = ids.trace, ids.span
	}
	h.e.log(rec)
	return nil
}

func (h *otlpHandler) WithAttrs(as []slog.Attr) slog.Handler {
	h2 := *h
	h2.attrs = slices.Clip(h.attrs)
	for _, a := range as {
		h2.attrs = appendAttr(h2.attrs, h.prefix, a)
	}
	return &h2
}

func (h *otlpHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.prefix += name + "."
	return &h2
}
