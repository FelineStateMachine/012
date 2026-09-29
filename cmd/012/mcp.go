package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/mcp"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// 012 mcp: a Model Context Protocol server over standard input and
// output on one workbook file (see docs/agents/mcp.md).

const mcpUsage = "usage: 012 mcp file.012 [--read-only] [--force] [--notebooks [--trust]] [--jev]"

// runMCP is 012 mcp: it serves until the client closes standard input.
// Nothing runs a program or reaches the network unless its flag says:
// --notebooks offers run_notebook_cell, --jev answers JEV functions.
func runMCP(args []string, e env) error {
	a, err := parseArgs(args, nil, []string{"read-only", "force", "notebooks", "trust", "jev", "help"})
	switch {
	case err != nil:
		return usageError(err.Error(), mcpUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, mcpUsage)
		return nil
	case len(a.pos) != 1:
		return usageError("", mcpUsage)
	case a.has("trust") && !a.has("notebooks"):
		return usageError("--trust goes with --notebooks", mcpUsage)
	}
	path := a.pos[0]
	if _, err := headless.Open(path, true); err != nil {
		return err
	}
	cfg, err := loadConfig(e, nil)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	b := &mcp.FileBackend{Path: path, Save: func(f *headless.File) error { return save(f, e) }}
	if a.has("jev") {
		b.Prepare = jevPreparer(cfg, e)
	}
	o := mcp.Options{Version: buildVersion(), ReadOnly: a.has("read-only"), Force: a.has("force")}
	if a.has("notebooks") && !a.has("read-only") {
		o.Notebooks = &headless.NotebookOptions{Runner: e.nuRunner(), Timeout: cfg.Duration("nu-timeout"), NuConfig: cfg.Bool("nu-config")}
		o.MayRun = func(w *sheet.Workbook) error {
			return mayRun(&headless.File{Path: path, Book: w}, cfg, a.has("trust"))
		}
	}
	t := &sdk.IOTransport{Reader: io.NopCloser(e.stdin), Writer: nopWriteCloser{e.stdout}}
	return mcp.New(b, o).Run(ctx, t)
}

// jevPreparer answers a workbook's JEV functions each time it's opened,
// connecting once and keeping the answers, so each question is asked
// once a session.
func jevPreparer(cfg *config.Config, e env) func(context.Context, *headless.File) error {
	var once sync.Once
	var client jev.Client
	var connErr error
	cache := jev.NewCache()
	return func(ctx context.Context, f *headless.File) error {
		once.Do(func() { client, connErr = jevClient(ctx, cfg, e) })
		if connErr != nil {
			return connErr
		}
		headless.AnswerJEVFrom(ctx, f.Book, client, cache, jevTimeout)
		return nil
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
