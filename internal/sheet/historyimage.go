package sheet

import "slices"

// An image holds an undo step's before-images of one sheet's cells in
// the form the sheet keeps them (store.go, slot.go): columns of blocks
// of 16-byte slots, the text in a table of strings kept once each, the
// formatting in a table of looks, and whole Cells only for rich cells
// (formulas, notes). A step that clears
// or rewrites a whole sheet so holds about what the sheet does, 20 B
// per number, rather than a Cell per cell. A cell that held nothing
// before the step is a bit in blank.
//
// The columns are indexed by position (occupancy.go), which costs a few
// KB for the first cell far down a sheet, so an image holds its first
// smallImage cells in a list and moves them into columns past that:
// the typical step, one cell or a few, costs what they do.
type image struct {
	small []imageCell  // the cells while there are at most smallImage
	few   [2]imageCell // small's first cells, without an allocation
	big   bool         // the cells are in cells and blank instead
	// cells holds the tables of strings, looks and rich cells, and the
	// columns once big; nil until one is needed, as a plain number
	// needs none.
	cells *cellStore
	blank occupancy
	// looks maps the sheet's looks to the image's: entry i is the
	// image's look for the sheet's look i, plus one; 0 for not yet
	// seen. A store's looks are only ever added, so the map holds.
	looks []uint16
}

// imageCell is a before-image in an image's list: the slot, in the
// image's tables, or blank for a cell that held nothing.
type imageCell struct {
	a     Addr
	sl    slot
	blank bool
}

// smallImage is how many cells an image lists before it keeps them in
// columns: a list's linear search stays cheaper than an index up to
// about this many.
const smallImage = 32

// Estimated heap of an image's parts beyond slotBytes a cell, as
// BenchmarkClearMax and BenchmarkHistoryWide measure it.
const (
	blockBytes = 256 // a block of blockRows rows, made for its first cell
	richBytes  = 24  // a rich cell's entry in the side table
)

// seen reports whether the image holds the cell at a, blank or not.
func (img *image) seen(a Addr) bool {
	if img.big {
		return img.cells.stored.has(a) || img.blank.has(a) // big has cells
	}
	for i := range img.small {
		if img.small[i].a == a {
			return true
		}
	}
	return false
}

// len is the number of cells the image holds, blank ones included.
func (img *image) len() int {
	n := len(img.small) + img.blank.n
	if img.cells != nil {
		n += img.cells.stored.n
	}
	return n
}

// store is img.cells, made if need be.
func (img *image) store() *cellStore {
	if img.cells == nil {
		img.cells = &cellStore{}
	}
	return img.cells
}

// tables is img.cells to read from: an empty store when there is none,
// as a slot then refers to no table.
func (img *image) tables() *cellStore {
	if img.cells == nil {
		return &noTables
	}
	return img.cells
}

// noTables is an empty store, never written.
var noTables cellStore

// keep records the cell at a of src, the sheet's store, as it stands,
// and returns the heap that adds.
func (img *image) keep(src *cellStore, a Addr) int64 {
	b, i := src.find(a)
	if b == nil {
		return img.put(a, slot{}, true)
	}
	sl := b.vals[i]
	if sl.kind == slotRich {
		return img.keepRich(a, src.rich[sl.ref].c.clone())
	}
	n := int64(slotBytes)
	if sl.look != 0 {
		lk, ok := img.look(src, sl.look)
		if !ok { // the image's table of looks is full
			return img.keepRich(a, src.view(sl))
		}
		sl.look = lk
	}
	if sl.ref != 0 {
		s := src.strs.strs[sl.ref]
		t := &img.store().strs
		sl.ref = t.add(s)
		if t.refs[sl.ref] == 1 {
			n += strBytes + int64(len(s))
		}
	}
	return n + img.put(a, sl, false)
}

// keepRich records c, a rich cell or one made to stand for a plain one,
// as the before-image at a.
func (img *image) keepRich(a Addr, c *Cell) int64 {
	n := int64(slotBytes+richBytes) + cellSize(c)
	return n + img.put(a, slot{kind: slotRich, ref: img.store().addRich(a, c)}, false)
}

// put stores sl at a, or a blank there, which the image doesn't hold,
// and returns the heap new blocks take.
func (img *image) put(a Addr, sl slot, blank bool) int64 {
	if !img.big {
		if img.small == nil {
			img.small = img.few[:0]
		}
		if len(img.small) < smallImage {
			img.small = append(img.small, imageCell{a, sl, blank})
			return 0
		}
		n := img.grow()
		return n + img.put(a, sl, blank)
	}
	o := &img.store().stored
	if blank {
		o = &img.blank
	}
	var n int64
	if o.col(a.Col).block(a.Row>>blockShift) == nil {
		n = blockBytes
	}
	b := o.mark(a)
	if !blank {
		i, _ := b.index(a.Row & (blockRows - 1))
		b.vals = slices.Insert(b.vals, i, sl)
	}
	return n
}

// grow moves the listed cells into columns and returns the heap their
// blocks take.
func (img *image) grow() int64 {
	img.big = true
	var n int64
	for _, ic := range img.small {
		n += img.put(ic.a, ic.sl, ic.blank)
	}
	img.small = nil
	return n
}

// look is the image's entry for the sheet's look id, added if new, and
// false when the image's table is full.
func (img *image) look(src *cellStore, id uint16) (uint16, bool) {
	if int(id) < len(img.looks) && img.looks[id] != 0 {
		return img.looks[id] - 1, true
	}
	l := src.looks[id]
	lk, ok := img.store().lookID(l)
	if !ok {
		return 0, false
	}
	if int(id) >= len(img.looks) {
		img.looks = slices.Grow(img.looks, int(id)+1-len(img.looks))[:int(id)+1]
	}
	img.looks[id] = lk + 1
	return lk, true
}

// each calls fn with every cell the image holds and its before-image as
// a Cell to place, nil for blank: a rich cell itself (the image is
// dropped once restored), a plain one made for the caller. fn mustn't
// change the image.
func (img *image) each(fn func(a Addr, c *Cell)) {
	st := img.tables()
	for _, ic := range img.small {
		var c *Cell
		if !ic.blank {
			c = st.cellOf(ic.sl)
		}
		fn(ic.a, c)
	}
	for _, col := range st.stored.colIDs {
		st.stored.colScan(col, 0, MaxRows-1, func(row int) bool {
			a := Addr{Col: col, Row: row}
			b, i := st.find(a)
			fn(a, st.cellOf(b.vals[i]))
			return true
		})
	}
	for _, col := range img.blank.colIDs {
		img.blank.colScan(col, 0, MaxRows-1, func(row int) bool {
			fn(Addr{Col: col, Row: row}, nil)
			return true
		})
	}
}

// dropBlanks forgets the blank before-images of cells that are still
// blank in s, and returns the heap that frees: the blocks emptied.
func (img *image) dropBlanks(s *Sheet) int64 {
	img.small = slices.DeleteFunc(img.small, func(ic imageCell) bool { return ic.blank && !s.cells.has(ic.a) })
	var drop []Addr
	for _, c := range img.blank.colIDs {
		img.blank.colScan(c, 0, MaxRows-1, func(row int) bool {
			if a := (Addr{Col: c, Row: row}); !s.cells.has(a) {
				drop = append(drop, a)
			}
			return true
		})
	}
	blocks := img.blank.blocks()
	for _, a := range drop {
		img.blank.unmark(a)
	}
	return int64(blocks-img.blank.blocks()) * blockBytes
}

// blocks is the number of blocks the occupancy holds.
func (o *occupancy) blocks() int {
	n := 0
	for _, c := range o.colIDs {
		n += len(o.cols[c].ids)
	}
	return n
}
