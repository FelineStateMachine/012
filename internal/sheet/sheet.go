// Package sheet is the spreadsheet engine: cell storage, formula parsing and
// dependency-ordered recalculation. It has no knowledge of the terminal UI.
package sheet

import (
	"maps"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// DefaultWidth is the initial column width, as in 1-2-3.
const DefaultWidth = 9

// Cell holds what the user typed and what it evaluates to.
type Cell struct {
	// Input is the entry as typed. Labels always carry an alignment
	// prefix: ' (left), " (right) or ^ (center).
	Input string
	Value Value

	expr   Node   // nil for labels
	refs   []Addr // single-cell references in expr
	ranges []Rect // range references in expr
}

// IsLabel reports whether the cell holds a label rather than a value.
func (c *Cell) IsLabel() bool { return c.expr == nil }

// Align returns the label prefix character, or 0 for values.
func (c *Cell) Align() byte {
	if c.IsLabel() {
		return c.Input[0]
	}
	return 0
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

// IsValueEntry reports whether 1-2-3 would treat input as a value (number or
// formula) rather than a label, based on its first character.
func IsValueEntry(input string) bool {
	if input == "" {
		return false
	}
	return strings.IndexByte("0123456789+-.(@#$=", input[0]) >= 0
}

// Set stores an entry at a and recalculates affected cells. An empty input
// erases the cell. Labels without a prefix get the default ' prefix. A
// value entry that fails to parse is rejected with a *ParseError and the
// sheet is left unchanged.
func (s *Sheet) Set(a Addr, input string) error {
	if err := s.put(a, input); err != nil {
		return err
	}
	s.recalc([]Addr{a})
	return nil
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
		s.erase(a)
		return nil
	}
	c := &Cell{Input: input}
	if IsValueEntry(input) {
		n, err := Parse(input)
		if err != nil {
			return err
		}
		c.expr = n
		walkRefs(n,
			func(r Addr) { c.refs = append(c.refs, r) },
			func(r Rect) { c.ranges = append(c.ranges, r) })
	} else if strings.IndexByte(`'"^`, input[0]) < 0 {
		c.Input = "'" + input
	}
	s.erase(a)
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
	return nil
}

// EraseRange blanks every cell in r.
func (s *Sheet) EraseRange(r Rect) {
	var changed []Addr
	for a := range s.cells {
		if r.Contains(a) {
			changed = append(changed, a)
		}
	}
	for _, a := range changed {
		s.erase(a)
	}
	s.recalc(changed)
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
	delete(s.cells, a)
}

// RecalcAll recomputes every formula, e.g. after loading a file.
func (s *Sheet) RecalcAll() {
	s.recalc(s.Addrs())
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
			return errValue
		case state[a] != dirty:
			return c.Value
		}
		state[a] = visiting
		if c.IsLabel() {
			c.Value = Value{Kind: Label, Str: c.Input[1:]}
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

// FormatValue renders v to fit in width columns using 1-2-3's General
// format: numbers are right-aligned with a trailing space, and a number
// that cannot fit is shown as asterisks.
func FormatValue(v Value, width int) string {
	switch v.Kind {
	case Number:
		return padLeft(formatGeneral(v.Num, width-1), width-1) + " "
	case Error:
		return padLeft(v.Str, width-1) + " "
	}
	return ""
}

func formatGeneral(v float64, width int) string {
	if width <= 0 {
		return ""
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if len(s) <= width {
		return s
	}
	// Try fewer decimals before falling back to scientific notation, but
	// never round a non-zero number down to zero.
	if dot := strings.IndexByte(s, '.'); dot >= 0 && dot <= width {
		for prec := max(width-dot-1, 0); prec >= 0; prec-- {
			f := strconv.FormatFloat(v, 'f', prec, 64)
			if len(f) > width {
				continue
			}
			if r, _ := strconv.ParseFloat(f, 64); r != 0 || v == 0 {
				return f
			}
			break
		}
	}
	for prec := width; prec >= 0; prec-- {
		if e := strconv.FormatFloat(v, 'E', prec, 64); len(e) <= width {
			return e
		}
	}
	return strings.Repeat("*", width)
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}
