package sheet

import "slices"

// Authors: a workbook several people edit at once (012 serve's shared
// rooms) is still changed through the one mutation path, Batch and the
// history's steps, in the one order the server runs them in. Each step
// records its author (SetAuthor names who is acting) and a sequence
// number, and the steps made, undone and redone are a stream of
// operations (Op) the room sends every participant.
//
// With the history shared (ShareHistory), Undo and Redo take back only
// the author's own steps: Undo reverts the author's latest step even
// when others' steps came after it, as long as none of those changed
// what it changed (the same cells, widths, heights, line formats or
// names; the same sheet's charts, pivot, rules, regions or view; the
// sheet list, the settings or the macros), and none of them, nor it,
// inserted or deleted rows or columns on a sheet the other changed. The
// step then leaves the middle of the history, and the later steps stay
// as they are, their before-images still true: they touched none of
// what it restores. Otherwise Undo refuses, and UndoBlocked says whose
// step is in the way. Redo takes back the author's latest undo on the
// same terms, against the steps made since. Formulas that read what an
// undone step changed recalculate; entries typed after it that don't
// overlap it stay as typed.

// OpKind is what an operation did to the history.
type OpKind int

const (
	OpDo   OpKind = iota // a step made
	OpUndo               // a step undone
	OpRedo               // a step redone
)

// Op is one operation in the order the workbook ran it: a step made,
// undone or redone, by its author, with what the step says of itself.
type Op struct {
	Seq    uint64 // its place in the order, from 1
	Author int
	Kind   OpKind
	Change
}

// opLogSize is how many of the latest operations the workbook keeps for
// OpsSince.
const opLogSize = 256

// Blocked is a later step that keeps an author's step from being
// undone or redone: whose it is, and what it did.
type Blocked struct {
	Author int
	Label  string
}

// SetAuthor names who makes the steps that follow; 0 is nobody in
// particular, as in a workbook only one person edits.
func (w *Workbook) SetAuthor(a int) { w.author = a }

// Author is who makes the steps that follow (SetAuthor).
func (w *Workbook) Author() int { return w.author }

// ShareHistory has undo and redo take back only each author's own
// steps, from now on.
func (w *Workbook) ShareHistory() { w.hist.shared = true }

// Version changes whenever the workbook changes: a step made, undone or
// redone, a recalculation, rows arriving at a region.
func (w *Workbook) Version() uint64 { return w.gen + w.hist.seq }

// OpsSince is the operations after seq, the last opLogSize at most, and
// the sequence number of the latest.
func (w *Workbook) OpsSince(seq uint64) ([]Op, uint64) {
	h := &w.hist
	i, _ := slices.BinarySearchFunc(h.log, seq+1, func(o Op, s uint64) int { return int(o.Seq) - int(s) })
	return slices.Clone(h.log[i:]), h.seq
}

// Checkpoint marks where the history stands, for UndoTo.
func (w *Workbook) Checkpoint() int { return int(w.hist.seq) }

// UndoTo undoes the current author's steps made after checkpoint c, as
// cancelling a dialog does, whatever others changed meanwhile; it stops
// at a step it can't undo.
func (w *Workbook) UndoTo(c int) {
	h := &w.hist
	for i := h.mine(w.author); i >= 0 && h.undo[i].seq > uint64(c); i = h.mine(w.author) {
		if _, ok := w.Undo(); !ok {
			return
		}
	}
}

// LastOp is the latest operation's sequence number, 0 before any.
func (w *Workbook) LastOp() uint64 { return w.hist.seq }

// EndStep closes the step left open (Begin) by a participant who is
// gone, as if they had ended it: what it did stays, one step.
func (w *Workbook) EndStep() {
	for w.hist.depth > 0 {
		w.finish()
	}
}

// InStep reports whether a step is open (Begin, a macro running), and
// whose it is.
func (w *Workbook) InStep() (bool, int) {
	if w.hist.open == nil {
		return false, 0
	}
	return true, w.hist.open.author
}

// logOp notes an operation in the order.
func (w *Workbook) logOp(kind OpKind, author int, c Change) uint64 {
	h := &w.hist
	h.seq++
	h.log = append(h.log, Op{Seq: h.seq, Author: author, Kind: kind, Change: c})
	if len(h.log) > opLogSize {
		h.log = slices.Delete(h.log, 0, len(h.log)-opLogSize)
	}
	return h.seq
}

// mine is the index in the undo stack of the author's latest step, or
// -1; with the history unshared, the top's.
func (h *history) mine(author int) int {
	if !h.shared {
		return len(h.undo) - 1
	}
	for i := len(h.undo) - 1; i >= 0; i-- {
		if h.undo[i].author == author {
			return i
		}
	}
	return -1
}

// mineRedo is the index in the redo stack of the author's latest undo.
func (h *history) mineRedo(author int) int {
	if !h.shared {
		return len(h.redo) - 1
	}
	for i := len(h.redo) - 1; i >= 0; i-- {
		if h.redo[i].author == author {
			return i
		}
	}
	return -1
}

// UndoBlocked reports whether the current author's latest step can't be
// undone, and the first later step in the way.
func (w *Workbook) UndoBlocked() (Blocked, bool) {
	h := &w.hist
	i := h.mine(w.author)
	if i < 0 {
		return Blocked{}, false
	}
	return h.blocking(h.undo[i], h.undo[i+1:], 0)
}

// RedoBlocked reports whether the current author's latest undo can't be
// redone, and the first step made since that's in the way.
func (w *Workbook) RedoBlocked() (Blocked, bool) {
	h := &w.hist
	i := h.mineRedo(w.author)
	if i < 0 {
		return Blocked{}, false
	}
	return h.blocking(h.redo[i], h.undo, h.redo[i].seq)
}

// blocking is the first of later, made after seq by another author,
// that overlaps st. The author's own steps after one of theirs are
// redos of their own undos, which came back in order; with the history
// unshared nothing blocks.
func (h *history) blocking(st *step, later []*step, seq uint64) (Blocked, bool) {
	if !h.shared {
		return Blocked{}, false
	}
	for _, o := range later {
		if o.seq > seq && o.author != st.author && st.overlaps(o) {
			return Blocked{Author: o.author, Label: o.label}, true
		}
	}
	return Blocked{}, false
}

// takeUndo removes the author's latest step from the undo stack, when
// nothing later is in the way, giving the steps after it new IDs: the
// states they lead to are new.
func (h *history) takeUndo(author int) *step {
	i := h.mine(author)
	if i < 0 {
		return nil
	}
	if _, blocked := h.blocking(h.undo[i], h.undo[i+1:], 0); blocked {
		return nil
	}
	st := h.undo[i]
	h.undo = slices.Delete(h.undo, i, i+1)
	h.bytes -= st.bytes
	for _, later := range h.undo[i:] {
		h.lastID++
		later.id = h.lastID
	}
	return st
}

// takeRedo removes the author's latest undo from the redo stack, when
// no step made since is in the way.
func (h *history) takeRedo(author int) *step {
	i := h.mineRedo(author)
	if i < 0 {
		return nil
	}
	if _, blocked := h.blocking(h.redo[i], h.undo, h.redo[i].seq); blocked {
		return nil
	}
	st := h.redo[i]
	h.redo = slices.Delete(h.redo, i, i+1)
	return st
}

// dropRedo forgets the author's undos, which a new step of theirs ends,
// or everyone's when the history isn't shared.
func (h *history) dropRedo(author int) {
	if !h.shared {
		h.redo = nil
		return
	}
	h.redo = slices.DeleteFunc(h.redo, func(st *step) bool { return st.author == author })
}

// recordShift notes that the open step moves the sheet's cells by
// inserting or deleting lines, which conflicts with any other step on
// the sheet.
func (s *Sheet) recordShift() {
	if st := s.wb.hist.open; st != nil {
		if st.shifts == nil {
			st.shifts = map[*Sheet]bool{}
		}
		st.shifts[s] = true
	}
}

// overlaps reports whether st and o change any of the same things, so
// one can't be undone past the other (see Authors above).
func (st *step) overlaps(o *step) bool {
	if st.sheets != nil || o.sheets != nil || st.settings != nil && o.settings != nil || st.macros != nil && o.macros != nil {
		return true
	}
	for s := range st.shifts {
		if o.touches(s) {
			return true
		}
	}
	for s := range o.shifts {
		if st.touches(s) {
			return true
		}
	}
	return st.sameCells(o) || sharesKey(st.widths, o.widths) || sharesKey(st.heights, o.heights) ||
		sharesKey(st.lines, o.lines) || sharesKey(st.names, o.names) || sharesKey(st.views, o.views) ||
		sharesKey(st.charts, o.charts) || sharesKey(st.pivots, o.pivots) || sharesKey(st.rules, o.rules) ||
		sharesKey(st.regions, o.regions)
}

// sameCells reports whether the steps changed any of the same cells.
func (st *step) sameCells(o *step) bool {
	for s, img := range st.cells {
		other := o.cells[s]
		if other == nil {
			continue
		}
		if img.len() > other.len() {
			img, other = other, img
		}
		if img.anyIn(other) {
			return true
		}
	}
	return false
}

// touches reports whether the step changed anything of sheet s.
func (st *step) touches(s *Sheet) bool {
	if st.cells[s] != nil || st.views[s] != nil || st.shifts[s] {
		return true
	}
	for _, m := range []map[*Sheet]bool{keysOf(st.charts), keysOf(st.pivots), keysOf(st.rules), keysOf(st.regions)} {
		if m[s] {
			return true
		}
	}
	for k := range st.widths {
		if k.s == s {
			return true
		}
	}
	for k := range st.heights {
		if k.s == s {
			return true
		}
	}
	for k := range st.lines {
		if k.s == s {
			return true
		}
	}
	return false
}

// keysOf is a map's sheets as a set.
func keysOf[V any](m map[*Sheet]V) map[*Sheet]bool {
	out := make(map[*Sheet]bool, len(m))
	for s := range m {
		out[s] = true
	}
	return out
}

// sharesKey reports whether two maps have a key in common.
func sharesKey[K comparable, V1, V2 any](a map[K]V1, b map[K]V2) bool {
	if len(a) > len(b) {
		for k := range b {
			if _, ok := a[k]; ok {
				return true
			}
		}
		return false
	}
	for k := range a {
		if _, ok := b[k]; ok {
			return true
		}
	}
	return false
}

// anyIn reports whether img holds a before-image of any cell other
// holds one of.
func (img *image) anyIn(other *image) bool {
	found := false
	img.addrs(func(a Addr) bool {
		found = other.seen(a)
		return !found
	})
	return found
}

// addrs calls fn with the address of every cell the image holds, until
// fn returns false.
func (img *image) addrs(fn func(Addr) bool) {
	for _, ic := range img.small {
		if !fn(ic.a) {
			return
		}
	}
	st := img.tables()
	more := true
	for _, occ := range []*occupancy{&st.stored, &img.blank} {
		for _, col := range occ.colIDs {
			occ.colScan(col, 0, MaxRows-1, func(row int) bool {
				more = fn(Addr{Col: col, Row: row})
				return more
			})
			if !more {
				return
			}
		}
	}
}
