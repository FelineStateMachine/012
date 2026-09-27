package sheet

import (
	"math/bits"
	"slices"
)

// Stats summarizes the values in r, as shown in the status line for a
// selection. Count is non-blank cells, Nums is numeric cells.
type Stats struct {
	Sum         float64
	Count, Nums int
}

func (st *Stats) add(c *Cell) {
	if c.Blank() {
		return
	}
	st.Count++
	if c.Value.Kind == Number {
		st.Nums++
		st.Sum += c.Value.Num
	}
}

// statsCache is the last RangeStats result: the status line asks for the
// selection's statistics on every frame.
type statsCache struct {
	r       Rect
	version uint64
	st      Stats
	ok      bool
}

// RangeStats computes Stats over r. The result is kept until a cell or
// value changes. A selection over a large, well-filled sheet is summed
// from the cells' statistics index (see rangeStats), so changing it costs
// a block per column plus the rows at its ends, not a read of every cell.
func (s *Sheet) RangeStats(r Rect) Stats {
	if c := s.stats; c.ok && c.r == r && c.version == s.version {
		return c.st
	}
	var st Stats
	area := (r.To.Col - r.From.Col + 1) * (r.To.Row - r.From.Row + 1)
	if area <= statsDirect || s.cells.len() <= statsSparse {
		for _, c := range s.cells.anyInRange(r) {
			st.add(c)
		}
	} else {
		st = s.cells.rangeStats(r)
	}
	s.stats = statsCache{r: r, version: s.version, st: st, ok: true}
	return st
}

const (
	// statsDirect: selections of at most this many cells are summed cell
	// by cell.
	statsDirect = 4096
	// statsSparse: sheets of at most this many cells are summed by
	// visiting their cells, which is cheaper than the index.
	statsSparse = 16384
)

// The statistics index keeps Stats for every 64 rows of every column
// that has contents, on the blocks of the index of filled cells
// (occupancy.go): one entry per word of a block's bitmap, so it takes
// memory only where there is data, whatever the grid's size. Entries are
// computed when a selection first covers them; storing or deleting a
// cell, or recalculating its value, marks its entry stale, and a stale
// entry is summed again (a lookup per cell it holds) when next needed.
// Entries are recomputed rather than adjusted, so sums don't drift with
// rounding.

// wordStats is Stats over the 64 rows of one word of a block.
type wordStats struct {
	sum         float64
	count, nums int32
}

// blockStats is the statistics of a block of the filled index, made on
// first use.
type blockStats struct {
	words   [blockWords]wordStats
	ok      uint16 // a bit per word: its entry is current
	total   wordStats
	totalOK bool // total is the sum of the words' entries, all current
}

// touch marks the entry holding a stale.
func (st *cellStore) touch(a Addr) {
	if b := st.filled.col(a.Col).block(a.Row >> blockShift); b != nil && b.stats != nil {
		b.stats.ok &^= 1 << ((a.Row & (blockRows - 1)) >> 6)
		b.stats.totalOK = false
	}
}

// changed notes that the value of the cell at a was recalculated.
func (st *cellStore) changed(a Addr) { st.touch(a) }

// word returns the statistics of word w of block id of column col, summing
// it again if it's stale.
func (st *cellStore) word(col, id int, b *rowBlock, w int) wordStats {
	if b.stats == nil {
		b.stats = &blockStats{}
	}
	x := b.stats
	if x.ok&(1<<w) == 0 {
		var s Stats
		st.scanWord(col, id, b, w, ^uint64(0), &s)
		x.words[w] = wordStats{sum: s.Sum, count: int32(s.Count), nums: int32(s.Nums)}
		x.ok |= 1 << w
	}
	return x.words[w]
}

// blockTotal returns the statistics of the whole block id of column col.
func (st *cellStore) blockTotal(col, id int, b *rowBlock) wordStats {
	if b.stats != nil && b.stats.totalOK {
		return b.stats.total
	}
	var t wordStats
	for w := range blockWords {
		ws := st.word(col, id, b, w)
		t.sum += ws.sum
		t.count += ws.count
		t.nums += ws.nums
	}
	b.stats.total, b.stats.totalOK = t, true
	return t
}

// scanWord adds the filled cells of word w of block id, among the rows
// mask keeps, into s.
func (st *cellStore) scanWord(col, id int, b *rowBlock, w int, mask uint64, s *Stats) {
	base := id<<blockShift + w<<6
	for x := b.bits[w] & mask; x != 0; x &= x - 1 {
		if c := st.m[Addr{Col: col, Row: base + bits.TrailingZeros64(x)}]; c != nil {
			s.add(c)
		}
	}
}

// rangeStats sums r from the index: in each column with contents, the
// whole words r covers from their entries, and the rows at its ends
// outside them cell by cell.
func (st *cellStore) rangeStats(r Rect) Stats {
	st.statsUsed = true
	var s Stats
	b0, b1 := r.From.Row>>blockShift, r.To.Row>>blockShift
	for _, col := range st.filled.colsIn(r.From.Col, r.To.Col) {
		ci := st.filled.col(col)
		i, _ := slices.BinarySearch(ci.ids, b0)
		for ; i < len(ci.ids) && ci.ids[i] <= b1; i++ {
			id := ci.ids[i]
			b := ci.blocks[id]
			base := id << blockShift
			lo, hi := max(r.From.Row-base, 0), min(r.To.Row-base, blockRows-1)
			if lo == 0 && hi == blockRows-1 {
				t := st.blockTotal(col, id, b)
				s.Sum += t.sum
				s.Count += int(t.count)
				s.Nums += int(t.nums)
				continue
			}
			for w := lo >> 6; w <= hi>>6; w++ {
				if mask := wordMask(w, lo, hi); mask != ^uint64(0) {
					st.scanWord(col, id, b, w, mask, &s)
					continue
				}
				ws := st.word(col, id, b, w)
				s.Sum += ws.sum
				s.Count += int(ws.count)
				s.Nums += int(ws.nums)
			}
		}
	}
	return s
}
