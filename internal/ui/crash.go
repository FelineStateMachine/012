package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Crashes: a panic anywhere in the model (Update, View, Init, or a
// command it started) stops the program without taking the terminal
// down with it. Guard wraps the model for Bubble Tea: it recovers the
// panic, remembers where it was and what with, and quits, so Bubble Tea
// restores the terminal as on any quit. View can't return a command to
// quit with, so a panic there goes on to Bubble Tea, which restores the
// terminal itself and prints the stack. The caller then keeps the
// unsaved work for recovery (Keep, recovery.go) and writes a report
// (Crash.Report). See docs/files/saving.md.

// Crash is a panic Guard caught.
type Crash struct {
	Where string // "update", "view", "init" or "a command"
	Value any    // what was panicked with
	Stack []byte // the panicking goroutine's stack; nil when Bubble Tea caught it
}

// Guarded is a model run under Guard.
type Guarded struct {
	m     *Model
	crash *Crash
	last  tea.View // the last frame drawn, shown until the program quits
	// viewPanics makes View panic, for crashTest.
	viewPanics bool
}

// Guard wraps m so a panic in it stops the program without losing the
// terminal; see Finish for what's left afterwards.
func Guard(m *Model) *Guarded {
	return &Guarded{m: m}
}

// Model is the model guarded.
func (g *Guarded) Model() *Model { return g.m }

// crashMsg is a panic caught in a command, brought to Update.
type crashMsg struct{ crash *Crash }

// caught makes a Crash of a recovered panic.
func caught(where string, r any) *Crash {
	return &Crash{Where: where, Value: r, Stack: debug.Stack()}
}

// Init implements tea.Model.
func (g *Guarded) Init() (cmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			g.crash = caught("init", r)
			cmd = tea.Quit
		}
	}()
	return g.guard(g.m.Init())
}

// Update implements tea.Model.
func (g *Guarded) Update(msg tea.Msg) (_ tea.Model, cmd tea.Cmd) {
	if c, ok := msg.(crashMsg); ok {
		if g.crash == nil {
			g.crash = c.crash
		}
		return g, tea.Quit
	}
	if g.crash != nil {
		return g, nil // quitting; the model may be broken
	}
	defer func() {
		if r := recover(); r != nil {
			g.crash = caught("update", r)
			cmd = tea.Quit
		}
	}()
	if crashTest != nil {
		if c := g.test(crashTest(msg)); c != nil {
			return g, c
		}
	}
	next, cmd := g.m.Update(msg)
	if m, ok := next.(*Model); ok {
		g.m = m
	}
	return g, g.guard(cmd)
}

// View implements tea.Model. It keeps showing the last frame while the
// program quits after a crash.
func (g *Guarded) View() tea.View {
	if g.crash != nil {
		return g.last
	}
	defer func() {
		if r := recover(); r != nil {
			g.crash = caught("view", r)
			panic(r) // for Bubble Tea to stop the program: see above
		}
	}()
	if g.viewPanics {
		panic("crash test in view")
	}
	g.last = g.m.View()
	return g.last
}

// do runs fn on the model as Update would, a panic in it stopping the
// program the same way: for what Shared does to the model between
// turns.
func (g *Guarded) do(fn func() tea.Cmd) (cmd tea.Cmd) {
	if g.crash != nil {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			g.crash = caught("update", r)
			cmd = tea.Quit
		}
	}()
	return g.guard(fn())
}

// guard wraps cmd so a panic in it comes to Update as a crashMsg, and
// so do the commands of a batch it returns. A sequence's (tea.Sequence)
// are run by Bubble Tea itself, which catches their panics: see Finish.
func (g *Guarded) guard(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = crashMsg{caught("a command", r)}
			}
		}()
		msg = cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			out := make(tea.BatchMsg, len(batch))
			for i, c := range batch {
				out[i] = g.guard(c)
			}
			return out
		}
		return msg
	}
}

// Finish is called once the program has stopped, with what Run
// returned. It reports the crash that stopped it, if one did: one Guard
// caught, or one only Bubble Tea caught (a panic in a sequence's
// command), whose stack it printed on the terminal.
func (g *Guarded) Finish(runErr error) *Crash {
	if g.m.share.seat == nil {
		g.m.closeStreams() // a stream runs until it's stopped; a room's, until the last one leaves (LeaveRoom)
	}
	if g.crash == nil && errors.Is(runErr, tea.ErrProgramPanic) {
		g.crash = &Crash{Where: "a command", Value: "a panic Bubble Tea caught, its stack printed on the terminal"}
	}
	return g.crash
}

// Keep writes the model's unsaved work to a recovery file, returning its
// name ("" when there was nothing unsaved). A model a panic left too
// broken to write is reported as an error rather than panicking again.
func (g *Guarded) Keep(now time.Time) (kept string, err error) {
	defer func() {
		if r := recover(); r != nil {
			kept, err = "", fmt.Errorf("writing the workbook failed too: %v", r)
		}
	}()
	if !g.m.Unsaved() {
		return "", nil
	}
	return g.m.Recover(now)
}

// crashReportsKept is how many reports Report leaves in its directory;
// older ones are removed.
const crashReportsKept = 10

// Report writes a short report of the crash into dir, for the user to
// send with an issue, and returns its path. It says where the panic was,
// what with, the stack, the build (version), the system, the file open
// and where its unsaved work was kept; never the workbook's contents.
func (c *Crash) Report(dir, version, file, kept string, keepErr error, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "012 stopped on an internal error (a panic in %s)\n\n", c.Where)
	fmt.Fprintf(&b, "Time:    %s\n", now.Format(time.RFC3339))
	fmt.Fprintf(&b, "Version: %s\n", version)
	fmt.Fprintf(&b, "System:  %s/%s, %s\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Fprintf(&b, "File:    %s\n", orNone(file, "(untitled)"))
	switch {
	case keepErr != nil:
		fmt.Fprintf(&b, "Kept:    couldn't keep the unsaved changes: %v\n", keepErr)
	default:
		fmt.Fprintf(&b, "Kept:    %s\n", orNone(kept, "nothing unsaved"))
	}
	fmt.Fprintf(&b, "\nPanic: %v\n", c.Value)
	if len(c.Stack) > 0 {
		fmt.Fprintf(&b, "\n%s", c.Stack)
	}
	path := filepath.Join(dir, "crash-"+now.Format(recoveryStamp)+".txt")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	for i := 2; errors.Is(err, os.ErrExist) && i < 100; i++ {
		path = filepath.Join(dir, fmt.Sprintf("crash-%s-%d.txt", now.Format(recoveryStamp), i))
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	}
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(b.String())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	pruneReports(dir)
	return path, err
}

// pruneReports removes the oldest crash reports in dir beyond
// crashReportsKept. Their names sort by the time they were written.
func pruneReports(dir string) {
	names, _ := filepath.Glob(filepath.Join(dir, "crash-*.txt"))
	for _, n := range names[:max(0, len(names)-crashReportsKept)] {
		os.Remove(n)
	}
}

func orNone(s, none string) string {
	if s == "" {
		return none
	}
	return s
}

// crashTest, when set (only in builds tagged crashtest, for the e2e
// tests: crashtest.go), says where a message should make the program
// panic: "update", "view", "command", or "" for nowhere.
var crashTest func(tea.Msg) string

// test panics where crashTest said, or returns a command that will.
func (g *Guarded) test(where string) tea.Cmd {
	switch where {
	case "update":
		panic("crash test in update")
	case "view":
		g.viewPanics = true
	case "command":
		return g.guard(func() tea.Msg { panic("crash test in a command") })
	}
	return nil
}
