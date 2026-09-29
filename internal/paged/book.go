package paged

import (
	"cmp"
	"context"
	"iter"
	"slices"

	"github.com/FelineStateMachine/012/internal/functions"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// book is the functions.Book a job computes a call through: the sources
// the call reads, each as a sheet named by the source's name, whose
// first row is its columns' names and the rest its rows. Ranges are
// streamed, a chunk at a time, through a cursor that goes on where the
// last chunk ended, so a function reading a column once reads the file
// once; a cell is fetched with its row, and the last chunk's cells are
// at hand, for lookups that walk a column and read what they pass.
type book struct {
	ctx  context.Context
	srcs map[string]snapshot
	err  error // the first a source gave: the answer is then that

	cursors map[cursorKey]*cursor
	chunk   []sheet.Addr // the last chunk Scan read, row by row
	chunkV  []sheet.Value
	chunkOf string
	rows    map[rowKey][]sheet.LiveCell // rows fetched, a few
}

type cursorKey struct {
	src    string
	c0, c1 int
}

type rowKey struct {
	src string
	row int
}

// maxRows bounds the rows fetched a book keeps.
const maxRows = 256

func (b *book) close() {
	for _, c := range b.cursors {
		c.stop()
	}
}

func (b *book) src(name string) (snapshot, bool) {
	s, ok := b.srcs[key(name)]
	return s, ok
}

// table is the source's table as a sheet holds it, header included.
func (s snapshot) table() sheet.Rect {
	return sheet.Rect{To: sheet.Addr{Col: len(s.shape.Cols) - 1, Row: s.shape.Rows}}
}

// clip is the part of r the source's table holds.
func (s snapshot) clip(r sheet.Rect) (sheet.Rect, bool) {
	t := s.table()
	if len(s.shape.Cols) == 0 {
		return sheet.Rect{}, false
	}
	out := sheet.Rect{
		From: sheet.Addr{Col: max(r.From.Col, t.From.Col), Row: max(r.From.Row, t.From.Row)},
		To:   sheet.Addr{Col: min(r.To.Col, t.To.Col), Row: min(r.To.Row, t.To.Row)},
	}
	return out, out.From.Col <= out.To.Col && out.From.Row <= out.To.Row
}

func (s snapshot) header(col int) sheet.Value {
	return sheet.Value{Kind: sheet.Text, Str: s.shape.Cols[col]}
}

// Cell is a cell's value: a column's name, a value of its row, or blank
// outside the table.
func (b *book) Cell(name string, a sheet.Addr) sheet.Value {
	s, ok := b.src(name)
	if !ok {
		return sheet.ErrRef
	}
	if !s.table().Contains(a) {
		return sheet.Value{}
	}
	if a.Row == 0 {
		return s.header(a.Col)
	}
	if b.chunkOf == key(name) {
		if i, found := slices.BinarySearchFunc(b.chunk, a, addrOrder); found {
			return b.chunkV[i]
		}
	}
	row, ok := b.fetch(s, a.Row-1)
	if !ok || a.Col >= len(row) {
		return sheet.Value{}
	}
	return row[a.Col].V
}

func addrOrder(x, y sheet.Addr) int {
	return cmp.Or(cmp.Compare(x.Row, y.Row), cmp.Compare(x.Col, y.Col))
}

// fetch is the source's row numbered row, every column.
func (b *book) fetch(s snapshot, row int) ([]sheet.LiveCell, bool) {
	k := rowKey{key(s.name), row}
	if r, ok := b.rows[k]; ok {
		return r, true
	}
	got, err := s.h.Fetch(b.ctx, []int64{int64(row)}, nil)
	if err != nil {
		b.fail(err)
		return nil, false
	}
	if b.rows == nil || len(b.rows) >= maxRows {
		b.rows = map[rowKey][]sheet.LiveCell{}
	}
	if len(got) == 0 {
		return nil, false
	}
	b.rows[k] = got[0]
	return got[0], true
}

func (b *book) fail(err error) {
	if b.err == nil && b.ctx.Err() == nil {
		b.err = err
	}
}

// Scan reads the cells of r holding something from from on, as
// functions.Book says: a chunk of a cursor streaming the rows.
func (b *book) Scan(name string, r sheet.Rect, from sheet.Addr, addrs []sheet.Addr, vals []sheet.Value) int {
	s, ok := b.src(name)
	if !ok {
		return -1
	}
	part, ok := s.clip(r)
	if !ok || from.Row > part.To.Row {
		return 0
	}
	if from.Row < part.From.Row || from.Row == part.From.Row && from.Col < part.From.Col {
		from = part.From
	}
	n := 0
	b.chunk, b.chunkV, b.chunkOf = b.chunk[:0], b.chunkV[:0], key(name)
	emit := func(a sheet.Addr, v sheet.Value) bool {
		if v.Kind == sheet.Empty {
			return true
		}
		addrs[n] = a
		if vals != nil {
			vals[n] = v
		}
		b.chunk, b.chunkV = append(b.chunk, a), append(b.chunkV, v)
		n++
		return n < len(addrs) && !(vals != nil && v.Kind == sheet.Error)
	}
	if from.Row == 0 {
		for c := from.Col; c <= part.To.Col; c++ {
			if !emit(sheet.Addr{Col: c, Row: 0}, s.header(c)) {
				return n
			}
		}
		from = sheet.Addr{Col: part.From.Col, Row: 1}
	}
	if from.Row > part.To.Row {
		return n
	}
	b.cursor(s, part, from).read(part, emit)
	return n
}

// cursor is the cursor over the columns of part from from on: the one
// the last chunk left there, or a new one.
func (b *book) cursor(s snapshot, part sheet.Rect, from sheet.Addr) *cursor {
	k := cursorKey{key(s.name), part.From.Col, part.To.Col}
	if c := b.cursors[k]; c != nil {
		if c.at == from {
			return c
		}
		c.stop()
	}
	cols := make([]int, 0, part.To.Col-part.From.Col+1)
	for c := part.From.Col; c <= part.To.Col; c++ {
		cols = append(cols, c)
	}
	c := &cursor{b: b, at: from}
	seq := func(yield func(int64, []sheet.LiveCell) bool) {
		if err := s.h.Scan(b.ctx, int64(from.Row-1), cols, yield); err != nil {
			b.fail(err)
		}
	}
	c.next, c.stop = iter.Pull2(seq)
	if b.cursors == nil {
		b.cursors = map[cursorKey]*cursor{}
	}
	b.cursors[k] = c
	return c
}

// cursor streams a range's rows for Scan, a chunk at a time.
type cursor struct {
	b    *book
	at   sheet.Addr // where the next chunk starts
	next func() (int64, []sheet.LiveCell, bool)
	stop func()
	row  []sheet.LiveCell // the row being read, from the range's first column
	have bool
}

// read gives emit the cells of part from the cursor on, until emit
// returns false or the part ends.
func (c *cursor) read(part sheet.Rect, emit func(sheet.Addr, sheet.Value) bool) {
	for c.at.Row <= part.To.Row {
		if !c.have {
			_, row, ok := c.next()
			if !ok {
				c.at.Row = part.To.Row + 1
				return
			}
			c.row, c.have = append(c.row[:0], row...), true
		}
		for c.at.Col <= part.To.Col {
			v := c.row[c.at.Col-part.From.Col].V
			a := c.at
			c.at.Col++
			if !emit(a, v) {
				return
			}
		}
		c.at, c.have = sheet.Addr{Col: part.From.Col, Row: c.at.Row + 1}, false
	}
}

// Bounds is the part of r the source's table holds.
func (b *book) Bounds(name string, r sheet.Rect) (sheet.Rect, bool, bool) {
	s, ok := b.src(name)
	if !ok {
		return sheet.Rect{}, false, false
	}
	part, any := s.clip(r)
	return part, any, true
}

// RangeAgg shares nothing: each call reads its ranges once.
func (b *book) RangeAgg(string, sheet.Rect) (functions.Agg, *sheet.Value, bool) {
	return functions.Agg{}, nil, false
}

// Fold adds the cells of r to agg, streaming the source's rows.
func (b *book) Fold(name string, r sheet.Rect, agg functions.Agg) (functions.Agg, *sheet.Value) {
	s, ok := b.src(name)
	if !ok {
		return agg, agg.Add(sheet.ErrRef, false)
	}
	part, ok := s.clip(r)
	if !ok {
		return agg, nil
	}
	var e *sheet.Value
	if part.From.Row == 0 {
		for c := part.From.Col; c <= part.To.Col && e == nil; c++ {
			e = agg.Add(s.header(c), false)
		}
		part.From.Row = 1
	}
	if e != nil || part.From.Row > part.To.Row {
		return agg, e
	}
	cols := make([]int, 0, part.To.Col-part.From.Col+1)
	for c := part.From.Col; c <= part.To.Col; c++ {
		cols = append(cols, c)
	}
	last := int64(part.To.Row - 1)
	err := s.h.Scan(b.ctx, int64(part.From.Row-1), cols, func(row int64, vals []sheet.LiveCell) bool {
		for _, v := range vals {
			if v.V.Kind != sheet.Empty {
				if e = agg.Add(v.V, false); e != nil {
					return false
				}
			}
		}
		return row < last
	})
	if err != nil {
		b.fail(err)
	}
	return agg, e
}

// Ask has no answers: sources don't ask JEV.
func (b *book) Ask(functions.RemoteCall) (functions.RemoteAnswer, sheet.Value) {
	return functions.RemoteAnswer{}, functions.ErrNoRemote
}

// Paged is false: a job reads its sources as sheets.
func (b *book) Paged(string) bool { return false }

// Stream isn't asked: nothing is paged here.
func (b *book) Stream(functions.StreamCall) (sheet.Value, *functions.Array) {
	return sheet.ErrValue, nil
}
