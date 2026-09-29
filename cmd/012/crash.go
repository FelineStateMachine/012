package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/ui"
)

// When the UI stops on an internal error (a panic, which ui.Guard
// catches), the terminal is already restored; what's left is to keep
// the unsaved work where the next start on that file offers it back,
// write a report, and say where both are. See docs/files/saving.md.

// recoveryDir is where local sessions keep unsaved work after a crash,
// <config.Dir>/recovery; "" when there's no config directory.
func recoveryDir() string {
	d, err := config.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "recovery")
}

// crashDir is where crash reports go, <config.Dir>/crashes.
func crashDir() string {
	d, err := config.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "crashes")
}

// offersKept reports whether a session started on args, reading
// standard input or not, offers the changes a crash kept: when it opens
// a .012 file or nothing, not when it imports a file or reads a table.
func offersKept(args []string, stdin bool) bool {
	if stdin {
		return false
	}
	if len(args) == 1 {
		if _, imports := fileio.KindOf(args[0]); imports {
			return false
		}
	}
	return true
}

// crashed keeps the unsaved work of the program g ran, writes the
// report, and returns the error that tells the user where both are.
func crashed(g *ui.Guarded, c *ui.Crash) error {
	now := time.Now()
	m := g.Model()
	kept, keepErr := g.Keep(now)
	lines := []string{fmt.Sprintf("stopped on an internal error (a panic in %s: %v)", c.Where, c.Value)}
	switch {
	case keepErr != nil:
		lines = append(lines, "couldn't keep the unsaved changes: "+keepErr.Error())
	case kept != "" && m.Filename() == "":
		lines = append(lines, "unsaved changes kept in "+kept+"; start 012 again to restore them")
	case kept != "":
		lines = append(lines, "unsaved changes kept in "+kept+"; open "+m.Filename()+" again to restore them")
	}
	if dir := crashDir(); dir != "" {
		report, err := c.Report(dir, buildVersion(), m.Filename(), kept, keepErr, now)
		if err != nil {
			lines = append(lines, "couldn't write a report: "+err.Error())
		} else {
			lines = append(lines, "a report is in "+report+"; include it when reporting the problem")
		}
	}
	return errors.New(strings.Join(lines, "\n012: "))
}
