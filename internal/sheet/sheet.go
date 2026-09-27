// Package sheet is the spreadsheet engine: cell storage, formula parsing and
// dependency-ordered recalculation. It has no knowledge of the terminal UI.
package sheet

import (
	"maps"
	"sort"
	"strings"
	"unicode"

	"github.com/FelineStateMachine/012/internal/formula"
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
	derived  bool     // a pivot table's result, owned by the engine: see pivot.go
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

	cells  cellStore // see store.go
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

	// calc is the recalculation state of cells while the workbook
	// recalculates, with calcGet and calcFmt, the lookups its formulas
	// read other cells and formats with; all nil otherwise.
	calc    map[Addr]int
	calcGet lookup
	calcFmt func(string, Addr) Format

	charts []Chart    // floating charts, bottom first; see chart.go
	pivot  pivotState // the sheet's pivot table, if any; see pivot.go

	view   viewState   // frozen panes and the filter, see view.go
	hidden hiddenCache // rows the filter hides, see filter.go
}

// New returns an empty worksheet, the only sheet of a new workbook.
func New() *Sheet {
	return NewBook().Sheet(0)
}

// Cell returns the cell at a, or nil if it has neither contents nor
// formatting. Use Blank to test for contents.
func (s *Sheet) Cell(a Addr) *Cell { return s.cells.get(a) }

// Value returns the computed value at a.
func (s *Sheet) Value(a Addr) Value {
	if c := s.cells.get(a); c != nil {
		return c.Value
	}
	return Value{}
}

// Len returns the number of non-blank cells.
func (s *Sheet) Len() int {
	n := 0
	for _, c := range s.cells.all() {
		if !c.Blank() {
			n++
		}
	}
	return n
}

// Addrs returns every non-blank cell in row-major order.
func (s *Sheet) Addrs() []Addr {
	out := make([]Addr, 0, s.cells.len())
	for a, c := range s.cells.all() {
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
	s.pivot.fit = false // the user's widths win over the pivot's
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
		return formula.Num{V: v}, f, nil
	}
	switch strings.ToUpper(input) {
	case "TRUE":
		return formula.Bool{V: true}, Format{}, nil
	case "FALSE":
		return formula.Bool{V: false}, Format{}, nil
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
// unchanged. A pivot table's results can't be set: that's ErrPivotEdit.
func (s *Sheet) Set(a Addr, input string) error {
	if s.InPivot(Rect{From: a, To: a}) {
		return ErrPivotEdit
	}
	var err error
	s.change("edit "+a.String(), Rect{From: a, To: a}, func() { err = s.put(a, input) })
	return err
}

// put stores an entry without recalculating, keeping the cell's
// formatting.
func (s *Sheet) put(a Addr, input string) error {
	var f Format
	var st Style
	if old := s.cells.get(a); old != nil {
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
		formula.WalkRefs(n,
			func(sheet string, r Addr) {
				if sheet != "" {
					c.xrefs = append(c.xrefs, xref{formula.SheetKey(sheet), Rect{From: r, To: r}})
					return
				}
				c.refs = append(c.refs, r)
			},
			func(sheet string, r Rect) {
				if sheet != "" {
					c.xrefs = append(c.xrefs, xref{formula.SheetKey(sheet), r})
					return
				}
				c.ranges = append(c.ranges, r)
			})
		formula.WalkNames(n, func(nn formula.Name) { c.names = append(c.names, nameKey(nn.Name)) })
		c.volatile = isVolatile(n)
	}
}

// place stores c at a (nil blanks it), recording the old cell for undo and
// keeping the dependency indexes current. Every cell mutation goes through
// here, except a pivot table writing its results (setDerived). Anything
// placed over those has the pivot recomputed, which reports it in the way.
func (s *Sheet) place(a Addr, c *Cell) {
	s.version++
	s.record(a)
	if s.pivot.def != nil && s.pivot.out.Contains(a) {
		s.pivot.stale = true
	}
	s.unlink(a)
	if c == nil {
		return
	}
	s.cells.set(a, c)
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
			if c := s.cells.get(a); !c.Blank() {
				s.place(a, formattingOnly(c.Format, c.Style))
			}
		}
	})
}

// cellsIn returns the cells in r that have contents or formatting.
func (s *Sheet) cellsIn(r Rect) []Addr {
	var out []Addr
	for a := range s.cells.inRange(r) {
		out = append(out, a)
	}
	return out
}

// unlink removes the cell at a and its dependency edges.
func (s *Sheet) unlink(a Addr) {
	old := s.cells.get(a)
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
	s.cells.delete(a)
}
