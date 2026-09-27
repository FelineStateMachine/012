package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
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
// users).
func (w *Workbook) recalcAll() {
	start := recalcStart()
	n := 0
	for _, s := range w.sheets {
		s.calc = make(map[Addr]int, len(s.cells))
		for a := range s.cells {
			s.calc[a] = dirty
		}
		n += len(s.calc)
	}
	w.evaluate()
	w.observe(true, start, n)
}

// Recalculation states of a cell.
const (
	dirty = iota + 1
	visiting
	done
)

// recalc recomputes the changed cells, volatile formulas, and everything
// that transitively depends on them, on any sheet.
func (w *Workbook) recalc(changed []loc) {
	start := recalcStart()
	n := w.affected(changed)
	w.evaluate()
	w.observe(false, start, n)
}

// affected marks dirty, in each sheet's calc, the changed cells, volatile
// formulas, and every formula that transitively reads them, on any sheet,
// and returns how many it marked.
func (w *Workbook) affected(changed []loc) int {
	m := marking{w: w, queue: append([]loc(nil), changed...), named: w.namedInUse(), byName: map[*Sheet]bool{}}
	for _, s := range w.sheets {
		s.calc = make(map[Addr]int)
		m.byName[s] = w.crossKeys[formula.SheetKey(s.name)] > 0
		for a := range s.volatile {
			m.queue = append(m.queue, loc{s, a})
		}
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

// marking is the state of affected: the cells still to mark, and what it
// looks up once rather than for every cell.
type marking struct {
	w      *Workbook
	queue  []loc
	named  []namedUsers
	byName map[*Sheet]bool // whether some formula names the sheet
}

// namedUsers are the formulas that use a named range, with its cells.
type namedUsers struct {
	s     *Sheet
	r     Rect
	users map[loc]struct{}
}

// namedInUse returns the defined named ranges that formulas use.
func (w *Workbook) namedInUse() []namedUsers {
	var named []namedUsers
	for k, users := range w.nameUsers {
		if nm, ok := w.names[k]; ok && !nm.Gone() {
			named = append(named, namedUsers{nm.Sheet, nm.Range, users})
		}
	}
	return named
}

// push queues the formula at u unless it's already marked, so each is
// queued once rather than once per changed cell it reads.
func (m *marking) push(u loc) {
	if u.s.calc[u.a] != dirty {
		m.queue = append(m.queue, u)
	}
}

// readersOf queues the formulas that read the cell l: by reference, by a
// range over it, through a named range, or by naming its sheet.
func (m *marking) readersOf(l loc) {
	s, a := l.s, l.a
	for d := range s.dependents[a] {
		m.push(loc{s, d})
	}
	for u := range s.rangeUsers.candidates(a.Col) {
		if s.cells[u].rangeHas(a) {
			m.push(loc{s, u})
		}
	}
	for _, nu := range m.named {
		if nu.s == s && nu.r.Contains(a) {
			for u := range nu.users {
				m.push(u)
			}
		}
	}
	if !m.byName[s] {
		return // no formula names this sheet
	}
	for u := range m.w.crossUsers {
		if m.w.crossReads(u, l) {
			m.push(u)
		}
	}
}

// rangeHas reports whether one of the ranges the formula reads on its own
// sheet contains a.
func (c *Cell) rangeHas(a Addr) bool {
	for _, r := range c.ranges {
		if r.Contains(a) {
			return true
		}
	}
	return false
}

// evaluate computes the cells marked dirty in each sheet's calc. Cells
// are evaluated lazily in dependency order: reading a dirty cell
// evaluates it first. A cell that is reached again while it is still
// being evaluated is part of a cycle and becomes ERR. The state lives on
// the sheets, keyed by address as on one sheet, and each sheet's lookup
// is made once, so evaluating a formula allocates nothing for sheets.
func (w *Workbook) evaluate() {
	w.Circular = false
	var compute func(s *Sheet, a Addr) Value
	compute = func(s *Sheet, a Addr) Value {
		// Every cell a formula reads comes through here, so it looks
		// each map up once: a SUM over 8192 cells makes 8192 calls.
		c := s.cells[a]
		switch st := s.calc[a]; {
		case c == nil:
			return Value{}
		case st == visiting:
			w.Circular = true
			return ErrRef
		case st != dirty:
			return c.Value
		}
		s.calc[a] = visiting
		c.auto = Format{}
		switch {
		case c.Input == "":
			c.Value = Value{}
		case c.expr == nil && c.Format.Kind == FmtText:
			c.Value = Value{Kind: Text, Str: c.Input}
		case c.expr == nil:
			c.Value = Value{Kind: Text, Str: strings.TrimPrefix(c.Input, "'")}
		default:
			expr := s.bound(c)
			c.Value = eval(w.arith(expr), s.calcGet)
			if _, lit := expr.(formula.Num); !lit {
				c.auto = inferFormat(expr, s.calcFmt)
			}
		}
		s.calc[a] = done
		return c.Value
	}
	for _, s := range w.sheets {
		s.version++
		s.hidden.valid = false // values may have changed what the filter hides
		s.calcGet = w.lookupOn(s, compute)
		s.calcFmt = w.formatFrom(s)
	}
	for _, s := range w.sheets {
		for a := range s.calc {
			compute(s, a)
		}
	}
	for _, s := range w.sheets {
		s.calc, s.calcGet, s.calcFmt = nil, nil, nil
	}
}

// crossReads reports whether the formula at u reads the cell l through a
// reference that names l's sheet.
func (w *Workbook) crossReads(u, l loc) bool {
	c := u.s.cells[u.a]
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

// lookupOn resolves references in a formula on s with get: references
// without a sheet read s, others the sheet they name (#REF! when no sheet
// has that name). The last sheet name is remembered, so a range on
// another sheet resolves its name once.
func (w *Workbook) lookupOn(s *Sheet, get func(*Sheet, Addr) Value) lookup {
	var lastName string
	var last *Sheet
	return func(sheet string, a Addr) Value {
		if sheet == "" {
			return get(s, a)
		}
		if sheet != lastName || last == nil {
			lastName, last = sheet, w.byKey[formula.SheetKey(sheet)]
		}
		if last == nil {
			return ErrRef
		}
		return get(last, a)
	}
}

// values reads current values for formulas on s, across sheets.
func (w *Workbook) values(s *Sheet) lookup {
	return w.lookupOn(s, func(t *Sheet, a Addr) Value { return t.Value(a) })
}

// formatFrom reads display formats for formulas on s, across sheets.
func (w *Workbook) formatFrom(s *Sheet) func(string, Addr) Format {
	return func(sheet string, a Addr) Format {
		if t := w.resolve(s, sheet); t != nil {
			return t.DisplayFormat(a)
		}
		return Format{}
	}
}
