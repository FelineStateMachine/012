package sheet

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
)

// Recalculation: a change marks the changed cells and everything that
// transitively reads them dirty, on any sheet, then evaluates the dirty
// cells lazily in dependency order.

// RecalcAll recomputes every formula in the workbook. Loaders call it
// once the sheets are built, so it also starts a fresh undo history:
// loading isn't undoable.
func (s *Sheet) RecalcAll() { s.wb.RecalcAll() }

// RecalcAll recomputes every formula and clears the undo history.
func (w *Workbook) RecalcAll() {
	w.recalcAll()
	w.ClearHistory()
}

// recalcAll recomputes every formula on every sheet, as after loading or
// after a sheet is added, renamed or deleted, when references by name may
// resolve differently. Every cell is dirty, so there is nothing to
// propagate: tracing dependents from each cell cost O(cells x range
// users). Plain cells hold their values already (store.go), so only rich
// ones are marked.
func (w *Workbook) recalcAll() {
	w.gen++
	start := w.recalcStart()
	n := 0
	for _, s := range w.sheets {
		s.calc = make(map[Addr]int, len(s.cells.rich)-len(s.cells.richFree))
		for a := range s.cells.richCells() {
			s.calc[a] = dirty
		}
		n += s.cells.len()
	}
	w.evaluate()
	w.settleSpills(nil)
	w.refreshPivots(w.allPivots())
	w.settleRegions()
	w.observe(true, start, n)
}

// Recalculation states of a cell.
const (
	dirty = iota + 1
	visiting
	done
	deferred // put off: evaluation went too deep, see evaluate.go
)

// recalc recomputes the changed cells, volatile formulas, and everything
// that transitively depends on them, on any sheet.
func (w *Workbook) recalc(changed []loc) { w.recalcFrom(changed, true) }

// recalcFrom recomputes the changed cells and everything that
// transitively depends on them, and the volatile formulas with theirs
// when volatiles is set.
func (w *Workbook) recalcFrom(changed []loc, volatiles bool) {
	w.gen++
	start := w.recalcStart()
	n := w.affected(changed, volatiles)
	stale := w.stalePivots()
	w.evaluate()
	w.refreshPivots(w.settleSpills(stale))
	w.settleRegions()
	w.observe(false, start, n)
}

// affected marks dirty, in each sheet's calc, the changed cells, volatile
// formulas if volatiles is set, and every formula that transitively reads
// them, on any sheet, and returns how many it marked.
func (w *Workbook) affected(changed []loc, volatiles bool) int {
	return w.affectedBy(changed, nil, volatiles)
}

// affectedBy is affected, with the formulas reading the cells of
// readers marked too, but not those cells: spilling changed what an
// anchor shows (spill.go), not its formula.
func (w *Workbook) affectedBy(changed, readers []loc, volatiles bool) int {
	m := marking{readerLookup: w.readerLookup(), queue: append([]loc(nil), changed...)}
	for _, s := range w.sheets {
		s.calc = make(map[Addr]int)
		if !volatiles {
			continue
		}
		for a := range s.volatile {
			m.queue = append(m.queue, loc{s, a})
		}
	}
	for _, l := range readers {
		m.readersOf(l)
	}
	n := 0
	for len(m.queue) > 0 {
		l := m.queue[0]
		m.queue = m.queue[1:]
		if !l.s.live || l.s.calc[l.a] == dirty {
			continue
		}
		l.s.calc[l.a] = dirty
		n++
		m.readersOf(l)
	}
	return n
}

// marking is the state of affected: the cells still to mark.
type marking struct {
	readerLookup
	queue []loc
}

// readerLookup finds the formulas that read a cell, with what it looks
// up once rather than for every cell.
type readerLookup struct {
	w      *Workbook
	named  []namedUsers
	byName map[*Sheet]bool // whether some formula names the sheet
}

func (w *Workbook) readerLookup() readerLookup {
	r := readerLookup{w: w, named: w.namedInUse(), byName: make(map[*Sheet]bool, len(w.sheets))}
	for _, s := range w.sheets {
		r.byName[s] = w.crossKeys[formula.SheetKey(s.name)] > 0
	}
	return r
}

// namedUsers are the formulas that use a named range, with its cells,
// and whether it is a region's table.
type namedUsers struct {
	s      *Sheet
	r      Rect
	users  map[loc]struct{}
	region bool
}

// namedInUse returns the defined named ranges, tables and regions that
// formulas use.
func (w *Workbook) namedInUse() []namedUsers {
	var named []namedUsers
	for k, users := range w.nameUsers {
		if nm, ok := w.names[k]; ok && !nm.Gone() {
			named = append(named, namedUsers{s: nm.Sheet, r: nm.Range, users: users})
		}
	}
	named = append(named, w.tablesInUse()...)
	return append(named, w.regionsInUse()...)
}

// push queues the formula at u unless it's already marked, so each is
// queued once rather than once per changed cell it reads.
func (m *marking) push(u loc) {
	if u.s.calc[u.a] != dirty {
		m.queue = append(m.queue, u)
	}
}

// readersOf queues the formulas that read the cell l.
func (m *marking) readersOf(l loc) { m.each(l, m.push) }

// each calls fn with the formulas that read the cell l: by reference, by
// a range over it, through a named range, or by naming its sheet.
func (r readerLookup) each(l loc, fn func(loc)) {
	s, a := l.s, l.a
	for d := range s.dependents[a] {
		fn(loc{s, d})
	}
	s.rangeUsers.readers(a, func(u Addr) { fn(loc{s, u}) })
	for _, nu := range r.named {
		if nu.s == s && nu.r.Contains(a) {
			for u := range nu.users {
				fn(u)
			}
		}
	}
	if !r.byName[s] {
		return // no formula names this sheet
	}
	for u := range r.w.crossUsers {
		if r.w.crossReads(u, l) {
			fn(u)
		}
	}
}

// crossReads reports whether the formula at u reads the cell l through a
// reference that names l's sheet.
func (w *Workbook) crossReads(u, l loc) bool {
	c := u.s.cells.get(u.a)
	if c == nil {
		return false
	}
	for _, x := range c.xrefs {
		if x.r.Contains(l.a) && w.byKey[x.key] == l.s {
			return true
		}
	}
	return false
}

// reader is the engine's side of how formulas on s read cells: the Book
// the function library reads through (functions.Book). It resolves
// references with read: references without a sheet ("") read s, others
// the sheet they name, with cells on a sheet no sheet has the name of
// reading as #REF!. The last sheet name is remembered, so a range on
// another sheet resolves its name once.
type reader struct {
	w        *Workbook
	s        *Sheet
	read     func(*Sheet, Addr) Value
	memo     *aggMemo // running aggregates shared by a recalculation; nil otherwise
	lastName string
	last     *Sheet
	lib      *functions.Reader // what the function library is given, reading through rd
	dense    bool              // denseReads when lib was made
}

func (w *Workbook) lookupOn(s *Sheet, read func(*Sheet, Addr) Value) *reader {
	rd := &reader{w: w, s: s, read: read, dense: denseReads}
	rd.lib = functions.NewReader(rd, &w.depth, denseReads)
	return rd
}

// recalcReader is the reader for formulas on s in a recalculation, which
// reads through e and shares running aggregates in memo: the one kept
// from the last recalculation, when there is one.
func (w *Workbook) recalcReader(s *Sheet, e *evaluator, memo *aggMemo) *reader {
	rd := s.recalcs
	if rd == nil || rd.w != w || rd.dense != denseReads {
		rd = w.lookupOn(s, nil)
		s.recalcs = rd
	}
	rd.read, rd.memo, rd.lastName, rd.last = e.compute, memo, "", nil
	rd.lib.Forget()
	return rd
}

// sheet is the sheet a reference written with the name sheet points at.
func (rd *reader) sheet(sheet string) *Sheet {
	if sheet == "" {
		return rd.s
	}
	if sheet != rd.lastName || rd.last == nil {
		rd.lastName, rd.last = sheet, rd.w.byKey[formula.SheetKey(sheet)]
	}
	return rd.last
}

// Cell returns the current value of a cell.
func (rd *reader) Cell(sheet string, a Addr) Value {
	t := rd.sheet(sheet)
	if t == nil {
		return ErrRef
	}
	if reg, ok := t.pagedRegion(); ok {
		return rd.pagedCell(t, reg, a)
	}
	return rd.read(t, a)
}

// Scan reads the stored cells of r from the cell from on, row by row, as
// functions.Book describes: blank cells are skipped without being
// visited, so a whole column costs what it holds. It finds the cells
// first, then reads their values, so the chunk is filled without a call
// per cell.
func (rd *reader) Scan(sheet string, r Rect, from Addr, addrs []Addr, vals []Value) int {
	t := rd.sheet(sheet)
	if t == nil {
		return -1
	}
	if reg, ok := t.pagedRegion(); ok {
		return rd.pagedScan(t, reg, r, from, addrs, vals)
	}
	n := fill(t, r, from, addrs)
	if vals == nil {
		return n
	}
	for i, a := range addrs[:n] {
		v := rd.read(t, a)
		vals[i] = v
		if v.Kind == Error {
			return i + 1
		}
	}
	return n
}

// fill writes the stored cells of r on t from the cell from on into dst,
// row by row, until it is full, and returns how many.
func fill(t *Sheet, r Rect, from Addr, dst []Addr) int {
	switch {
	case denseReads:
		return fillDense(r, from, dst)
	case r.From.Col == r.To.Col:
		return t.cells.colFill(r.From.Col, from.Row, r.To.Row, dst)
	}
	return t.cells.rangeFill(r, from, dst)
}

// fillDense writes every address of r from the cell from on into dst,
// row by row, until it is full, as tests ask with denseReads.
func fillDense(r Rect, from Addr, dst []Addr) int {
	n := 0
	for a := from; a.Row <= r.To.Row && n < len(dst); n++ {
		dst[n] = a
		if a.Col++; a.Col > r.To.Col {
			a = Addr{Col: r.From.Col, Row: a.Row + 1}
		}
	}
	return n
}

// Bounds is the smallest range holding every stored cell of r.
func (rd *reader) Bounds(sheet string, r Rect) (b Rect, any, exists bool) {
	t := rd.sheet(sheet)
	if t == nil {
		return Rect{}, false, false
	}
	if reg, ok := t.pagedRegion(); ok {
		b, any = t.pagedBounds(reg, r)
		return b, any, true
	}
	b, any = t.cells.bounds(r)
	return b, any, true
}

// Ask looks up the answer to a JEV function's question in the workbook's
// RemoteSource, noting that the formula being evaluated waits for it
// when it isn't known yet.
func (rd *reader) Ask(call RemoteCall) (RemoteAnswer, Value) {
	w := rd.w
	if w.remote == nil {
		return RemoteAnswer{}, ErrNoRemote
	}
	ans, ok := w.remote.Lookup(call)
	if !ok {
		w.wait(call)
		return RemoteAnswer{}, Pending
	}
	return ans, Value{}
}

// denseReads makes formulas read every address of their ranges, as they
// did before ranges were clipped to what they hold, so tests can check
// that the two agree.
var denseReads bool

// values reads current values for formulas on s, across sheets.
func (w *Workbook) values(s *Sheet) *reader {
	return w.lookupOn(s, func(t *Sheet, a Addr) Value { return t.Value(a) })
}

// formatFrom reads display formats for formulas on s, across sheets: on
// a linked source's tab, the format its column's type shows in.
func (w *Workbook) formatFrom(s *Sheet) func(string, Addr) Format {
	return func(sheet string, a Addr) Format {
		t := w.resolve(s, sheet)
		if t == nil {
			return Format{}
		}
		if r, ok := t.pagedRegion(); ok {
			return t.sourceFormat(r, a)
		}
		return t.DisplayFormat(a)
	}
}
