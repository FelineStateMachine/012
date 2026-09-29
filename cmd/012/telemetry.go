package main

import (
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// telemetryConfig is where telemetry goes: the log file, level and OTLP
// endpoint from the config (whose flags and variables win over the
// file), and the rest of OTLP's settings from the OTEL_* variables.
// Telemetry stays off when neither a log file nor an endpoint is set.
func telemetryConfig(c *config.Config) telemetry.Config {
	tc := telemetry.ConfigFromEnv()
	tc.LogPath = c.String("log-file")
	tc.OTLP.Endpoint = c.String("otlp-endpoint")
	tc.Level.UnmarshalText([]byte(c.String("log-level")))
	return tc
}

// startTelemetry opens the log and reports every recalculation and pivot
// refresh to it, as spans nested in their workbook's trace.
func startTelemetry(c telemetry.Config) (func() error, error) {
	if info, ok := debug.ReadBuildInfo(); ok {
		c.Version = info.Main.Version
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				c.Version = s.Value
			}
		}
	}
	stop, err := telemetry.Setup(c)
	if err != nil || !telemetry.Enabled() {
		return stop, err
	}
	sheet.OnBegin = func(trace any, op string) {
		if t, ok := trace.(*telemetry.Trace); ok {
			t.Begin(op)
		}
	}
	sheet.OnRecalc = func(trace any, i sheet.RecalcInfo) {
		telemetry.Set("cells", int64(i.Cells))
		endSpan(trace, "recalc", i.Duration,
			slog.Bool("full", i.Full), slog.Int("evaluated", i.Evaluated), slog.Int("cells", i.Cells),
			slog.Int("volatile", i.Volatile), slog.Bool("circular", i.Circular))
	}
	sheet.OnPivot = func(trace any, i sheet.PivotInfo) {
		endSpan(trace, "pivot", i.Duration,
			slog.Int("records", i.Records), slog.Int("groups", i.Groups), slog.Int("cells", i.Cells), slog.Bool("failed", i.Failed))
	}
	sheet.OnLive = func(trace any, i sheet.LiveInfo) {
		endSpan(trace, "live", i.Duration, slog.Int("rows", i.Rows), slog.Int("cells", i.Cells), slog.Bool("reset", i.Reset))
	}
	return stop, nil
}

// endSpan ends the span sheet.OnBegin began in the workbook's trace, or
// logs the operation as a root span of its own when the workbook has no
// trace (one read before the UI took it, say).
func endSpan(trace any, name string, d time.Duration, attrs ...slog.Attr) {
	if t, ok := trace.(*telemetry.Trace); ok && t.End(attrs...) {
		return
	}
	telemetry.Event(name, d, attrs...)
}
