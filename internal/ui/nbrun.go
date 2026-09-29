package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
)

// Running cells. One runs at a time, in the background, so the screen
// stays live and Stop kills it: the cells asked for are a queue, each
// started when the one before has finished. What a cell reads (other
// cells' outputs, ranges of sheets) is taken as it starts; what it
// printed becomes its output as it ends, and goes on to the sheet it was
// sent to. A failure stops the queue, as Jupyter's run all stops.

// nbState is the notebooks' part of the model.
type nbState struct {
	views   map[*sheet.Sheet]*nbview.View
	runner  nushell.Runner // nu, or a fake in tests
	queue   []nbQueued
	running *nbRun
	gen     int
	count   int // runs this session, for [n]
	clip    []notebook.Cell
	// words are nu's command names, asked for once (asked).
	words []string
	asked bool
	// served is set in a session of 012 serve, where cells run only
	// when the server's config allows them.
	served bool
	// startup shows the notebook once the program starts (012 nu).
	startup bool
	// fromFile is set when the workbook opened with cells of its own,
	// which may have been made on another computer.
	fromFile bool
	// grid is the sheet shown last that isn't a notebook, and sel its
	// selection then: what $selection reads.
	grid *sheet.Sheet
	sel  sheet.Rect
	// stale caches which cells are stale, for the state it was worked
	// out in.
	stale    map[int]bool
	staleKey [2]int
	// still stops the running cell's head turning, so tests that run
	// commands in line don't wait on its ticks.
	still bool
	// saved is the outputs' change count when the file was last saved,
	// and saving when it was being saved.
	saved, saving int
	// lang asks nu about the cells being written (nblang.go), and
	// langKey is what the words it was last given were worked out from.
	lang    *nbview.NuSession
	langKey nbLangKey
	// follow scrolls the view to each cell as it starts, while a run of
	// several cells goes on and the user hasn't scrolled away.
	follow bool
	// out is the outputs' grids (nbgrid.go), and frame counts the frames
	// drawn, which the grids clear what they keep for a frame by.
	out   outGrids
	frame int
}

// nbQueued is a cell waiting to run.
type nbQueued struct {
	s  *sheet.Sheet
	id int
}

// nbRun is a cell running.
type nbRun struct {
	nbQueued
	gen       int
	cancel    context.CancelFunc
	start     time.Time
	source    string
	reads     map[string]int
	selection string
}

type (
	// nbDoneMsg is a run finished: what it printed, or why it failed.
	nbDoneMsg struct {
		run  *nbRun
		nuon []byte
		err  error
	}
	// nbWordsMsg brings nu's command names.
	nbWordsMsg struct{ words []string }
	// nbTickMsg redraws a running cell's head.
	nbTickMsg struct{ gen int }
)

// SetShellRunner makes cells run, and the code editor ask, with r
// rather than nu, for tests; it's set before anything is asked.
func (m *Model) SetShellRunner(r nushell.Runner) {
	m.nb.runner = r
	if m.nb.lang != nil {
		m.nb.lang.Runner = r
	}
}

func (m *Model) runner() nushell.Runner {
	if m.nb.runner == nil {
		return nushell.Nu{}
	}
	return m.nb.runner
}

// cellState is how cell id of notebook s stands.
func (m *Model) cellState(s *sheet.Sheet, id int) nbview.State {
	var st nbview.State
	if r := m.nb.running; r != nil && r.s == s && r.id == id {
		st.Running, st.Started = true, r.start
	}
	st.Waiting = slices.Contains(m.nb.queue, nbQueued{s, id})
	st.Stale = m.staleCells(s)[id]
	cells := s.NotebookCells()
	if i := slices.IndexFunc(cells, func(c notebook.Cell) bool { return c.ID == id }); i >= 0 {
		st.Problem = notebook.Taken(cells, i)
	}
	return st
}

// staleCells are notebook s's stale cells, worked out once a state.
func (m *Model) staleCells(s *sheet.Sheet) map[int]bool {
	key := [2]int{m.book().StateID(), m.book().OutputsChanged()}
	if m.nb.stale == nil || m.nb.staleKey != key {
		m.nb.stale = map[int]bool{}
		for _, t := range m.book().Sheets() {
			if t.IsNotebook() {
				for id := range notebook.Stale(t.NotebookCells(), m.book().Output) {
					m.nb.stale[id] = true
				}
			}
		}
		m.nb.staleKey = key
	}
	return m.nb.stale
}

// runSelected runs the selected cells: then, with next 1, selects the
// cell below them (adding one at the end), or with 2 adds one under
// them, as Jupyter's Shift+Enter and Alt+Enter.
func (m *Model) runSelected(next int) tea.Cmd {
	v := m.nbView()
	v.StopEdit()
	from, to := v.Range()
	cells := m.sheet.NotebookCells()
	if from >= len(cells) {
		return nil
	}
	var cmd tea.Cmd
	switch {
	case from < to:
		cmd = m.runRange(from, to+1)
	case cells[from].Kind == notebook.Code:
		cmd = m.runCells(m.sheet, from, cells)
	}
	switch {
	case next == 2 || next == 1 && to == len(cells)-1:
		return tea.Batch(cmd, m.addCell(1, next == 2))
	case next == 1:
		v.Select(to+1, false)
	}
	return cmd
}

// runRange runs the code cells from index from up to to (-1 for the
// last), each after the cells it reads, the view following the cell
// running.
func (m *Model) runRange(from, to int) tea.Cmd {
	v := m.nbView()
	v.StopEdit()
	cells := m.sheet.NotebookCells()
	if to < 0 {
		to = len(cells)
	}
	var want []int
	for i := from; i < to && i < len(cells); i++ {
		want = append(want, i)
	}
	m.nb.follow = len(want) > 1
	return m.queueCells(m.sheet, cells, want)
}

// runCells runs cell i, after what it reads that hasn't run, and in a
// reactive notebook then the cells reading it.
func (m *Model) runCells(s *sheet.Sheet, i int, cells []notebook.Cell) tea.Cmd {
	want := append(notebook.Inputs(cells, i, m.book().Output), i)
	if s.Reactive() {
		want = append(want, notebook.Dependents(cells, i)...)
	}
	return m.queueCells(s, cells, want)
}

// queueCells queues the code cells of want, in the order they run, asking
// first when the cells came from another computer.
func (m *Model) queueCells(s *sheet.Sheet, cells []notebook.Cell, want []int) tea.Cmd {
	order, cycle := notebook.Order(cells, want)
	for _, i := range cycle {
		m.fail(fmt.Sprintf("Cell %d reads itself, through the cells it reads: it doesn't run", i+1))
	}
	if len(order) == 0 {
		return nil
	}
	return m.trustNotebook(func(m *Model) tea.Cmd {
		for _, i := range order {
			q := nbQueued{s, cells[i].ID}
			if !slices.Contains(m.nb.queue, q) && (m.nb.running == nil || m.nb.running.nbQueued != q) {
				m.nb.queue = append(m.nb.queue, q)
			}
		}
		if m.nb.running != nil {
			return nil
		}
		return m.nextCell()
	})
}

// shellOff says why cells can't run here, or "".
func (m *Model) shellOff() string {
	switch {
	case m.nb.served && !m.configBool("serve-shell", false):
		return "Notebook cells don't run in 012 serve (serve-shell in the server's config)"
	case m.configString("shell", "ask") == "off":
		return "Notebook cells don't run (shell = off in your config)"
	}
	return ""
}

// trustNotebook runs run once cells may run: shell = on, or the file's
// cells were made or trusted here, or the user agrees now.
func (m *Model) trustNotebook(run func(*Model) tea.Cmd) tea.Cmd {
	if why := m.shellOff(); why != "" {
		m.fail(why)
		return nil
	}
	if m.configString("shell", "ask") == "on" || m.macroTrusted() {
		return run(m)
	}
	m.ask(question{
		msg:  "Run this file's notebook cells?",
		desc: "It was saved on another computer: its cells run as you, with your files",
		choices: []choice{
			{key: "enter", label: "Run", run: func(m *Model) tea.Cmd { m.trustHere(); return run(m) }},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}

// nextCell starts the first cell queued.
func (m *Model) nextCell() tea.Cmd {
	for len(m.nb.queue) > 0 {
		q := m.nb.queue[0]
		m.nb.queue = m.nb.queue[1:]
		c, ok := cellOf(q)
		if !ok || c.Kind != notebook.Code {
			continue
		}
		cells := q.s.NotebookCells()
		i := slices.IndexFunc(cells, func(x notebook.Cell) bool { return x.ID == c.ID })
		if why := notebook.Taken(cells, i); why != "" {
			m.failCell(q, c, why)
			continue
		}
		run := &nbRun{nbQueued: q, source: c.Source, start: time.Now()}
		job, err := m.jobFor(q.s, c, run)
		if err != nil {
			m.failCell(q, c, err.Error())
			continue
		}
		return m.startCell(run, job)
	}
	m.nb.follow = false // nothing left to follow
	return nil
}

// cellOf is the cell q names, if it's still there.
func cellOf(q nbQueued) (notebook.Cell, bool) {
	for _, c := range q.s.NotebookCells() {
		if c.ID == q.id {
			return c, true
		}
	}
	return notebook.Cell{}, false
}

// failCell gives a cell that can't run an output saying why, and stops
// the queue.
func (m *Model) failCell(q nbQueued, c notebook.Cell, why string) {
	m.nb.count++
	m.book().SetOutput(q.id, &notebook.Output{Err: why, Count: m.nb.count, Source: c.Source})
	m.nb.queue = nil
}

// startCell runs a cell in the background.
func (m *Model) startCell(run *nbRun, job nushell.Job) tea.Cmd {
	m.nb.gen++
	ctx, cancel := context.WithCancel(context.Background())
	run.gen, run.cancel = m.nb.gen, cancel
	m.nb.running = run
	if m.nb.follow {
		m.viewOf(run.s).Reveal(run.id) // Run all: the view follows the cell running
	}
	runner, timeout, parent := m.runner(), m.configDuration("nu-timeout", 30*time.Second), m.spans.Parent()
	exec := func() tea.Msg {
		defer cancel()
		span := parent.Start("nu", slog.Int("tables", len(job.Tables)))
		out, err := nushell.Exec(ctx, runner, job, timeout, maxOutput)
		if err != nil {
			span.Fail(err)
		} else {
			span.End(slog.Int("bytes", len(out)))
		}
		return nbDoneMsg{run: run, nuon: out, err: err}
	}
	return tea.Batch(exec, m.tickCell(run.gen))
}

// maxOutput is the most a run may print: past it the run is stopped.
const maxOutput = 512 << 20

// tickCell redraws the running cell's head every tenth of a second.
func (m *Model) tickCell(gen int) tea.Cmd {
	if m.nb.still {
		return nil
	}
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return nbTickMsg{gen} })
}

// ticked keeps the running cell's head turning.
func (m *Model) ticked(msg nbTickMsg) tea.Cmd {
	if r := m.nb.running; r != nil && r.gen == msg.gen {
		return m.tickCell(msg.gen)
	}
	return nil
}

// finishCell keeps what a run made, sends it on to its sheet, and
// starts the next cell queued.
func (m *Model) finishCell(msg nbDoneMsg) tea.Cmd {
	r := msg.run
	if m.nb.running != r {
		return nil // stopped, and another started since
	}
	m.nb.running = nil
	c, ok := cellOf(r.nbQueued)
	if !ok {
		return m.nextCell()
	}
	m.nb.count++
	o := &notebook.Output{Count: m.nb.count, Took: time.Since(r.start), Source: r.source, Reads: r.reads, Selection: r.selection}
	if msg.err != nil {
		o.Err = runError(msg.err)
		var nuErr *nushell.Error
		if errors.As(msg.err, &nuErr) {
			o.Detail = nuErr.Help()
		}
		m.nb.queue = nil
	} else {
		o.NUON = msg.nuon
		if o.NUON == nil {
			o.NUON = []byte{}
		}
	}
	m.book().SetOutput(c.ID, o)
	m.feedOutput(c.Name())
	if msg.err == nil && r.s.Reactive() {
		cells := r.s.NotebookCells()
		if i := slices.IndexFunc(cells, func(x notebook.Cell) bool { return x.ID == c.ID }); i >= 0 {
			for _, j := range notebook.Dependents(cells, i) {
				if q := (nbQueued{r.s, cells[j].ID}); !slices.Contains(m.nb.queue, q) {
					m.nb.queue = append(m.nb.queue, q)
				}
			}
		}
	}
	if len(m.nb.queue) == 0 && time.Since(r.start) > 5*time.Second {
		return tea.Batch(m.nextCell(), m.term.notify("Notebook cells finished in "+m.displayName()))
	}
	return m.nextCell()
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

// stopCells stops the cell running, killing its process, and forgets
// those waiting.
func (m *Model) stopCells() {
	if r := m.nb.running; r != nil {
		r.cancel()
	}
	m.nb.queue = nil
}

// clearOutputs clears every output of the notebook shown; restart also
// stops what's running and counts runs from 1 again.
func (m *Model) clearOutputs(restart bool) {
	if restart {
		m.stopCells()
		m.nb.running = nil
		m.nb.count = 0
	}
	for _, c := range m.sheet.NotebookCells() {
		if m.book().Output(c.ID) != nil {
			m.book().SetOutput(c.ID, nil)
			m.feedOutput(c.Name())
		}
	}
	m.note = "Cleared every output"
	if restart {
		m.note = "Restarted: every output cleared, runs counted from 1"
	}
}
