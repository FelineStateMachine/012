package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// What a cell's run reads: the outputs of the cells it names ($sales),
// linked files ($app), the selection on the sheet shown last
// ($selection) and ranges of sheets ($sheet.A1:C9), each handed to nu as
// NUON, never spliced into the pipeline.

// jobFor is what running cell c takes: the outputs, variables, linked
// files and ranges it reads, as NUON. As a stream, the cell hands back
// no variables but its output (notebook.Source.StreamCommand).
func (m *Model) jobFor(s *sheet.Sheet, c notebook.Cell, run *nbRun, stream bool) (nushell.Job, error) {
	cells := s.NotebookCells()
	i := slices.IndexFunc(cells, func(x notebook.Cell) bool { return x.ID == c.ID })
	p, err := notebook.Prepare(cells, i, m.book().Output)
	job := nushell.Job{Command: p.Command, Vars: p.Exports, Tables: p.Tables, Config: m.configBool("nu-config", false)}
	if stream {
		job.Command, job.Vars = p.Stream, nil
	}
	run.reads = p.Reads
	if err != nil {
		return job, err
	}
	for _, name := range p.Others {
		if t, r, ok := m.book().Region(name); ok && r.Linked() && r.Name == name {
			data, err := regionNUON(t, r)
			if err != nil {
				return job, err
			}
			job.Tables[name] = data
		}
	}
	if p.Selection {
		if err := m.bindSelection(&job, run); err != nil {
			return job, err
		}
	}
	for _, ref := range p.Ranges {
		t, rng, err := m.rangeRef(ref.Ref)
		if err != nil {
			return job, err
		}
		data, err := encodeNUON(fileio.Snap(t, rng, t.Name()))
		if err != nil {
			return job, err
		}
		job.Tables[ref.Var] = data
	}
	return job, nil
}

// bindSelection gives the job the selection on the sheet shown last.
func (m *Model) bindSelection(job *nushell.Job, run *nbRun) error {
	t := m.nb.grid
	if t == nil || m.book().Index(t) < 0 {
		return errors.New("it reads $selection: select a range on a sheet first")
	}
	data, err := encodeNUON(fileio.Snap(t, m.nb.sel, t.Name()))
	if err != nil {
		return err
	}
	job.Tables[notebook.Selection] = data
	run.selection = sheet.Qualified(t.Name(), m.nb.sel)
	return nil
}

// rangeRef is the sheet and range $sheet.ref reads: a range of the
// sheet shown last, or of the sheet it names.
func (m *Model) rangeRef(ref string) (*sheet.Sheet, sheet.Rect, error) {
	name, cells := sheet.SplitSheet(ref)
	t := m.nb.grid
	if name != "" {
		t = m.book().Lookup(name)
	}
	if t == nil || t.IsNotebook() {
		for _, s := range m.book().Sheets() {
			if !s.IsNotebook() && name == "" {
				t = s
				break
			}
		}
	}
	rng, ok := sheet.ParseRange(cells)
	if t == nil || t.IsNotebook() || !ok {
		return nil, sheet.Rect{}, fmt.Errorf("$sheet.%s isn't a range of a sheet", ref)
	}
	return t, rng, nil
}

// regionNUON is a linked file's table as NUON, read whole whatever its
// filter shows.
func regionNUON(t *sheet.Sheet, r sheet.Region) ([]byte, error) {
	rng, ok := t.RegionTable(r.Name)
	if !ok {
		rng = sheet.Rect{From: r.At, To: r.At}
	}
	snap := fileio.Snap(t, rng, r.Name)
	snap.HiddenRows = nil
	if !ok {
		snap.Cells = nil
	}
	return encodeNUON(snap)
}

func encodeNUON(snap *fileio.Snapshot) ([]byte, error) {
	var b bytes.Buffer
	if _, err := fileio.Encode(&b, fileio.NUON, snap); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// loadWords asks nu for its command names, once a session, for
// completing them.
func (m *Model) loadWords() tea.Cmd {
	if m.nb.asked || m.shellOff() != "" {
		return nil
	}
	m.nb.asked = true
	runner := m.runner()
	return func() tea.Msg {
		words, _ := nushell.Commands(context.Background(), runner, 30*time.Second)
		return nbWordsMsg{words}
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
