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

// 012 - and 012 --pipe (see docs/nushell/pipelines.md): standard input
// may be a table to read, and with --pipe standard output is where the
// result goes when the UI quits. The UI can't use either, so it runs on
// the terminal itself, opened as /dev/tty (CONIN$ and CONOUT$ on
// Windows).

// pipeMode is what the command line asked of standard input and output.
type pipeMode struct {
	stdin  bool        // 012 -, or --pipe without a file: read a table from standard input
	pipe   bool        // --pipe: write the result to standard output on quitting
	to     fileio.Kind // --to: the format to write, or 0 for the input's
	send   ui.Send     // --send: what quitting sends, or ask
	sendAs string      // --send's value, or "" without it
}

// sends are --send's values.
var sends = map[string]ui.Send{"ask": ui.SendAsk, "selection": ui.SendSelection, "sheet": ui.SendSheet}

// on reports whether the UI must run on the terminal rather than on
// standard input and output.
func (p pipeMode) on() bool { return p.stdin || p.pipe }

// errNotSent is 012 --pipe quitting without sending, which exits with
// status 1 so the pipeline can tell.
var errNotSent = errors.New("quit without sending anything to the pipeline")

// parsePipe takes -, --pipe, --to and --send from args, returning the
// rest.
func parsePipe(args []string) (pipeMode, []string, error) {
	var p pipeMode
	var rest []string
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		if name != "--to" && name != "--send" {
			switch args[i] {
			case "-":
				p.stdin = true
			case "--pipe":
				p.pipe = true
			default:
				rest = append(rest, args[i])
			}
			continue
		}
		if !hasValue {
			if i+1 == len(args) {
				return p, nil, fmt.Errorf("%s needs a value: %s", name, pipeValues[name])
			}
			i++
			value = args[i]
		}
		if err := p.set(name, value); err != nil {
			return p, nil, err
		}
	}
	return p, rest, nil
}

// pipeValues are what --to and --send take, for their errors.
var pipeValues = map[string]string{"--to": "nuon, json, csv or tsv", "--send": "ask, selection or sheet"}

// set takes --to's or --send's value.
func (p *pipeMode) set(name, value string) error {
	if name == "--send" {
		s, ok := sends[value]
		if !ok {
			return fmt.Errorf("--send %s: 012 sends %s", value, pipeValues[name])
		}
		p.send, p.sendAs = s, value
		return nil
	}
	k, ok := fileio.KindNamed(value)
	if !ok || !k.IsText() {
		return fmt.Errorf("--to %s: 012 writes %s", value, pipeValues[name])
	}
	p.to = k
	return nil
}

// check reports what doesn't go together: --to or --send without
// --pipe, a file and -, or nothing to read.
func (p *pipeMode) check(files []string, stdinTTY bool) error {
	switch {
	case p.to != 0 && !p.pipe:
		return errors.New("--to goes with --pipe")
	case p.sendAs != "" && !p.pipe:
		return errors.New("--send goes with --pipe: 012 --pipe --send " + p.sendAs)
	case p.stdin && len(files) > 0:
		return errors.New("012 - reads standard input; name a file or -, not both")
	case p.pipe && len(files) == 0:
		p.stdin = true // --pipe reads standard input unless given a file
	}
	if p.stdin && stdinTTY {
		return errors.New("nothing piped in: 012 - reads a table from standard input, e.g. 012 - < sales.csv, or ls | to nuon | ^012 - in nushell")
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
		m.SetSend(p.send)
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
