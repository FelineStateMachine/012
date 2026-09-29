package ui

import (
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
)

// 012 as a stage in a pipeline: 012 - reads a table from standard input
// into a new, unsaved sheet named stdin, and 012 --pipe also writes a
// table to standard output when it quits: the selection or the sheet,
// as the user chooses, in the input's format unless another was asked
// for. The screen is the terminal's own (cmd/012 opens it), so nothing
// else reaches standard output. Quitting asks what to send, or sends
// what --send chose without asking, and quitting without sending lets
// the pipeline know by the exit status.

// StdinName is what a table read from standard input is called: its
// sheet's name.
const StdinName = "stdin"

// pipeState is the pipeline 012 is a stage of, if any.
type pipeState struct {
	in io.Reader // standard input, read as the program starts; nil once read

	on   bool        // --pipe: quitting writes a table to standard output
	to   fileio.Kind // the format asked for (--to), or 0 for the input's
	send Send        // what quitting sends (--send)

	// Set by a quit that sends: what to write, and in which format.
	sent *fileio.Snapshot
	kind fileio.Kind
}

// ReadStdin sets a table to read from r, standard input, as the program
// starts, into a new unsaved sheet named stdin.
func (m *Model) ReadStdin(r io.Reader) { m.pipe.in = r }

// SetPipe makes quitting send a table to standard output, in format to
// (a text format; 0 for the input's, or NUON when the input isn't text).
func (m *Model) SetPipe(to fileio.Kind) { m.pipe.on, m.pipe.to = true, to }

// Send is what quitting sends in a pipeline (012 --pipe --send).
type Send int

const (
	SendAsk       Send = iota // ask: the selection, the sheet, or nothing
	SendSelection             // the selection, or the sheet when only one cell is selected
	SendSheet                 // the sheet, whatever is selected
)

// SetSend makes quitting in a pipeline send s rather than ask.
func (m *Model) SetSend(s Send) { m.pipe.send = s }

// Piped is what quitting sent: the table and its format, or false when
// the user quit without sending.
func (m *Model) Piped() (*fileio.Snapshot, fileio.Kind, bool) {
	return m.pipe.sent, m.pipe.kind, m.pipe.sent != nil
}

func init() {
	register(
		&command{id: "pipe.send_selection", macro: macroNever, title: "Quit and send selection",
			desc:    "Quit, writing the selection to standard output for the next command in the pipeline",
			enabled: func(m *Model) bool { return m.pipe.on && m.hasRange() },
			hidden:  func(m *Model) bool { return !m.pipe.on },
			run:     func(m *Model) tea.Cmd { return m.send(m.selection()) }},
		&command{id: "pipe.send_sheet", macro: macroNever, title: "Quit and send sheet",
			desc:    "Quit, writing the sheet's data to standard output for the next command in the pipeline",
			enabled: func(m *Model) bool { return m.pipe.on },
			hidden:  func(m *Model) bool { return !m.pipe.on },
			run:     func(m *Model) tea.Cmd { return m.send(sheet.Rect{}) }},
		&command{id: "pipe.quit_unsent", macro: macroNever, title: "Quit without sending",
			desc:   "Quit, writing nothing to standard output: the pipeline sees exit status 1",
			hidden: func(m *Model) bool { return !m.pipe.on },
			run:    (*Model).exit},
	)
}

// startStdin starts reading standard input, if it's to be read.
func (m *Model) startStdin() tea.Cmd {
	r := m.pipe.in
	if r == nil {
		return nil
	}
	m.pipe.in = nil
	return m.xfer.StartReader(StdinName, r, fileio.Options{Locale: m.locale()}, m.spans.Parent())
}

// streamed opens a table read from standard input: a new spreadsheet
// with no file, unsaved.
func (m *Model) streamed(msg transfer.ImportedMsg) {
	m.reset(msg.Res.Sheet, "")
	m.xfer.Kind = msg.Res.Kind
	m.changed, m.saved = true, -1
	m.note = "Read " + transfer.Rows(msg.Res.Rows) + " of " + msg.Res.Kind.String() + " from standard input"
	if len(msg.Res.Notes) > 0 {
		m.note += "; " + strings.Join(msg.Res.Notes, "; ")
	}
}

// pipeKind is the format sent: the one asked for, or the input's when
// it's text, or NUON.
func (m *Model) pipeKind() fileio.Kind {
	switch {
	case m.pipe.to != 0:
		return m.pipe.to
	case m.xfer.Kind.IsText():
		return m.xfer.Kind
	}
	return fileio.NUON
}

// sendRange is what quitting would send: the selection, when a range
// is selected and --send doesn't ask for the sheet, or else the sheet
// (the zero Rect).
func (m *Model) sendRange() sheet.Rect {
	if m.pipe.send != SendSheet && m.hasRange() {
		return m.selection()
	}
	return sheet.Rect{}
}

// sendWhat names what quitting would send: "B2:D9" or "the sheet".
func (m *Model) sendWhat() string {
	if r := m.sendRange(); r != (sheet.Rect{}) {
		return r.String()
	}
	return "the sheet"
}

// quitPiped is Quit in a pipeline: send what --send chose, or ask.
func (m *Model) quitPiped() tea.Cmd {
	if m.pipe.send == SendAsk {
		return m.askSend()
	}
	return m.send(m.sendRange())
}

// askSend asks what to send: the selection, if there is one, or the
// sheet, or nothing.
func (m *Model) askSend() tea.Cmd {
	var choices []choice
	if m.hasRange() {
		choices = append(choices,
			choice{key: "enter", label: "Send selection", run: func(m *Model) tea.Cmd { return m.send(m.selection()) }},
			choice{key: "s", label: "Send sheet", run: func(m *Model) tea.Cmd { return m.send(sheet.Rect{}) }})
	} else {
		choices = append(choices, choice{key: "enter", label: "Send sheet", run: func(m *Model) tea.Cmd { return m.send(sheet.Rect{}) }})
	}
	choices = append(choices,
		choice{key: "d", label: "Don't send", run: (*Model).exit},
		choice{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }})
	m.ask(question{
		msg:     "Send " + m.sendWhat() + " as " + m.pipeKind().String() + "?",
		desc:    "To standard output, for the next command; quitting without sending exits with status 1",
		choices: choices,
	})
	return nil
}

// send quits, keeping r (the sheet's data when zero) to write to
// standard output once the terminal is given back.
func (m *Model) send(r sheet.Rect) tea.Cmd {
	m.pipe.sent = fileio.Snap(m.sheet, r, m.sheet.Name())
	m.pipe.kind = m.pipeKind()
	return m.exit()
}

// pipeStatus is what the status line says in a pipeline: the key that
// sends, what and as what.
func (m *Model) pipeStatus() string {
	if !m.pipe.on {
		return ""
	}
	return "  " + m.th.KeyHints(m.shortcut("quit"), "send "+m.sendWhat()+" as "+m.pipeKind().String())
}
