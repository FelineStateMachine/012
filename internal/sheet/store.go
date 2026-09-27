package sheet

import (
	"iter"
	"maps"
	"math/bits"
	"slices"
)

// cellStore holds a sheet's cells: those with contents or formatting, by
// address. The rest of the engine reads and writes cells only through
// these methods, never the map beneath, so the representation can change
// (to column blocks of compact values, with formulas and formats in side
// tables; see docs/limits.md) without touching it.
//
// Next to the map, an occupancy index records which rows of each column
// hold a cell, as bitmaps of 1024 rows, so reading a range costs what the
// range holds rather than its area: a whole column of a million rows with
// ten cells is ten visits, in row order.
//
// Iteration order of all is unspecified; inRange yields in row-major
// order. Cells may be set or deleted while iterating, with a map's rules:
// a deleted cell not yet reached isn't yielded, a new one may or may not
// be.
type cellStore struct {
	m      map[Addr]*Cell
	cols   map[int]*colIndex
	colIDs []int // the columns holding cells, ascending
}

// Rows of a column are indexed in blocks of blockRows, one bit each.
const (
	blockShift = 10
	blockRows  = 1 << blockShift
	blockWords = blockRows / 64
)

type rowBlock struct {
	bits [blockWords]uint64
	n    int
}

// colIndex is the occupancy of one column: its blocks by number, and the
// numbers in order.
type colIndex struct {
	blocks map[int]*rowBlock
	ids    []int
	n      int
}

func newCellStore() cellStore {
	return cellStore{m: make(map[Addr]*Cell), cols: make(map[int]*colIndex)}
}

// get returns the cell at a, or nil.
func (st *cellStore) get(a Addr) *Cell { return st.m[a] }

// set stores c, which isn't nil, at a.
func (st *cellStore) set(a Addr, c *Cell) {
	if _, ok := st.m[a]; !ok {
		st.mark(a)
	}
	st.m[a] = c
}

// delete removes the cell at a, if any.
func (st *cellStore) delete(a Addr) {
	if _, ok := st.m[a]; ok {
		st.unmark(a)
		delete(st.m, a)
	}
}

func (st *cellStore) mark(a Addr) {
	ci := st.cols[a.Col]
	if ci == nil {
		ci = &colIndex{blocks: make(map[int]*rowBlock)}
		st.cols[a.Col] = ci
		st.colIDs = insertSorted(st.colIDs, a.Col)
	}
	id := a.Row >> blockShift
	b := ci.blocks[id]
	if b == nil {
		b = &rowBlock{}
		ci.blocks[id] = b
		ci.ids = insertSorted(ci.ids, id)
	}
	bit := a.Row & (blockRows - 1)
	b.bits[bit>>6] |= 1 << (bit & 63)
	b.n++
	ci.n++
}

func (st *cellStore) unmark(a Addr) {
	ci := st.cols[a.Col]
	id := a.Row >> blockShift
	b := ci.blocks[id]
	bit := a.Row & (blockRows - 1)
	b.bits[bit>>6] &^= 1 << (bit & 63)
	b.n--
	ci.n--
	if b.n == 0 {
		delete(ci.blocks, id)
		ci.ids = removeSorted(ci.ids, id)
	}
	if ci.n == 0 {
		delete(st.cols, a.Col)
		st.colIDs = removeSorted(st.colIDs, a.Col)
	}
}

func insertSorted(s []int, v int) []int {
	i, found := slices.BinarySearch(s, v)
	if found {
		return s
	}
	return slices.Insert(s, i, v)
}

func removeSorted(s []int, v int) []int {
	if i, found := slices.BinarySearch(s, v); found {
		return slices.Delete(s, i, i+1)
	}
	return s
}

// len is the number of cells stored, formatting-only ones included.
func (st *cellStore) len() int { return len(st.m) }

// all yields every stored cell.
func (st *cellStore) all() iter.Seq2[Addr, *Cell] { return maps.All(st.m) }

// colsIn returns the stored columns from c0 to c1, ascending. The slice
// is shared: callers mustn't keep it across changes.
func (st *cellStore) colsIn(c0, c1 int) []int {
	i, _ := slices.BinarySearch(st.colIDs, c0)
	j, _ := slices.BinarySearch(st.colIDs, c1+1)
	return st.colIDs[i:j]
}

// colCells is the number of cells stored in column c.
func (st *cellStore) colCells(c int) int {
	if ci := st.cols[c]; ci != nil {
		return ci.n
	}
	return 0
}

// inRange yields the stored cells in r in row-major order, visiting only
// the rows of each column that hold cells.
func (st *cellStore) inRange(r Rect) iter.Seq2[Addr, *Cell] {
	return func(yield func(Addr, *Cell) bool) {
		cols := slices.Clone(st.colsIn(r.From.Col, r.To.Col))
		if len(cols) == 0 || r.From.Row > r.To.Row {
			return
		}
		b0, b1 := r.From.Row>>blockShift, r.To.Row>>blockShift
		for _, id := range st.blockIDs(cols, b0, b1) {
			var in []colBlock
			for _, c := range cols {
				if b := st.cols[c].blocks[id]; b != nil {
					in = append(in, colBlock{c, b})
				}
			}
			if !st.yieldBlock(in, id, r, yield) {
				return
			}
		}
	}
}

type colBlock struct {
	col int
	b   *rowBlock
}

// blockIDs returns the numbers from b0 to b1 of the blocks that any of
// cols has, ascending.
func (st *cellStore) blockIDs(cols []int, b0, b1 int) []int {
	if len(cols) == 1 {
		ids := st.cols[cols[0]].ids
		i, _ := slices.BinarySearch(ids, b0)
		j, _ := slices.BinarySearch(ids, b1+1)
		return slices.Clone(ids[i:j])
	}
	var out []int
	for _, c := range cols {
		ids := st.cols[c].ids
		i, _ := slices.BinarySearch(ids, b0)
		j, _ := slices.BinarySearch(ids, b1+1)
		out = append(out, ids[i:j]...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// yieldBlock yields the cells of block id in the columns in, row by row,
// limited to the rows of r.
func (st *cellStore) yieldBlock(in []colBlock, id int, r Rect, yield func(Addr, *Cell) bool) bool {
	base := id << blockShift
	lo, hi := max(r.From.Row-base, 0), min(r.To.Row-base, blockRows-1)
	for w := lo >> 6; w <= hi>>6; w++ {
		var any uint64
		for _, cb := range in {
			any |= cb.b.bits[w]
		}
		any &= wordMask(w, lo, hi)
		for any != 0 {
			bit := bits.TrailingZeros64(any)
			any &^= 1 << bit
			row := base + w<<6 + bit
			for _, cb := range in {
				if cb.b.bits[w]&(1<<bit) == 0 {
					continue
				}
				a := Addr{Col: cb.col, Row: row}
				if c := st.m[a]; c != nil && !yield(a, c) {
					return false
				}
			}
		}
	}
	return true
}

// wordMask keeps the bits of word w that fall in rows lo..hi of a block.
func wordMask(w, lo, hi int) uint64 {
	m := ^uint64(0)
	if first := w << 6; lo > first {
		m &= ^uint64(0) << (lo - first)
	}
	if last := w<<6 + 63; hi < last {
		m &= ^uint64(0) >> (last - hi)
	}
	return m
}

// colScan calls fn with each row from r0 to r1 of column c that holds a
// cell, in order, until fn returns false, and reports whether it went to
// the end. It allocates nothing; the column's cells must not be added
// or removed meanwhile.
func (st *cellStore) colScan(c, r0, r1 int, fn func(row int) bool) bool {
	ci := st.cols[c]
	if ci == nil || r0 > r1 {
		return true
	}
	i, _ := slices.BinarySearch(ci.ids, r0>>blockShift)
	for ; i < len(ci.ids) && ci.ids[i] <= r1>>blockShift; i++ {
		id := ci.ids[i]
		b, base := ci.blocks[id], id<<blockShift
		lo, hi := max(r0-base, 0), min(r1-base, blockRows-1)
		for w := lo >> 6; w <= hi>>6; w++ {
			x := b.bits[w] & wordMask(w, lo, hi)
			for x != 0 {
				bit := bits.TrailingZeros64(x)
				x &^= 1 << bit
				if !fn(base + w<<6 + bit) {
					return false
				}
			}
		}
	}
	return true
}

// nextRow returns the first row of column c at or after row (dir 1), or
// at or before it (dir -1), that holds a cell.
func (st *cellStore) nextRow(c, row, dir int) (int, bool) {
	ci := st.cols[c]
	if ci == nil || row < 0 {
		return 0, false
	}
	id := row >> blockShift
	i, _ := slices.BinarySearch(ci.ids, id)
	if dir > 0 {
		for ; i < len(ci.ids); i++ {
			b := ci.ids[i]
			from := 0
			if b == id {
				from = row & (blockRows - 1)
			}
			if r, ok := ci.blocks[b].first(from); ok {
				return b<<blockShift + r, true
			}
		}
		return 0, false
	}
	if i == len(ci.ids) || ci.ids[i] != id {
		i--
	}
	for ; i >= 0; i-- {
		b := ci.ids[i]
		to := blockRows - 1
		if b == id {
			to = row & (blockRows - 1)
		}
		if r, ok := ci.blocks[b].last(to); ok {
			return b<<blockShift + r, true
		}
	}
	return 0, false
}

// first is the lowest set row at or after from.
func (b *rowBlock) first(from int) (int, bool) {
	for w := from >> 6; w < blockWords; w++ {
		x := b.bits[w]
		if w == from>>6 {
			x &= ^uint64(0) << (from & 63)
		}
		if x != 0 {
			return w<<6 + bits.TrailingZeros64(x), true
		}
	}
	return 0, false
}

// last is the highest set row at or before to.
func (b *rowBlock) last(to int) (int, bool) {
	for w := to >> 6; w >= 0; w-- {
		x := b.bits[w]
		if w == to>>6 {
			x &= ^uint64(0) >> (63 - to&63)
		}
		if x != 0 {
			return w<<6 + 63 - bits.LeadingZeros64(x), true
		}
	}
	return 0, false
}

// bounds returns the smallest range holding every stored cell of r, and
// false if r holds none. It costs a few steps per stored column of r.
func (st *cellStore) bounds(r Rect) (Rect, bool) {
	out, found := Rect{}, false
	for _, c := range st.colsIn(r.From.Col, r.To.Col) {
		top, ok := st.nextRow(c, r.From.Row, 1)
		if !ok || top > r.To.Row {
			continue
		}
		bottom, _ := st.nextRow(c, r.To.Row, -1)
		if !found {
			out, found = Rect{From: Addr{Col: c, Row: top}, To: Addr{Col: c, Row: bottom}}, true
			continue
		}
		out.From.Row, out.To.Row = min(out.From.Row, top), max(out.To.Row, bottom)
		out.To.Col = c
	}
	return out, found
}
