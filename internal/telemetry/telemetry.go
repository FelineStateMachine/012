// Package telemetry records what 012 spends its time on, as structured
// JSON events written with log/slog to a file (stdout belongs to the
// terminal UI). It is off unless a log file is given (012 --log path, or
// O12_LOG); then spans are timed, frame times are summarized once a
// second, and everything lands in one JSON line per event that DuckDB or
// an OpenTelemetry Collector's filelog receiver can read (see
// docs/observability.md).
//
// Events carry sizes, counts and durations only: never cell contents,
// file contents or secrets.
//
// The API is small on purpose, so call sites stay few and a different
// backend (OTLP) can sit behind it without touching them.
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
)

// Config says where events go.
type Config struct {
	// LogPath is the JSON log file, appended to. Empty turns telemetry
	// off.
	LogPath string
	// Level is the least severe level logged; Info by default. Debug adds
	// an event for every frame and every command.
	Level slog.Level
	// Version is 012's version or commit, added to the start event.
	Version string
}

// ConfigFromEnv reads O12_LOG and O12_LOG_LEVEL (debug, info, warn).
func ConfigFromEnv() Config {
	c := Config{LogPath: os.Getenv("O12_LOG")}
	c.Level.UnmarshalText([]byte(os.Getenv("O12_LOG_LEVEL")))
	return c
}

// Setup starts recording as c says. The returned function flushes and
// closes the log; call it on exit.
func Setup(c Config) (func() error, error) {
	if c.LogPath == "" {
		return func() error { return nil }, nil
	}
	f, err := os.OpenFile(c.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	start(f, c)
	slog.Info("start", "version", c.Version, "go", runtime.Version(), "os", runtime.GOOS, "arch", runtime.GOARCH,
		"cpus", runtime.NumCPU(), "pid", os.Getpid())
	return Close, nil
}

// start sends events to w; tests use it with a buffer.
func start(w io.Writer, c Config) {
	mu.Lock()
	defer mu.Unlock()
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: c.Level})
	logger = slog.New(h).With("service", "012")
	slog.SetDefault(logger)
	if cl, ok := w.(io.Closer); ok {
		closer = cl
	}
	frames.reset(time.Now())
	on.Store(true)
}

// Close stops recording and closes the log file.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if !on.Load() {
		return nil
	}
	on.Store(false)
	logger = slog.New(slog.DiscardHandler)
	slog.SetDefault(logger)
	if closer != nil {
		err := closer.Close()
		closer = nil
		return err
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

// Span times one operation. The zero Span, returned while off, does
// nothing.
type Span struct {
	name  string
	start time.Time
	// attrs holds up to four attributes given to Start, in the Span
	// itself: keeping the variadic slice would move it to the heap at
	// every call site, telemetry on or off.
	attrs [4]slog.Attr
	n     int
}

// Start begins timing an operation named like "recalc" or "import", with
// up to four attributes known at the start.
func Start(name string, attrs ...slog.Attr) Span {
	if !on.Load() {
		return Span{}
	}
	s := Span{name: name, start: time.Now()}
	s.n = copy(s.attrs[:], attrs)
	return s
}

// End logs the operation with its duration in milliseconds and any
// attributes learned along the way, such as rows read.
func (s Span) End(attrs ...slog.Attr) {
	if s.name == "" || !on.Load() {
		return
	}
	Event(s.name, time.Since(s.start), append(s.attrs[:s.n:s.n], attrs...)...)
}

// Fail ends the span with an error, logged at Warn with its message.
func (s Span) Fail(err error, attrs ...slog.Attr) {
	if err == nil {
		s.End(attrs...)
		return
	}
	if s.name == "" || !on.Load() {
		return
	}
	attrs = append(append(s.attrs[:s.n:s.n], attrs...), slog.String("error", err.Error()))
	log(slog.LevelWarn, s.name, time.Since(s.start), attrs)
}

// Event logs a finished operation that took d.
func Event(name string, d time.Duration, attrs ...slog.Attr) {
	if !on.Load() {
		return
	}
	log(slog.LevelInfo, name, d, attrs)
}

// Debug logs a finished operation at Debug level, for events too
// frequent to keep by default.
func Debug(name string, d time.Duration, attrs ...slog.Attr) {
	if !on.Load() {
		return
	}
	log(slog.LevelDebug, name, d, attrs)
}

func log(level slog.Level, name string, d time.Duration, attrs []slog.Attr) {
	l := Logger()
	ctx := context.Background()
	if !l.Enabled(ctx, level) {
		return
	}
	all := make([]slog.Attr, 0, len(attrs)+2)
	all = append(all, slog.String("event", name), slog.Float64("dur_ms", ms(d)))
	l.LogAttrs(ctx, level, name, append(all, attrs...)...)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
