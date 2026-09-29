package ui

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/nushell"
)

// streamPolls are the stream polls commands returned in tests, which
// run doesn't follow on its own (see pumpStreams).
var streamPolls []nbStreamMsg

// pumpStreams hands the model the stream polls waiting, and those they
// start, until a poll finds nothing new after one found rows.
func pumpStreams(m *Model) {
	rows := false
	for i := 0; i < 50 && len(streamPolls) > 0; i++ {
		msg := streamPolls[0]
		streamPolls = streamPolls[1:]
		send(m, msg)
		if msg.ok {
			rows = true
		} else if rows {
			return
		}
	}
}

// streamNu prints each line sent on lines, as a stream's pipeline
// would, until lines is closed or the run is stopped.
type streamNu struct {
	lines   chan string
	scripts chan string
}

func newStreamNu() *streamNu {
	return &streamNu{lines: make(chan string), scripts: make(chan string, 8)}
}

func (f *streamNu) Run(ctx context.Context, job nushell.Job, script string, stdout io.Writer) error {
	if len(job.IDE) > 0 || job.Command == "help commands | get name" {
		return nushell.ErrMissing
	}
	f.scripts <- script
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case l, ok := <-f.lines:
			if !ok {
				return nil
			}
			io.WriteString(stdout, l+"\n")
		}
	}
}

// A cell run as a stream shows as live, its rows reaching its output
// and the sheet it was sent to as the pipeline prints them, and Stop
// ends it keeping them.
func TestNotebookStream(t *testing.T) {
	streamPolls = nil
	m, _ := notebookModel(t, nil)
	nu := newStreamNu()
	m.SetShellRunner(nu)
	write(t, m, "log = tail -f app.log | lines | parse '{level} {msg}'")
	press(t, m, "f")
	if len(m.nb.streams) != 1 || !strings.Contains(screen(m), "● live, 0 rows") {
		t.Fatalf("not live:\n%s", screen(m))
	}
	if script := <-nu.scripts; !strings.Contains(script, "to nuon --raw | print") {
		t.Errorf("not run as a stream: %s", script)
	}
	// Send it to a new sheet while it runs.
	press(t, m, "G", "<enter>")
	logs := m.book().Lookup("log")
	if logs == nil {
		t.Fatalf("no sheet log: %q", m.note)
	}
	nb := m.sheet
	m.showSheet(m.book().Sheets()[0])

	nu.lines <- "{level: info, msg: started}"
	pumpStreams(m)
	nu.lines <- "{level: warn, msg: slow}"
	pumpStreams(m)
	if got := logs.Value(addr("B3")).String(); got != "slow" {
		t.Errorf("B3 = %q", got)
	}
	if got := logs.Value(addr("A1")).String(); got != "level" {
		t.Errorf("A1 = %q", got)
	}
	st := m.cellState(nb, firstStream(m))
	if !st.Live || !st.Running || st.Rows != 2 {
		t.Errorf("state %+v", st)
	}
	run(m, m.runCommand("nb.stop"))
	if len(m.nb.streams) != 0 {
		t.Fatal("still streaming after Stop")
	}
	m.showSheet(nb)
	c, _ := m.nbCell()
	o := m.book().Output(c.ID)
	if o == nil || o.Failed() || !strings.Contains(string(o.NUON), "slow") {
		t.Errorf("output after Stop: %+v", o)
	}
	if got := logs.Value(addr("B3")).String(); got != "slow" {
		t.Errorf("after Stop B3 = %q", got)
	}
	if strings.Contains(screen(m), "live") {
		t.Errorf("still shown live:\n%s", screen(m))
	}
}

func firstStream(m *Model) int {
	for id := range m.nb.streams {
		return id
	}
	return -1
}

// A cell running as a stream reads without color: ● live and its rows
// in words, and ■ for ▶.
func TestMonochromeStream(t *testing.T) {
	streamPolls = nil
	m, _ := notebookModel(t, nil)
	nu := newStreamNu()
	m.SetShellRunner(nu)
	write(t, m, "tail -f app.log | lines")
	press(t, m, "f")
	defer m.stopCells()
	nu.lines <- `"started"`
	pumpStreams(m)
	findLine(t, m, "● live, 1 row")
	findLine(t, m, "■")
}
