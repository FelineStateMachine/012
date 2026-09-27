package main

import (
	"errors"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// logFlag takes --log path (or --log=path) out of args. Without it the
// log file comes from O12_LOG, and telemetry stays off when neither is
// set.
func logFlag(args []string) (telemetry.Config, []string, error) {
	c := telemetry.ConfigFromEnv()
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--log":
			if i+1 >= len(args) {
				return c, nil, errors.New("--log needs a file")
			}
			c.LogPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--log="):
			c.LogPath = strings.TrimPrefix(a, "--log=")
		default:
			rest = append(rest, a)
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
