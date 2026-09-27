package functions

import "github.com/FelineStateMachine/012/internal/value"

// Book is how formulas reach the workbook: the engine implements it, once
// for each sheet whose formulas it evaluates. Sheets are named as
// references write them; "" is the formula's own sheet.
//
// Every method takes and returns plain values, never a callback: a
// function value or pointer passed through an interface escapes to the
// heap, so a callback per range read would cost an allocation where the
// engine used to cost none. Ranges are read instead in chunks, into
// buffers the Reader keeps and reuses (Scan), and each function walks
// them with callbacks of its own that stay on the stack.
type Book interface {
	// Cell is the current value of a cell, evaluated first if it's
	// dirty; #REF! on a sheet that doesn't exist.
	Cell(sheet string, a Addr) Value
	// Scan reads the cells of r that hold something, row by row, from
	// the cell from on (those before it in that order are skipped): their
	// addresses into addrs and, unless vals is nil, their values into
	// vals, evaluating them as Cell does. It stops when addrs is full or
	// just after a value that is an error, so no cell past the first
	// error is evaluated, and returns how many it read: -1 when the sheet
	// doesn't exist.
	Scan(sheet string, r Rect, from Addr, addrs []Addr, vals []Value) int
	// Bounds is the smallest range holding every stored cell of r, with
	// false if r holds none; exists is false when the sheet doesn't.
	Bounds(sheet string, r Rect) (b Rect, any, exists bool)
	// RangeAgg is the aggregate of r that SUM-like functions share within
	// a recalculation (see Agg), with the first error in it, or false
	// when r should be read directly.
	RangeAgg(sheet string, r Rect) (Agg, *Value, bool)
	// Ask looks up the answer to a remote question (JEV functions). The
	// value is an error when there is none to give: Pending while it is
	// on its way, ErrNoRemote when nothing answers them.
	Ask(call RemoteCall) (RemoteAnswer, Value)
}

// Reader is what a function is given to read the cells its arguments
// refer to: a Book, and the buffers ranges are read through. It is a
// concrete type so the callbacks functions pass to it stay on the stack.
type Reader struct {
	book  Book
	depth *int // how deeply evaluation is nested; see the engine's evaluate.go
	dense bool // read every address of a range; see NewReader
	bufs  []*scanBuf
	level int // how many of bufs are in use by range reads in progress
}

// lookup is the parameter every evaluator takes.
type lookup = *Reader

// NewReader reads cells through book. depth counts how deeply evaluation
// nests (cells, operators and calls), for the engine to put off cells
// past its limit. With dense, functions read every address of their
// ranges, as they did before ranges were clipped to what they hold, so
// tests can check that the two agree.
func NewReader(book Book, depth *int, dense bool) *Reader {
	return &Reader{book: book, depth: depth, dense: dense}
}

// A range is read in chunks that start small, for the many short ranges,
// and double up to maxChunk as a range proves long. Nested range reads (a
// SUM whose cells hold SUMs) each use a buffer of their own.
const (
	firstChunk = 8
	maxChunk   = 1024
)

type scanBuf struct {
	addrs []Addr
	vals  []Value
}

// take returns the buffer for a range read starting now; give returns it.
func (rd *Reader) take() *scanBuf {
	if rd.level == len(rd.bufs) {
		rd.bufs = append(rd.bufs, &scanBuf{addrs: make([]Addr, firstChunk), vals: make([]Value, firstChunk)})
	}
	b := rd.bufs[rd.level]
	rd.level++
	return b
}

func (rd *Reader) give() { rd.level-- }

// fit makes the buffer hold at least n cells.
func (b *scanBuf) fit(n int) {
	if len(b.addrs) < n {
		b.addrs, b.vals = make([]Addr, n), make([]Value, n)
	}
}

// next is the cell after a in r, row by row.
func next(r Rect, a Addr) Addr {
	if a.Col < r.To.Col {
		return Addr{Col: a.Col + 1, Row: a.Row}
	}
	return Addr{Col: r.From.Col, Row: a.Row + 1}
}

// cell returns the current value of a cell.
func (rd *Reader) cell(sheet string, a Addr) Value { return rd.book.Cell(sheet, a) }

// cells calls fn with the address and value of every cell of r that
// holds something, row by row, until fn returns false: blank cells are
// skipped without being visited, so a whole column costs what it holds.
// A range on a sheet that doesn't exist reads as one #REF!. Cells are
// evaluated a chunk ahead of fn, but never past an error: every caller
// stops at the first error, so none evaluates a cell it wouldn't have
// read one at a time.
func (rd *Reader) cells(sheet string, r Rect, fn func(Addr, Value) bool) {
	b := rd.take()
	defer rd.give()
	for from := r.From; from.Row <= r.To.Row; {
		n := rd.book.Scan(sheet, r, from, b.addrs, b.vals)
		if n < 0 {
			fn(r.From, value.ErrRef)
			return
		}
		for i := range n {
			if !fn(b.addrs[i], b.vals[i]) {
				return
			}
		}
		switch {
		case n == len(b.addrs):
			from = next(r, b.addrs[n-1])
			b.fit(min(2*n, maxChunk))
		case n > 0 && b.vals[n-1].Kind == value.Error:
			from = next(r, b.addrs[n-1])
		default:
			return
		}
	}
}

// stored calls fn with every cell of r that holds something, row by row,
// until fn returns false, without evaluating any. A sheet that doesn't
// exist holds none. Lookups stop at the first match, so each call starts
// with a small chunk, however long earlier ranges were, and doubles it
// as the range goes on: finding a key near the top of a whole column
// costs the cells before it.
func (rd *Reader) stored(sheet string, r Rect, fn func(Addr) bool) {
	b := rd.take()
	defer rd.give()
	size := firstChunk
	for from := r.From; from.Row <= r.To.Row; {
		n := rd.book.Scan(sheet, r, from, b.addrs[:size], nil)
		for i := range n {
			if !fn(b.addrs[i]) {
				return
			}
		}
		if n < size {
			return
		}
		from = next(r, b.addrs[n-1])
		if size < maxChunk {
			size *= 2
			b.fit(size)
		}
	}
}
