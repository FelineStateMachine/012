package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/ui"
)

// 012 - and 012 --pipe (see docs/terminal/nushell.md): standard input
// may be a table to read, and with --pipe standard output is where the
// result goes when the UI quits. The UI can't use either, so it runs on
// the terminal itself, opened as /dev/tty (CONIN$ and CONOUT$ on
// Windows).

// pipeMode is what the command line asked of standard input and output.
type pipeMode struct {
	stdin bool        // 012 -, or --pipe without a file: read a table from standard input
	pipe  bool        // --pipe: write the result to standard output on quitting
	to    fileio.Kind // --to: the format to write, or 0 for the input's
}

// on reports whether the UI must run on the terminal rather than on
// standard input and output.
func (p pipeMode) on() bool { return p.stdin || p.pipe }

// errNotSent is 012 --pipe quitting without sending, which exits with
// status 1 so the pipeline can tell.
var errNotSent = errors.New("quit without sending anything to the pipeline")

// parsePipe takes -, --pipe and --to from args, returning the rest.
func parsePipe(args []string) (pipeMode, []string, error) {
	var p pipeMode
	var rest []string
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		switch {
		case args[i] == "-":
			p.stdin = true
		case args[i] == "--pipe":
			p.pipe = true
		case name == "--to":
			if !hasValue {
				if i+1 == len(args) {
					return p, nil, errors.New("--to needs a format: nuon, json, csv or tsv")
				}
				i++
				value = args[i]
			}
			k, ok := fileio.KindNamed(value)
			if !ok || !k.IsText() {
				return p, nil, fmt.Errorf("--to %s: 012 writes nuon, json, csv or tsv", value)
			}
			p.to = k
		default:
			rest = append(rest, args[i])
		}
	}
	return p, rest, nil
}

// check reports what doesn't go together: --to without --pipe, a file
// and -, or nothing to read.
func (p *pipeMode) check(files []string, stdinTTY bool) error {
	switch {
	case p.to != 0 && !p.pipe:
		return errors.New("--to goes with --pipe")
	case p.stdin && len(files) > 0:
		return errors.New("012 - reads standard input; name a file or -, not both")
	case p.pipe && len(files) == 0:
		p.stdin = true // --pipe reads standard input unless given a file
	}
	if p.stdin && stdinTTY {
		return errors.New("nothing piped in: 012 - reads a table from standard input, e.g. ls | to nuon | 012 -")
	}
	return nil
}

// tuiOptions opens the terminal for the UI when standard input or output
// is the pipeline's, returning the options that point Bubble Tea at it
// and a function that closes it.
func tuiOptions(p pipeMode, e env) ([]tea.ProgramOption, func(), error) {
	if !p.on() {
		return nil, func() {}, nil
	}
	in, out, err := e.openTTY()
	if err != nil {
		return nil, nil, fmt.Errorf("no terminal to draw on: %w", err)
	}
	return []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(out)}, func() { in.Close(); out.Close() }, nil
}

// setPipe gives the model standard input to read and a pipeline to send
// to, as p asks.
func setPipe(m *ui.Model, p pipeMode, stdin io.Reader) {
	if p.stdin {
		m.ReadStdin(stdin)
	}
	if p.pipe {
		m.SetPipe(p.to)
	}
}

// sendPiped writes what the UI sent on quitting to standard output.
func sendPiped(m *ui.Model, p pipeMode, stdout io.Writer) error {
	if !p.pipe {
		return nil
	}
	snap, k, ok := m.Piped()
	if !ok {
		return errNotSent
	}
	_, err := fileio.Encode(stdout, k, snap)
	return err
}
