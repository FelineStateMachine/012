package sheet

import (
	"iter"
	"math"
	"math/bits"
	"slices"
	"strconv"

	"github.com/FelineStateMachine/012/internal/formula"
)

// cellStore holds a sheet's cells: those with contents or formatting, by
// address. The rest of the engine reads and writes cells only through
// these methods, never the representation beneath.
//
// Cells live in columns of blocks of blockRows rows: the stored
// occupancy index (occupancy.go) has a bitmap per block of the rows
// holding a cell, and each block keeps their slots (slot.go) in row
// order, so a cell is found by indexing a column, a block and a popcount,
// without hashing. A plain number costs its 16-byte slot, and so does a
// pivot's or a spill's result (a derived cell, marked in its slot). Rich
// cells (formulas, notes) are whole Cells in a side table the slot
// points at.
//
// get returns a rich cell's own Cell, which recalculation updates in
// place, as ever, and a plain cell as a Cell made for the caller (a
// view): the caller may keep it, but changing it changes nothing
// stored; set is the one way to change a cell. Views cost an allocation
// or three, so what reads many cells asks for what it needs: peek and
// value for values, has and filledAt for presence, richAt for formulas,
// look for formatting, and the key iterators for addresses.
//
// A second occupancy index records which cells have contents, so the
// used range and data edges are found without scanning.
//
// Iteration yields column by column, except inRange and keysIn, which
// yield in row-major order. Cells may be set or deleted while iterating,
// with a map's rules: a deleted cell not yet reached isn't yielded, a new
// one may or may not be.
type cellStore struct {
	stored    occupancy // every cell, with the blocks holding their slots
	filled    occupancy // the cells with contents (not Blank), with their statistics; see stats.go
	statsUsed bool      // a selection has been summed from the statistics index

	strs     strTable        // plain cells' text
	looks    []look          // formats and styles, by slot.look
	lookIdx  map[look]uint16 // looks' entries
	rich     []richCell      // rich cells, by slot.ref
	richFree []uint32        // entries of rich not in use

	// pivots counts the pivot's results stored (Cell.derived), and
	// pivotArea holds them all while there are any, so they are found
	// without visiting every cell.
	pivots    int
	pivotArea Rect
}

func newCellStore() cellStore { return cellStore{} }

// find returns the block holding a's slot and its position there, or nil.
func (st *cellStore) find(a Addr) (*rowBlock, int) {
	b := st.stored.col(a.Col).block(a.Row >> blockShift)
	if b == nil {
		return nil, 0
	}
	i, ok := b.index(a.Row & (blockRows - 1))
	if !ok {
		return nil, 0
	}
	return b, i
}

// get returns the cell at a, or nil: a rich cell itself, a plain one as
// a view.
func (st *cellStore) get(a Addr) *Cell {
	b, i := st.find(a)
	if b == nil {
		return nil
	}
	return st.cellOf(b.vals[i])
}

func (st *cellStore) cellOf(sl slot) *Cell {
	if sl.kind == slotRich {
		return st.rich[sl.ref].c
	}
	return st.view(sl)
}

// peek returns the rich cell at a, or else the plain cell's value, and
// whether a holds a cell at all. It allocates nothing: recalculation
// reads every cell through it.
func (st *cellStore) peek(a Addr) (*Cell, Value, bool) {
	b, i := st.find(a)
	if b == nil {
		return nil, Value{}, false
	}
	sl := b.vals[i]
	if sl.kind == slotRich {
		return st.rich[sl.ref].c, Value{}, true
	}
	return nil, st.slotValue(sl), true
}

// value returns the value of the cell at a.
func (st *cellStore) value(a Addr) Value {
	c, v, _ := st.peek(a)
	if c != nil {
		return c.Value
	}
	return v
}

// has reports whether a holds a cell, if only formatting.
func (st *cellStore) has(a Addr) bool { return st.stored.has(a) }

// filledAt reports whether the cell at a has contents: !get(a).Blank().
func (st *cellStore) filledAt(a Addr) bool { return st.filled.has(a) }

// richAt returns the cell at a if it is rich (a formula or a note, or a
// cell a slot can't hold), or nil.
func (st *cellStore) richAt(a Addr) *Cell {
	b, i := st.find(a)
	if b == nil || b.vals[i].kind != slotRich {
		return nil
	}
	return st.rich[b.vals[i].ref].c
}

// look returns the format and style of the cell at a, and whether there
// is one.
func (st *cellStore) look(a Addr) (Format, Style, bool) {
	b, i := st.find(a)
	if b == nil {
		return Format{}, Style{}, false
	}
	if sl := b.vals[i]; sl.kind == slotRich {
		c := st.rich[sl.ref].c
		return c.Format, c.Style, true
	} else {
		l := st.lookOf(sl)
		return l.f, l.st, true
	}
}

// holds reports whether the cell at a is c: the same Cell if rich, the
// same contents and formatting if plain; nil for none.
func (st *cellStore) holds(a Addr, c *Cell) bool {
	b, i := st.find(a)
	if b == nil || c == nil {
		return b == nil && c == nil
	}
	sl := b.vals[i]
	if sl.kind == slotRich {
		return st.rich[sl.ref].c == c
	}
	if l := st.lookOf(sl); c.Note != "" || c.auto != l.auto || c.Format != l.f || c.Style != l.st {
		return false
	}
	if c.derived != (sl.kind&slotPivot != 0) || c.spilled != (sl.kind&slotSpill != 0) {
		return false
	}
	if sl.kind&slotDerived != 0 {
		return c.expr == nil && c.Value == st.slotValue(sl)
	}
	switch sl.kind {
	case slotText:
		return c.expr == nil && c.Input == st.strs.strs[sl.ref]
	case slotNum:
		e, ok := c.expr.(formula.Num)
		return ok && math.Float64bits(e.V) == math.Float64bits(sl.num) && st.inputIs(sl, c.Input)
	case slotBool:
		e, ok := c.expr.(formula.Bool)
		return ok && e.V == (sl.num != 0) && st.inputIs(sl, c.Input)
	}
	return c.expr == nil && c.Input == ""
}

// inputIs reports whether input is the entry of the plain number or
// boolean sl, without making it.
func (st *cellStore) inputIs(sl slot, input string) bool {
	switch {
	case sl.ref != 0:
		return input == st.strs.strs[sl.ref]
	case sl.kind == slotBool:
		return input == boolText(sl.num != 0)
	}
	var buf [64]byte
	return string(strconv.AppendFloat(buf[:0], sl.num, 'f', int(sl.dec)-1, 64)) == input
}

// copyOf is the cell at a as a Cell the caller owns, or nil.
func (st *cellStore) copyOf(a Addr) *Cell {
	b, i := st.find(a)
	if b == nil {
		return nil
	}
	if sl := b.vals[i]; sl.kind == slotRich {
		return st.rich[sl.ref].c.clone()
	} else {
		return st.view(sl)
	}
}

// set stores c, which isn't nil, at a. A rich c is kept as it is, so
// recalculation can update it in place; a plain one is copied into a
// slot, and its value is its entry's from then on.
func (st *cellStore) set(a Addr, c *Cell) {
	b, i := st.find(a)
	if b == nil {
		b = st.stored.mark(a)
		i, _ = b.index(a.Row & (blockRows - 1))
		b.vals = slices.Insert(b.vals, i, slot{kind: slotBlank})
	}
	old := b.vals[i]
	st.countPivot(a, st.isPivot(old), c.derived)
	sl, ok := st.plainSlot(c) // before releasing old, which may share its strings
	switch {
	case ok:
		st.releaseSlot(old)
	case old.kind == slotRich:
		st.rich[old.ref] = richCell{c, a}
		sl = old
	default:
		st.releaseSlot(old)
		sl = slot{kind: slotRich, ref: st.addRich(a, c)}
	}
	b.vals[i] = sl
	st.filledAs(a, st.filled.has(a), !c.Blank())
	st.touch(a)
}

// setSlot stores sl, a plain slot whose strings and look st already
// counts, at a: set for a slot made directly, as a file's numbers are.
func (st *cellStore) setSlot(a Addr, sl slot) {
	b, i := st.find(a)
	if b == nil {
		b = st.stored.mark(a)
		i, _ = b.index(a.Row & (blockRows - 1))
		b.vals = slices.Insert(b.vals, i, slot{kind: slotBlank})
	}
	st.countPivot(a, st.isPivot(b.vals[i]), false)
	st.releaseSlot(b.vals[i])
	b.vals[i] = sl
	st.filledAs(a, st.filled.has(a), sl.kind != slotBlank)
	st.touch(a)
}

// filledAs updates the index of filled cells for a cell at a that was
// and is filled or not.
func (st *cellStore) filledAs(a Addr, was, is bool) {
	switch {
	case is && !was:
		st.filled.mark(a)
	case was && !is:
		st.filled.unmark(a)
	}
}

// delete removes the cell at a, if any.
func (st *cellStore) delete(a Addr) {
	b, i := st.find(a)
	if b == nil {
		return
	}
	st.touch(a)
	old := b.vals[i]
	st.countPivot(a, st.isPivot(old), false)
	if st.filled.has(a) {
		st.filled.unmark(a)
	}
	b.vals = slices.Delete(b.vals, i, i+1)
	st.stored.unmark(a)
	st.releaseSlot(old)
}

// len is the number of cells stored, formatting-only ones included.
func (st *cellStore) len() int { return st.stored.n }

// filledLen is the number of cells with contents.
func (st *cellStore) filledLen() int { return st.filled.n }

// all yields every stored cell, plain ones as views.
func (st *cellStore) all() iter.Seq2[Addr, *Cell] { return st.cellsOf(st.keys()) }

// keys yields the address of every stored cell, column by column.
func (st *cellStore) keys() iter.Seq[Addr] {
	return st.anyKeysIn(Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}})
}

// richCells yields every rich cell: the formulas, notes and results of
// pivots and spills.
func (st *cellStore) richCells() iter.Seq2[Addr, *Cell] {
	return func(yield func(Addr, *Cell) bool) {
		for i := 0; i < len(st.rich); i++ {
			if rc := st.rich[i]; rc.c != nil && !yield(rc.a, rc.c) {
				return
			}
		}
	}
}

// cellsOf yields the cells at the addresses of keys, skipping any no
// longer stored.
func (st *cellStore) cellsOf(keys iter.Seq[Addr]) iter.Seq2[Addr, *Cell] {
	return func(yield func(Addr, *Cell) bool) {
		for a := range keys {
			if c := st.get(a); c != nil && !yield(a, c) {
				return
			}
		}
	}
}

// bounds returns the smallest range holding every stored cell of r, and
// false if r holds none.
func (st *cellStore) bounds(r Rect) (Rect, bool) { return st.stored.bounds(r) }

// colScan calls fn with each row from r0 to r1 of column c that holds a
// cell; see occupancy.colScan.
func (st *cellStore) colScan(c, r0, r1 int, fn func(row int) bool) bool {
	return st.stored.colScan(c, r0, r1, fn)
}

// colFill writes the rows from r0 to r1 of column c that hold a cell
// into dst, until it is full; see occupancy.colFill.
func (st *cellStore) colFill(c, r0, r1 int, dst []Addr) int {
	return st.stored.colFill(c, r0, r1, dst)
}

// inRange yields the stored cells in r in row-major order, visiting only
// the rows of each column that hold cells.
func (st *cellStore) inRange(r Rect) iter.Seq2[Addr, *Cell] { return st.cellsOf(st.keysIn(r)) }

// valuesIn yields the values of the stored cells in r in row-major
// order, without making views of them.
func (st *cellStore) valuesIn(r Rect) iter.Seq2[Addr, Value] {
	return func(yield func(Addr, Value) bool) {
		for a := range st.keysIn(r) {
			if !yield(a, st.value(a)) {
				return
			}
		}
	}
}

// keysIn yields the addresses of the stored cells in r in row-major
// order.
func (st *cellStore) keysIn(r Rect) iter.Seq[Addr] {
	return func(yield func(Addr) bool) {
		cols := slices.Clone(st.stored.colsIn(r.From.Col, r.To.Col))
		if len(cols) == 0 || r.From.Row > r.To.Row {
			return
		}
		b0, b1 := r.From.Row>>blockShift, r.To.Row>>blockShift
		for _, id := range st.stored.blockIDs(cols, b0, b1) {
			var in []colBlock
			for _, c := range cols {
				if st.stored.col(c).block(id) != nil {
					in = append(in, colBlock{col: c})
				}
			}
			if !st.yieldBlock(in, id, r, yield) {
				return
			}
		}
	}
}

// rangeFill writes the stored cells of r from the cell from on, in
// row-major order, into dst until it is full, and returns how many.
func (st *cellStore) rangeFill(r Rect, from Addr, dst []Addr) int {
	n := 0
	if from.Col > r.From.Col { // the rest of a row first
		for a := range st.keysIn(Rect{From: from, To: Addr{Col: r.To.Col, Row: from.Row}}) {
			dst[n] = a
			if n++; n == len(dst) {
				return n
			}
		}
		from = Addr{Col: r.From.Col, Row: from.Row + 1}
	}
	for a := range st.keysIn(Rect{From: from, To: r.To}) {
		dst[n] = a
		if n++; n == len(dst) {
			break
		}
	}
	return n
}

// anyInRange yields the stored cells in r in no particular order.
func (st *cellStore) anyInRange(r Rect) iter.Seq2[Addr, *Cell] {
	return st.cellsOf(st.anyKeysIn(r))
}

// anyKeysIn yields the addresses of the stored cells in r column by
// column, which walks each block's bitmap once.
func (st *cellStore) anyKeysIn(r Rect) iter.Seq[Addr] {
	return func(yield func(Addr) bool) {
		if r.From.Row > r.To.Row {
			return
		}
		b0, b1 := r.From.Row>>blockShift, r.To.Row>>blockShift
		for _, c := range slices.Clone(st.stored.colsIn(r.From.Col, r.To.Col)) {
			if st.stored.col(c) == nil {
				continue
			}
			for _, id := range st.stored.blockIDs([]int{c}, b0, b1) {
				if !st.yieldBlock([]colBlock{{col: c}}, id, r, yield) {
					return
				}
			}
		}
	}
}

type colBlock struct {
	col int
	b   *rowBlock
}

// yieldBlock yields the addresses of the cells of block id in the
// columns in, row by row, limited to the rows of r. Each word is read
// from the blocks as they are then, and each address checked before it
// is yielded, so cells may be set and deleted meanwhile.
func (st *cellStore) yieldBlock(in []colBlock, id int, r Rect, yield func(Addr) bool) bool {
	base := id << blockShift
	lo, hi := max(r.From.Row-base, 0), min(r.To.Row-base, blockRows-1)
	for w := lo >> 6; w <= hi>>6; w++ {
		var occ uint64
		for i := range in {
			in[i].b = st.stored.col(in[i].col).block(id)
			if in[i].b != nil {
				occ |= in[i].b.bits[w]
			}
		}
		occ &= wordMask(w, lo, hi)
		for occ != 0 {
			bit := bits.TrailingZeros64(occ)
			occ &^= 1 << bit
			row := base + w<<6 + bit
			for _, cb := range in {
				if cb.b == nil || cb.b.bits[w]&(1<<bit) == 0 {
					continue
				}
				if a := (Addr{Col: cb.col, Row: row}); st.stored.has(a) && !yield(a) {
					return false
				}
			}
		}
	}
	return true
}
