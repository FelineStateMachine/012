package functions

import (
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Parts of a formula evaluated where they stand, for stepping through it
// as Excel's Evaluate Formula does (the engine's steps.go). Rather than a
// second evaluator, the formula is evaluated once as usual with each
// part wrapped in a probe, which records what the part computed there:
// in its array context or not, inside LET with its names bound, and not
// at all in an IF's branch not taken. Only operators, calls and LAMBDA
// calls are wrapped: functions look at their arguments' nodes only to
// tell references, ranges, names and literals apart, which stay as they
// are, so a probe changes nothing a function sees.

// Part is what one part of a formula computed the first time it was
// computed.
type Part struct {
	Value Value
	// Array holds the first rows and columns of an array the part
	// computed, with Rows and Cols its whole size; nil otherwise.
	Array *Array
	// Lambda is set when the part computed a LAMBDA.
	Lambda bool
	// Reached is false for a part the formula never computed.
	Reached bool
}

// previewSize is how many rows and columns of an array a Part keeps.
const previewSize = 8

// probe is a part of a formula being recorded (see Part).
type probe struct {
	x     Node
	i     int
	parts []Part
}

// EvalParts evaluates n in the cell at, as EvalCell does, and returns
// what it computed, with the array it spills, and what each part at
// paths computed. A path is child
// indexes from the root, in formula.EachChild's order; the empty path is
// the whole formula. A path to a reference, name or literal records
// nothing: those are read where they stand.
func EvalParts(n Node, paths [][]int, get *Reader, at Addr) (Part, []Part) {
	parts := make([]Part, len(paths))
	for i, p := range paths {
		n = wrapAt(n, p, probe{i: i, parts: parts})
	}
	top := Part{Value: EvalCell(n, get, at), Reached: true}
	if sp := get.Spilled(); sp != nil {
		top.Array = preview(sp)
	}
	return top, parts
}

// probed reports whether a part of this kind is wrapped in a probe.
func probed(n Node) bool {
	switch n.(type) {
	case formula.Unary, formula.Binary, formula.Call, formula.Invoke, decUnary, decBinary:
		return true
	}
	return false
}

// ReplaceAt returns a copy of n with the node at path, child indexes in
// formula.EachChild's order, replaced by what f makes of it. The nodes
// on the way are copied; n itself is left as it is.
func ReplaceAt(n Node, path []int, f func(Node) Node) Node {
	if len(path) == 0 {
		return f(n)
	}
	return withChild(n, path[0], func(k Node) Node { return ReplaceAt(k, path[1:], f) })
}

// wrapAt returns n with the node at path wrapped in p, when it is one
// probed.
func wrapAt(n Node, path []int, p probe) Node {
	if len(path) == 0 {
		if !probed(n) {
			return n
		}
		p.x = n
		return p
	}
	return withChild(n, path[0], func(k Node) Node { return wrapAt(k, path[1:], p) })
}

// withChild returns a copy of n with its child i replaced by f's result,
// looking through probes already in place.
func withChild(n Node, i int, f func(Node) Node) Node {
	switch n := n.(type) {
	case probe:
		n.x = withChild(n.x, i, f)
		return n
	case formula.Unary:
		n.X = f(n.X)
		return n
	case decUnary:
		n.X = f(n.X)
		return n
	case formula.Binary:
		n.L, n.R = pick(i, n.L, n.R, f)
		return n
	case decBinary:
		n.L, n.R = pick(i, n.L, n.R, f)
		return n
	case formula.Call:
		n.Args = slices.Clone(n.Args)
		n.Args[i] = f(n.Args[i])
		return n
	case formula.Invoke:
		if i == 0 {
			n.Fn = f(n.Fn)
			return n
		}
		n.Args = slices.Clone(n.Args)
		n.Args[i-1] = f(n.Args[i-1])
		return n
	case formula.Array:
		rows := slices.Clone(n.Rows)
		for r, row := range rows {
			if i < len(row) {
				rows[r] = slices.Clone(row)
				rows[r][i] = f(row[i])
				break
			}
			i -= len(row)
		}
		n.Rows = rows
		return n
	}
	return n
}

// pick applies f to l or r, by i.
func pick(i int, l, r Node, f func(Node) Node) (Node, Node) {
	if i == 0 {
		return f(l), r
	}
	return l, f(r)
}

// eval computes the part and records it, the first time.
func (p probe) eval(get lookup) Value {
	v := eval(p.x, get)
	if pt := &p.parts[p.i]; !pt.Reached {
		pt.Value, pt.Reached = v, true
		if arr := get.arrayOf(v); arr != nil {
			pt.Array = preview(arr)
		}
		pt.Lambda = get.lambdaOf(v) != nil
	}
	return v
}

// preview copies the first rows and columns of a.
func preview(a *Array) *Array {
	rows, cols := min(a.Rows, previewSize), min(a.Cols, previewSize)
	out := &Array{Rows: a.Rows, Cols: a.Cols, DRows: rows, DCols: cols, V: make([]Value, rows*cols), Fill: a.Fill}
	for r := range rows {
		for c := range cols {
			out.V[r*cols+c] = a.At(r, c)
		}
	}
	return out
}
