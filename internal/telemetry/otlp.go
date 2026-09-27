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
	"runtime"
	"slices"
	"sync"
	"time"
)

// OTLP sends events to an OpenTelemetry Collector (or any OTLP/HTTP
// endpoint) with the JSON encoding, using only the standard library:
// slog events as logs, spans as traces, and the frame summaries and
// operation durations as metrics. A background goroutine batches them
// from bounded queues; when a queue is full the newest items are dropped
// and counted, so the UI never waits on the network.
//
// This file is the exporter's queues and sender; otlpconfig.go says
// where to send, otlplog.go is the slog handler queuing log records,
// otlpmetrics.go builds the metrics and otlpjson.go is the wire format.

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
