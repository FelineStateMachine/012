// Command 012 is a Lotus 1-2-3 style spreadsheet for the terminal.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "012:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	tc, args, err := telemetryFlags(args)
	if err != nil || len(args) > 1 {
		return errors.New("usage: 012 [--log file.jsonl] [--otlp http://localhost:4318] [file]: a " + sheet.FileExt + " sheet, or a .csv, .tsv, .xlsx, .sqlite, .parquet or .wk1 file to import")
	}
	stopTelemetry, err := startTelemetry(tc)
	if err != nil {
		return err
	}
	defer stopTelemetry()
	// JEV functions run when an API key is set, in the environment or a
	// .env file next to the sheet or in the current directory. The cache
	// goes in before loading so the file's JEV cells queue their questions.
	var jevClient jev.Client
	var jevCache *jev.Cache
	dirs := []string{"."}
	if len(args) == 1 {
		dirs = append(dirs, filepath.Dir(args[0]))
	}
	if cfg, ok := jev.LoadConfig(dirs...); ok {
		c, err := jev.NewClient(cfg)
		if err != nil {
			return fmt.Errorf("JEV: %w", err)
		}
		jevClient, jevCache = c, jev.NewCache()
		sheet.Remote = jevCache
	}

	s, name := sheet.New(), ""
	var importName string
	if len(args) == 1 {
		if _, ok := fileio.KindOf(args[0]); ok {
			// Other formats are imported in the background once the
			// screen is up, with progress; the file must exist.
			if _, err := os.Stat(args[0]); err != nil {
				return err
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
			return err
		default:
			defer f.Close()
			if s, err = sheet.Read(f); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	m := ui.New(s, name)
	if importName != "" {
		m.Import(importName)
	}
	if jevClient != nil {
		m.EnableJEV(jevClient, jevCache)
	}
	_, err = tea.NewProgram(m).Run()
	return err
}
