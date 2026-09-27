package formula

// Reference rewriting: copying a formula shifts its relative references,
// and moving or inserting and deleting cells updates every reference to
// the cells that moved, as Sheets does. A reference that can't be kept
// becomes #REF!.

// Rewriter maps the references in a formula. Any function may return
// RefErr; a nil function leaves those nodes alone.
type Rewriter struct {
	Ref   func(Ref) Node
	Range func(Range) Node
	Name  func(Name) Node
}

// Rewrite returns n with its references mapped, and whether anything
// changed. Unchanged subtrees are shared.
func Rewrite(n Node, rw Rewriter) (Node, bool) {
	switch n := n.(type) {
	case Ref, Range, Name:
		out := rw.leaf(n)
		return out, out != n
	case Unary:
		x, ok := Rewrite(n.X, rw)
		if ok {
			n.X = x
		}
		return n, ok
	case Binary:
		l, okL := Rewrite(n.L, rw)
		r, okR := Rewrite(n.R, rw)
		if okL || okR {
			n.L, n.R = l, r
		}
		return n, okL || okR
	case Call:
		return rewriteCall(n, rw)
	}
	return n, false
}

// leaf maps a reference, range or name.
func (rw Rewriter) leaf(n Node) Node {
	switch n := n.(type) {
	case Ref:
		if rw.Ref != nil {
			return rw.Ref(n)
		}
	case Range:
		if rw.Range != nil {
			return rw.Range(n)
		}
	case Name:
		if rw.Name != nil {
			return rw.Name(n)
		}
	}
	return n
}

// rewriteCall rewrites a call's arguments, copying them only if one
// changed.
func rewriteCall(n Call, rw Rewriter) (Node, bool) {
	var args []Node
	for i, a := range n.Args {
		if b, ok := Rewrite(a, rw); ok {
			if args == nil {
				args = append([]Node(nil), n.Args...)
			}
			args[i] = b
		}
	}
	if args == nil {
		return n, false
	}
	n.Args = args
	return n, true
}

// Shift is the rewrite for copying a formula by (dc, dr): relative parts
// move, $absolute parts stay, and references pushed off the sheet become
// #REF!.
func Shift(dc, dr int) Rewriter {
	move := func(a Addr, abs Abs) (Addr, bool) {
		if abs&AbsCol == 0 {
			a.Col += dc
		}
		if abs&AbsRow == 0 {
			a.Row += dr
		}
		return a, a.Valid()
	}
	return Rewriter{
		Ref: func(n Ref) Node {
			a, ok := move(n.Addr, n.Abs)
			if !ok {
				return RefErr{}
			}
			return Ref{a, n.Abs, n.Sheet}
		},
		Range: func(n Range) Node {
			from, ok1 := move(n.Rect.From, n.Abs[0])
			to, ok2 := move(n.Rect.To, n.Abs[1])
			if !ok1 || !ok2 {
				return RefErr{}
			}
			r := NewRange(from, to, n.Abs[0], n.Abs[1])
			r.Sheet = n.Sheet
			return r
		},
	}
}

// Relocate is the rewrite for cells that move on one sheet: cell maps
// where each cell went (false if it's gone), and rng maps whole ranges.
// on reports whether a reference written with a sheet name ("" for none)
// points at the sheet whose cells moved; other references stay.
func Relocate(on func(sheet string) bool, cell func(Addr) (Addr, bool), rng func(Rect) (Rect, bool)) Rewriter {
	return Rewriter{
		Ref: func(n Ref) Node {
			if !on(n.Sheet) {
				return n
			}
			a, ok := cell(n.Addr)
			if !ok {
				return RefErr{}
			}
			return Ref{a, n.Abs, n.Sheet}
		},
		Range: func(n Range) Node {
			if !on(n.Sheet) {
				return n
			}
			r, ok := rng(n.Rect)
			if !ok {
				return RefErr{}
			}
			return Range{r, n.Abs, n.Sheet}
		},
	}
}

// Span inserts (N > 0) or deletes (N < 0) lines starting at index At,
// along an axis with Size lines.
type Span struct{ At, N, Size int }

// Point maps a line index; false if the line was deleted or pushed off
// the end.
func (sp Span) Point(v int) (int, bool) {
	switch {
	case v < sp.At:
		return v, true
	case sp.N < 0 && v < sp.At-sp.N:
		return 0, false
	}
	v += sp.N
	return v, v < sp.Size
}

// Interval maps a range's lines lo..hi as Sheets does: inserting inside
// the range grows it, deleting part of it shrinks it, and deleting all of
// it leaves #REF!.
func (sp Span) Interval(lo, hi int) (int, int, bool) {
	if sp.N > 0 {
		lo, okLo := sp.Point(lo)
		if !okLo {
			return 0, 0, false
		}
		hi, _ := sp.Point(hi)
		return lo, min(hi, sp.Size-1), true
	}
	end := sp.At - sp.N // first line after the deletion
	if lo >= sp.At && hi < end {
		return 0, 0, false
	}
	if lo >= sp.At {
		lo = max(lo, end) + sp.N
	}
	if hi >= sp.At {
		hi = max(hi+sp.N, sp.At-1)
	}
	return lo, hi, true
}

// AxisMaps are the cell and range mappings for inserting or deleting
// rows (rows true) or columns, for Relocate.
func AxisMaps(rows bool, sp Span) (func(Addr) (Addr, bool), func(Rect) (Rect, bool)) {
	cell := func(a Addr) (Addr, bool) {
		var ok bool
		if rows {
			a.Row, ok = sp.Point(a.Row)
		} else {
			a.Col, ok = sp.Point(a.Col)
		}
		return a, ok
	}
	rng := func(r Rect) (Rect, bool) {
		var ok bool
		if rows {
			r.From.Row, r.To.Row, ok = sp.Interval(r.From.Row, r.To.Row)
		} else {
			r.From.Col, r.To.Col, ok = sp.Interval(r.From.Col, r.To.Col)
		}
		return r, ok
	}
	return cell, rng
}
