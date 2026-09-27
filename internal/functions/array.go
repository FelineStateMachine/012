package functions

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Arrays, as Sheets computes them. A range read whole, an array literal
// ({1,2;3,4}) and what FILTER, SORT, SEQUENCE and the like return are
// arrays; operators over arrays work element by element, and in an array
// context (ARRAYFORMULA, or an argument that takes ranges, as in
// SUM(B2:B4*2)) functions that take one value are applied to each
// element (lift.go). A formula whose result is an array spills it into
// the cells to its right and below (the engine's spill.go).
//
// While a formula is evaluated, an array travels as a Value of kind
// value.Array whose Num indexes the Reader's arena, so the Value every
// cell holds stays as small as it is. Only callers that asked for an
// array get one (Reader.wantArr); anywhere else it reads as its first
// element, as Sheets reads an array where one value is wanted.

// Array is the value of a formula that computes several: Rows by Cols
// entries. Only the top-left DRows by DCols hold values of their own, in
// V row by row; every other entry is Fill. A whole column read as an
// array costs what it holds: A:A is 1,048,576 rows, of which the rows
// holding data are stored, the rest blank.
type Array struct {
	Rows, Cols   int
	DRows, DCols int
	V            []Value
	Fill         Value
}

// maxArray caps how many values an array stores (DRows x DCols), so a
// formula can't exhaust memory: past it the formula is #VALUE!.
const maxArray = 1 << 21

// NewArray is a dense array of rows x cols values, all blank.
func NewArray(rows, cols int) *Array {
	return &Array{Rows: rows, Cols: cols, DRows: rows, DCols: cols, V: make([]Value, rows*cols)}
}

// At is the entry at row r and column c, counting from 0.
func (a *Array) At(r, c int) Value {
	if r < a.DRows && c < a.DCols {
		return a.V[r*a.DCols+c]
	}
	return a.Fill
}

// Set stores v at row r and column c of a dense array.
func (a *Array) Set(r, c int, v Value) { a.V[r*a.DCols+c] = v }

// size is how many entries the array has.
func (a *Array) size() int { return a.Rows * a.Cols }

// bcast is the entry at r, c of a broadcast to more rows or columns: a
// single row repeats down, a single column across. Past a's size in a
// dimension it doesn't broadcast in, the entry is #N/A, as Sheets shows
// arrays of different sizes lined up.
func (a *Array) bcast(r, c int) Value {
	if a.Rows == 1 {
		r = 0
	}
	if a.Cols == 1 {
		c = 0
	}
	if r >= a.Rows || c >= a.Cols {
		return value.ErrNA
	}
	return a.At(r, c)
}

// tooBig reports whether an array of rows x cols stored values is past
// maxArray.
func tooBig(rows, cols int) bool {
	return rows < 0 || cols < 0 || rows > 0 && cols > maxArray/rows
}

// lambda is a LAMBDA's value: its parameters, its expression and the
// names bound where it was written (lambda.go).
type lambda struct {
	params []string
	body   Node
	scope  []binding
}

// arrayValue is a as a value: its only entry when it has one, else a
// handle to it in the arena.
func (rd *Reader) arrayValue(a *Array) Value {
	if a.Rows == 1 && a.Cols == 1 {
		return a.At(0, 0)
	}
	if a.size() == 0 {
		return value.ErrNA
	}
	return rd.handle(a)
}

// handle stores x (an *Array or a *lambda) in the arena and returns the
// value that stands for it.
func (rd *Reader) handle(x any) Value {
	rd.arena = append(rd.arena, x)
	return Value{Kind: value.Array, Num: float64(len(rd.arena) - 1)}
}

// arrayOf is the array v stands for, or nil when v is one value (or a
// LAMBDA).
func (rd *Reader) arrayOf(v Value) *Array {
	if v.Kind != value.Array {
		return nil
	}
	a, _ := rd.arena[int(v.Num)].(*Array)
	return a
}

// lambdaOf is the LAMBDA v stands for, or nil.
func (rd *Reader) lambdaOf(v Value) *lambda {
	if v.Kind != value.Array {
		return nil
	}
	l, _ := rd.arena[int(v.Num)].(*lambda)
	return l
}

// first is v as one value: an array's first entry. A LAMBDA isn't a
// value a cell can show: #VALUE!, as a LAMBDA not called with arguments
// is in Sheets.
func (rd *Reader) first(v Value) Value {
	if v.Kind != value.Array {
		return v
	}
	if a := rd.arrayOf(v); a != nil {
		return a.At(0, 0)
	}
	return value.ErrValue
}

// reduce is v as the caller wants it: an array only if it asked for one.
func (rd *Reader) reduce(v Value) Value {
	if v.Kind == value.Array && !rd.wantArr {
		return rd.first(v)
	}
	return v
}

// eval1 evaluates n where one value is wanted, whatever the context: a
// range reads as one of its cells and an array as its first entry.
func eval1(n Node, get lookup) Value {
	if get.lift == 0 && !get.wantArr {
		return eval(n, get)
	}
	lift, want := get.lift, get.wantArr
	get.lift, get.wantArr = 0, false
	v := eval(n, get)
	get.lift, get.wantArr = lift, want
	return v
}

// evalArr evaluates n in an array context: ranges read whole, operators
// work element by element and functions of one value are applied to each
// element. Arguments that take ranges (SUM(B2:B4*2)) and array functions
// (FILTER, SORT) read theirs this way.
func evalArr(n Node, get lookup) Value {
	lift, want := get.lift, get.wantArr
	get.lift, get.wantArr = lift+1, true
	v := eval(n, get)
	get.lift, get.wantArr = lift, want
	return v
}

// arrayArg is argument n read as an array: a range, an array, or one
// value as a 1x1 array.
func arrayArg(n Node, get lookup) (*Array, *Value) {
	v := evalArr(n, get)
	if a := get.arrayOf(v); a != nil {
		return a, nil
	}
	if v.Kind == value.Error {
		return nil, errOf(v)
	}
	if v.Kind == value.Array { // a LAMBDA
		return nil, &value.ErrValue
	}
	a := NewArray(1, 1)
	a.V[0] = v
	return a, nil
}

// rangeArray reads the range r on sheet as an array: its cells up to
// the last one holding something, blank past them. Arrays are never
// changed once made, so a range read before (see Forget) is shared.
func (rd *Reader) rangeArray(sheet string, r Rect) Value {
	if r.From == r.To {
		return rd.cell(sheet, r.From)
	}
	k := rangeKey{sheet, r}
	if a, ok := rd.ranges[k]; ok {
		return rd.arrayValue(a)
	}
	v := rd.readRange(sheet, r)
	if a := rd.arrayOf(v); a != nil {
		if rd.ranges == nil {
			rd.ranges = map[rangeKey]*Array{}
		}
		rd.ranges[k] = a
	}
	return v
}

// readRange reads a range as an array.
func (rd *Reader) readRange(sheet string, r Rect) Value {
	a := &Array{Rows: r.To.Row - r.From.Row + 1, Cols: r.To.Col - r.From.Col + 1}
	b, any, exists := rd.book.Bounds(sheet, r)
	switch {
	case !exists:
		return value.ErrRef
	case rd.dense:
		a.DRows, a.DCols = a.Rows, a.Cols
	case any:
		a.DRows, a.DCols = b.To.Row-r.From.Row+1, b.To.Col-r.From.Col+1
	}
	if tooBig(a.DRows, a.DCols) {
		return value.ErrValue
	}
	a.V = make([]Value, a.DRows*a.DCols)
	data := Rect{From: r.From, To: Addr{Col: r.From.Col + a.DCols - 1, Row: r.From.Row + a.DRows - 1}}
	if a.DRows > 0 && a.DCols > 0 {
		rd.cells(sheet, data, func(at Addr, v Value) bool {
			a.Set(at.Row-r.From.Row, at.Col-r.From.Col, v)
			return true
		})
	}
	return rd.arrayValue(a)
}

// literal evaluates an array literal: each element in an array context,
// elements side by side in a row and rows stacked, so {A1:A3,B1:B3} is
// three rows of two. Rows of different widths are #VALUE!, as Sheets
// says an array literal is missing values.
func (rd *Reader) literal(n formula.Array) Value {
	var rows []*Array
	for _, row := range n.Rows {
		var parts []*Array
		for _, e := range row {
			a, err := arrayArg(e, rd)
			if err != nil {
				return *err
			}
			parts = append(parts, a)
		}
		joined, ok := hstack(parts)
		if !ok {
			return value.ErrValue
		}
		rows = append(rows, joined)
	}
	a, ok := vstack(rows)
	if !ok {
		return value.ErrValue
	}
	return rd.arrayValue(a)
}

// hstack puts arrays of the same height side by side.
func hstack(parts []*Array) (*Array, bool) {
	if len(parts) == 1 {
		return parts[0], true
	}
	rows, cols := parts[0].Rows, 0
	for _, p := range parts {
		if p.Rows != rows {
			return nil, false
		}
		cols += p.Cols
	}
	if tooBig(rows, cols) {
		return nil, false
	}
	out := NewArray(rows, cols)
	at := 0
	for _, p := range parts {
		for r := range rows {
			for c := range p.Cols {
				out.Set(r, at+c, p.At(r, c))
			}
		}
		at += p.Cols
	}
	return out, true
}

// vstack stacks arrays of the same width.
func vstack(parts []*Array) (*Array, bool) {
	if len(parts) == 1 {
		return parts[0], true
	}
	rows, cols := 0, parts[0].Cols
	for _, p := range parts {
		if p.Cols != cols {
			return nil, false
		}
		rows += p.Rows
	}
	if tooBig(rows, cols) {
		return nil, false
	}
	out := NewArray(rows, cols)
	at := 0
	for _, p := range parts {
		for r := range p.Rows {
			for c := range cols {
				out.Set(at+r, c, p.At(r, c))
			}
		}
		at += p.Rows
	}
	return out, true
}

// shape is the size of arrays lined up for an element-wise operation,
// and how much of it holds values of its own.
type shape struct{ rows, cols, drows, dcols int }

// fit widens s to take in a: sizes must match, or be 1 to broadcast.
func (s *shape) fit(a *Array) bool {
	if a == nil {
		return true
	}
	if s.rows == 0 {
		*s = shape{rows: a.Rows, cols: a.Cols}
	}
	var ok bool
	if s.rows, ok = fitDim(s.rows, a.Rows); !ok {
		return false
	}
	if s.cols, ok = fitDim(s.cols, a.Cols); !ok {
		return false
	}
	return true
}

// fitDim is the length two broadcast dimensions line up to: the longer,
// when one is 1 or both are equal. Arrays of different sizes otherwise
// line up to the longer, the missing entries #N/A (Array.bcast).
func fitDim(a, b int) (int, bool) {
	return max(a, b), true
}

// data sets how much of s holds values of their own, given the arrays
// lined up in it.
func (s *shape) data(arrs ...*Array) {
	s.drows, s.dcols = 0, 0
	for _, a := range arrs {
		if a == nil {
			continue
		}
		r, c := a.DRows, a.DCols
		if a.Rows == 1 && r > 0 {
			r = s.rows
		}
		if a.Cols == 1 && c > 0 {
			c = s.cols
		}
		if a.Rows != s.rows && a.Rows != 1 || a.Cols != s.cols && a.Cols != 1 {
			r, c = s.rows, s.cols // #N/A past its end
		}
		s.drows, s.dcols = max(s.drows, r), max(s.dcols, c)
	}
	s.drows, s.dcols = min(s.drows, s.rows), min(s.dcols, s.cols)
}

// newArray is an array of shape s, its values to fill in.
func (s shape) newArray() (*Array, bool) {
	if tooBig(s.drows, s.dcols) {
		return nil, false
	}
	return &Array{Rows: s.rows, Cols: s.cols, DRows: s.drows, DCols: s.dcols, V: make([]Value, s.drows*s.dcols)}, true
}

// entry is v at r, c: its entry when it is an array, else v itself.
func entry(a *Array, v Value, r, c int) Value {
	if a == nil {
		return v
	}
	return a.bcast(r, c)
}

// fillOf is the value past v's data: its fill when it is an array.
func fillOf(a *Array, v Value) Value {
	if a == nil {
		return v
	}
	return a.Fill
}

// elementwise applies op to the entries of l and r lined up, one of
// them at least an array.
func (rd *Reader) elementwise(l, r Value, op func(l, r Value) Value) Value {
	la, ra := rd.arrayOf(l), rd.arrayOf(r)
	if la == nil && l.Kind == value.Array || ra == nil && r.Kind == value.Array {
		return value.ErrValue // a LAMBDA
	}
	var s shape
	if !s.fit(la) || !s.fit(ra) {
		return value.ErrValue
	}
	s.data(la, ra)
	out, ok := s.newArray()
	if !ok {
		return value.ErrValue
	}
	for i := range s.drows {
		for j := range s.dcols {
			out.Set(i, j, op(entry(la, l, i, j), entry(ra, r, i, j)))
		}
	}
	out.Fill = op(fillOf(la, l), fillOf(ra, r))
	return rd.arrayValue(out)
}
