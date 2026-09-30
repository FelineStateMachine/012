package headless

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// NotebookOptions say how RunNotebooks runs cells.
type NotebookOptions struct {
	Runner   nushell.Runner // nu, or a fake in tests
	Timeout  time.Duration  // for each cell
	NuConfig bool           // run nu with the user's config files
}

// maxOutput is the most a run may print, as on the screen.
const maxOutput = 512 << 20

// RunNotebooks runs the code cells of each notebook tab, each after the
// cells it reads, as Run all does on the screen: each cell's output
// replaces the one the file kept and goes on to the sheet it was sent
// to. A failure stops its notebook's run, as on the screen. It returns
// a line for each cell that failed or couldn't run.
func RunNotebooks(ctx context.Context, w *sheet.Workbook, o NotebookOptions) []string {
	var failed []string
	for _, s := range w.Sheets() {
		if s.IsNotebook() {
			failed = append(failed, runNotebook(ctx, w, s, o)...)
		}
	}
	return failed
}

func runNotebook(ctx context.Context, w *sheet.Workbook, s *sheet.Sheet, o NotebookOptions) []string {
	cells := s.NotebookCells()
	all := make([]int, len(cells))
	for i := range all {
		all[i] = i
	}
	order, cycle := notebook.Order(cells, all)
	var failed []string
	for _, i := range cycle {
		failed = append(failed, fmt.Sprintf("%s cell %d reads itself, through the cells it reads: it doesn't run", s.Name(), i+1))
	}
	for n, i := range order {
		if err := runCell(ctx, w, cells, i, n+1, o); err != nil {
			return append(failed, fmt.Sprintf("%s cell %d: %s", s.Name(), i+1, err))
		}
	}
	return failed
}

// runCell runs cell i, the count-th run, keeping its output or why it
// failed.
func runCell(ctx context.Context, w *sheet.Workbook, cells []notebook.Cell, i, count int, o NotebookOptions) error {
	c := cells[i]
	out := &notebook.Output{Count: count, Source: c.Source}
	defer func() {
		w.SetOutput(c.ID, out)
		feedOutput(w, c.Name())
	}()
	if why := notebook.Taken(cells, i); why != "" {
		out.Err = why
		return errors.New(why)
	}
	job, reads, err := jobFor(w, cells, i, o.NuConfig)
	if err != nil {
		out.Err = err.Error()
		return err
	}
	out.Reads = reads
	start := time.Now()
	data, vars, err := nushell.ExecVars(ctx, o.Runner, job, o.Timeout, maxOutput)
	out.Took = time.Since(start)
	if err != nil {
		out.Err = err.Error()
		var nuErr *nushell.Error
		if errors.As(err, &nuErr) {
			out.Err, out.Detail = nuErr.Msg, nuErr.Help()
		}
		return errors.New(out.Err)
	}
	out.NUON, out.Vars = append([]byte{}, data...), vars
	return nil
}

// jobFor is what running cell i takes: the outputs, variables, linked
// files and ranges it reads, as NUON, and the outputs it read by name.
func jobFor(w *sheet.Workbook, cells []notebook.Cell, i int, config bool) (nushell.Job, map[string]int, error) {
	p, err := notebook.Prepare(cells, i, w.Output)
	job := nushell.Job{Command: p.Command, Vars: p.Exports, Tables: p.Tables, Config: config}
	if err != nil {
		return job, nil, err
	}
	for _, name := range p.Others {
		if t, r, ok := w.Region(name); ok && r.Linked() && r.Name == name {
			data, err := regionNUON(t, r)
			if err != nil {
				return job, nil, err
			}
			job.Tables[name] = data
		}
	}
	if p.Selection {
		return job, nil, errors.New("it reads $selection, which only the screen has")
	}
	for _, ref := range p.Ranges {
		t, rng, err := rangeRef(w, ref.Ref)
		if err != nil {
			return job, nil, err
		}
		data, err := encodeNUON(fileio.Snap(t, rng, t.Name()))
		if err != nil {
			return job, nil, err
		}
		job.Tables[ref.Var] = data
	}
	return job, p.Reads, nil
}

// regionNUON is a linked file's table as NUON, read whole whatever its
// filter shows, as on the screen.
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

// rangeRef is the sheet and range $sheet.ref reads: of the sheet it
// names, or else of the sheet shown when the file was saved, or the
// first sheet that isn't a notebook.
func rangeRef(w *sheet.Workbook, ref string) (*sheet.Sheet, sheet.Rect, error) {
	name, cells := sheet.SplitSheet(ref)
	t := w.Sheet(w.Active())
	if name != "" {
		t = w.Lookup(name)
	} else if t.IsNotebook() {
		for _, s := range w.Sheets() {
			if !s.IsNotebook() {
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

func encodeNUON(snap *fileio.Snapshot) ([]byte, error) {
	var b bytes.Buffer
	_, err := fileio.Encode(&b, fileio.NUON, snap)
	return b.Bytes(), err
}

// SyncOutputs sends the outputs a file kept to the sheets they were sent
// to, as opening the file on the screen does: reading, not running.
func SyncOutputs(w *sheet.Workbook) {
	for _, name := range w.StaleOutputs() {
		feedOutput(w, name)
	}
}

// feedOutput sends the output of the cell named name to the region it
// was sent to, if it was: its rows, none when it hasn't run, or why when
// it failed or is gone.
func feedOutput(w *sheet.Workbook, name string) {
	if name == "" {
		return
	}
	_, r, ok := w.Region(name)
	if !ok || !r.Output {
		return
	}
	op := sheet.LiveOp{Region: r.Name, Reset: true}
	c, ok := cellNamed(w, r.Name)
	switch o := w.Output(c.ID); {
	case !ok:
		op = sheet.LiveOp{Region: r.Name, Err: "its cell is gone"}
	case o == nil || o.Unsaved:
	case o.Failed():
		op = sheet.LiveOp{Region: r.Name, Err: o.Err}
	default:
		rows, note, err := fileio.NUONRows(context.Background(), o.NUON, 0)
		if err != nil {
			op = sheet.LiveOp{Region: r.Name, Err: err.Error()}
			break
		}
		op.Header, op.Rows, op.Note = rows.Header, rows.Rows, note
	}
	w.ApplyLive(op)
}

// MissingOutputs names the notebook outputs sent to s that show no rows
// because the file holds no output for their cells: never run, too
// large to save, or their cells gone.
func MissingOutputs(s *sheet.Sheet) []string {
	var out []string
	for _, r := range s.Regions() {
		if !r.Output {
			continue
		}
		c, ok := cellNamed(s.Book(), r.Name)
		if o := s.Book().Output(c.ID); !ok || o == nil || o.Unsaved {
			out = append(out, r.Name)
		}
	}
	return out
}

// cellNamed is the notebook cell that gives its output name.
func cellNamed(w *sheet.Workbook, name string) (notebook.Cell, bool) {
	for _, s := range w.Sheets() {
		cells := s.NotebookCells()
		if i, ok := notebook.Names(cells)[name]; ok {
			return cells[i], true
		}
	}
	return notebook.Cell{}, false
}
