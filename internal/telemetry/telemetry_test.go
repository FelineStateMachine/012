package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// record turns telemetry on into a buffer for the test.
func record(t testing.TB, level slog.Level) *bytes.Buffer {
	var buf bytes.Buffer
	start(&buf, nil, Config{Level: level})
	t.Cleanup(func() { Close() })
	return &buf
}

func events(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func TestOffRecordsNothing(t *testing.T) {
	if Enabled() {
		t.Fatal("on without Setup")
	}
	s := Start("recalc")
	s.End(slog.Int("cells", 1))
	Event("x", time.Second)
	Frame(time.Millisecond, time.Millisecond)
	Set("cells", 3)
	if s.name != "" || s.n != 0 {
		t.Errorf("span while off = %+v", s)
	}
	stop, err := Setup(Config{})
	if err != nil || Enabled() {
		t.Fatalf("Setup without a path: %v, enabled %v", err, Enabled())
	}
	stop()
}

func TestSpansAreJSONEvents(t *testing.T) {
	buf := record(t, slog.LevelInfo)
	s := Start("import", slog.String("format", "CSV"))
	s.End(slog.Int("rows", 42))
	Start("open").Fail(errors.New("bad file"))
	Debug("frame", time.Millisecond) // below the level
	evs := events(t, buf)
	if len(evs) != 2 {
		t.Fatalf("%d events: %s", len(evs), buf)
	}
	imp := evs[0]
	if imp["event"] != "import" || imp["format"] != "CSV" || imp["rows"] != 42.0 || imp["service"] != "012" || imp["level"] != "INFO" {
		t.Errorf("import event %v", imp)
	}
	if _, ok := imp["dur_ms"].(float64); !ok {
		t.Errorf("no duration in %v", imp)
	}
	if evs[1]["level"] != "WARN" || evs[1]["error"] != "bad file" {
		t.Errorf("failure event %v", evs[1])
	}
}

func TestFramesAreSummarized(t *testing.T) {
	buf := record(t, slog.LevelInfo)
	Set("cells", 1234)
	for i := range 10 {
		Frame(time.Duration(i+1)*time.Millisecond, 0)
	}
	Frame(time.Millisecond, 20*time.Millisecond)
	if buf.Len() > 0 {
		t.Fatalf("summary before the window closed: %s", buf)
	}
	frames.mu.Lock()
	frames.since = time.Now().Add(-2 * window)
	frames.mu.Unlock()
	Frame(time.Millisecond, 0)
	evs := events(t, buf)
	if len(evs) != 1 {
		t.Fatalf("%d events: %s", len(evs), buf)
	}
	ev := evs[0]
	want := map[string]float64{"frames": 12, "render_max_ms": 10, "keys": 1, "key_p95_ms": 20, "cells": 1234}
	for k, v := range want {
		if ev[k] != v {
			t.Errorf("%s = %v, want %v", k, ev[k], v)
		}
	}
	if h, _ := ev["heap_bytes"].(float64); h <= 0 {
		t.Errorf("heap_bytes = %v", ev["heap_bytes"])
	}
}

func TestPercentile(t *testing.T) {
	ds := []time.Duration{5, 1, 4, 2, 3}
	tests := []struct {
		p    int
		want time.Duration
	}{{0, 1}, {50, 3}, {95, 5}, {100, 5}}
	for _, tt := range tests {
		if got := percentile(ds, tt.p); got != tt.want {
			t.Errorf("p%d = %v, want %v", tt.p, got, tt.want)
		}
	}
	if percentile(nil, 50) != 0 {
		t.Error("empty percentile")
	}
}

// Call sites must not allocate while telemetry is off, OTLP compiled in
// or not.
func TestOffAllocatesNothing(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		s := Start("recalc", slog.Int("n", 1))
		s.End(slog.Int("cells", 2))
		Event("recalc", time.Millisecond, slog.Int("cells", 2))
		Frame(time.Millisecond, 0)
	})
	if n != 0 {
		t.Errorf("%v allocations per call while off", n)
	}
}

// The cost at a call site while telemetry is off: what every keystroke
// pays for having it compiled in.
func BenchmarkSpanOff(b *testing.B) {
	for b.Loop() {
		s := Start("recalc", slog.Int("n", 1))
		s.End(slog.Int("cells", 2))
	}
}

func BenchmarkFrameOff(b *testing.B) {
	for b.Loop() {
		Frame(time.Millisecond, 0)
	}
}

// The cost while on, writing JSON (to io.Discard).
func BenchmarkSpanOn(b *testing.B) {
	start(io.Discard, nil, Config{})
	defer Close()
	for b.Loop() {
		s := Start("recalc", slog.Int("n", 1))
		s.End(slog.Int("cells", 2))
	}
}

func BenchmarkFrameOn(b *testing.B) {
	start(io.Discard, nil, Config{})
	defer Close()
	for b.Loop() {
		Frame(time.Millisecond, 0)
	}
}
