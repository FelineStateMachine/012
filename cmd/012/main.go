// Command 012 is a Lotus 1-2-3 style spreadsheet for the terminal.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/keyring"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui"
)

func main() {
	if err := run(os.Args[1:], system()); err != nil {
		fmt.Fprintln(os.Stderr, "012:", err)
		os.Exit(1)
	}
}

// env is what 012 takes from the process, so tests can substitute a
// temporary config and a fake credential store.
type env struct {
	getenv   func(string) string
	keys     keyring.Store // the OS credential store
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
	readKey  func(prompt string) (string, error) // reads a secret from the terminal without echo
	isTTY    bool                                // stdin is a terminal
	runTUI   func(m tea.Model, opts ...tea.ProgramOption) error
	editorIO bool // run the editor attached to the terminal
	// openTTY opens the terminal for the UI when standard input and
	// output belong to a pipeline (012 -, --pipe).
	openTTY func() (io.ReadCloser, io.WriteCloser, error)
}

func system() env {
	return env{
		getenv: os.Getenv, keys: keyring.System(),
		stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr,
		readKey: readPassword, isTTY: isTerminal(os.Stdin), editorIO: true,
		runTUI: func(m tea.Model, opts ...tea.ProgramOption) error {
			_, err := tea.NewProgram(m, append([]tea.ProgramOption{tea.WithFPS(ui.FrameRate)}, opts...)...).Run()
			return err
		},
		openTTY: func() (io.ReadCloser, io.WriteCloser, error) {
			in, out, err := openTTY()
			if err != nil {
				return nil, nil, err
			}
			return in, out, nil
		},
	}
}

func usage() error {
	return errors.New("usage: 012 " + config.FlagUsage() + " [file]: a " + sheet.FileExt +
		" sheet, or a .csv, .tsv, .json, .nuon, .xlsx, .sqlite, .parquet or .wk1 file to import\n" +
		"       012 [flags] -: a table from standard input (NUON, JSON, CSV or TSV)\n" +
		"       012 [flags] --pipe [--to nuon|json|csv|tsv] [file]: on quitting, send the table to standard output\n" +
		"       012 nu [flags] [file]: a nushell notebook, at its prompt (see docs/terminal/nushell.md)\n" +
		"       012 serve [flags] [dir]: serve sheets in dir over SSH (see docs/terminal/ssh.md)\n" +
		"       012 config [path|edit|default|themes|set-key|delete-key]\n" +
		"       012 version")
}

func run(args []string, e env) error {
	if len(args) > 0 && args[0] == "config" {
		return runConfig(args[1:], e)
	}
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		return runVersion(e)
	}
	if len(args) > 0 && args[0] == "serve" {
		// A file called serve opens as ./serve.
		return runServe(args[1:], e)
	}
	// A file called nu opens as ./nu.
	notebook := len(args) > 0 && args[0] == "nu"
	if notebook {
		args = args[1:]
	}
	flags, args, err := config.ParseFlags(args)
	if err != nil {
		return usage()
	}
	pipe, args, err := parsePipe(args)
	if err != nil {
		return err
	}
	if len(args) > 1 {
		return usage()
	}
	if err := pipe.check(args, e.isTTY); err != nil {
		return err
	}
	cfg, err := loadConfig(e, flags)
	if err != nil {
		return err
	}
	stopTelemetry, err := startTelemetry(telemetryConfig(cfg))
	if err != nil {
		return err
	}
	defer stopTelemetry()

	m, err := openModel(args)
	if err != nil {
		return err
	}
	settings := ui.Settings{
		Config:    cfg,
		Reload:    func() *config.Config { c, _ := loadConfig(e, flags); return c },
		ThemesDir: config.ThemesDir(),
		Keys:      keyStore(cfg, e),
		Connect:   func(key string) (jev.Client, error) { return jev.NewClient(jevConfig(cfg, key)) },
		Notes:     configNotes(cfg),
	}
	client, notes := startJEV(cfg, e, settings.Keys)
	settings.Notes = append(settings.Notes, notes...)
	if client != nil {
		m.EnableJEV(client, jev.NewCache())
	}
	m.Configure(settings)
	if notebook {
		m.OpenShell()
	}
	setPipe(m, pipe, e.stdin)
	opts, closeTTY, err := tuiOptions(pipe, e)
	if err != nil {
		return err
	}
	err = e.runTUI(m, opts...)
	closeTTY()
	if err != nil {
		return err
	}
	return sendPiped(m, pipe, e.stdout)
}

// loadConfig reads the config file with the environment and flags.
func loadConfig(e env, flags map[string]string) (*config.Config, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, fmt.Errorf("finding the config directory: %w", err)
	}
	return config.Load(path, e.getenv, flags), nil
}

// configNotes summarizes the config's warnings for the context line.
func configNotes(c *config.Config) []string {
	switch n := len(c.Warnings); n {
	case 0:
		return nil
	case 1:
		return []string{"Config: " + c.Warnings[0].Short()}
	default:
		return []string{fmt.Sprintf("Config: %s (and %d more; run 012 config)", c.Warnings[0].Short(), n-1)}
	}
}

// keyStore is the credential store, unless the config turns it off.
func keyStore(c *config.Config, e env) keyring.Store {
	if !c.Bool("jev-credential-store") {
		return nil
	}
	return e.keys
}

func jevConfig(c *config.Config, key string) jev.Config {
	return jev.Config{APIKey: key, BaseURL: c.String("jev-base-url"), Model: c.String("jev-model")}
}

// startJEV finds the API key and connects, returning notes for the
// context line: why the key couldn't be read, or that a .env file holds
// a key, which 012 doesn't read.
//
// jev-api-key-command runs only when the environment and the credential
// store have no key, and then only when a sheet first asks JEV
// something, so a password manager doesn't prompt on every start.
func startJEV(c *config.Config, e env, store keyring.Store) (jev.Client, []string) {
	ctx, cancel := context.WithTimeout(context.Background(), jev.KeyCommandTimeout)
	defer cancel()
	key, _, err := jev.ResolveKey(ctx, jev.KeySources{Getenv: e.getenv, Store: store})
	var notes []string
	if err != nil {
		notes = append(notes, "JEV API key: "+err.Error())
	}
	if command := c.String("jev-api-key-command"); key == "" && command != "" {
		return jev.Lazy(func() (jev.Client, error) {
			key, _, err := jev.ResolveKey(context.Background(), jev.KeySources{Command: command})
			if err != nil {
				return nil, err
			}
			return jev.NewClient(jevConfig(c, key))
		}), notes
	}
	if key == "" {
		if jev.DotEnvHasKey(".") {
			notes = append(notes, "012 no longer reads TYPESAFE_API_KEY from .env files: run 012 config set-key to store it")
		}
		return nil, notes
	}
	client, err := jev.NewClient(jevConfig(c, key))
	if err != nil {
		return nil, append(notes, "JEV: "+err.Error())
	}
	return client, notes
}

// openModel opens the sheet named in args, or starts an import of it.
func openModel(args []string) (*ui.Model, error) {
	s, name := sheet.New(), ""
	var importName string
	if len(args) == 1 {
		if _, ok := fileio.KindOf(args[0]); ok {
			// Other formats are imported in the background once the
			// screen is up, with progress; the file must exist.
			if _, err := os.Stat(args[0]); err != nil {
				return nil, err
			}
			importName, args = args[0], nil
		}
	}
	if len(args) == 1 {
		name = args[0]
		f, err := os.Open(name)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// Start a new worksheet that will be saved under this name.
		case err != nil:
			return nil, err
		default:
			defer f.Close()
			if s, err = sheet.Read(f); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	m := ui.New(s, name)
	m.SetMachine(machineID())
	m.AllowEditor()
	if importName != "" {
		m.Import(importName)
	}
	return m, nil
}
