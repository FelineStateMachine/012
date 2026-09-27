// Package telemetry records what 012 spends its time on, as structured
// events written with log/slog. It is off unless a destination is given:
// a JSON log file (012 --log path, or O12_LOG), one line per event that
// DuckDB or an OpenTelemetry Collector's filelog receiver can read,
// and/or an OTLP/HTTP endpoint (012 --otlp URL, or
// OTEL_EXPORTER_OTLP_ENDPOINT) that gets the events as logs, spans as
// traces and frame summaries as metrics. Stdout belongs to the terminal
// UI. See docs/observability.md.
//
// Events carry sizes, counts and durations only: never cell contents,
// file contents or secrets.
//
// The API is small on purpose, so call sites stay few and backends can
// change behind it without touching them.
package telemetry

import (
	"context"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// on is read by every call site, so being off costs one atomic load.
var on atomic.Bool

var (
	mu     sync.Mutex
	logger = slog.New(slog.DiscardHandler)
	closer io.Closer
	exp    *exporter // nil unless OTLP is on
)

// Config says where events go.
type Config struct {
	// LogPath is the JSON log file, appended to. Empty (and no OTLP)
	// turns telemetry off.
	LogPath string
	// Level is the least severe level logged; Info by default. Debug adds
	// an event for every frame and every command.
	Level slog.Level
	// Version is 012's version or commit, added to the start event and
	// sent as the OTLP service.version.
	Version string
	// OTLP sends events to an OpenTelemetry endpoint too, or instead.
	OTLP OTLPConfig
}

// ConfigFromEnv reads O12_LOG, O12_LOG_LEVEL (debug, info, warn) and the
// standard OTEL_* variables described at OTLPConfig.
func ConfigFromEnv() Config {
	c := Config{LogPath: os.Getenv("O12_LOG"), OTLP: otlpFromEnv()}
	c.Level.UnmarshalText([]byte(os.Getenv("O12_LOG_LEVEL")))
	return c
}

// Setup starts recording as c says. The returned function flushes and
// closes the log and sends what OTLP has queued; call it on exit.
func Setup(c Config) (func() error, error) {
	u := c.OTLP.urls()
	if c.LogPath == "" && !u.any() {
		return func() error { return nil }, nil
	}
	var w io.Writer
	if c.LogPath != "" {
		f, err := os.OpenFile(c.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, err
		}
		w = f
	}
	var e *exporter
	if u.any() {
		e = newExporter(c.OTLP, c.Version)
	}
	start(w, e, c)
	Logger().Info("start", "event", "start", "version", c.Version, "go", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH,
		"cpus", runtime.NumCPU(), "pid", os.Getpid(), "file", w != nil, "otlp", e != nil)
	return Close, nil
}

// start sends events to w and e, either of which may be nil; tests use
// it with a buffer.
func start(w io.Writer, e *exporter, c Config) {
	mu.Lock()
	defer mu.Unlock()
	var hs []slog.Handler
	if w != nil {
		// OTLP carries the service in its resource instead.
		hs = append(hs, slog.NewJSONHandler(w, &slog.HandlerOptions{Level: c.Level}).
			WithAttrs([]slog.Attr{slog.String("service", "012")}))
	}
	if e != nil && e.urls.logs != "" {
		hs = append(hs, &otlpHandler{e: e, level: c.Level})
	}
	switch len(hs) {
	case 0:
		logger = slog.New(slog.DiscardHandler)
	case 1:
		logger = slog.New(hs[0])
	default:
		logger = slog.New(slog.NewMultiHandler(hs...))
	}
	slog.SetDefault(logger)
	closer = nil
	if cl, ok := w.(io.Closer); ok {
		closer = cl
	}
	exp = e
	frames.reset(time.Now())
	on.Store(true)
}

// Close stops recording, sends what OTLP has queued (waiting at most
// about one request timeout) and closes the log file.
func Close() error {
	mu.Lock()
	if !on.Load() {
		mu.Unlock()
		return nil
	}
	e, l, cl := exp, logger, closer
	if e != nil {
		// How OTLP fared so far, for the file log (and the collector).
		sent, dropped, failed := e.counts()
		attrs := []slog.Attr{slog.String("event", "otlp")}
		for i, name := range signalNames {
			attrs = append(attrs, slog.Group(name, "sent", sent[i], "dropped", dropped[i], "failed", failed[i]))
		}
		l.LogAttrs(context.Background(), slog.LevelInfo, "otlp", attrs...)
	}
	on.Store(false)
	logger = slog.New(slog.DiscardHandler)
	slog.SetDefault(logger)
	exp, closer = nil, nil
	mu.Unlock()
	if e != nil {
		e.shutdown()
	}
	if cl != nil {
		return cl.Close()
	}
	return nil
}

// Enabled reports whether events are being recorded.
func Enabled() bool { return on.Load() }

// Logger is the event logger; it discards everything while off.
func Logger() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	return logger
}

// Debug logs a finished operation at Debug level, for events too
// frequent to keep by default (and to send as spans).
func Debug(name string, d time.Duration, attrs ...slog.Attr) {
	if !on.Load() {
		return
	}
	emit(slog.LevelDebug, name, d, attrs, spanIDs{})
}

// current is the logger and the OTLP exporter (nil unless OTLP is on).
func current() (*slog.Logger, *exporter) {
	mu.Lock()
	defer mu.Unlock()
	return logger, exp
}

// emit logs an event that took d and ended now. With ids set it is a
// span: its log record carries the ids, and with OTLP on it is also sent
// as a span and a sample of the operation histogram. attrs is not kept.
func emit(level slog.Level, name string, d time.Duration, attrs []slog.Attr, ids spanIDs) {
	l, e := current()
	span := !ids.span.IsZero()
	if span && e != nil {
		end := time.Now()
		e.span(name, end.Add(-d), end, level >= slog.LevelWarn, attrs, ids)
	}
	ctx := context.Background()
	if !l.Enabled(ctx, level) {
		return
	}
	all := make([]slog.Attr, 0, len(attrs)+5)
	all = append(all, slog.String("event", name), slog.Float64("dur_ms", ms(d)))
	if span {
		all = idAttrs(all, ids)
		if e != nil && e.urls.logs != "" { // the OTLP log handler's
			ctx = context.WithValue(ctx, spanKey{}, ids)
		}
	}
	l.LogAttrs(ctx, level, name, append(all, attrs...)...)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
