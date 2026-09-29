package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/nushell"
)

// jevTimeout bounds each JEV question, as on the screen.
const jevTimeout = 60 * time.Second

// evaluate runs what the flags ask of a workbook before it's read:
// --regions runs its notebook regions' commands with nu, when the
// workbook is trusted here or --trust says so, as the screen asks before
// running them; --jev answers its JEV functions over the network. Region
// failures are reported on standard error as they happen.
func evaluate(f *headless.File, a cliArgs, e env) evalFailures {
	if a.has("trust") && !a.has("regions") {
		return evalFailures{stop: usageError("--trust goes with --regions", "")}
	}
	if !a.has("regions") && !a.has("jev") {
		return evalFailures{}
	}
	cfg, err := loadConfig(e, nil)
	if err != nil {
		return evalFailures{stop: err}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var out evalFailures
	if a.has("regions") {
		if err := mayRun(f, cfg, a.has("trust")); err != nil {
			return evalFailures{stop: err}
		}
		out.regions = headless.RunRegions(ctx, f.Book, headless.RegionOptions{
			Runner: e.nuRunner(), Timeout: cfg.Duration("nu-timeout"), NuConfig: cfg.Bool("nu-config")})
		for _, r := range out.regions {
			fmt.Fprintln(e.stderr, "012: "+r)
		}
	}
	if a.has("jev") {
		client, err := jevClient(ctx, cfg, e)
		if err != nil {
			return evalFailures{stop: err}
		}
		headless.AnswerJEV(ctx, f.Book, client, jevTimeout)
	}
	return out
}

// mayRun reports why a workbook's commands may not run: shell = off, or
// a workbook from another computer without --trust (or shell = on). A
// trusted workbook is marked trusted here, which a command that saves
// keeps, as answering Run on the screen does.
func mayRun(f *headless.File, cfg *config.Config, trust bool) error {
	switch cfg.String("shell") {
	case "off":
		return errors.New("shell commands are off (shell = off in your config)")
	case "on":
		return nil
	}
	machine := machineID()
	if headless.Trusted(f.Book, machine) {
		return nil
	}
	if !trust {
		return fmt.Errorf("%s was saved on another computer: its commands would run as you, with your files; --trust runs them", f.Path)
	}
	if machine != "" {
		f.Book.SetMacroOrigin(machine)
	}
	return nil
}

// nuRunner runs region commands: nu, or the test's fake.
func (e env) nuRunner() nushell.Runner {
	if e.nu != nil {
		return e.nu
	}
	return nushell.Nu{}
}

// jevClient connects to JEV with the key from the environment, the
// credential store or jev-api-key-command, as the screen finds it.
func jevClient(ctx context.Context, cfg *config.Config, e env) (jev.Client, error) {
	key, _, err := jev.ResolveKey(ctx, jev.KeySources{Getenv: e.getenv, Store: keyStore(cfg, e), Command: cfg.String("jev-api-key-command")})
	if err != nil {
		return nil, fmt.Errorf("JEV API key: %w", err)
	}
	if key == "" {
		return nil, errors.New("--jev needs an API key: run 012 config set-key, or set " + jev.KeyEnv)
	}
	return jev.NewClient(jevConfig(cfg, key))
}
