package ui

import (
	"context"
	"io"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/live"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
)

// Cells run as streams (Run as stream): the pipeline runs until it ends
// or is stopped, and each value it yields reaches the cell's output, and
// the sheet the output was sent to, as soon as nu prints it, so
// `tail -f app.log | lines | parse ...` or `watch` is followed live. The
// run is a live.Stream, the linked regions' source: it is polled one
// poll at a time, each poll waiting for what the pipeline prints, and
// what it gives is applied as a live operation to the output's region
// (sheet.LiveOp, outside the undo history) and kept as the cell's
// output, its last live.StreamKeep values. Streams run beside the queue
// of cells run once, each on its own; Stop ends them all.

// nbStream is a cell running as a stream.
type nbStream struct {
	*nbRun
	src   *live.Stream
	count int // the run's number, [n]
	rows  int // rows printed so far
	// shown is when the output was last updated, so a fast stream
	// updates it a few times a second rather than on every poll, and
	// behind is set when rows came since.
	shown  time.Time
	behind bool
}

// nbStreamMsg is what a stream's poll found.
type nbStreamMsg struct {
	st *nbStream
	u  live.Update
	ok bool
}

// streamOutputEvery is how often a stream's output is rebuilt, at most:
// rows reach a sheet the output was sent to on every poll.
const streamOutputEvery = 100 * time.Millisecond

// streamSelected runs the selected code cell as a stream.
func (m *Model) streamSelected() tea.Cmd {
	v := m.nbView()
	v.StopEdit()
	c, ok := v.Cell()
	if !ok || c.Kind != notebook.Code {
		return nil
	}
	s := m.sheet
	return m.trustNotebook(func(m *Model) tea.Cmd { return m.startStream(nbQueued{s, c.ID}) })
}

// startStream starts cell q as a stream, in place of any stream or
// queued run of it.
func (m *Model) startStream(q nbQueued) tea.Cmd {
	c, ok := cellOf(q)
	if !ok {
		return nil
	}
	m.stopStream(q.id)
	m.nb.runs.queue = slices.DeleteFunc(m.nb.runs.queue, func(x nbQueued) bool { return x == q })
	cells := q.s.NotebookCells()
	i := slices.IndexFunc(cells, func(x notebook.Cell) bool { return x.ID == c.ID })
	if why := notebook.Taken(cells, i); why != "" {
		m.failCell(q, c, why)
		return nil
	}
	run := &nbRun{nbQueued: q, source: c.Source, start: time.Now()}
	job, err := m.jobFor(q.s, c, run, true)
	if err != nil {
		m.failCell(q, c, err.Error())
		return nil
	}
	runner, parent := m.runner(), m.spans.Parent()
	src := live.NewStream(func(ctx context.Context, w io.Writer) error {
		span := parent.Start("nu.stream")
		err := nushell.Stream(ctx, runner, job, w)
		span.End()
		return err
	})
	src.Wait = streamOutputEvery // an idle poll brings the output up to date
	m.nb.runs.count++
	st := &nbStream{nbRun: run, src: src, count: m.nb.runs.count}
	if m.nb.runs.streams == nil {
		m.nb.runs.streams = map[int]*nbStream{}
	}
	m.nb.runs.streams[q.id] = st
	m.showStream(st)
	st.shown = time.Time{} // the first rows show at once
	m.feedOutput(c.Name())
	m.note = "Following cell " + strconv.Itoa(i+1) + " as a stream: Stop (i i) ends it"
	return tea.Batch(m.roomOwned(pollStream(st)), m.tickStreams())
}

// pollStream polls st's stream on a goroutine of its own.
func pollStream(st *nbStream) tea.Cmd {
	return func() tea.Msg {
		u, ok := st.src.Poll(context.Background())
		return nbStreamMsg{st, u, ok}
	}
}

// streamPolled applies what a stream's poll found: its rows to the region
// the output was sent to, and to the output, until the stream ends.
func (m *Model) streamPolled(msg nbStreamMsg) tea.Cmd {
	st := msg.st
	if m.nb.runs.streams[st.id] != st {
		st.src.Close() // stopped, or started again since
		return nil
	}
	c, ok := cellOf(st.nbQueued)
	if !ok {
		m.stopStream(st.id)
		return nil
	}
	st.rows = st.src.Rows()
	st.behind = st.behind || msg.ok
	if msg.ok {
		if _, r, ok := m.book().Region(c.Name()); ok && r.Output {
			if err := m.book().ApplyLive(msg.u.Op(r.Name)); err != nil {
				m.warn = err.Error()
			}
		}
	}
	if done, err := st.src.Ended(); done {
		delete(m.nb.runs.streams, st.id)
		m.endStream(st, err)
		return nil
	}
	if st.behind && time.Since(st.shown) >= streamOutputEvery {
		m.showStream(st)
	}
	return m.roomOwned(pollStream(st))
}

// showStream keeps what the stream has printed so far as the cell's
// output.
func (m *Model) showStream(st *nbStream) {
	st.shown, st.behind = time.Now(), false
	m.book().SetOutput(st.id, &notebook.Output{NUON: st.src.NUON(), Count: st.count, Took: time.Since(st.start),
		Source: st.source, Reads: st.reads, Selection: st.selection})
}

// endStream keeps a stream's output as it ended: its rows, and why it
// failed if it did.
func (m *Model) endStream(st *nbStream, err error) {
	m.showStream(st)
	if err != nil {
		o := *m.book().Output(st.id)
		o.Err = runError(err)
		m.book().SetOutput(st.id, &o)
	}
}

// stopStream stops cell id's stream, keeping what it printed.
func (m *Model) stopStream(id int) {
	st := m.nb.runs.streams[id]
	if st == nil {
		return
	}
	delete(m.nb.runs.streams, id)
	st.src.Close()
	m.showStream(st)
}

// stopStreams stops every stream.
func (m *Model) stopStreams() {
	for id := range m.nb.runs.streams {
		m.stopStream(id)
	}
}

// closeStreams stops every stream's process as the program ends,
// touching nothing else: the model may be broken by a crash.
func (m *Model) closeStreams() {
	for _, st := range m.nb.runs.streams {
		st.src.Close()
	}
}

// tickStreams redraws streaming cells' heads while any runs, so the
// toolbar and the output's run time keep up.
func (m *Model) tickStreams() tea.Cmd {
	if m.nb.still || m.nb.streamTick {
		return nil
	}
	m.nb.streamTick = true
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return nbStreamTickMsg{} })
}

// nbStreamTickMsg redraws streaming cells.
type nbStreamTickMsg struct{}

// streamTicked keeps the ticks going while a stream runs.
func (m *Model) streamTicked() tea.Cmd {
	m.nb.streamTick = false
	if len(m.nb.runs.streams) == 0 {
		return nil
	}
	return m.tickStreams()
}
