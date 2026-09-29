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
// --notebooks runs its notebook cells with nu, when the workbook is
// trusted here or --trust says so, as the screen asks before running
// them; --jev answers its JEV functions over the network. Cells that
// fail are reported on standard error.
func evaluate(f *headless.File, a cliArgs, e env) evalFailures {
	if a.has("trust") && !a.has("notebooks") {
		return evalFailures{stop: usageError("--trust goes with --notebooks", "")}
	}
	if !a.has("notebooks") && !a.has("jev") {
		return evalFailures{}
	}
	cfg, err := loadConfig(e, nil)
	if err != nil {
		return evalFailures{stop: err}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var out evalFailures
	if a.has("notebooks") {
		if err := mayRun(f, cfg, a.has("trust")); err != nil {
			return evalFailures{stop: err}
		}
		out.cells = headless.RunNotebooks(ctx, f.Book, headless.NotebookOptions{
			Runner: e.nuRunner(), Timeout: cfg.Duration("nu-timeout"), NuConfig: cfg.Bool("nu-config")})
		for _, c := range out.cells {
			fmt.Fprintln(e.stderr, "012: "+c)
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

// mayRun reports why a workbook's notebook cells may not run: shell =
// off, or a workbook from another computer without --trust (or shell =
// on). A trusted workbook is marked trusted here, which a command that
// saves keeps, as answering Run on the screen does.
func mayRun(f *headless.File, cfg *config.Config, trust bool) error {
	switch cfg.String("shell") {
	case "off":
		return errors.New("notebook cells don't run (shell = off in your config)")
	case "on":
		return nil
	}
	machine := machineID()
	if headless.Trusted(f.Book, machine) {
		return nil
	}
	if !trust {
		return fmt.Errorf("%s was saved on another computer: its notebook cells would run as you, with your files; --trust runs them", f.Path)
	}
	if machine != "" {
		f.Book.SetMacroOrigin(machine)
	}
	return nil
}

// nuRunner runs notebook cells: nu, or the test's fake.
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
