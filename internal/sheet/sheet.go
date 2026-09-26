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

	auto     Format // format inferred from a formula, shown when Format is Automatic
	expr     Node   // nil for text
	refs     []Addr // single-cell references in expr
	ranges   []Rect // range references in expr
	volatile bool   // expr calls TODAY, NOW, RAND...
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
	// scanned separately, so a huge range doesn't create millions of edges.
	dependents map[Addr]map[Addr]struct{}
	rangeUsers map[Addr]struct{}
	volatile   map[Addr]struct{} // formulas recalculated on every change

	// Circular is set when the last recalculation found a cycle.
	Circular bool
}

// New returns an empty worksheet.
func New() *Sheet {
	return &Sheet{
		cells:      make(map[Addr]*Cell),
		widths:     make(map[int]int),
		dependents: make(map[Addr]map[Addr]struct{}),
		rangeUsers: make(map[Addr]struct{}),
		volatile:   make(map[Addr]struct{}),
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
	if err := s.put(a, input); err != nil {
		return err
	}
	s.recalc([]Addr{a})
	return nil
}

// put stores an entry without recalculating, keeping the cell's
// formatting. An entry that implies a format, such as "$5" or a date,
// sets it, as in Sheets. In a Plain text cell every entry is text.
// Control characters are dropped so a worksheet file can't smuggle escape
// sequences to the terminal.
func (s *Sheet) put(a Addr, input string) error {
	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, input)
	var f Format
	var st Style
	if old := s.cells[a]; old != nil {
		f, st = old.Format, old.Style
	}
	if input == "" {
		s.erase(a)
		s.restoreFormatting(a, f, st)
		return nil
	}
	var n Node
	if f.Kind != FmtText {
		var implied Format
		var err error
		if n, implied, err = classify(input); err != nil {
			return err
		}
		if !implied.IsZero() {
			f = implied
		}
	}
	c := &Cell{Input: input, expr: n, Format: f, Style: st}
	if n != nil {
		walkRefs(n,
			func(r Addr) { c.refs = append(c.refs, r) },
			func(r Rect) { c.ranges = append(c.ranges, r) })
		c.volatile = isVolatile(n)
	}
	s.erase(a)
	s.cells[a] = c
	if c.volatile {
		s.volatile[a] = struct{}{}
	}
	for _, r := range c.refs {
		if s.dependents[r] == nil {
			s.dependents[r] = make(map[Addr]struct{})
		}
		s.dependents[r][a] = struct{}{}
	}
	if len(c.ranges) > 0 {
		s.rangeUsers[a] = struct{}{}
	}
	return nil
}

// EraseRange clears the contents of every cell in r, keeping their
// formatting as Sheets' Delete does.
func (s *Sheet) EraseRange(r Rect) {
	var changed []Addr
	for a, c := range s.cells {
		if r.Contains(a) && !c.Blank() {
			changed = append(changed, a)
		}
	}
	for _, a := range changed {
		c := s.cells[a]
		s.erase(a)
		s.restoreFormatting(a, c.Format, c.Style)
	}
	s.recalc(changed)
}

// restoreFormatting leaves a blank cell holding just formatting, or no
// cell at all when there is none.
func (s *Sheet) restoreFormatting(a Addr, f Format, st Style) {
	if f.IsZero() && st.IsZero() {
		return
	}
	s.cells[a] = &Cell{Format: f, Style: st}
}

func (s *Sheet) erase(a Addr) {
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
	delete(s.volatile, a)
	delete(s.cells, a)
}

// RecalcAll recomputes every formula, e.g. after loading a file.
func (s *Sheet) RecalcAll() {
	s.recalc(s.Addrs())
}

// recalc recomputes the changed cells, volatile formulas, and everything
// that transitively depends on them. Cells are evaluated lazily in
// dependency order: reading a dirty cell evaluates it first. A cell that
// is reached again while it is still being evaluated is part of a cycle
// and becomes ERR.
func (s *Sheet) recalc(changed []Addr) {
	const (
		dirty = iota + 1
		visiting
		done
	)
	state := make(map[Addr]int)
	queue := append([]Addr(nil), changed...)
	for a := range s.volatile {
		queue = append(queue, a)
	}
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
		c.auto = Format{}
		switch {
		case c.Input == "":
			c.Value = Value{}
		case c.expr == nil && c.Format.Kind == FmtText:
			c.Value = Value{Kind: Text, Str: c.Input}
		case c.expr == nil:
			c.Value = Value{Kind: Text, Str: strings.TrimPrefix(c.Input, "'")}
		default:
			c.Value = eval(c.expr, compute)
			if _, lit := c.expr.(numLit); !lit {
				c.auto = inferFormat(c.expr, s.DisplayFormat)
			}
		}
		state[a] = done
		return c.Value
	}
	for a := range state {
		compute(a)
	}
}
