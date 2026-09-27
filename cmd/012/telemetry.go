package main

import (
	"errors"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// telemetryFlags takes --log path and --otlp url (or --log=path,
// --otlp=url) out of args. Without them the log file comes from O12_LOG
// and the OTLP endpoint from OTEL_EXPORTER_OTLP_ENDPOINT (and the other
// OTEL_* variables), and telemetry stays off when none is set.
func telemetryFlags(args []string) (telemetry.Config, []string, error) {
	c := telemetry.ConfigFromEnv()
	flags := map[string]*string{"--log": &c.LogPath, "--otlp": &c.OTLP.Endpoint}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, value, hasValue := strings.Cut(a, "=")
		dst, ok := flags[name]
		switch {
		case !ok:
			rest = append(rest, a)
		case hasValue:
			*dst = value
		case i+1 < len(args):
			*dst = args[i+1]
			i++
		default:
			return c, nil, errors.New(name + " needs a value")
		}
	}
	return c, rest, nil
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
