// Package sheet is the spreadsheet engine: cell storage, formula parsing and
// dependency-ordered recalculation. It has no knowledge of the terminal UI.
package sheet

import (
	"maps"
	"sort"
	"strings"
	"unicode"
)

// DefaultWidth is the initial column width: nine characters plus padding.
const DefaultWidth = 10

// Cell holds what the user typed, what it evaluates to, and how it is
// formatted. A cell may have formatting but no contents (Sheets lets you
// format blank cells before typing); such a cell counts as blank
// everywhere contents matter.
type Cell struct {
	// Input is the entry exactly as typed: text, a number such as "$1,200"
	// or "12%", or a formula starting with "=". Empty for a blank cell
	// that only has formatting.
	Input string
	Value Value

	// Format and Style are plain values, so copying a Cell copies its
	// formatting. They survive clearing the contents, as in Sheets.
	Format Format
	Style  Style

	auto     Format   // format inferred from a formula, shown when Format is Automatic
	expr     Node     // nil for text
	refs     []Addr   // single-cell references in expr to its own sheet
	ranges   []Rect   // range references in expr to its own sheet
	xrefs    []xref   // references that name a sheet, e.g. Sheet2!A1
	names    []string // names in expr, as keys of Workbook.names
	volatile bool     // expr calls TODAY, NOW, RAND...
}

// IsFormula reports whether the cell holds a formula.
func (c *Cell) IsFormula() bool {
	return c.expr != nil && IsFormulaEntry(c.Input)
}

// Blank reports whether the cell has no contents (it may still have
// formatting).
func (c *Cell) Blank() bool { return c == nil || c.Input == "" }

// Sheet is a sparse worksheet, one of a Workbook's sheets.
type Sheet struct {
	wb   *Workbook
	name string
	live bool // in the workbook's list; false once deleted

	cells  map[Addr]*Cell
	widths map[int]int

	// dependents maps a cell to the formula cells that reference it
	// directly. Range references are kept on the formula cell itself and
	// found through rangeUsers, by column, so a huge range doesn't create
	// millions of edges.
	dependents map[Addr]map[Addr]struct{}
	rangeUsers rangeIndex
	volatile   map[Addr]struct{} // formulas recalculated on every change

	// version counts changes to cells and their values, so what is derived
	// from them can be cached; see RangeStats.
	version uint64
	stats   statsCache

	charts []Chart // floating charts, bottom first; see chart.go

	view   viewState   // frozen panes and the filter, see view.go
	hidden hiddenCache // rows the filter hides, see filter.go
}

// New returns an empty worksheet, the only sheet of a new workbook.
func New() *Sheet {
	return NewBook().Sheet(0)
}

// Cell returns the cell at a, or nil if it has neither contents nor
// formatting. Use Blank to test for contents.
func (s *Sheet) Cell(a Addr) *Cell { return s.cells[a] }

// Value returns the computed value at a.
func (s *Sheet) Value(a Addr) Value {
	if c := s.cells[a]; c != nil {
		return c.Value
	}
	return Value{}
}

// Len returns the number of non-blank cells.
func (s *Sheet) Len() int {
	n := 0
	for _, c := range s.cells {
		if !c.Blank() {
			n++
		}
	}
	return n
}

// Addrs returns every non-blank cell in row-major order.
func (s *Sheet) Addrs() []Addr {
	out := make([]Addr, 0, len(s.cells))
	for a, c := range s.cells {
		if !c.Blank() {
			out = append(out, a)
		}
	}
	sortAddrs(out)
	return out
}

func sortAddrs(out []Addr) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Row != out[j].Row {
			return out[i].Row < out[j].Row
		}
		return out[i].Col < out[j].Col
	})
}

// ColWidth returns the display width of column c.
func (s *Sheet) ColWidth(c int) int {
	if w, ok := s.widths[c]; ok {
		return w
	}
	return DefaultWidth
}

// SetColWidth sets column c's width; w <= 0 resets it to the default.
func (s *Sheet) SetColWidth(c, w int) {
	s.change("column width", colRect(c, c), func() { s.setWidth(c, w) })
}

func (s *Sheet) setWidth(c, w int) {
	s.recordWidth(c)
	if w <= 0 || w == DefaultWidth { // keep the map to non-default widths
		delete(s.widths, c)
		return
	}
	s.widths[c] = min(w, 240)
}

// Widths returns the columns that have a non-default width.
func (s *Sheet) Widths() map[int]int {
	return maps.Clone(s.widths)
}

// IsFormulaEntry reports whether input is written as a formula: it starts
// with "=", or with "+" or "-" followed by something that isn't a plain
// number (as Sheets accepts "+A1").
func IsFormulaEntry(input string) bool {
	if strings.HasPrefix(input, "=") {
		return true
	}
	if !strings.HasPrefix(input, "+") && !strings.HasPrefix(input, "-") {
		return false
	}
	if _, _, isNum := ParseValue(input); isNum {
		return false
	}
	_, err := Parse(input)
	return err == nil
}

// classify turns an entry into an expression, or nil for text, and
// returns the format the entry implies (Currency for "$5", Date for
// "9/26/2026"). Entries starting with ' are always text.
func classify(input string) (Node, Format, error) {
	if strings.HasPrefix(input, "'") {
		return nil, Format{}, nil
	}
	if v, f, ok := ParseValue(input); ok {
		return numLit{v}, f, nil
	}
	switch strings.ToUpper(input) {
	case "TRUE":
		return boolLit{true}, Format{}, nil
	case "FALSE":
		return boolLit{false}, Format{}, nil
	}
	if IsFormulaEntry(input) {
		n, err := Parse(input)
		return n, Format{}, err
	}
	return nil, Format{}, nil
}

// Set stores an entry at a and recalculates affected cells. An empty input
// erases the contents but keeps the cell's formatting. A formula that
// fails to parse is rejected with a *ParseError and the sheet is left
// unchanged.
func (s *Sheet) Set(a Addr, input string) error {
	var err error
	s.change("edit "+a.String(), Rect{a, a}, func() { err = s.put(a, input) })
	return err
}

// put stores an entry without recalculating, keeping the cell's
// formatting.
func (s *Sheet) put(a Addr, input string) error {
	var f Format
	var st Style
	if old := s.cells[a]; old != nil {
		f, st = old.Format, old.Style
	}
	c, err := newCell(input, f, st, true)
	if err != nil {
		return err
	}
	s.place(a, c)
	return nil
}

// newCell builds a cell for an entry with formatting, or returns nil when
// there is neither. With implied, an entry that implies a format, such as
// "$5" or a date, sets it, as in Sheets. In a Plain text cell every entry
// is text. Control characters are dropped so a worksheet file can't
// smuggle escape sequences to the terminal.
func newCell(input string, f Format, st Style, implied bool) (*Cell, error) {
	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, input)
	if input == "" {
		return formattingOnly(f, st), nil
	}
	var n Node
	if f.Kind != FmtText {
		var fi Format
		var err error
		if n, fi, err = classify(input); err != nil {
			return nil, err
		}
		if implied && !fi.IsZero() {
			f = fi
		}
	}
	c := &Cell{Input: input, Format: f, Style: st}
	c.setExpr(n)
	return c, nil
}

// formattingOnly is a blank cell holding formatting, or nil when there is
// none.
func formattingOnly(f Format, st Style) *Cell {
	if f.IsZero() && st.IsZero() {
		return nil
	}
	return &Cell{Format: f, Style: st}
}

// setExpr sets c's expression and the references indexed from it.
func (c *Cell) setExpr(n Node) {
	c.expr, c.refs, c.ranges, c.xrefs, c.names, c.volatile = n, nil, nil, nil, nil, false
	if n != nil {
		walkRefs(n,
			func(sheet string, r Addr) {
				if sheet != "" {
					c.xrefs = append(c.xrefs, xref{sheetKey(sheet), Rect{r, r}})
					return
				}
				c.refs = append(c.refs, r)
			},
			func(sheet string, r Rect) {
				if sheet != "" {
					c.xrefs = append(c.xrefs, xref{sheetKey(sheet), r})
					return
				}
				c.ranges = append(c.ranges, r)
			})
		walkNames(n, func(nn nameNode) { c.names = append(c.names, nameKey(nn.name)) })
		c.volatile = isVolatile(n)
	}
}

// place stores c at a (nil blanks it), recording the old cell for undo and
// keeping the dependency indexes current. Every cell mutation goes through
// here.
func (s *Sheet) place(a Addr, c *Cell) {
	s.version++
	s.record(a)
	s.unlink(a)
	if c == nil {
		return
	}
	s.cells[a] = c
	for _, r := range c.refs {
		if s.dependents[r] == nil {
			s.dependents[r] = make(map[Addr]struct{})
		}
		s.dependents[r][a] = struct{}{}
	}
	s.rangeUsers.add(a, c.ranges)
	if c.volatile {
		s.volatile[a] = struct{}{}
	}
	if s.live {
		s.wb.index(loc{s, a}, c)
	}
}

// EraseRange clears the contents of every cell in r, keeping their
// formatting as Sheets' Delete does.
func (s *Sheet) EraseRange(r Rect) {
	s.change("clear "+r.String(), r, func() {
		for _, a := range s.cellsIn(r) {
			if c := s.cells[a]; !c.Blank() {
				s.place(a, formattingOnly(c.Format, c.Style))
			}
		}
	})
}

// cellsIn returns the cells in r that have contents or formatting.
func (s *Sheet) cellsIn(r Rect) []Addr {
	var out []Addr
	for a := range s.cells {
		if r.Contains(a) {
			out = append(out, a)
		}
	}
	return out
}

// unlink removes the cell at a and its dependency edges.
func (s *Sheet) unlink(a Addr) {
	old := s.cells[a]
	if old == nil {
		return
	}
	for _, r := range old.refs {
		delete(s.dependents[r], a)
		if len(s.dependents[r]) == 0 {
			delete(s.dependents, r)
		}
	}
	if s.live {
		s.wb.unindex(loc{s, a}, old)
	}
	s.rangeUsers.remove(a, old.ranges)
	delete(s.volatile, a)
	delete(s.cells, a)
}

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
		n += len(s.cells)
	}
	state := make(map[loc]int, n)
	for _, s := range w.sheets {
		for a := range s.cells {
			state[loc{s, a}] = dirty
		}
	}
	w.evaluate(state)
	w.observe(true, start, len(state))
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
	state := w.affected(changed)
	w.evaluate(state)
	w.observe(false, start, len(state))
}

// affected marks dirty the changed cells, volatile formulas, and every
// formula that transitively reads them, on any sheet.
func (w *Workbook) affected(changed []loc) map[loc]int {
	state := make(map[loc]int)
	queue := append([]loc(nil), changed...)
	for _, s := range w.sheets {
		for a := range s.volatile {
			queue = append(queue, loc{s, a})
		}
	}
	// The named ranges in use, looked up once rather than for every cell.
	type namedUsers struct {
		s     *Sheet
		r     Rect
		users map[loc]struct{}
	}
	var named []namedUsers
	for k, users := range w.nameUsers {
		if nm, ok := w.names[k]; ok && !nm.Gone() {
			named = append(named, namedUsers{nm.Sheet, nm.Range, users})
		}
	}
	// Queue each formula once, not once per changed cell it reads.
	push := func(u loc) {
		if state[u] != dirty {
			queue = append(queue, u)
		}
	}
	for len(queue) > 0 {
		l := queue[0]
		queue = queue[1:]
		if state[l] == dirty || !l.s.live {
			continue
		}
		state[l] = dirty
		s, a := l.s, l.a
		for d := range s.dependents[a] {
			push(loc{s, d})
		}
		for u := range s.rangeUsers.candidates(a.Col) {
			for _, r := range s.cells[u].ranges {
				if r.Contains(a) {
					push(loc{s, u})
					break
				}
			}
		}
		for _, n := range named {
			if n.s == s && n.r.Contains(a) {
				for u := range n.users {
					push(u)
				}
			}
		}
		if w.crossKeys[sheetKey(s.name)] == 0 {
			continue // no formula names this sheet
		}
		for u := range w.crossUsers {
			if w.crossReads(u, l) {
				push(u)
			}
		}
	}
	return state
}

// evaluate computes the cells marked dirty in state. Cells are evaluated
// lazily in dependency order: reading a dirty cell evaluates it first. A
// cell that is reached again while it is still being evaluated is part of
// a cycle and becomes ERR.
func (w *Workbook) evaluate(state map[loc]int) {
	w.Circular = false
	for _, s := range w.sheets {
		s.version++
		s.hidden.valid = false // values may have changed what the filter hides
	}
	var compute func(loc) Value
	compute = func(l loc) Value {
		// Every cell a formula reads comes through here, so it looks
		// each map up once: a SUM over 8192 cells makes 8192 calls.
		c := l.s.cells[l.a]
		switch st := state[l]; {
		case c == nil:
			return Value{}
		case st == visiting:
			w.Circular = true
			return ErrRef
		case st != dirty:
			return c.Value
		}
		state[l] = visiting
		c.auto = Format{}
		switch {
		case c.Input == "":
			c.Value = Value{}
		case c.expr == nil && c.Format.Kind == FmtText:
			c.Value = Value{Kind: Text, Str: c.Input}
		case c.expr == nil:
			c.Value = Value{Kind: Text, Str: strings.TrimPrefix(c.Input, "'")}
		default:
			expr := w.arith(l.s.bound(c))
			// One small closure per formula, to resolve sheet names.
			c.Value = eval(expr, w.lookupFrom(l.s, compute))
			if _, lit := expr.(numLit); !lit {
				c.auto = inferFormat(expr, w.formatFrom(l.s))
			}
		}
		state[l] = done
		return c.Value
	}
	for l := range state {
		compute(l)
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

// lookupFrom resolves references in a formula on s with get: references
// without a sheet read s, others the sheet they name (#REF! when no sheet
// has that name). The last sheet name is remembered, so a range on
// another sheet resolves its name once.
func (w *Workbook) lookupFrom(s *Sheet, get func(loc) Value) lookup {
	var lastName string
	var last *Sheet
	return func(sheet string, a Addr) Value {
		if sheet == "" {
			return get(loc{s, a})
		}
		if sheet != lastName || last == nil {
			lastName, last = sheet, w.byKey[sheetKey(sheet)]
		}
		if last == nil {
			return ErrRef
		}
		return get(loc{last, a})
	}
}

// values reads current values for formulas on s, across sheets.
func (w *Workbook) values(s *Sheet) lookup {
	return w.lookupFrom(s, func(l loc) Value { return l.s.Value(l.a) })
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
