package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/cowork"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/mcp"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// 012 mcp: a Model Context Protocol server over standard input and
// output on the workbooks in the folders open to it (see
// docs/agents/mcp.md): the client's roots, else --root's, else the
// working directory. A file named is the default of tools called
// without a path.

const mcpUsage = "usage: 012 mcp [file.012] [--root dir]... [--read-only] [--force] [--notebooks [--trust]] [--jev]\n       012 mcp --attach [name|socket]: join a running 012 session listening for agents (see docs/agents/live.md)"

// runMCP is 012 mcp: it serves until the client closes standard input.
// Nothing runs a program or reaches the network unless its flag says:
// --notebooks offers run_notebook_cell, --jev answers JEV functions.
func runMCP(args []string, e env) error {
	a, err := parseArgs(args, []string{"root"}, []string{"read-only", "force", "notebooks", "trust", "jev", "help", "attach"})
	switch {
	case err != nil:
		return usageError(err.Error(), mcpUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, mcpUsage)
		return nil
	case a.has("attach"):
		return attach(a, e)
	case len(a.pos) > 1:
		return usageError("", mcpUsage)
	case a.has("trust") && !a.has("notebooks"):
		return usageError("--trust goes with --notebooks", mcpUsage)
	}
	roots, err := mcp.NewRoots(rootDirs(a.all["root"]))
	if err != nil {
		return err
	}
	o := mcp.Options{Version: buildVersion(), ReadOnly: a.has("read-only"), Force: a.has("force"), Roots: roots}
	if len(a.pos) == 1 {
		if o.Default, err = filepath.Abs(a.pos[0]); err != nil {
			return err
		}
		if _, err := headless.Open(o.Default, true); err != nil {
			return err
		}
	}
	cfg, err := loadConfig(e, nil)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	o.Save = func(f *headless.File) error { return save(f, e) }
	if a.has("jev") {
		o.Prepare = jevPreparer(cfg, e)
	}
	if a.has("notebooks") && !a.has("read-only") {
		o.Notebooks = &headless.NotebookOptions{Runner: e.nuRunner(), Timeout: cfg.Duration("nu-timeout"), NuConfig: cfg.Bool("nu-config")}
		o.MayRun = func(path string, w *sheet.Workbook) error {
			return mayRun(&headless.File{Path: path, Book: w}, cfg, a.has("trust"))
		}
	}
	t := &sdk.IOTransport{Reader: io.NopCloser(e.stdin), Writer: nopWriteCloser{e.stdout}}
	return mcp.New(o).Run(ctx, t)
}

// attach is 012 mcp --attach [name|socket]: live mode, the agent a
// participant of a running session, whose person decides what it may
// do, so no other flag goes with it.
func attach(a cliArgs, e env) error {
	if len(a.flags) > 1 || len(a.pos) > 1 {
		return usageError("--attach takes a session's name or socket and no other flag: the session decides what the agent may do", mcpUsage)
	}
	target := ""
	if len(a.pos) == 1 {
		target = a.pos[0]
	}
	return cowork.Attach(target, e.stdin, e.stdout)
}

// rootDirs are the folders --root gave, else the working directory,
// else, when that's the file system's root (where some hosts start
// servers), the home folder.
func rootDirs(given []string) []string {
	if len(given) > 0 {
		return given
	}
	wd, err := os.Getwd()
	if err != nil || filepath.Dir(wd) != wd {
		return []string{"."}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return []string{home}
	}
	return []string{"."}
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
