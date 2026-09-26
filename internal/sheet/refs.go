package sheet

// Reference rewriting: copying a formula shifts its relative references,
// and moving or inserting and deleting cells updates every reference to
// the cells that moved, as Sheets does. A reference that can't be kept
// becomes #REF!.

// refRewrite maps the references in a formula. Either function may return
// refErrNode.
type refRewrite struct {
	ref func(refNode) Node
	rng func(rangeNode) Node
}

// rewrite returns n with its references mapped, and whether anything
// changed. Unchanged subtrees are shared.
func rewrite(n Node, rw refRewrite) (Node, bool) {
	switch n := n.(type) {
	case refNode:
		out := rw.ref(n)
		return out, out != Node(n)
	case rangeNode:
		out := rw.rng(n)
		return out, out != Node(n)
	case unaryNode:
		x, ok := rewrite(n.x, rw)
		if ok {
			n.x = x
		}
		return n, ok
	case binaryNode:
		l, okL := rewrite(n.l, rw)
		r, okR := rewrite(n.r, rw)
		if okL || okR {
			n.l, n.r = l, r
		}
		return n, okL || okR
	case callNode:
		var args []Node
		for i, a := range n.args {
			if b, ok := rewrite(a, rw); ok {
				if args == nil {
					args = append([]Node(nil), n.args...)
				}
				args[i] = b
			}
		}
		if args == nil {
			return n, false
		}
		n.args = args
		return n, true
	}
	return n, false
}

// shiftRefs is the rewrite for copying a formula by (dc, dr): relative
// parts move, $absolute parts stay, and references pushed off the sheet
// become #REF!.
func shiftRefs(dc, dr int) refRewrite {
	move := func(a Addr, abs absFlags) (Addr, bool) {
		if abs&absCol == 0 {
			a.Col += dc
		}
		if abs&absRow == 0 {
			a.Row += dr
		}
		return a, a.Valid()
	}
	return refRewrite{
		ref: func(n refNode) Node {
			a, ok := move(n.a, n.abs)
			if !ok {
				return refErrNode{}
			}
			return refNode{a, n.abs}
		},
		rng: func(n rangeNode) Node {
			from, ok1 := move(n.r.From, n.abs[0])
			to, ok2 := move(n.r.To, n.abs[1])
			if !ok1 || !ok2 {
				return refErrNode{}
			}
			return newRange(from, to, n.abs[0], n.abs[1])
		},
	}
}

// relocate is the rewrite for cells that move: cell maps where each cell
// went (false if it's gone), and rng maps whole ranges.
func relocate(cell func(Addr) (Addr, bool), rng func(Rect) (Rect, bool)) refRewrite {
	return refRewrite{
		ref: func(n refNode) Node {
			a, ok := cell(n.a)
			if !ok {
				return refErrNode{}
			}
			return refNode{a, n.abs}
		},
		rng: func(n rangeNode) Node {
			r, ok := rng(n.r)
			if !ok {
				return refErrNode{}
			}
			return rangeNode{r, n.abs}
		},
	}
}

// withFormula returns a copy of c holding expression n, printed as its
// new input. Other fields (formats, say) come along unchanged.
func (c *Cell) withFormula(n Node) *Cell {
	cp := c.clone()
	cp.Input = formulaText(n)
	cp.setExpr(n)
	return cp
}

// rewritten returns c with its references rewritten, or c itself if none
// changed.
func (c *Cell) rewritten(rw refRewrite) *Cell {
	if c.expr == nil || (len(c.refs) == 0 && len(c.ranges) == 0) {
		return c
	}
	n, changed := rewrite(c.expr, rw)
	if !changed {
		return c
	}
	return c.withFormula(n)
}

// span inserts (n > 0) or deletes (n < 0) lines starting at index at,
// along an axis with size lines.
type span struct{ at, n, size int }

// point maps a line index; false if the line was deleted or pushed off
// the end.
func (sp span) point(v int) (int, bool) {
	switch {
	case v < sp.at:
		return v, true
	case sp.n < 0 && v < sp.at-sp.n:
		return 0, false
	}
	v += sp.n
	return v, v < sp.size
}

// interval maps a range's lines lo..hi as Sheets does: inserting inside
// the range grows it, deleting part of it shrinks it, and deleting all of
// it leaves #REF!.
func (sp span) interval(lo, hi int) (int, int, bool) {
	if sp.n > 0 {
		lo, okLo := sp.point(lo)
		if !okLo {
			return 0, 0, false
		}
		hi, _ := sp.point(hi)
		return lo, min(hi, sp.size-1), true
	}
	end := sp.at - sp.n // first line after the deletion
	if lo >= sp.at && hi < end {
		return 0, 0, false
	}
	if lo >= sp.at {
		lo = max(lo, end) + sp.n
	}
	if hi >= sp.at {
		hi = max(hi+sp.n, sp.at-1)
	}
	return lo, hi, true
}

// axisRewrite is the cell and range mapping for inserting or deleting rows
// (rows true) or columns.
func axisRewrite(rows bool, sp span) (func(Addr) (Addr, bool), func(Rect) (Rect, bool)) {
	cell := func(a Addr) (Addr, bool) {
		var ok bool
		if rows {
			a.Row, ok = sp.point(a.Row)
		} else {
			a.Col, ok = sp.point(a.Col)
		}
		return a, ok
	}
	rng := func(r Rect) (Rect, bool) {
		var ok bool
		if rows {
			r.From.Row, r.To.Row, ok = sp.interval(r.From.Row, r.To.Row)
		} else {
			r.From.Col, r.To.Col, ok = sp.interval(r.From.Col, r.To.Col)
		}
		return r, ok
	}
	return cell, rng
}
