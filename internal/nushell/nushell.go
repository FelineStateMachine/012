// Package nushell runs a notebook's code cells: each is a nushell
// pipeline run by `nu` as a separate process, reading other cells'
// outputs and ranges of sheets as variables ($sales, $selection), and
// printing what it made as NUON, which the notebook keeps as the cell's
// output. Tables reach nu as NUON in files it opens, never as text
// spliced into the pipeline. The package knows neither the terminal nor
// the notebook: the UI hands it a Job and gets back NUON.
package nushell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/nuon"
)

// Job is a pipeline to run.
type Job struct {
	Command string
	// Tables are what the pipeline reads, by variable name, as NUON:
	// each becomes the variable of its name.
	Tables map[string][]byte
	// Config runs nu with the user's config files rather than without.
	Config bool
}

// Runner runs a job's script, writing what it prints to stdout. A test
// substitutes a fake for the real nu.
type Runner interface {
	Run(ctx context.Context, job Job, script string, stdout io.Writer) error
}

// ErrMissing is what running a cell says when nu isn't installed.
var ErrMissing = errors.New("nu isn't installed or isn't on your PATH: see docs/nushell/notebooks.md")

// Error is a pipeline that failed: nu's message, and all it wrote to
// standard error.
type Error struct {
	Msg    string
	Stderr string
}

func (e *Error) Error() string { return e.Msg }

// Help is the help line of nu's message, "help: ...", or "".
func (e *Error) Help() string {
	for line := range strings.SplitSeq(e.Stderr, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "help:") {
			return line
		}
	}
	return ""
}

// Script is what nu runs for job: the tables bound to their names, then
// the pipeline, its output as NUON. The files are named by environment
// variables (NU012_TABLE_0 and so on) the runner sets, so no data and no
// path is written into the script.
func Script(job Job, names []string) string {
	var b strings.Builder
	for i, name := range names {
		fmt.Fprintf(&b, "let %s = (open --raw $env.NU012_TABLE_%d | from nuon)\n", name, i)
	}
	b.WriteString("do {\n" + job.Command + "\n} | to nuon\n")
	return b.String()
}

// ErrTooLarge is a run that printed more than it may keep.
var ErrTooLarge = errors.New("the output is too large to keep")

// Exec runs job and returns what it printed, NUON, at most maxBytes of
// it, within timeout.
func Exec(ctx context.Context, r Runner, job Job, timeout time.Duration, maxBytes int) ([]byte, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, timeout, fmt.Errorf("stopped after %s", timeout))
		defer cancel()
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	out := &capped{max: maxBytes, stop: func() { cancel(ErrTooLarge) }}
	err := r.Run(ctx, job, Script(job, sortedNames(job.Tables)), out)
	switch {
	case ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case err != nil:
		return nil, err
	}
	return bytes.TrimSpace(out.b.Bytes()), nil
}

// capped keeps at most max bytes, stopping the run past them.
type capped struct {
	b    bytes.Buffer
	max  int
	stop func()
}

func (c *capped) Write(p []byte) (int, error) {
	if c.max > 0 && c.b.Len()+len(p) > c.max {
		c.stop()
		return 0, ErrTooLarge
	}
	return c.b.Write(p)
}

func sortedNames(tables map[string][]byte) []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Commands lists nu's command names, for completing them.
func Commands(ctx context.Context, r Runner, timeout time.Duration) ([]string, error) {
	data, err := Exec(ctx, r, Job{Command: "help commands | get name"}, timeout, 0)
	if err != nil {
		return nil, err
	}
	v, err := nuon.Parse(data)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range v.List {
		if n.Kind == nuon.String {
			out = append(out, n.Str)
		}
	}
	return out, nil
}

// Nu is the real runner: `nu -c` in a process of its own.
type Nu struct {
	// Path is the program; "" finds nu on the PATH.
	Path string
}

// Run implements Runner.
func (n Nu) Run(ctx context.Context, job Job, script string, stdout io.Writer) error {
	path := n.Path
	if path == "" {
		var err error
		if path, err = exec.LookPath("nu"); err != nil {
			return ErrMissing
		}
	}
	dir, err := os.MkdirTemp("", "012-nu-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	env := append(os.Environ(), "NO_COLOR=1")
	for i, name := range sortedNames(job.Tables) {
		file := dir + string(os.PathSeparator) + strconv.Itoa(i) + ".nuon"
		if err := os.WriteFile(file, job.Tables[name], 0o600); err != nil {
			return err
		}
		env = append(env, "NU012_TABLE_"+strconv.Itoa(i)+"="+file)
	}
	args := []string{"-c", script}
	if !job.Config {
		args = append([]string{"--no-config-file"}, args...)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.Stdout = env, stdout
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &limited{w: &stderr, n: 64 << 10}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		text := ansi.Strip(stderr.String())
		return &Error{Msg: Message(text, err), Stderr: text}
	}
	return nil
}

// Message is the line of nu's standard error that says what went wrong:
// the one marked ×, or the first, or how the process ended.
func Message(stderr string, err error) string {
	first := ""
	for line := range strings.SplitSeq(stderr, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "×"); ok {
			return strings.TrimSpace(rest)
		}
		if first == "" && line != "" {
			first = line
		}
	}
	if first != "" {
		return first
	}
	return err.Error()
}

// limited writes at most n bytes to w, dropping the rest.
type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.n -= k
		if _, err := l.w.Write(p[:k]); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
