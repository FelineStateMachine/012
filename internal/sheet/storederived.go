package sheet

import (
	"iter"
	"slices"
)

// The store's side of derived cells (slot.go): finding the pivot's
// results, reading what a derived cell shows without making a Cell, and
// writing a spilled value straight into its slot.

// isPivot reports whether sl holds a pivot's result.
func (st *cellStore) isPivot(sl slot) bool {
	if sl.kind == slotRich {
		return st.rich[sl.ref].c.derived
	}
	return sl.kind&slotPivot != 0
}

// countPivot counts the cell at a, which was a pivot's result or not and
// becomes one or not.
func (st *cellStore) countPivot(a Addr, was, is bool) {
	switch {
	case is && !was:
		if st.pivots++; st.pivots == 1 {
			st.pivotArea = Rect{From: a, To: a}
		} else {
			st.pivotArea = union(st.pivotArea, Rect{From: a, To: a})
		}
	case was && !is:
		st.pivots--
	}
}

// pivotKeys yields the addresses of the pivot's results stored.
func (st *cellStore) pivotKeys() iter.Seq[Addr] {
	return func(yield func(Addr) bool) {
		if st.pivots == 0 {
			return
		}
		for a := range st.anyKeysIn(st.pivotArea) {
			if b, i := st.find(a); b != nil && st.isPivot(b.vals[i]) && !yield(a) {
				return
			}
		}
	}
}

// derivedOf returns what the cell at a holds as a derived cell would:
// its value, format, style and inferred format, and slotPivot or
// slotSpill for a pivot's result or a spilled cell, 0 for any other
// cell (none included). It makes no Cell, so the derived cells being
// rewritten are compared cheaply; the look mustn't be changed.
func (st *cellStore) derivedOf(a Addr) (v Value, l *look, kind uint8) {
	b, i := st.find(a)
	if b == nil {
		return Value{}, &noLook, 0
	}
	sl := b.vals[i]
	if sl.kind != slotRich {
		return st.slotValue(sl), st.lookOf(sl), sl.kind & slotDerived
	}
	c := st.rich[sl.ref].c
	return c.Value, &look{c.Format, c.Style, c.auto}, richDerived(c)
}

// richDerived is derivedOf's kind for a rich cell.
func richDerived(c *Cell) uint8 {
	switch {
	case c.derived:
		return slotPivot
	case c.spilled:
		return slotSpill
	}
	return 0
}

// derivedAt is derivedOf's kind alone.
func (st *cellStore) derivedAt(a Addr) uint8 {
	b, i := st.find(a)
	switch {
	case b == nil:
		return 0
	case b.vals[i].kind == slotRich:
		return richDerived(st.rich[b.vals[i].ref].c)
	}
	return b.vals[i].kind & slotDerived
}

// autoAt is the format the cell at a infers: a formula's or a spilled
// cell's.
func (st *cellStore) autoAt(a Addr) Format {
	b, i := st.find(a)
	if b == nil {
		return Format{}
	}
	if sl := b.vals[i]; sl.kind == slotRich {
		return st.rich[sl.ref].c.auto
	} else {
		return st.lookOf(sl).auto
	}
}

// setSpilled makes the cell at a, blank, formatting only or spilled, a
// spilled cell showing v with the inferred format auto, keeping its
// formatting, straight in its slot. It reports false, changing nothing,
// for a cell a slot can't hold (one with a note), which the caller
// stores as a Cell.
func (st *cellStore) setSpilled(a Addr, v Value, auto Format) bool {
	b, i := st.find(a)
	var old slot
	if b != nil {
		old = b.vals[i]
	}
	if old.kind == slotRich {
		return false
	}
	lk := old.look
	if l := st.lookOf(old); l.auto != auto {
		var ok bool
		if lk, ok = st.lookID(look{l.f, l.st, auto}); !ok {
			return false
		}
	}
	if b == nil {
		b = st.stored.mark(a)
		i, _ = b.index(a.Row & (blockRows - 1))
		b.vals = slices.Insert(b.vals, i, slot{kind: slotBlank})
	}
	st.countPivot(a, st.isPivot(old), false)
	sl := st.derivedValue(v, lk, false) // before releasing old, which may share its string
	st.releaseSlot(old)
	b.vals[i] = sl
	st.filledAs(a, st.filled.has(a), !derivedBlank(v))
	st.touch(a)
	return true
}

// derivedBlank reports whether a derived cell showing v is blank: its
// entry, the value's text, is empty.
func derivedBlank(v Value) bool {
	return v.Kind == Empty || v.Kind == Text && v.Str == ""
}
