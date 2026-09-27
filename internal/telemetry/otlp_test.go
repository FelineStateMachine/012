package telemetry

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// collector is a fake OTLP/HTTP endpoint that decodes what it gets.
type collector struct {
	t     testing.TB
	srv   *httptest.Server
	delay time.Duration

	mu   sync.Mutex
	reqs []request
}

type request struct {
	path   string
	header http.Header
	body   map[string]any
}

func newCollector(t testing.TB) *collector {
	c := &collector{t: t}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(c.delay)
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("%s: gzip: %v", r.URL.Path, err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body = zr
		}
		var m map[string]any
		if err := json.NewDecoder(body).Decode(&m); err != nil {
			t.Errorf("%s: %v", r.URL.Path, err)
		}
		c.mu.Lock()
		c.reqs = append(c.reqs, request{r.URL.Path, r.Header.Clone(), m})
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{}")
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *collector) requests() []request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.reqs)
}

// items gathers the records of one signal from every request: log
// records, spans or metrics, with the resource of the last request.
func (c *collector) items(path string) (items []map[string]any, res map[string]any) {
	keys := map[string][3]string{
		"/v1/logs":    {"resourceLogs", "scopeLogs", "logRecords"},
		"/v1/traces":  {"resourceSpans", "scopeSpans", "spans"},
		"/v1/metrics": {"resourceMetrics", "scopeMetrics", "metrics"},
	}[path]
	outer, inner, leaf := keys[0], keys[1], keys[2]
	for _, r := range c.requests() {
		if r.path != path {
			continue
		}
		for _, rs := range list(r.body[outer]) {
			rs := rs.(map[string]any)
			res = attrMap(c.t, rs["resource"].(map[string]any)["attributes"])
			for _, ss := range list(rs[inner]) {
				for _, it := range list(ss.(map[string]any)[leaf]) {
					items = append(items, it.(map[string]any))
				}
			}
		}
	}
	return items, res
}

func list(v any) []any { l, _ := v.([]any); return l }

// attrMap turns OTLP attributes into key -> AnyValue, checking each
// value holds exactly one field.
func attrMap(t testing.TB, v any) map[string]any {
	m := map[string]any{}
	for _, kv := range list(v) {
		kv := kv.(map[string]any)
		val := kv["value"].(map[string]any)
		if len(val) != 1 {
			t.Errorf("attribute %v: value %v", kv["key"], val)
		}
		m[kv["key"].(string)] = val
	}
	return m
}

var (
	hexTrace = regexp.MustCompile(`^[0-9a-f]{32}$`)
	hexSpan  = regexp.MustCompile(`^[0-9a-f]{16}$`)
	decimal  = regexp.MustCompile(`^[0-9]+$`)
)

// isInt64 checks v is an int64 as the JSON mapping writes it: a string.
func isInt64(t testing.TB, what string, v any) {
	t.Helper()
	if s, ok := v.(string); !ok || !decimal.MatchString(s) {
		t.Errorf("%s = %#v, want a decimal string", what, v)
	}
}

func otlpSetup(t testing.TB, c OTLPConfig) {
	t.Helper()
	if _, err := Setup(Config{Version: "v-test", OTLP: c}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Close() })
}

func TestOTLPExport(t *testing.T) {
	c := newCollector(t)
	otlpSetup(t, OTLPConfig{Endpoint: c.srv.URL, Headers: map[string]string{"Authorization": "Bearer t"}})
	Start("import", slog.String("format", "CSV")).End(slog.Int("rows", 42))
	Start("open").Fail(errors.New("bad file"))
	Set("cells", 1234)
	for i := range 5 {
		Frame(time.Duration(i+1)*time.Millisecond, 3*time.Millisecond)
	}
	frames.mu.Lock()
	frames.since = time.Now().Add(-2 * window)
	frames.mu.Unlock()
	Frame(time.Millisecond, 0)
	if len(c.requests()) != 0 {
		t.Fatal("sent before a flush")
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}

	checkRequests(t, c)
	imp := checkLogs(t, c)
	checkSpans(t, c, imp)
	checkMetrics(t, c)
}

// checkRequests checks every request is gzipped JSON with the configured
// headers, and that all three signals were sent.
func checkRequests(t *testing.T, c *collector) {
	t.Helper()
	reqs := c.requests()
	paths := map[string]bool{}
	for _, r := range reqs {
		paths[r.path] = true
		if r.header.Get("Content-Encoding") != "gzip" || r.header.Get("Content-Type") != "application/json" {
			t.Errorf("%s headers %v", r.path, r.header)
		}
		if r.header.Get("Authorization") != "Bearer t" {
			t.Errorf("%s: no configured header", r.path)
		}
	}
	if !paths["/v1/logs"] || !paths["/v1/traces"] || !paths["/v1/metrics"] {
		t.Fatalf("paths %v", paths)
	}

}

// checkLogs checks the resource and the log records, returning the
// import's record for checkSpans.
func checkLogs(t *testing.T, c *collector) map[string]any {
	t.Helper()
	logs, res := c.items("/v1/logs")
	for k, want := range map[string]string{"service.name": "012", "service.version": "v-test", "os.type": "", "host.arch": "", "service.instance.id": ""} {
		v, ok := res[k].(map[string]any)["stringValue"].(string)
		if !ok || want != "" && v != want {
			t.Errorf("resource %s = %v", k, res[k])
		}
	}
	byBody := map[string]map[string]any{}
	for _, l := range logs {
		byBody[l["body"].(map[string]any)["stringValue"].(string)] = l
		isInt64(t, "timeUnixNano", l["timeUnixNano"])
		isInt64(t, "observedTimeUnixNano", l["observedTimeUnixNano"])
	}
	imp := byBody["import"]
	if imp == nil || byBody["start"] == nil || byBody["open"] == nil || byBody["frames"] == nil || byBody["otlp"] == nil {
		t.Fatalf("log bodies %v", slices.Collect(maps.Keys(byBody)))
	}
	if imp["severityNumber"] != 9.0 || imp["severityText"] != "INFO" || byBody["open"]["severityNumber"] != 13.0 {
		t.Errorf("severity %v %v", imp["severityNumber"], byBody["open"]["severityNumber"])
	}
	ia := attrMap(t, imp["attributes"])
	if ia["rows"].(map[string]any)["intValue"] != "42" || ia["format"].(map[string]any)["stringValue"] != "CSV" ||
		ia["event"].(map[string]any)["stringValue"] != "import" {
		t.Errorf("import attributes %v", ia)
	}
	if _, ok := ia["dur_ms"].(map[string]any)["doubleValue"].(float64); !ok {
		t.Errorf("dur_ms %v", ia["dur_ms"])
	}
	if _, ok := ia["service"]; ok {
		t.Error("service is a log attribute; it belongs in the resource")
	}
	if tr, _ := imp["traceId"].(string); !hexTrace.MatchString(tr) {
		t.Errorf("import log traceId %v", imp["traceId"])
	}
	if _, ok := byBody["frames"]["traceId"]; ok {
		t.Error("the frames summary is not a span, yet its log has a trace id")
	}
	oa := attrMap(t, byBody["otlp"]["attributes"])
	if oa["logs.dropped"].(map[string]any)["intValue"] != "0" {
		t.Errorf("otlp event %v", oa)
	}

	return imp
}

// checkSpans checks the spans, and that the import's matches its log.
func checkSpans(t *testing.T, c *collector, imp map[string]any) {
	t.Helper()
	spans, _ := c.items("/v1/traces")
	if len(spans) != 2 {
		t.Fatalf("%d spans: %v", len(spans), spans)
	}
	for _, s := range spans {
		if !hexTrace.MatchString(s["traceId"].(string)) || !hexSpan.MatchString(s["spanId"].(string)) || s["kind"] != 1.0 {
			t.Errorf("span ids %v", s)
		}
		isInt64(t, "startTimeUnixNano", s["startTimeUnixNano"])
		isInt64(t, "endTimeUnixNano", s["endTimeUnixNano"])
		start, _ := strconv.ParseUint(s["startTimeUnixNano"].(string), 10, 64)
		end, _ := strconv.ParseUint(s["endTimeUnixNano"].(string), 10, 64)
		if start == 0 || start > end {
			t.Errorf("span ends before it starts: %v", s)
		}
	}
	if spans[0]["name"] != "import" || spans[0]["traceId"] != imp["traceId"] || spans[0]["spanId"] != imp["spanId"] {
		t.Errorf("import span %v does not match its log %v", spans[0], imp)
	}
	if st := spans[1]["status"].(map[string]any); spans[1]["name"] != "open" || st["code"] != 2.0 || st["message"] != "bad file" {
		t.Errorf("failed span %v", spans[1])
	}

}

// checkMetrics checks the frame summary, counters, gauges and the
// operation histogram.
func checkMetrics(t *testing.T, c *collector) {
	t.Helper()
	metrics, _ := c.items("/v1/metrics")
	byName := map[string]map[string]any{}
	for _, m := range metrics {
		byName[m["name"].(string)] = m
	}
	sumr := byName["o12.frame.duration"]["summary"].(map[string]any)["dataPoints"].([]any)[0].(map[string]any)
	isInt64(t, "summary count", sumr["count"])
	if sumr["count"] != "6" || sumr["sum"] != 16.0 || len(list(sumr["quantileValues"])) != 3 {
		t.Errorf("frame summary %v", sumr)
	}
	fr := byName["o12.frames"]["sum"].(map[string]any)
	if fr["aggregationTemporality"] != 2.0 || fr["isMonotonic"] != true {
		t.Errorf("frames sum %v", fr)
	}
	if p := list(fr["dataPoints"])[0].(map[string]any); p["asInt"] != "6" {
		t.Errorf("frames %v", p)
	}
	if p := list(byName["o12.cells"]["gauge"].(map[string]any)["dataPoints"])[0].(map[string]any); p["asInt"] != "1234" {
		t.Errorf("cells gauge %v", p)
	}
	if p := list(byName["o12.heap"]["gauge"].(map[string]any)["dataPoints"])[0].(map[string]any); p["asInt"] == "0" {
		t.Errorf("heap gauge %v", p)
	}
	hs := byName["o12.operation.duration"]["histogram"].(map[string]any)
	if len(list(hs["dataPoints"])) != 2 {
		t.Fatalf("operation histogram %v", hs)
	}
	for _, p := range list(hs["dataPoints"]) {
		p := p.(map[string]any)
		isInt64(t, "histogram count", p["count"])
		if len(list(p["bucketCounts"])) != len(list(p["explicitBounds"]))+1 {
			t.Errorf("buckets %v", p)
		}
		for _, b := range list(p["bucketCounts"]) {
			isInt64(t, "bucket count", b)
		}
	}
	if byName["o12.telemetry.dropped"] == nil {
		t.Error("no drop counter")
	}
}

func TestOTLPOverflowDrops(t *testing.T) {
	defer func(q, f int, e time.Duration) { queueLimit, batchSize, flushEvery = q, f, e }(queueLimit, batchSize, flushEvery)
	queueLimit, batchSize, flushEvery = 3, 1000, time.Hour
	c := newCollector(t)
	otlpSetup(t, OTLPConfig{Endpoint: c.srv.URL, NoTraces: true, NoMetrics: true})
	for range 10 {
		Event("recalc", time.Millisecond) // start + 2 fit
	}
	_, e := current()
	_, dropped, _ := e.counts()
	if dropped[sigLogs] != 8 {
		t.Errorf("dropped %d logs, want 8", dropped[sigLogs])
	}
	Close()
	logs, _ := c.items("/v1/logs")
	if len(logs) != 3 {
		t.Errorf("%d logs sent, want 3", len(logs))
	}
	for _, r := range c.requests() {
		if r.path != "/v1/logs" {
			t.Errorf("sent %s with only logs on", r.path)
		}
	}
}

func TestOTLPFlushesInTheBackground(t *testing.T) {
	defer func(f int) { batchSize = f }(batchSize)
	batchSize = 4
	c := newCollector(t)
	otlpSetup(t, OTLPConfig{Endpoint: c.srv.URL, NoMetrics: true})
	for range 4 {
		Event("recalc", time.Millisecond)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(c.requests()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("a full batch was not sent before Close")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOTLPOffSendsNothing(t *testing.T) {
	c := newCollector(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", c.srv.URL)
	t.Setenv("OTEL_SDK_DISABLED", "true")
	stop, err := Setup(ConfigFromEnv())
	if err != nil || Enabled() {
		t.Fatalf("OTEL_SDK_DISABLED: %v, enabled %v", err, Enabled())
	}
	Start("recalc").End()
	Event("x", time.Second)
	stop()

	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	stop, err = Setup(ConfigFromEnv())
	if err != nil || Enabled() {
		t.Fatalf("no endpoint: %v, enabled %v", err, Enabled())
	}
	Start("recalc").End()
	stop()

	// After Close, nothing more goes out.
	otlpSetup(t, OTLPConfig{Endpoint: c.srv.URL})
	Close()
	n := len(c.requests())
	Start("recalc").End()
	Frame(time.Millisecond, 0)
	time.Sleep(50 * time.Millisecond)
	if len(c.requests()) != n {
		t.Error("sent after Close")
	}
	for _, r := range c.requests()[:n] {
		if r.path == "/v1/traces" {
			t.Error("a span was sent though none was recorded while on")
		}
	}
}

func TestOTLPConfigFromEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318/")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://traces.example/v1/t")
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "Authorization=Bearer%20abc, x-team = 012 ,bad")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=dev,service.name=other")
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "500")
	c := ConfigFromEnv().OTLP
	u := c.urls()
	want := urls{logs: "http://localhost:4318/v1/logs", traces: "https://traces.example/v1/t"}
	if u != want {
		t.Errorf("urls %+v, want %+v", u, want)
	}
	if c.Headers["Authorization"] != "Bearer abc" || c.Headers["x-team"] != "012" || len(c.Headers) != 2 {
		t.Errorf("headers %v", c.Headers)
	}
	if c.Timeout != 500*time.Millisecond {
		t.Errorf("timeout %v", c.Timeout)
	}
	e := newExporter(c, "")
	defer e.shutdown()
	b, _ := json.Marshal(e.resource)
	var rm map[string]any
	if err := json.Unmarshal(b, &rm); err != nil {
		t.Fatal(err)
	}
	res := attrMap(t, rm["attributes"])
	if res["service.name"].(map[string]any)["stringValue"] != "012" || res["deployment.environment"].(map[string]any)["stringValue"] != "dev" {
		t.Errorf("resource %v", res)
	}
	c.Disabled = true
	if c.urls().any() {
		t.Error("disabled but still sending")
	}
}

func TestOTLPAndFileTogether(t *testing.T) {
	c := newCollector(t)
	path := t.TempDir() + "/log.jsonl"
	if _, err := Setup(Config{LogPath: path, OTLP: OTLPConfig{Endpoint: c.srv.URL}}); err != nil {
		t.Fatal(err)
	}
	Event("recalc", time.Millisecond, slog.Int("cells", 3))
	Close()
	logs, _ := c.items("/v1/logs")
	if len(logs) != 3 { // start, recalc, otlp
		t.Errorf("%d OTLP logs", len(logs))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(regexp.MustCompile(`"service":"012"`).FindAll(b, -1)); n != 3 {
		t.Errorf("%d file events: %s", n, b)
	}
}

// The on-path cost with OTLP, queueing to a local endpoint.
func BenchmarkSpanOTLP(b *testing.B) {
	c := newCollector(b)
	otlpSetup(b, OTLPConfig{Endpoint: c.srv.URL})
	for b.Loop() {
		s := Start("recalc", slog.Int("n", 1))
		s.End(slog.Int("cells", 2))
	}
}

// Nested spans go out with their parent's span id and trace id; roots
// without a parentSpanId.
func TestOTLPNestedSpans(t *testing.T) {
	c := newCollector(t)
	otlpSetup(t, OTLPConfig{Endpoint: c.srv.URL})
	tr := &Trace{}
	cmd := tr.Start("command")
	tr.Begin("recalc")
	tr.End()
	cmd.End()
	Close()
	spans, _ := c.items("/v1/traces")
	if len(spans) != 2 {
		t.Fatalf("%d spans: %v", len(spans), spans)
	}
	recalc, command := spans[0], spans[1]
	if recalc["parentSpanId"] != command["spanId"] || recalc["traceId"] != command["traceId"] || !hexSpan.MatchString(recalc["parentSpanId"].(string)) {
		t.Errorf("recalc %v is not under command %v", recalc, command)
	}
	if _, ok := command["parentSpanId"]; ok {
		t.Errorf("root span with a parent: %v", command)
	}
}
