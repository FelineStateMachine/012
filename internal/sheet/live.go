package sheet

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/FelineStateMachine/012/internal/value"
)

// The change stream: what reads a linked region's source sends the
// workbook as LiveOps, each applied at once through ApplyLive, outside
// the undo history, as a spill is written. An op names the region by
// ID and carries whole rows of values with the formats they arrived in,
// so it depends on nothing but the region: the same op applied to the
// same workbook elsewhere makes the same cells, which is what a session
// following another's would be sent.

// LiveCell is a value as a source gave it, with its format (a date's,
// a file size's), Automatic for none.
type LiveCell struct {
	V Value
	F Format
}

// LiveRow is a row of a linked region, from its first column.
type LiveRow []LiveCell

// LiveOp is one change a source makes to its linked region.
type LiveOp struct {
	// Link is the region's ID (LinkedRegion.ID).
	Link int
	// At is when the source changed.
	At time.Time
	// Reset has Rows replace every row under the header; otherwise they
	// follow the last.
	Reset bool
	// Header is the region's first row, when it is new or changed: the
	// source's column names.
	Header LiveRow
	// Rows are data rows, under the header.
	Rows []LiveRow
	// Err says why the source can't be read, "" when it can. An op with
	// an error changes no cells: the region keeps what it shows.
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

// ApplyLive applies op to its region: its rows are written, the window
// drops the oldest, and what reads the cells that changed recalculates.
// It returns the region as the op leaves it, or ErrNoLinked for a
// region the workbook doesn't hold.
func (w *Workbook) ApplyLive(op LiveOp) (LinkedRegion, error) {
	s, l := w.findLinked(op.Link)
	if l == nil {
		return LinkedRegion{}, ErrNoLinked
	}
	var start time.Time
	if OnLive != nil {
		if OnBegin != nil {
			OnBegin(w.trace, "live")
		}
		start = time.Now()
	}
	changed := s.applyLive(l, op)
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
	return l.info(s), nil
}

// applyLive writes op's rows into l and returns the cells that changed.
func (s *Sheet) applyLive(l *linked, op LiveOp) []loc {
	if !op.At.IsZero() && op.Err == "" {
		l.updated = op.At
	}
	if op.Reset {
		l.stale = false
	}
	if op.Err != "" {
		l.err = op.Err
		if l.rows == 0 {
			return s.writeLinkedError(l)
		}
		return nil
	}
	l.err, l.note = "", op.Note
	rows, header := s.liveRows(l, op)
	return s.writeLinkedRows(l, header, rows, op.Reset)
}

// liveRows is the data rows the region is to show after op, with the
// header: when rows are dropped (by the window, or when the region is
// full) the rows kept are read back from the region's own cells.
// Rows that already show and stay are nil, so only new ones are written.
func (s *Sheet) liveRows(l *linked, op LiveOp) (rows []LiveRow, header LiveRow) {
	header = op.Header
	if header == nil && l.rows > 0 {
		header = s.readLinkedRow(l, 0)
	}
	cols := max(len(header), l.cols)
	for _, r := range op.Rows {
		cols = max(cols, len(r))
	}
	old := max(l.rows-1, 0)
	if op.Reset {
		old = 0
	}
	total := old + len(op.Rows)
	keep := total
	if l.src.Window > 0 {
		keep = min(keep, l.src.Window)
	}
	// Whole rows, as many as fit in max-cells.
	if fit := MaxCells()/max(cols, 1) - 1; keep > fit {
		keep = max(fit, 0)
		l.note = fmt.Sprintf("only the last %d rows fit in max-cells (%d cells)", keep, MaxCells())
		if l.src.Window == 0 {
			l.note = fmt.Sprintf("only the first %d rows fit in max-cells (%d cells)", keep, MaxCells())
		}
	}
	if op.Reset {
		l.dropped = 0
		l.data = 0
	}
	l.data += len(op.Rows)
	if keep < total && l.src.Window == 0 {
		// Keep everything up to the budget: the newest rows are left out.
		rows = make([]LiveRow, keep)
		copy(rows[min(old, keep):], op.Rows[:max(keep-old, 0)])
		return rows[:keep], header
	}
	drop := total - keep
	l.dropped += drop
	rows = make([]LiveRow, keep)
	for i := range keep {
		switch j := i + drop; {
		case j >= old:
			rows[i] = op.Rows[j-old]
		case drop > 0:
			rows[i] = s.readLinkedRow(l, j+1)
		}
	}
	return rows, header
}

// readLinkedRow reads row r of the region (0 for the header) back from
// its cells.
func (s *Sheet) readLinkedRow(l *linked, r int) LiveRow {
	row := make(LiveRow, l.cols)
	for c := range l.cols {
		v, lk, kind := s.cells.derivedOf(Addr{Col: l.anchor.Col + c, Row: l.anchor.Row + r})
		if kind == slotSpill {
			row[c] = LiveCell{V: v, F: lk.auto}
		}
	}
	return row
}

// writeLinkedRows makes the region show header and rows, writing the
// rows that aren't nil (nil ones show already), and returns the cells
// that changed. When a cell it needs holds something of its own, it
// writes nothing and says so.
func (s *Sheet) writeLinkedRows(l *linked, header LiveRow, rows []LiveRow, reset bool) []loc {
	cols, nrows := linkedSize(l, header, rows, reset)
	area := Rect{From: l.anchor, To: Addr{Col: l.anchor.Col + max(cols, 1) - 1, Row: l.anchor.Row + max(nrows, 1) - 1}}
	if at, ok := s.linkedBlocked(l, area); ok {
		l.err = "The linked rows would overwrite data in " + at.String()
		if area.To.Row >= MaxRows || area.To.Col >= MaxCols {
			l.err = "The linked rows would go past the edge of the sheet"
		}
		return nil
	}
	keep := area
	if nrows == 0 {
		keep = noRect
	}
	changed := s.clearSpilled(l.area(), keep)
	if cols > 0 {
		changed = s.writeLinkedRow(l, 0, header, cols, true, changed)
	}
	for i, row := range rows {
		// A row written over one that showed clears what it doesn't
		// fill; one below the old region has nothing to clear.
		if row != nil {
			changed = s.writeLinkedRow(l, i+1, row, cols, i+1 < l.rows || reset, changed)
		}
	}
	l.cols, l.rows = cols, nrows
	if !l.fitted && nrows > 1 {
		l.fitted = true
		s.fitLinked(l)
	}
	for _, c := range changed {
		s.freedFor(c.a)
	}
	return changed
}

// linkedSize is how many columns and rows the region takes to show
// header and rows: none when there's nothing to show.
func linkedSize(l *linked, header LiveRow, rows []LiveRow, reset bool) (cols, nrows int) {
	cols = len(header)
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if !reset {
		cols = max(cols, l.cols)
	}
	if cols > 0 {
		nrows = 1 + len(rows)
	}
	return cols, nrows
}

// writeLinkedRow writes row r of the region (0 for the header), cols
// cells wide, adding the cells that changed to changed. With all, the
// cells the row leaves blank are cleared; otherwise they're blank
// already.
func (s *Sheet) writeLinkedRow(l *linked, r int, row LiveRow, cols int, all bool, changed []loc) []loc {
	for c := range cols {
		var lc LiveCell
		if c < len(row) {
			lc = row[c]
		}
		if lc.V.Kind == Empty && !all {
			continue
		}
		a := Addr{Col: l.anchor.Col + c, Row: l.anchor.Row + r}
		if s.writeSpilled(a, lc.V, lc.F) {
			changed = append(changed, loc{s, a})
		}
	}
	return changed
}

// linkedBlocked returns a cell of area the region can't write: one
// holding something that isn't the region's, or past the sheet's edge.
func (s *Sheet) linkedBlocked(l *linked, area Rect) (Addr, bool) {
	if area.To.Row >= MaxRows || area.To.Col >= MaxCols {
		return area.To, true
	}
	own := l.area()
	for a := range s.cells.anyKeysIn(area) {
		if !s.cells.filledAt(a) || own.Contains(a) && s.cells.derivedAt(a) == slotSpill {
			continue
		}
		return a, true
	}
	if ms := s.MergesIn(area); len(ms) > 0 {
		return ms[0].From, true
	}
	return Addr{}, false
}

// writeLinkedError shows the region's error in its anchor, as #REF!,
// while it has nothing else to show.
func (s *Sheet) writeLinkedError(l *linked) []loc {
	if s.cells.filledAt(l.anchor) && s.cells.derivedAt(l.anchor) != slotSpill {
		return nil // never over what the user typed
	}
	l.cols, l.rows = 0, 0
	if s.writeSpilled(l.anchor, ErrRef, Format{}) {
		return []loc{{s, l.anchor}}
	}
	return nil
}

// freedFor has a spill waiting for the cell at a try again, now that the
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

// maxLinkedWidth caps how wide fitting makes a region's column.
const maxLinkedWidth = 30

// fitLinked widens the region's columns, the first time rows arrive, to
// their widest text in the first rows, never below the default width,
// outside the undo history as a pivot's are.
func (s *Sheet) fitLinked(l *linked) {
	const sample = 200
	for c := range l.cols {
		col := l.anchor.Col + c
		if s.widths[col] != 0 {
			continue // set by hand, or by the file
		}
		widest := 0
		for r := range min(l.rows, sample) {
			widest = max(widest, len([]rune(s.ShownText(Addr{Col: col, Row: l.anchor.Row + r}))))
		}
		if widest+2 > DefaultWidth {
			s.setWidth(col, min(widest+2, maxLinkedWidth))
		}
	}
}

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
