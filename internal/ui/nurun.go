package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
)

// Running regions' commands. One runs at a time, in the background, so
// the screen stays live and Esc stops it: a region and those waiting
// after it (what reads it) are a queue, each run when the one before
// has shown its table. The tables a command reads are taken when it
// starts; its own table is shown as one undo step when it ends.

// shellState is the notebook's part of the model.
type shellState struct {
	runner  nushell.Runner // nu, or a fake in tests
	queue   []string       // regions waiting to run, in order
	running *nuJob
	gen     int
	// input is the range the prompt was opened on, which commands read
	// as $in; said is what the last run said, for the prompt's context
	// line; failed is why each region's last run failed, by name key.
	input  string
	said   string
	failed map[string]string
	// words are nu's command names, asked for once (asked).
	words []string
	asked bool
	// served is set in a session of 012 serve, where commands run only
	// when the server's config allows them.
	served bool
	// startup opens the prompt once the program starts (012 nu).
	startup bool
}

// nuJob is a command running.
type nuJob struct {
	name   string
	gen    int
	cancel context.CancelFunc
	start  time.Time
}

type (
	// nuDoneMsg is a command finished: its table, or why it has none.
	nuDoneMsg struct {
		job  *nuJob
		data *sheet.RegionData
		err  error
	}
	// nuWordsMsg brings nu's command names.
	nuWordsMsg struct{ words []string }
)

// SetShellRunner makes commands run with r rather than nu, for tests.
func (m *Model) SetShellRunner(r nushell.Runner) { m.shell.runner = r }

func (m *Model) runner() nushell.Runner {
	if m.shell.runner == nil {
		return nushell.Nu{}
	}
	return m.shell.runner
}

// runRegions queues names to run, after what's queued, and starts the
// first unless one is running.
func (m *Model) runRegions(names []string) tea.Cmd {
	for _, name := range names {
		if !containsFold(m.shell.queue, name) && (m.shell.running == nil || !strings.EqualFold(m.shell.running.name, name)) {
			m.shell.queue = append(m.shell.queue, name)
			m.setStatus(name, "waiting")
		}
	}
	if m.shell.running != nil {
		return nil
	}
	return m.nextRegion()
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// setStatus sets what a region's label says.
func (m *Model) setStatus(name, status string) {
	if s, _, ok := m.book().Region(name); ok {
		s.SetRegionStatus(name, status)
	}
}

// nextRegion starts the first region queued.
func (m *Model) nextRegion() tea.Cmd {
	for len(m.shell.queue) > 0 {
		name := m.shell.queue[0]
		m.shell.queue = m.shell.queue[1:]
		s, r, ok := m.book().Region(name)
		if !ok {
			continue
		}
		job, err := m.jobFor(r)
		if err != nil {
			m.failRegion(r.Name, err.Error())
			continue
		}
		s.SetRegionStatus(r.Name, "Running…")
		return m.startJob(r.Name, job)
	}
	return nil
}

// jobFor is what running r takes: the tables it reads, as NUON.
func (m *Model) jobFor(r sheet.Region) (nushell.Job, error) {
	job := nushell.Job{Command: r.Command, Tables: map[string][]byte{}, Config: m.configBool("nu-config", false)}
	w := m.book()
	for _, d := range r.Deps {
		s, dr, ok := w.Region(d)
		if !ok {
			return job, fmt.Errorf("it reads $%s, which isn't a region any more", d)
		}
		t, ok := s.RegionTable(dr.Name)
		if !ok {
			t = sheet.Rect{From: dr.At, To: dr.At} // nothing to read: an empty table
		}
		snap := fileio.Snap(s, t, dr.Name)
		snap.HiddenRows = nil // a region reads another whole, whatever its filter shows
		if !ok {
			snap.Cells = nil
		}
		data, err := encodeNUON(snap)
		if err != nil {
			return job, err
		}
		job.Tables[d] = data
	}
	if r.Input != "" {
		name, ref := sheet.SplitSheet(r.Input)
		s, rng, ok := w.Lookup(name), sheet.Rect{}, false
		if s != nil {
			rng, ok = sheet.ParseRange(ref)
		}
		if !ok {
			return job, fmt.Errorf("its input %s is gone", r.Input)
		}
		data, err := encodeNUON(fileio.Snap(s, rng, s.Name()))
		if err != nil {
			return job, err
		}
		job.Input = data
	}
	return job, nil
}

func encodeNUON(snap *fileio.Snapshot) ([]byte, error) {
	var b bytes.Buffer
	if _, err := fileio.Encode(&b, fileio.NUON, snap); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// startJob runs a job in the background.
func (m *Model) startJob(name string, job nushell.Job) tea.Cmd {
	m.shell.gen++
	ctx, cancel := context.WithCancel(context.Background())
	j := &nuJob{name: name, gen: m.shell.gen, cancel: cancel, start: time.Now()}
	m.shell.running = j
	runner, timeout, parent := m.runner(), m.configDuration("nu-timeout", 30*time.Second), m.spans.Parent()
	return func() tea.Msg {
		defer cancel()
		span := parent.Start("nu", slog.Int("tables", len(job.Tables)))
		data, err := nushell.Exec(ctx, runner, job, timeout, 0)
		if err != nil {
			span.Fail(err)
		} else {
			span.End(slog.Int("rows", data.Rows))
		}
		return nuDoneMsg{job: j, data: data, err: err}
	}
}

// finishRegion shows what a command made, or why it failed, and starts
// the next region queued.
func (m *Model) finishRegion(msg nuDoneMsg) tea.Cmd {
	if m.shell.running != msg.job {
		return nil // stopped, and another started since
	}
	m.shell.running = nil
	name := msg.job.name
	s, r, ok := m.book().Region(name)
	switch {
	case !ok:
	case msg.err != nil:
		m.failRegion(r.Name, runError(msg.err))
		m.clearQueue()
		return nil
	default:
		delete(m.shell.failed, strings.ToUpper(r.Name))
		s.SetRegionStatus(r.Name, "")
		if err := s.ShowRegion(r.Name, msg.data); err != nil {
			m.failRegion(r.Name, err.Error())
			break
		}
		m.shell.said = m.th.Hint.Render(r.Name + ": " + transfer.Rows(msg.data.Rows) + noteSuffix(msg.data.Note))
	}
	if len(m.shell.queue) == 0 && time.Since(msg.job.start) > 5*time.Second {
		return tea.Batch(m.nextRegion(), m.term.notify("Shell commands finished in "+m.displayName()))
	}
	return m.nextRegion()
}

func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	return "; " + note
}

// runError is what a failed run says.
func runError(err error) string {
	var nuErr *nushell.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "stopped"
	case errors.As(err, &nuErr):
		return nuErr.Msg
	}
	return err.Error()
}

// failRegion marks a region failed, saying why on its label and the
// context line.
func (m *Model) failRegion(name, why string) {
	if m.shell.failed == nil {
		m.shell.failed = map[string]string{}
	}
	m.shell.failed[strings.ToUpper(name)] = why
	m.setStatus(name, "failed")
	if why == "stopped" {
		m.setStatus(name, "stopped")
	}
	m.shell.said = m.th.Warning.Render(name + " failed: " + why)
	if why == "stopped" {
		m.shell.said = m.th.Hint.Render("Stopped " + name)
	}
	m.warn = m.shell.said
}

// clearQueue forgets the regions waiting to run.
func (m *Model) clearQueue() {
	for _, name := range m.shell.queue {
		m.setStatus(name, "")
	}
	m.shell.queue = nil
}

// stopShell stops the command running and those waiting.
func (m *Model) stopShell() {
	if j := m.shell.running; j != nil {
		j.cancel()
	}
	m.clearQueue()
}

// loadWords asks nu for its command names, once a session, for the
// prompt's completions.
func (m *Model) loadWords() tea.Cmd {
	if m.shell.asked || m.shellOff() != "" {
		return nil
	}
	m.shell.asked = true
	runner := m.runner()
	return func() tea.Msg {
		words, _ := nushell.Commands(context.Background(), runner, 30*time.Second)
		return nuWordsMsg{words}
	}
}

// Config helpers that work without a config, as in tests.

func (m *Model) configString(name, def string) string {
	if c := m.prefs.Config; c != nil {
		return c.String(name)
	}
	return def
}

func (m *Model) configBool(name string, def bool) bool {
	if c := m.prefs.Config; c != nil {
		return c.Bool(name)
	}
	return def
}

func (m *Model) configDuration(name string, def time.Duration) time.Duration {
	if c := m.prefs.Config; c != nil {
		return c.Duration(name)
	}
	return def
}

// startShell opens the prompt as the program starts, when 012 nu asked.
func (m *Model) startShell() tea.Cmd {
	if !m.shell.startup {
		return nil
	}
	m.shell.startup = false
	return m.openShell("")
}
