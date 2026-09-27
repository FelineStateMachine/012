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
	refs     []Addr   // single-cell references in expr
	ranges   []Rect   // range references in expr
	names    []string // names in expr, as keys of Sheet.names
	volatile bool     // expr calls TODAY, NOW, RAND...
}

// IsFormula reports whether the cell holds a formula.
func (c *Cell) IsFormula() bool {
	return c.expr != nil && IsFormulaEntry(c.Input)
}

// Blank reports whether the cell has no contents (it may still have
// formatting).
func (c *Cell) Blank() bool { return c == nil || c.Input == "" }

// Sheet is a sparse worksheet.
type Sheet struct {
	cells  map[Addr]*Cell
	widths map[int]int

	// dependents maps a cell to the formula cells that reference it
	// directly. Range references are kept on the formula cell itself and
	// found through rangeUsers, by column, so a huge range doesn't create
	// millions of edges.
	dependents map[Addr]map[Addr]struct{}
	rangeUsers rangeIndex
	volatile   map[Addr]struct{} // formulas recalculated on every change

	// names are the named ranges by upper-case name (see names.go), and
	// nameUsers the formula cells that mention each name, defined or not,
	// so defining a name recalculates the formulas waiting for it.
	names     map[string]Name
	nameUsers map[string]map[Addr]struct{}

	// Circular is set when the last recalculation found a cycle.
	Circular bool

	// version counts changes to cells and their values, so what is derived
	// from them can be cached; see RangeStats.
	version uint64
	stats   statsCache

	charts []Chart // floating charts, bottom first; see chart.go

	hist history // undo and redo, see history.go

	view   viewState   // frozen panes and the filter, see view.go
	hidden hiddenCache // rows the filter hides, see filter.go
}

// New returns an empty worksheet.
func New() *Sheet {
	return &Sheet{
		cells:      make(map[Addr]*Cell),
		widths:     make(map[int]int),
		dependents: make(map[Addr]map[Addr]struct{}),
		volatile:   make(map[Addr]struct{}),
		names:      make(map[string]Name),
		nameUsers:  make(map[string]map[Addr]struct{}),
	}
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
	c.expr, c.refs, c.ranges, c.names, c.volatile = n, nil, nil, nil, false
	if n != nil {
		walkRefs(n,
			func(r Addr) { c.refs = append(c.refs, r) },
			func(r Rect) { c.ranges = append(c.ranges, r) })
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
	for _, k := range c.names {
		if s.nameUsers[k] == nil {
			s.nameUsers[k] = make(map[Addr]struct{})
		}
		s.nameUsers[k][a] = struct{}{}
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
	for _, k := range old.names {
		delete(s.nameUsers[k], a)
		if len(s.nameUsers[k]) == 0 {
			delete(s.nameUsers, k)
		}
	}
	s.rangeUsers.remove(a, old.ranges)
	delete(s.volatile, a)
	delete(s.cells, a)
}

// RecalcAll recomputes every formula. Loaders call it once the sheet is
// built, so it also starts a fresh undo history: loading isn't undoable.
func (s *Sheet) RecalcAll() {
	// Every cell is dirty, so there is nothing to propagate: tracing
	// dependents from each cell cost O(cells x range users).
	start := recalcStart()
	state := make(map[Addr]int, len(s.cells))
	for a := range s.cells {
		state[a] = dirty
	}
	s.evaluate(state)
	s.observe(true, start, len(state))
	s.ClearHistory()
}

// Recalculation states of a cell.
const (
	dirty = iota + 1
	visiting
	done
)

// recalc recomputes the changed cells, volatile formulas, and everything
// that transitively depends on them.
func (s *Sheet) recalc(changed []Addr) {
	start := recalcStart()
	state := s.affected(changed)
	s.evaluate(state)
	s.observe(false, start, len(state))
}

// affected marks dirty the changed cells, volatile formulas, and every
// formula that transitively reads them.
func (s *Sheet) affected(changed []Addr) map[Addr]int {
	state := make(map[Addr]int)
	queue := append([]Addr(nil), changed...)
	for a := range s.volatile {
		queue = append(queue, a)
	}
	// The named ranges in use, looked up once rather than for every cell.
	type namedUsers struct {
		r     Rect
		users map[Addr]struct{}
	}
	var named []namedUsers
	for k, users := range s.nameUsers {
		if nm, ok := s.names[k]; ok && !nm.Lost {
			named = append(named, namedUsers{nm.Range, users})
		}
	}
	// Queue each formula once, not once per changed cell it reads.
	push := func(u Addr) {
		if state[u] != dirty {
			queue = append(queue, u)
		}
	}
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		if state[a] == dirty {
			continue
		}
		state[a] = dirty
		for d := range s.dependents[a] {
			push(d)
		}
		for u := range s.rangeUsers.candidates(a.Col) {
			for _, r := range s.cells[u].ranges {
				if r.Contains(a) {
					push(u)
					break
				}
			}
		}
		for _, n := range named {
			if n.r.Contains(a) {
				for u := range n.users {
					push(u)
				}
			}
		}
	}
	return state
}

// evaluate computes the cells marked dirty in state. Cells are evaluated
// lazily in dependency order: reading a dirty cell evaluates it first. A
// cell that is reached again while it is still being evaluated is part of
// a cycle and becomes ERR.
func (s *Sheet) evaluate(state map[Addr]int) {
	s.version++
	s.Circular = false
	s.hidden.valid = false // values may have changed what the filter hides
	var compute func(Addr) Value
	compute = func(a Addr) Value {
		// Every cell a formula reads comes through here, so it looks
		// each map up once: a SUM over 8192 cells makes 8192 calls.
		c := s.cells[a]
		switch st := state[a]; {
		case c == nil:
			return Value{}
		case st == visiting:
			s.Circular = true
			return ErrRef
		case st != dirty:
			return c.Value
		}
		state[a] = visiting
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
			c.Value = eval(expr, compute)
			if _, lit := expr.(numLit); !lit {
				c.auto = inferFormat(expr, s.DisplayFormat)
			}
		}
		state[a] = done
		return c.Value
	}
	for a := range state {
		compute(a)
	}
}
