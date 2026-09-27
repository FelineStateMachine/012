package sheet

import (
	"iter"
	"maps"
)

// cellStore holds a sheet's cells: those with contents or formatting, by
// address. The rest of the engine reads and writes cells only through
// these methods, never the map beneath, so the representation can change
// (to column blocks of compact values, with formulas and formats in side
// tables; see docs/limits.md) without touching it.
//
// Iteration order is unspecified. Cells may be set or deleted while
// iterating, with a map's rules: a deleted cell not yet reached isn't
// yielded, a new one may or may not be.
type cellStore struct {
	m     map[Addr]*Cell
	stats *statsIndex // statistics by block, once a selection needs them; see stats.go
}

func newCellStore() cellStore { return cellStore{m: make(map[Addr]*Cell)} }

// get returns the cell at a, or nil.
func (st *cellStore) get(a Addr) *Cell { return st.m[a] }

// set stores c, which isn't nil, at a.
func (st *cellStore) set(a Addr, c *Cell) {
	st.m[a] = c
	st.stats.touch(a)
}

// delete removes the cell at a, if any.
func (st *cellStore) delete(a Addr) {
	delete(st.m, a)
	st.stats.touch(a)
}

// changed notes that the value of the cell at a was recalculated.
func (st *cellStore) changed(a Addr) { st.stats.touch(a) }

// len is the number of cells stored, formatting-only ones included.
func (st *cellStore) len() int { return len(st.m) }

// all yields every stored cell.
func (st *cellStore) all() iter.Seq2[Addr, *Cell] { return maps.All(st.m) }

// inRange yields the stored cells in r, visiting whichever is cheaper:
// each address of r (a lookup each), or every stored cell (iterating the
// map costs about a quarter of a lookup per cell).
func (st *cellStore) inRange(r Rect) iter.Seq2[Addr, *Cell] {
	return func(yield func(Addr, *Cell) bool) {
		area := (r.To.Col - r.From.Col + 1) * (r.To.Row - r.From.Row + 1)
		if area <= len(st.m)/4 {
			for row := r.From.Row; row <= r.To.Row; row++ {
				for col := r.From.Col; col <= r.To.Col; col++ {
					a := Addr{Col: col, Row: row}
					if c := st.m[a]; c != nil && !yield(a, c) {
						return
					}
				}
			}
			return
		}
		for a, c := range st.m {
			if r.Contains(a) && !yield(a, c) {
				return
			}
		}
	}
}
