package main

import (
	"log/slog"
	"os"
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

// telemetryFlags takes the telemetry flags (--log, --otlp) out of args,
// and reads the rest of telemetry's settings from the config file and
// environment. 012 serve uses it before parsing its own flags.
func telemetryFlags(args []string) (telemetry.Config, []string, error) {
	flags, rest, err := config.ParseFlags(args)
	if err != nil {
		return telemetry.Config{}, nil, err
	}
	path, err := config.DefaultPath()
	if err != nil {
		return telemetry.Config{}, nil, err
	}
	return telemetryConfig(config.Load(path, os.Getenv, flags)), rest, nil
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
	sheet.OnPivot = func(i sheet.PivotInfo) {
		telemetry.Event("pivot", i.Duration,
			slog.Int("records", i.Records), slog.Int("groups", i.Groups), slog.Int("cells", i.Cells), slog.Bool("failed", i.Failed))
	}
	return stop, nil
}
