package sheet

import (
	"errors"
	"path/filepath"
	"slices"
	"time"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Linked regions: a range of a sheet whose cells come from outside the
// workbook, such as a file followed as it grows or is rewritten. A
// region is an anchor (its top-left cell) whose rows arrive as live
// operations (LiveOp, live.go) from whatever reads the source; the
// engine owns its cells as it owns a spill's: they are derived cells,
// written outside the undo history, refused by Set, constants to the
// formulas reading them, values to copies and exports, and never saved.
// The file keeps the region's source instead (linkfile.go), and the UI
// reads it again on opening.
//
// The first row is the source's header and stays; the rows under it are
// the data, all of them up to max-cells, or the last Window of them. A
// region grows right and down as rows arrive; one that would write over
// cells holding something stops, showing why, until it's read again.

// LinkSource is where a linked region's rows come from, as the file
// keeps it.
type LinkSource struct {
	// Path is the file, as the UI resolves it: relative to the
	// workbook's folder when it can be (see RebaseLinks).
	Path string
	// Format is the name of the file's format ("CSV", "NUON"), or "" to
	// tell it by the extension.
	Format string
	// Table is a SQLite table to read, or Query a query to run.
	Table, Query string
	// Window keeps the last Window rows under the header, dropping older
	// ones, as tail -f does; 0 keeps every row, up to max-cells.
	Window int
}

// LinkedRegion is a linked region as the UI shows it: where it is, what
// it reads, and how the reading goes.
type LinkedRegion struct {
	// ID names the region in the workbook while it's open, in LiveOps
	// and to the UI.
	ID     int
	Sheet  *Sheet
	Anchor Addr
	// Area is the cells it covers, the anchor alone while it has none.
	Area   Rect
	Source LinkSource
	// Rows counts the data rows it shows, under the header; Dropped the
	// older rows the window let go.
	Rows, Dropped int
	// Updated is when the source was last read, zero until it is.
	Updated time.Time
	// Err says why the source can't be read, or the rows can't be
	// written, "" when all is well; Note says what was left out.
	Err, Note string
	// Paused is set while the region isn't following its source.
	Paused bool
	// Stale is set when its cells may no longer be the source's (an undo
	// took them back, or the source changed): the source is to be read
	// again, whole.
	Stale bool
}

// ErrLinkedEdit is what typing into a linked region says.
var ErrLinkedEdit = errors.New("That cell shows part of a linked file: unlink it (Data > Linked file > Unlink) to edit it")

// linked is a region's state: its definition, replaced whole when it
// changes so undo steps keep the old one, and what the rows written
// have made it.
type linked struct {
	id     int
	anchor Addr
	src    LinkSource
	*liveState
}

// liveState is what the source's rows have made of a region, shared by
// the copies of its definition that undo steps keep.
type liveState struct {
	cols, rows int // the region's size, header included; 0 by 0 when empty
	data       int // data rows written, before the window
	dropped    int
	updated    time.Time
	err, note  string
	paused     bool
	stale      bool
	fitted     bool // its columns were widened to its text once
}

// area is the cells the region covers, the anchor alone when empty.
func (l *linked) area() Rect {
	return Rect{From: l.anchor, To: Addr{Col: l.anchor.Col + max(l.cols, 1) - 1, Row: l.anchor.Row + max(l.rows, 1) - 1}}
}

// info describes l on s.
func (l *linked) info(s *Sheet) LinkedRegion {
	return LinkedRegion{ID: l.id, Sheet: s, Anchor: l.anchor, Area: l.area(), Source: l.src,
		Rows: max(l.rows-1, 0), Dropped: l.dropped, Updated: l.updated, Err: l.err, Note: l.note,
		Paused: l.paused, Stale: l.stale}
}

// linkedAt is the region covering a, or nil.
func (s *Sheet) linkedAt(a Addr) *linked {
	for _, l := range s.links {
		if l.area().Contains(a) {
			return l
		}
	}
	return nil
}

// LinkedAt returns the linked region covering the cell at a.
func (s *Sheet) LinkedAt(a Addr) (LinkedRegion, bool) {
	if l := s.linkedAt(a); l != nil {
		return l.info(s), true
	}
	return LinkedRegion{}, false
}

// InLinked returns a cell of r in a linked region, which an edit of r
// can't change.
func (s *Sheet) InLinked(r Rect) (Addr, bool) {
	for _, l := range s.links {
		if l.rows == 0 && l.err == "" {
			continue // nothing written yet
		}
		if overlap, ok := intersectRect(l.area(), r); ok {
			return overlap.From, true
		}
	}
	return Addr{}, false
}

// HasLinked reports whether the sheet holds a linked region.
func (s *Sheet) HasLinked() bool { return len(s.links) > 0 }

// LinkedRegions returns the sheet's linked regions, in the order they
// were made.
func (s *Sheet) LinkedRegions() []LinkedRegion {
	out := make([]LinkedRegion, 0, len(s.links))
	for _, l := range s.links {
		out = append(out, l.info(s))
	}
	return out
}

// LinkedRegions returns every live sheet's linked regions, sheet by
// sheet.
func (w *Workbook) LinkedRegions() []LinkedRegion {
	var out []LinkedRegion
	for _, s := range w.sheets {
		out = append(out, s.LinkedRegions()...)
	}
	return out
}

// LinkedRegion returns the region id names, on a live sheet.
func (w *Workbook) LinkedRegion(id int) (LinkedRegion, bool) {
	if s, l := w.findLinked(id); l != nil {
		return l.info(s), true
	}
	return LinkedRegion{}, false
}

// findLinked is the region id names and its sheet, or nil.
func (w *Workbook) findLinked(id int) (*Sheet, *linked) {
	for _, s := range w.sheets {
		for _, l := range s.links {
			if l.id == id {
				return s, l
			}
		}
	}
	return nil, nil
}

// errLinkOver is why a region can't be made where it's asked.
var errLinkOver = errors.New("A linked file needs an empty cell to start in")

// AddLinked makes a linked region at a reading src, as one undo step,
// and returns its ID. It starts empty and stale: the UI reads the
// source and sends its rows.
func (s *Sheet) AddLinked(a Addr, src LinkSource) (int, error) {
	switch {
	case !a.Valid():
		return 0, errLinkOver
	case s.cells.filledAt(a) || s.linkedAt(a) != nil || s.InPivot(Rect{From: a, To: a}):
		return 0, errLinkOver
	}
	s.wb.linkSeq++
	l := &linked{id: s.wb.linkSeq, anchor: a, src: src, liveState: &liveState{stale: true}}
	s.change("link "+filepath.Base(src.Path), Rect{From: a, To: a}, func() {
		s.recordLinks()
		s.links = append(slices.Clone(s.links), l)
	})
	return l.id, nil
}

// SetLinkSource changes what a region reads (its window, say), as one
// undo step; the region is read again.
func (w *Workbook) SetLinkSource(id int, src LinkSource) error {
	s, l := w.findLinked(id)
	if l == nil {
		return ErrNoLinked
	}
	s.change("change link "+filepath.Base(src.Path), l.area(), func() {
		s.recordLinks()
		nl := *l
		nl.src = src
		nl.stale = true
		s.links = slices.Clone(s.links)
		s.links[slices.Index(s.links, l)] = &nl
	})
	return nil
}

// ErrNoLinked is returned for a region the workbook doesn't hold:
// unlinked, undone, or on a sheet deleted.
var ErrNoLinked = errors.New("That linked file is no longer linked")

// Unlink turns a region into the values it shows, as one undo step:
// they stay as ordinary cells, keeping their formatting and notes, and
// nothing reads the source for them.
func (w *Workbook) Unlink(id int) error {
	s, l := w.findLinked(id)
	if l == nil {
		return ErrNoLinked
	}
	area := l.area()
	var cells []*Cell
	var at []Addr
	for a := range s.cells.anyKeysIn(area) {
		if c := s.cells.get(a); c.Spilled() {
			cells, at = append(cells, c.plain().withNote(c.Note)), append(at, a)
		}
	}
	s.change("unlink "+filepath.Base(l.src.Path), area, func() {
		s.recordLinks()
		s.links = slices.DeleteFunc(slices.Clone(s.links), func(x *linked) bool { return x == l })
		for i, a := range at {
			s.place(a, cells[i])
		}
	})
	return nil
}

// PauseLinked stops a region following its source, or has it follow
// again. It isn't an edit: what follows the source reads it.
func (w *Workbook) PauseLinked(id int, paused bool) {
	if _, l := w.findLinked(id); l != nil {
		l.paused = paused
	}
}

// ReloadLinked marks a region stale, so the UI reads its source again,
// whole.
func (w *Workbook) ReloadLinked(id int) {
	if _, l := w.findLinked(id); l != nil {
		l.stale = true
	}
}

// RebaseLinks rewrites the path of every region with fn, as the UI does
// when the workbook is saved in another folder. It isn't an edit: the
// regions read the same files.
func (w *Workbook) RebaseLinks(fn func(string) string) {
	for _, s := range w.sheets {
		for _, l := range s.links {
			l.src.Path = fn(l.src.Path)
		}
	}
}

// recordLinks saves the sheet's regions before their first change in
// the open step.
func (s *Sheet) recordLinks() {
	if st := s.wb.hist.open; st != nil {
		if _, seen := st.links[s]; !seen {
			st.links[s] = s.links
		}
	}
}

// restoreLinks puts back a sheet's regions from an undo step. The
// regions there now are emptied first, and those put back read their
// sources again, as the cells undo put back may not be theirs.
func (s *Sheet) restoreLinks(links []*linked) []loc {
	changed := s.emptyLinked()
	for _, l := range links {
		l.rows, l.cols, l.data, l.stale = 0, 0, 0, true
	}
	s.links = links
	return changed
}

// noRect contains no cell.
var noRect = Rect{From: Addr{Col: -1, Row: -1}, To: Addr{Col: -1, Row: -1}}

// emptyLinked clears every region's cells (see emptyRegion).
func (s *Sheet) emptyLinked() []loc {
	var changed []loc
	for _, l := range s.links {
		changed = append(changed, s.emptyRegion(l)...)
	}
	return changed
}

// emptyRegion clears a region's cells, outside the undo history, leaving
// it empty and stale, and returns the cells that changed.
func (s *Sheet) emptyRegion(l *linked) []loc {
	changed := s.clearSpilled(l.area(), noRect)
	l.rows, l.cols, l.data, l.stale = 0, 0, 0, true
	return changed
}

// shiftLinked moves the regions' anchors with inserted or deleted rows
// or columns (at sp, a span of rows when rows is set, mapped by cell); a
// region whose anchor is deleted goes. A region the lines reach is
// emptied first, so cells moving where it was aren't mistaken for its
// own, and read again, whole, at its new place.
func (s *Sheet) shiftLinked(rows bool, sp formula.Span, cell func(Addr) (Addr, bool)) {
	if len(s.links) == 0 {
		return
	}
	s.recordLinks()
	var next []*linked
	for _, l := range s.links {
		end := l.area().To.Col
		if rows {
			end = l.area().To.Row
		}
		if end < sp.At {
			next = append(next, l) // before the lines: untouched
			continue
		}
		for _, c := range s.emptyRegion(l) {
			s.wb.markDirty(c)
		}
		to, ok := cell(l.anchor)
		if !ok {
			continue
		}
		nl := *l
		nl.anchor = to
		next = append(next, &nl)
	}
	s.links = next
}

// copyLinks gives cp, a copy of s, regions of its own reading the same
// sources, to be read again.
func (s *Sheet) copyLinks(cp *Sheet) {
	for _, l := range s.links {
		s.wb.linkSeq++
		cp.links = append(cp.links, &linked{id: s.wb.linkSeq, anchor: l.anchor, src: l.src, liveState: &liveState{stale: true}})
	}
}

// sameLinks reports whether two lists of regions define the same ones.
func sameLinks(a, b []*linked) bool {
	return slices.EqualFunc(a, b, func(x, y *linked) bool { return x.id == y.id && x.anchor == y.anchor && x.src == y.src })
}
