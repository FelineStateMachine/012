// Package transfer runs imports in the background and draws their
// progress: the mode indicator says WAIT, the context line offers Esc,
// the status line counts rows with a progress bar, and the terminal
// shows its own progress indicator. It remembers where the sheet came
// from, so Save can offer to export it back. What an import's result
// does to the workbook is the UI's; this package needs no host.
package transfer

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Place is where an import goes.
type Place int

const (
	Book      Place = iota // replace the spreadsheet
	NewSheets              // insert new sheets
	Sheet                  // replace the sheet shown
)

// Transfer is the model's import state.
type Transfer struct {
	job     *job
	lastID  int
	Source  string      // the file the sheet was imported from, if any
	Kind    fileio.Kind // its format
	Startup string      // a file to import as the program starts
}

// job is an import in progress.
type job struct {
	id     int
	name   string
	prog   *fileio.Progress
	cancel func()
	place  Place
}

// ImportedMsg is an import's result.
type ImportedMsg struct {
	ID    int
	Name  string
	Opt   fileio.Options
	Place Place
	Res   *fileio.Result
	Err   error
	// Stream is set for a table read from a stream (StartReader), which
	// has no file to save back to.
	Stream bool
}

// TickMsg refreshes the progress display.
type TickMsg struct{ id int }

// tick is how often the progress display refreshes.
const tick = 100 * time.Millisecond

// Busy reports whether an import is running.
func (x *Transfer) Busy() bool { return x.job != nil }

// Start reads name, at path on disk, in the background, replacing any
// import running. Its span nests under parent.
func (x *Transfer) Start(name, path string, opt fileio.Options, place Place, parent telemetry.Parent) tea.Cmd {
	ctx, cancel := context.WithCancel(telemetry.WithParent(context.Background(), parent))
	prog := fileio.NewProgress()
	id := x.Begin(name, prog, cancel, place)
	opt.Progress = prog
	return tea.Batch(
		func() tea.Msg {
			res, err := fileio.Import(ctx, path, opt)
			return ImportedMsg{ID: id, Name: name, Opt: opt, Place: place, Res: res, Err: err}
		},
		tickCmd(id),
	)
}

// StartReader reads a table from r in the background, as Start reads a
// file, into a sheet called name: standard input, for 012 -. Its result
// is marked Stream.
func (x *Transfer) StartReader(name string, r io.Reader, opt fileio.Options, parent telemetry.Parent) tea.Cmd {
	ctx, cancel := context.WithCancel(telemetry.WithParent(context.Background(), parent))
	prog := fileio.NewProgress()
	id := x.Begin(name, prog, cancel, Book)
	opt.Progress = prog
	return tea.Batch(
		func() tea.Msg {
			res, err := fileio.ImportReader(ctx, name, r, opt)
			return ImportedMsg{ID: id, Name: name, Opt: opt, Place: Book, Res: res, Err: err, Stream: true}
		},
		tickCmd(id),
	)
}

// Begin tracks an import of name reporting to prog, cancelled by cancel,
// replacing any import running, and returns its ID for its ImportedMsg.
func (x *Transfer) Begin(name string, prog *fileio.Progress, cancel func(), place Place) int {
	if x.job != nil {
		x.job.cancel()
	}
	x.lastID++
	x.job = &job{id: x.lastID, name: name, prog: prog, cancel: cancel, place: place}
	return x.lastID
}

func tickCmd(id int) tea.Cmd {
	return tea.Tick(tick, func(time.Time) tea.Msg { return TickMsg{id} })
}

// Cancel stops the import at once and returns the name of its file; its
// result, if it still arrives, is ignored.
func (x *Transfer) Cancel() string {
	j := x.job
	j.cancel()
	x.job = nil
	return j.name
}

// Update handles the transfer's own messages: a tick of the running
// import asks for the next one, and the running import's result ends it
// and is returned for the model to use. Stale ticks and results, from an
// import replaced or cancelled, are dropped.
func (x *Transfer) Update(msg tea.Msg) (res ImportedMsg, done bool, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		if x.job != nil && x.job.id == msg.id {
			return res, false, tickCmd(msg.id)
		}
	case ImportedMsg:
		if x.job == nil || x.job.id != msg.ID {
			return res, false, nil
		}
		x.job.cancel()
		x.job = nil
		return msg, true, nil
	}
	return res, false, nil
}

// Line is the context line during an import.
func (x *Transfer) Line(th *theme.Theme) string {
	return "Importing " + filepath.Base(x.job.name) + "…   " + th.KeyHints("Esc", "cancel")
}

// Status is the status line during an import, width wide: the file, the
// rows read, and a bar when the total is known.
func (x *Transfer) Status(th *theme.Theme, width int) (left, right string) {
	rows, frac := x.job.prog.Get()
	left = th.Key.Render("Importing " + filepath.Base(x.job.name))
	right = Rows(rows) + " read"
	if frac < 0 {
		return left, right
	}
	pct := fmt.Sprintf(" %3d%%", int(frac*100))
	w := max(0, min(width-ansi.StringWidth(left)-ansi.StringWidth(right)-len(pct)-6, 30))
	if w < 8 {
		return left, right + pct
	}
	done := int(frac * float64(w))
	return left, right + "  " + th.Progress.Render(strings.Repeat("━", done)) +
		th.ProgressTodo.Render(strings.Repeat("─", w-done)) + pct
}

// ProgressBar is the terminal's own progress indicator (OSC 9;4), shown
// in the tab or title bar where supported.
func (x *Transfer) ProgressBar() *tea.ProgressBar {
	if x.job == nil {
		return nil
	}
	if _, frac := x.job.prog.Get(); frac >= 0 {
		return tea.NewProgressBar(tea.ProgressBarDefault, int(frac*100))
	}
	return tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
}

// Rows is n rows in words: "1 row", "4,096 rows".
func Rows(n int) string {
	if n == 1 {
		return "1 row"
	}
	return Thousands(n) + " rows"
}

// Thousands writes n with commas between thousands.
func Thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
