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

// Cell holds what the user typed and what it evaluates to.
type Cell struct {
	// Input is the entry exactly as typed: text, a number such as "$1,200"
	// or "12%", or a formula starting with "=".
	Input string
	Value Value

	expr   Node   // nil for text
	refs   []Addr // single-cell references in expr
	ranges []Rect // range references in expr
}

// IsFormula reports whether the cell holds a formula.
func (c *Cell) IsFormula() bool {
	return c.expr != nil && IsFormulaEntry(c.Input)
}

// Sheet is a sparse worksheet.
type Sheet struct {
	cells  map[Addr]*Cell
	widths map[int]int

	// dependents maps a cell to the formula cells that reference it
	// directly. Range references are kept on the formula cell itself and
	// scanned separately, so a huge range doesn't create millions of edges.
	dependents map[Addr]map[Addr]struct{}
	rangeUsers map[Addr]struct{}

	// Circular is set when the last recalculation found a cycle.
	Circular bool

	hist history // undo and redo, see history.go
}

// New returns an empty worksheet.
func New() *Sheet {
	return &Sheet{
		cells:      make(map[Addr]*Cell),
		widths:     make(map[int]int),
		dependents: make(map[Addr]map[Addr]struct{}),
		rangeUsers: make(map[Addr]struct{}),
	}
}

// Cell returns the cell at a, or nil if it is blank.
func (s *Sheet) Cell(a Addr) *Cell { return s.cells[a] }

// Value returns the computed value at a.
func (s *Sheet) Value(a Addr) Value {
	if c := s.cells[a]; c != nil {
		return c.Value
	}
	return Value{}
}

// Len returns the number of non-blank cells.
func (s *Sheet) Len() int { return len(s.cells) }

// Addrs returns every non-blank cell in row-major order.
func (s *Sheet) Addrs() []Addr {
	out := make([]Addr, 0, len(s.cells))
	for a := range s.cells {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Row != out[j].Row {
			return out[i].Row < out[j].Row
		}
		return out[i].Col < out[j].Col
	})
	return out
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
	s.change(colRect(c, c), func() { s.setWidth(c, w) })
}

func (s *Sheet) setWidth(c, w int) {
	s.recordWidth(c)
	if w <= 0 {
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
	if _, isNum := ParseNumber(input); isNum {
		return false
	}
	_, err := Parse(input)
	return err == nil
}

// classify turns an entry into an expression, or nil for text. Entries
// starting with ' are always text.
func classify(input string) (Node, error) {
	if strings.HasPrefix(input, "'") {
		return nil, nil
	}
	if v, ok := ParseNumber(input); ok {
		return numLit{v}, nil
	}
	switch strings.ToUpper(input) {
	case "TRUE":
		return boolLit{true}, nil
	case "FALSE":
		return boolLit{false}, nil
	}
	if IsFormulaEntry(input) {
		return Parse(input)
	}
	return nil, nil
}

// Set stores an entry at a and recalculates affected cells. An empty input
// erases the cell. A formula that fails to parse is rejected with a
// *ParseError and the sheet is left unchanged.
func (s *Sheet) Set(a Addr, input string) error {
	var err error
	s.change(Rect{a, a}, func() { err = s.put(a, input) })
	return err
}

// put stores an entry without recalculating. Control characters are
// dropped so a worksheet file can't smuggle escape sequences to the terminal.
func (s *Sheet) put(a Addr, input string) error {
	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, input)
	if input == "" {
		s.place(a, nil)
		return nil
	}
	n, err := classify(input)
	if err != nil {
		return err
	}
	c := &Cell{Input: input}
	c.setExpr(n)
	s.place(a, c)
	return nil
}

// setExpr sets c's expression and the references indexed from it.
func (c *Cell) setExpr(n Node) {
	c.expr, c.refs, c.ranges = n, nil, nil
	if n != nil {
		walkRefs(n,
			func(r Addr) { c.refs = append(c.refs, r) },
			func(r Rect) { c.ranges = append(c.ranges, r) })
	}
}

// place stores c at a (nil blanks it), recording the old cell for undo and
// keeping the dependency indexes current. Every cell mutation goes through
// here.
func (s *Sheet) place(a Addr, c *Cell) {
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
	if len(c.ranges) > 0 {
		s.rangeUsers[a] = struct{}{}
	}
}

// EraseRange blanks every cell in r.
func (s *Sheet) EraseRange(r Rect) {
	s.change(r, func() {
		for _, a := range s.cellsIn(r) {
			s.place(a, nil)
		}
	})
}

// cellsIn returns the non-blank cells in r.
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
	delete(s.rangeUsers, a)
	delete(s.cells, a)
}

// RecalcAll recomputes every formula. Loaders call it once the sheet is
// built, so it also starts a fresh undo history: loading isn't undoable.
func (s *Sheet) RecalcAll() {
	s.recalc(s.Addrs())
	s.ClearHistory()
}

// recalc recomputes the changed cells and everything that transitively
// depends on them. Cells are evaluated lazily in dependency order: reading a
// dirty cell evaluates it first. A cell that is reached again while it is
// still being evaluated is part of a cycle and becomes ERR.
func (s *Sheet) recalc(changed []Addr) {
	const (
		dirty = iota + 1
		visiting
		done
	)
	state := make(map[Addr]int)
	queue := append([]Addr(nil), changed...)
	for len(queue) > 0 {
		a := queue[0]
		queue = queue[1:]
		if state[a] == dirty {
			continue
		}
		state[a] = dirty
		for d := range s.dependents[a] {
			queue = append(queue, d)
		}
		for u := range s.rangeUsers {
			for _, r := range s.cells[u].ranges {
				if r.Contains(a) {
					queue = append(queue, u)
					break
				}
			}
		}
	}

	s.Circular = false
	var compute func(Addr) Value
	compute = func(a Addr) Value {
		c := s.cells[a]
		switch {
		case c == nil:
			return Value{}
		case state[a] == visiting:
			s.Circular = true
			return ErrRef
		case state[a] != dirty:
			return c.Value
		}
		state[a] = visiting
		if c.expr == nil {
			c.Value = Value{Kind: Text, Str: strings.TrimPrefix(c.Input, "'")}
		} else {
			c.Value = eval(c.expr, compute)
		}
		state[a] = done
		return c.Value
	}
	for a := range state {
		compute(a)
	}
}
