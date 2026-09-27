package sheet

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
// from the cells' statistics index (see statsIndex), so changing it costs
// a block per column plus the rows at its ends, not a read of every cell.
func (s *Sheet) RangeStats(r Rect) Stats {
	if c := s.stats; c.ok && c.r == r && c.version == s.version {
		return c.st
	}
	var st Stats
	area := (r.To.Col - r.From.Col + 1) * (r.To.Row - r.From.Row + 1)
	if area <= statsDirect || s.cells.len() <= statsSparse {
		for _, c := range s.cells.inRange(r) {
			st.add(c)
		}
	} else {
		st = s.cells.rangeStats(r)
	}
	s.stats = statsCache{r: r, version: s.version, st: st, ok: true}
	return st
}

const (
	// statsBlock is how many rows of a column one entry of the index
	// sums: at most twice that many cells per column are read at a
	// selection's ends.
	statsBlock = 64
	// statsBlocks is the number of blocks down a column.
	statsBlocks = (MaxRows + statsBlock - 1) / statsBlock
	// statsDirect: selections of at most this many cells are summed cell
	// by cell.
	statsDirect = 4096
	// statsSparse: sheets of at most this many cells are summed by
	// visiting their cells, which is cheaper than the index.
	statsSparse = 16384
)

// blockStats is Stats over one block of a column.
type blockStats struct {
	sum         float64
	count, nums int32
}

// statsIndex keeps Stats for every block of statsBlock rows of every
// column, so a large selection is summed a block at a time. It's built on
// first use, from one pass over the cells. After that, storing or
// deleting a cell, or recalculating its value, marks its block stale, and
// stale blocks are summed again (a lookup per row) when a selection next
// covers them. Blocks are recomputed rather than adjusted, so sums don't
// drift with rounding.
type statsIndex struct {
	blocks []blockStats // by col*statsBlocks + row/statsBlock
	stale  []uint64     // a bit per block
}

func blockOf(a Addr) int { return a.Col*statsBlocks + a.Row/statsBlock }

// touch marks the block holding a stale. A nil index has nothing to mark.
func (x *statsIndex) touch(a Addr) {
	if x != nil {
		i := blockOf(a)
		x.stale[i>>6] |= 1 << (i & 63)
	}
}

// buildStats makes the index from one pass over every cell.
func (st *cellStore) buildStats() *statsIndex {
	n := MaxCols * statsBlocks
	x := &statsIndex{blocks: make([]blockStats, n), stale: make([]uint64, (n+63)/64)}
	for a, c := range st.m {
		if c.Blank() {
			continue
		}
		b := &x.blocks[blockOf(a)]
		b.count++
		if c.Value.Kind == Number {
			b.nums++
			b.sum += c.Value.Num
		}
	}
	st.stats = x
	return x
}

// block returns the statistics of block b of column col, summing it
// again if it's stale.
func (st *cellStore) block(col, b int) blockStats {
	x := st.stats
	i := col*statsBlocks + b
	if x.stale[i>>6]&(1<<(i&63)) != 0 {
		var s Stats
		st.scan(col, b*statsBlock, min((b+1)*statsBlock, MaxRows)-1, &s)
		x.blocks[i] = blockStats{sum: s.Sum, count: int32(s.Count), nums: int32(s.Nums)}
		x.stale[i>>6] &^= 1 << (i & 63)
	}
	return x.blocks[i]
}

// scan adds the cells of column col from row from to row to into s.
func (st *cellStore) scan(col, from, to int, s *Stats) {
	for row := from; row <= to; row++ {
		if c := st.m[Addr{Col: col, Row: row}]; c != nil {
			s.add(c)
		}
	}
}

// rangeStats sums r from the index: in each column, the whole blocks r
// covers, and the rows at its ends outside them cell by cell.
func (st *cellStore) rangeStats(r Rect) Stats {
	if st.stats == nil {
		st.buildStats()
	}
	var s Stats
	first := (r.From.Row + statsBlock - 1) / statsBlock // first whole block
	last := first                                       // one past the last whole block
	for last < statsBlocks && min((last+1)*statsBlock, MaxRows)-1 <= r.To.Row {
		last++
	}
	for col := r.From.Col; col <= r.To.Col; col++ {
		if first >= last {
			st.scan(col, r.From.Row, r.To.Row, &s)
			continue
		}
		st.scan(col, r.From.Row, first*statsBlock-1, &s)
		for b := first; b < last; b++ {
			bs := st.block(col, b)
			s.Sum += bs.sum
			s.Count += int(bs.count)
			s.Nums += int(bs.nums)
		}
		st.scan(col, last*statsBlock, r.To.Row, &s)
	}
	return s
}
