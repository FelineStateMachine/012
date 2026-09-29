// Package nushell runs a notebook's commands: each command is a nushell
// pipeline run by `nu` as a separate process, reading other regions'
// tables as variables ($r1) and the selection as $in, and writing a
// table back as NUON, which becomes the region's cells. Tables reach nu
// as NUON in files it opens, never as text spliced into the command.
// The package knows neither the terminal nor the workbook's regions:
// the UI hands it a Job and gets back a table.
package nushell

import (
	"bufio"
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

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Job is a command to run.
type Job struct {
	Command string
	// Tables are the regions the command reads, by name, as NUON: each
	// becomes the variable of its name.
	Tables map[string][]byte
	// Input is the table the command reads as $in, as NUON, or nil.
	Input []byte
	// Config runs nu with the user's config files rather than without.
	Config bool
}

// Runner runs a job's script, writing what it prints to stdout. A test
// substitutes a fake for the real nu.
type Runner interface {
	Run(ctx context.Context, job Job, script string, stdout io.Writer) error
}

// ErrMissing is what running a command says when nu isn't installed.
var ErrMissing = errors.New("nu isn't installed or isn't on your PATH: see docs/nushell/notebooks.md")

// Error is a command that failed: nu's message, and all it wrote to
// standard error.
type Error struct {
	Msg    string
	Stderr string
}

func (e *Error) Error() string { return e.Msg }

// Script is what nu runs for job: the tables bound to their names, then
// the command, its output as NUON. The files are named by environment
// variables (NU012_TABLE_0 and so on) the runner sets, so no data and no
// path is written into the script.
func Script(job Job, names []string) string {
	var b strings.Builder
	for i, name := range names {
		fmt.Fprintf(&b, "let %s = (open --raw $env.NU012_TABLE_%d | from nuon)\n", name, i)
	}
	if job.Input != nil {
		b.WriteString("$in | from nuon | ")
	}
	b.WriteString("do {\n" + job.Command + "\n} | to nuon\n")
	return b.String()
}

// Exec runs job and reads what it prints as a table, keeping at most
// maxCells cells (0 for the max-cells setting), within timeout.
func Exec(ctx context.Context, r Runner, job Job, timeout time.Duration, maxCells int) (*sheet.RegionData, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, timeout, fmt.Errorf("stopped after %s", timeout))
		defer cancel()
	}
	names := sortedNames(job.Tables)
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := r.Run(ctx, job, Script(job, names), pw)
		pw.CloseWithError(err)
		done <- err
	}()
	data, readErr := readTable(ctx, pr, maxCells)
	pr.CloseWithError(errors.New("done reading")) // the command stops if it's still writing
	runErr := <-done
	switch {
	case ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case runErr != nil:
		return nil, runErr
	case readErr != nil:
		return nil, fmt.Errorf("nu's output isn't NUON: %w", readErr)
	}
	return data, nil
}

// readTable reads NUON as a region's table. A value that isn't a table,
// record or list (a number, a string, null) is read as a list of one,
// a table of one column named value.
func readTable(ctx context.Context, r io.Reader, maxCells int) (*sheet.RegionData, error) {
	br := bufio.NewReader(r)
	first, err := firstByte(br)
	if err != nil {
		return nil, err
	}
	in := io.Reader(br)
	if first != '[' && first != '{' {
		in = io.MultiReader(strings.NewReader("["), br, strings.NewReader("]"))
	}
	res, err := fileio.ImportReader(ctx, "nu", in, fileio.Options{MaxCells: maxCells})
	if err != nil {
		return nil, err
	}
	d := sheet.RegionDataOf(res.Sheet)
	d.Note = strings.Join(res.Notes, "; ")
	return d, nil
}

// firstByte is the first byte past spaces, without taking it; 0 when
// there's nothing else.
func firstByte(br *bufio.Reader) (byte, error) {
	for {
		c, err := br.ReadByte()
		switch {
		case errors.Is(err, io.EOF):
			return 0, nil
		case err != nil:
			return 0, err
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			continue
		}
		return c, br.UnreadByte()
	}
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
	d, err := Exec(ctx, r, Job{Command: "help commands | get name"}, timeout, -1)
	if err != nil {
		return nil, err
	}
	var out []string
	for row := 1; row < d.Rows; row++ {
		if v, _ := d.At(row, 0); v.Kind == sheet.Text {
			out = append(out, v.Str)
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
	if job.Input != nil {
		args = append([]string{"--stdin"}, args...)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.Stdout = env, stdout
	cmd.WaitDelay = 2 * time.Second
	if job.Input != nil {
		cmd.Stdin = bytes.NewReader(job.Input)
	}
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
