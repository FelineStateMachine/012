package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

const filesNUON = "[[name, size]; [a, 1646b], [b, 2kb]]"

// stdinModel is a model that has read text from standard input, in a
// pipeline (to, or the input's format when 0) when pipe is set.
func stdinModel(t *testing.T, text string, pipe bool, to fileio.Kind) *Model {
	t.Helper()
	m := newModel()
	if pipe {
		m.SetPipe(to)
	}
	m.ReadStdin(strings.NewReader(text))
	run(m, m.startStdin())
	if m.xfer.Busy() {
		t.Fatal("still reading")
	}
	return m
}

func TestStdinOpensAnUnsavedSheet(t *testing.T) {
	m := stdinModel(t, filesNUON, false, 0)
	if m.sheet.Name() != StdinName || !m.changed || m.filename != "" || m.xfer.Source != "" {
		t.Errorf("sheet %q, changed %v, file %q, source %q", m.sheet.Name(), m.changed, m.filename, m.xfer.Source)
	}
	if got := line(m, contextLine); !strings.Contains(got, "Read 3 rows of NUON from standard input") {
		t.Errorf("context line %q", got)
	}
	if got := sheet.FormatText(m.sheet.Value(addr("B2")), m.sheet.DisplayFormat(addr("B2"))); got != "1.6 kB" {
		t.Errorf("B2 shows %q", got)
	}
	press(t, m, "<ctrl+q>")
	if got := line(m, contextLine); !strings.Contains(got, "You have unsaved changes.") {
		t.Errorf("quitting outside a pipeline: %q", got)
	}
	if st := line(m, m.height-1); strings.Contains(st, "send") {
		t.Errorf("status line outside a pipeline: %q", st)
	}
}

func TestPipeSendsTheSheet(t *testing.T) {
	m := stdinModel(t, filesNUON, true, 0)
	if st := line(m, m.height-1); !strings.Contains(st, "Ctrl+Q  send the sheet as NUON") {
		t.Errorf("status line %q", st)
	}
	press(t, m, "<ctrl+q>")
	if got := line(m, contextLine); !strings.Contains(got, "Send the sheet as NUON?") ||
		!strings.Contains(got, "Enter  Send sheet") || !strings.Contains(got, "D  Don't send") || strings.Contains(got, "Send selection") {
		t.Errorf("question %q", got)
	}
	if _, quit := press(t, m, "<enter>").(tea.QuitMsg); !quit {
		t.Fatal("didn't quit")
	}
	snap, k, ok := m.Piped()
	if !ok || k != fileio.NUON || snap.Range.String() != "A1:B3" {
		t.Errorf("sent %v %v %v", snap, k, ok)
	}
}

func TestPipeSendsTheSelection(t *testing.T) {
	m := stdinModel(t, filesNUON, true, fileio.CSV)
	press(t, m, "<shift+down>")
	if st := line(m, m.height-1); !strings.Contains(st, "send A1:A2 as CSV") {
		t.Errorf("status line %q", st)
	}
	press(t, m, "<ctrl+q>")
	if got := line(m, contextLine); !strings.Contains(got, "Send A1:A2 as CSV?") || !strings.Contains(got, "S  Send sheet") {
		t.Errorf("question %q", got)
	}
	press(t, m, "<enter>")
	if snap, k, ok := m.Piped(); !ok || k != fileio.CSV || snap.Range.String() != "A1:A2" {
		t.Errorf("sent %v %v %v", snap.Range, k, ok)
	}

	m = stdinModel(t, "a\tb\n1\t2\n", true, 0)
	press(t, m, "<shift+down>", "<ctrl+q>", "s")
	if snap, k, ok := m.Piped(); !ok || k != fileio.TSV || snap.Range.String() != "A1:B2" {
		t.Errorf("sent the sheet? %v %v %v", snap.Range, k, ok)
	}
}

func TestPipeQuitWithoutSending(t *testing.T) {
	m := stdinModel(t, filesNUON, true, 0)
	press(t, m, "<ctrl+q>", "<esc>")
	if m.mode != modeReady {
		t.Errorf("Esc left mode %v", m.mode)
	}
	if _, quit := press(t, m, "<ctrl+q>", "d").(tea.QuitMsg); !quit {
		t.Fatal("didn't quit")
	}
	if _, _, ok := m.Piped(); ok {
		t.Error("sent anyway")
	}
}

// TestPipeSendSelection: with --send selection, Quit sends the
// selection without asking, or the sheet when only one cell is
// selected, and the status line says which.
func TestPipeSendSelection(t *testing.T) {
	m := stdinModel(t, filesNUON, true, 0)
	m.SetSend(SendSelection)
	if st := line(m, m.height-1); !strings.Contains(st, "Ctrl+Q  send the sheet as NUON") {
		t.Errorf("status line %q", st)
	}
	press(t, m, "<shift+down>", "<shift+right>")
	if st := line(m, m.height-1); !strings.Contains(st, "Ctrl+Q  send A1:B2 as NUON") {
		t.Errorf("status line %q", st)
	}
	if _, quit := press(t, m, "<ctrl+q>").(tea.QuitMsg); !quit {
		t.Fatal("asked rather than quit")
	}
	if snap, _, ok := m.Piped(); !ok || snap.Range.String() != "A1:B2" {
		t.Errorf("sent %v %v", snap.Range, ok)
	}

	m = stdinModel(t, filesNUON, true, 0)
	m.SetSend(SendSelection)
	press(t, m, "<down>")
	cmd, _ := m.runCmdLine("q")
	if _, quit := run(m, cmd).(tea.QuitMsg); !quit {
		t.Fatal(":q asked rather than quit")
	}
	if snap, _, ok := m.Piped(); !ok || snap.Range.String() != "A1:B3" {
		t.Errorf("one cell selected sent %v %v", snap.Range, ok)
	}
}

// TestPipeSendSheet: with --send sheet, Quit sends the sheet whatever
// is selected, and Quit without sending is still in the palette.
func TestPipeSendSheet(t *testing.T) {
	m := stdinModel(t, filesNUON, true, fileio.CSV)
	m.SetSend(SendSheet)
	press(t, m, "<shift+down>")
	if st := line(m, m.height-1); !strings.Contains(st, "Ctrl+Q  send the sheet as CSV") {
		t.Errorf("status line %q", st)
	}
	if _, quit := run(m, m.runCommand("quit")).(tea.QuitMsg); !quit {
		t.Fatal("File > Quit asked rather than quit")
	}
	if snap, k, ok := m.Piped(); !ok || k != fileio.CSV || snap.Range.String() != "A1:B3" {
		t.Errorf("sent %v %v %v", snap.Range, k, ok)
	}

	m = stdinModel(t, filesNUON, true, 0)
	m.SetSend(SendSheet)
	if _, quit := run(m, m.runCommand("pipe.quit_unsent")).(tea.QuitMsg); !quit {
		t.Fatal("Quit without sending didn't quit")
	}
	if _, _, ok := m.Piped(); ok {
		t.Error("Quit without sending sent")
	}
}

// TestPipeCommands: the send commands and Quit without sending are in
// File and the palette only in a pipeline, and run from there.
func TestPipeCommands(t *testing.T) {
	titles := func(m *Model) []string {
		var out []string
		for _, it := range paletteItems(m) {
			out = append(out, it.Title)
		}
		return out
	}
	m := newModel()
	if slices.Contains(titles(m), "Quit and send sheet") || len(m.applicable(menuBar[0].items)) != len(menuBar[0].items)-8 { // the send commands, sharing outside 012 serve, and agents before one is invited
		t.Error("send commands outside a pipeline")
	}
	m = stdinModel(t, filesNUON, true, 0)
	if !slices.Contains(titles(m), "Quit and send sheet") || !slices.Contains(titles(m), "Quit and send selection") ||
		!slices.Contains(titles(m), "Quit without sending") {
		t.Error("send commands missing in a pipeline")
	}
	if commands["pipe.send_selection"].available(m) {
		t.Error("send selection without a selection")
	}
	if _, quit := run(m, m.runCommand("pipe.send_sheet")).(tea.QuitMsg); !quit {
		t.Fatal("didn't quit")
	}
	if _, _, ok := m.Piped(); !ok {
		t.Error("nothing sent")
	}
}

// TestPipeSendsAFileArgument: 012 --pipe data.nuon sends in the file's
// format, and a .012 file as NUON.
func TestPipeFormatOfAFile(t *testing.T) {
	m := newModel()
	m.SetPipe(0)
	if m.pipeKind() != fileio.NUON {
		t.Errorf("a .012 sheet sends %v", m.pipeKind())
	}
	m.xfer.Kind = fileio.JSON
	if m.pipeKind() != fileio.JSON {
		t.Errorf("a JSON file sends %v", m.pipeKind())
	}
	m.xfer.Kind = fileio.XLSX
	if m.pipeKind() != fileio.NUON {
		t.Errorf("an Excel file sends %v", m.pipeKind())
	}
}
