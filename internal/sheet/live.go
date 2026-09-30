package sheet

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/FelineStateMachine/012/internal/value"
)

// The change stream: rows reach a region as LiveOps, each applied at
// once through ApplyLive, outside the undo history, as a spill is
// written. An op names its region by name, which the file keeps, and
// carries whole rows of values with the formats they arrived in, so it
// depends on nothing but the region: the same op applied to the same
// workbook elsewhere makes the same cells, which is what a session
// following another's would be sent. Whatever reads a linked file sends
// them, and the UI sends a notebook cell's output to the region it was
// sent to whenever the cell runs.

// LiveCell is a value as a source gave it, with its format (a date's,
// a file size's), Automatic for none.
type LiveCell struct {
	V Value
	F Format
}

// LiveRow is a row of a region's table, from its first column.
type LiveRow []LiveCell

// LiveOp is one change a source makes to its region.
type LiveOp struct {
	// Region is the region's name (Region.Name).
	Region string
	// At is when the source changed.
	At time.Time
	// Reset has Rows replace every row under the header; otherwise they
	// follow the last.
	Reset bool
	// Header is the table's first row, when it is new or changed: the
	// source's column names.
	Header LiveRow
	// Rows are data rows, under the header.
	Rows []LiveRow
	// Err says why the source can't be read, "" when it can. An op with
	// an error changes no rows: the region keeps what it shows.
	Err string
	// Note says what the source left out, if anything.
	Note string
}

// LiveInfo describes one op applied, for telemetry. It holds counts
// only, never contents.
type LiveInfo struct {
	Rows     int // rows the op brought
	Cells    int // cells that changed
	Reset    bool
	Duration time.Duration // writing the cells and recalculating what reads them
}

// OnLive, when set, is called after every op applied, like OnRecalc
// (see OnBegin, whose op is "live").
var OnLive func(trace any, i LiveInfo)

// ApplyLive applies op to its region: its rows are written, a window
// drops the oldest, and what reads the cells that changed recalculates.
// It returns ErrNoRegion for a region the workbook doesn't hold.
func (w *Workbook) ApplyLive(op LiveOp) error {
	s, r, ok := w.Region(op.Region)
	if !ok {
		return ErrNoRegion
	}
	var start time.Time
	if OnLive != nil {
		if OnBegin != nil {
			OnBegin(w.trace, "live")
		}
		start = time.Now()
	}
	changed := s.applyLive(r, s.meta(nameKey(r.Name)), op)
	if len(changed) > 0 {
		if w.hist.depth > 0 {
			w.hist.dirty = append(w.hist.dirty, changed...) // recalculated as the open step ends
		} else {
			w.recalc(changed)
		}
	}
	if OnLive != nil {
		OnLive(w.trace, LiveInfo{Rows: len(op.Rows), Cells: len(changed), Reset: op.Reset, Duration: time.Since(start)})
	}
	return nil
}

// applyLive writes op's rows into r and returns the cells that changed.
func (s *Sheet) applyLive(r Region, me *regionMeta, op LiveOp) []loc {
	if !op.At.IsZero() && op.Err == "" {
		me.updated = op.At
	}
	if op.Reset {
		me.stale = false
	}
	if op.Err != "" {
		me.err = op.Err
		if me.rows == 0 {
			return s.writeTable(r, me, nil, nil, true)
		}
		return nil
	}
	me.err, me.note = "", op.Note
	rows, header := s.liveRows(r, me, op)
	return s.writeTable(r, me, header, rows, op.Reset)
}

// liveRows is the data rows the region is to show after op, with the
// header: when rows are dropped (by the window, or when the table is
// full) the rows kept are read back from the region's own cells. Rows
// that already show and stay are nil, so only new ones are written.
func (s *Sheet) liveRows(r Region, me *regionMeta, op LiveOp) (rows []LiveRow, header LiveRow) {
	header = op.Header
	if header == nil && me.rows > 0 {
		header = s.readRow(r, me, 0)
	}
	cols := max(len(header), me.cols)
	for _, row := range op.Rows {
		cols = max(cols, len(row))
	}
	old := max(me.rows-1, 0)
	if op.Reset {
		old = 0
	}
	window := r.File.Window
	total := old + len(op.Rows)
	keep := total
	if window > 0 {
		keep = min(keep, window)
	}
	// Whole rows, as many as fit in max-cells.
	if fit := MaxCells()/max(cols, 1) - 1; keep > fit {
		keep = max(fit, 0)
		me.note = fmt.Sprintf("only the last %d rows fit in max-cells (%d cells)", keep, MaxCells())
		if window == 0 {
			me.note = fmt.Sprintf("only the first %d rows fit in max-cells (%d cells)", keep, MaxCells())
		}
	}
	if op.Reset {
		me.dropped, me.data = 0, 0
	}
	me.data += len(op.Rows)
	if keep < total && window == 0 {
		// Keep everything up to the budget: the newest rows are left out.
		rows = make([]LiveRow, keep)
		copy(rows[min(old, keep):], op.Rows[:max(keep-old, 0)])
		return rows, header
	}
	drop := total - keep
	me.dropped += drop
	rows = make([]LiveRow, keep)
	for i := range keep {
		switch j := i + drop; {
		case j >= old:
			rows[i] = op.Rows[j-old]
		case drop > 0:
			rows[i] = s.readRow(r, me, j+1)
		}
	}
	return rows, header
}

// readRow reads row i of the region's table (0 for the header) back
// from its cells.
func (s *Sheet) readRow(r Region, me *regionMeta, i int) LiveRow {
	o := r.At
	row := make(LiveRow, me.cols)
	for c := range me.cols {
		v, lk, kind := s.cells.derivedOf(Addr{Col: o.Col + c, Row: o.Row + i})
		if kind == slotSpill {
			row[c] = LiveCell{V: v, F: lk.auto}
		}
	}
	return row
}

// freedFor has a spill waiting for the cell at a try again, now that a
// region wrote or cleared it.
func (s *Sheet) freedFor(a Addr) {
	if s.spills == nil {
		return
	}
	s.spillAt.readers(a, func(anchor Addr) {
		if sp := s.spills[anchor]; sp != nil && sp.why != "" {
			sp.stale = true
			s.wb.markDirty(loc{s, anchor})
		}
	})
}

// maxFitWidth caps how wide fitting makes a table's column.
const maxFitWidth = 30

// fitTable widens the columns of a table at o, the first time rows
// arrive, to their widest text in the first rows, never below the
// default width, outside the undo history as a pivot's are.
func (s *Sheet) fitTable(o Addr, rows, cols int) {
	const sample = 200
	for c := range cols {
		col := o.Col + c
		if s.widths[col] != 0 {
			continue // set by hand, or by the file
		}
		widest := 0
		for r := range min(rows, sample) {
			widest = max(widest, FitWidth(s.ShownText(Addr{Col: col, Row: o.Row + r})))
		}
		if widest > DefaultWidth {
			s.setWidth(col, widest)
		}
	}
}

// FitWidth is how wide fitting a table (LoadFitWidths) makes a column
// for text: its characters and a space either side, capped.
func FitWidth(text string) int { return min(len([]rune(text))+2, maxFitWidth) }

// EntryValue is the value and the format an entry implies ("$5", a
// date), as typing it into a cell would store them, for a source whose
// rows arrive as text. Formulas stay text, as a data file's should.
func EntryValue(input string) (Value, Format) {
	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, input)
	if input == "" {
		return Value{}, Format{}
	}
	if rest, ok := strings.CutPrefix(input, "'"); ok {
		return value.Str(rest), Format{}
	}
	if n, f, ok := ParseValue(input); ok {
		return value.Num(n), f
	}
	switch strings.ToUpper(input) {
	case "TRUE":
		return value.Boolean(true), Format{}
	case "FALSE":
		return value.Boolean(false), Format{}
	}
	return value.Str(input), Format{}
}
