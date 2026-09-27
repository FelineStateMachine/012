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
// Next to the map, two occupancy indexes (occupancy.go) record which
// cells are stored and which have contents, so a range is read at the
// cost of what it holds, and the used range and data edges are found
// without scanning.
//
// Iteration order of all is unspecified; inRange yields in row-major
// order. Cells may be set or deleted while iterating, with a map's rules:
// a deleted cell not yet reached isn't yielded, a new one may or may not
// be.
type cellStore struct {
	m      map[Addr]*Cell
	stored occupancy // every cell in m
	filled occupancy // the cells with contents (not Blank)
}

func newCellStore() cellStore { return cellStore{m: make(map[Addr]*Cell)} }

// get returns the cell at a, or nil.
func (st *cellStore) get(a Addr) *Cell { return st.m[a] }

// set stores c, which isn't nil, at a.
func (st *cellStore) set(a Addr, c *Cell) {
	old, ok := st.m[a]
	if !ok {
		st.stored.mark(a)
	}
	switch was, is := ok && !old.Blank(), !c.Blank(); {
	case is && !was:
		st.filled.mark(a)
	case was && !is:
		st.filled.unmark(a)
	}
	st.m[a] = c
}

// delete removes the cell at a, if any.
func (st *cellStore) delete(a Addr) {
	if old, ok := st.m[a]; ok {
		st.stored.unmark(a)
		if !old.Blank() {
			st.filled.unmark(a)
		}
		delete(st.m, a)
	}
}

// len is the number of cells stored, formatting-only ones included.
func (st *cellStore) len() int { return len(st.m) }

// all yields every stored cell.
func (st *cellStore) all() iter.Seq2[Addr, *Cell] { return maps.All(st.m) }

// bounds returns the smallest range holding every stored cell of r, and
// false if r holds none.
func (st *cellStore) bounds(r Rect) (Rect, bool) { return st.stored.bounds(r) }

// colScan calls fn with each row from r0 to r1 of column c that holds a
// cell; see occupancy.colScan.
func (st *cellStore) colScan(c, r0, r1 int, fn func(row int) bool) bool {
	return st.stored.colScan(c, r0, r1, fn)
}

// inRange yields the stored cells in r in row-major order, visiting only
// the rows of each column that hold cells.
func (st *cellStore) inRange(r Rect) iter.Seq2[Addr, *Cell] {
	return func(yield func(Addr, *Cell) bool) {
		cols := slices.Clone(st.stored.colsIn(r.From.Col, r.To.Col))
		if len(cols) == 0 || r.From.Row > r.To.Row {
			return
		}
		b0, b1 := r.From.Row>>blockShift, r.To.Row>>blockShift
		for _, id := range st.stored.blockIDs(cols, b0, b1) {
			var in []colBlock
			for _, c := range cols {
				if b := st.stored.cols[c].blocks[id]; b != nil {
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
