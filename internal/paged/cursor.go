package paged

import (
	"context"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A cursor streams a range's rows for the book's Scan, a chunk at a
// time: a goroutine of its own scans the source and hands the rows over
// in batches, so the source's scan runs on while functions read what
// it read, and taking a row costs a slice index.

// batchRows is how many rows a batch holds.
const batchRows = 4096

// cursor is a range's rows from where the last chunk ended.
type cursor struct {
	at      sheet.Addr // where the next chunk starts
	width   int
	batches chan []sheet.LiveCell
	free    chan []sheet.LiveCell
	cancel  context.CancelFunc
	err     error // what the scan failed with, read once batches is closed
	cur     []sheet.LiveCell
	i       int              // the next row of cur
	row     []sheet.LiveCell // the row being read
	have    bool
}

// cursor is the cursor over the columns of part from from on: the one
// the last chunk left there, or a new one.
func (b *book) cursor(s snapshot, part sheet.Rect, from sheet.Addr) *cursor {
	k := cursorKey{key(s.name), part.From.Col, part.To.Col}
	if c := b.cursors[k]; c != nil {
		if c.at == from {
			return c
		}
		b.stopCursor(c)
	}
	cols := make([]int, 0, part.To.Col-part.From.Col+1)
	for c := part.From.Col; c <= part.To.Col; c++ {
		cols = append(cols, c)
	}
	c := startCursor(b.ctx, s, from, cols)
	if b.cursors == nil {
		b.cursors = map[cursorKey]*cursor{}
	}
	b.cursors[k] = c
	return c
}

// startCursor scans the source s's columns cols from the sheet row of
// from on.
func startCursor(ctx context.Context, s snapshot, from sheet.Addr, cols []int) *cursor {
	ctx, cancel := context.WithCancel(ctx)
	c := &cursor{at: from, width: len(cols), batches: make(chan []sheet.LiveCell, 2), free: make(chan []sheet.LiveCell, 3), cancel: cancel}
	go func() {
		defer close(c.batches)
		batch := c.take()
		err := s.h.Scan(ctx, int64(from.Row-1), cols, func(_ int64, vals []sheet.LiveCell) bool {
			if batch = append(batch, vals...); len(batch) < batchRows*c.width {
				return true
			}
			if !c.send(ctx, batch) {
				return false
			}
			batch = c.take()
			return true
		})
		if len(batch) > 0 {
			c.send(ctx, batch)
		}
		if ctx.Err() == nil {
			c.err = err
		}
	}()
	return c
}

// take is a batch to fill: one read already, or a new one.
func (c *cursor) take() []sheet.LiveCell {
	select {
	case b := <-c.free:
		return b[:0]
	default:
		return make([]sheet.LiveCell, 0, batchRows*c.width)
	}
}

// send hands a batch over, reporting false once the cursor is stopped.
func (c *cursor) send(ctx context.Context, batch []sheet.LiveCell) bool {
	select {
	case c.batches <- batch:
		return true
	case <-ctx.Done():
		return false
	}
}

// next is the next row, with false after the last.
func (c *cursor) next() ([]sheet.LiveCell, bool) {
	if c.i*c.width >= len(c.cur) {
		if c.cur != nil {
			select {
			case c.free <- c.cur:
			default:
			}
		}
		b, ok := <-c.batches
		if !ok {
			c.cur = nil
			return nil, false
		}
		c.cur, c.i = b, 0
	}
	row := c.cur[c.i*c.width : (c.i+1)*c.width]
	c.i++
	return row, true
}

// stopCursor stops c's scan, noting what it failed with.
func (b *book) stopCursor(c *cursor) {
	c.cancel()
	for range c.batches {
	}
	if c.err != nil {
		b.fail(c.err)
	}
}

// read gives emit the cells of part from the cursor on, until emit
// returns false or the part ends.
func (c *cursor) read(part sheet.Rect, emit func(sheet.Addr, sheet.Value) bool) {
	for c.at.Row <= part.To.Row {
		if !c.have {
			row, ok := c.next()
			if !ok {
				c.at.Row = part.To.Row + 1
				return
			}
			c.row, c.have = row, true
		}
		more := true
		for more && c.at.Col <= part.To.Col {
			v := c.row[c.at.Col-part.From.Col].V
			a := c.at
			c.at.Col++
			more = emit(a, v)
		}
		if c.at.Col > part.To.Col {
			// The row is read: the next chunk starts at the next one's
			// first cell, as the Reader asks for it.
			c.at, c.have = sheet.Addr{Col: part.From.Col, Row: c.at.Row + 1}, false
		}
		if !more {
			return
		}
	}
}
