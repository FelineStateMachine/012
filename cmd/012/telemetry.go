package main

import (
	"log/slog"
	"runtime/debug"

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

// startTelemetry opens the log and reports every recalculation to it.
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
	sheet.OnRecalc = func(i sheet.RecalcInfo) {
		telemetry.Set("cells", int64(i.Cells))
		telemetry.Event("recalc", i.Duration,
			slog.Bool("full", i.Full), slog.Int("evaluated", i.Evaluated), slog.Int("cells", i.Cells),
			slog.Int("volatile", i.Volatile), slog.Bool("circular", i.Circular))
	}
	return stop, nil
}
