package functions

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// In an array context (ARRAYFORMULA, or an argument that takes ranges) a
// function of one value given arrays is applied to each of their
// entries, as Sheets does: ARRAYFORMULA(LEN(A1:A3)) is the length of
// each cell, and ARRAYFORMULA(VLOOKUP(A1:A3, D:E, 2, FALSE)) looks each
// one up. Which arguments are mapped over isn't listed anywhere: the
// function is called once with its arguments standing in (liftArg), and
// those it reads as one value are the ones mapped over, while those it
// reads whole (VLOOKUP's table, SUMIF's range) stay whole. So
// SUM(LEN(A1:A3)) adds three lengths, and SUMIF(A:A, D1:D3) inside
// ARRAYFORMULA sums once per criterion.

// liftFrame is one function call being mapped over arrays: its
// arguments, their values in the array context once read, and the entry
// the call is computing.
type liftFrame struct {
	args []Node
	vals []Value
	got  []bool // vals[i] is computed
	read []bool // argument i was read as one value, and is an array
	r, c int    // the entry being computed
	fill bool   // computing the value past every argument's data
}

// liftArg stands for argument i of a call being mapped over arrays.
// Read as one value (eval), it is the entry being computed; read whole
// (matrixArg, eachOf, arrayArg), it is the argument itself.
type liftArg struct {
	f *liftFrame
	i int
}

// value is the argument in the array context: evaluated once.
func (a liftArg) value(get lookup) Value {
	f := a.f
	if !f.got[a.i] {
		f.vals[a.i], f.got[a.i] = evalArr(f.args[a.i], get), true
	}
	return f.vals[a.i]
}

// one is the argument's entry for the call being computed.
func (a liftArg) one(get lookup) Value {
	v := a.value(get)
	arr := get.arrayOf(v)
	if arr == nil {
		return v
	}
	f := a.f
	f.read[a.i] = true
	if f.fill {
		return arr.Fill
	}
	return arr.bcast(f.r, f.c)
}

// unlift is the node an argument stands for, when it is a reference or
// range a function reads whole, or n itself.
func unlift(n Node) Node {
	if a, ok := n.(liftArg); ok {
		switch orig := a.f.args[a.i].(type) {
		case formula.Range, formula.Ref:
			return orig
		}
	}
	return n
}

// liftCall calls f with args in an array context, mapping it over the
// arrays it reads as one value.
func liftCall(f *FuncDef, args []Node, get lookup) Value {
	fr := &liftFrame{args: args, vals: make([]Value, len(args)), got: make([]bool, len(args)), read: make([]bool, len(args))}
	stand := make([]Node, len(args))
	for i, a := range args {
		switch a.(type) {
		case formula.Num, formula.Str, formula.Bool, formula.Empty, formula.Ref:
			stand[i] = a // one value however it's read
		default:
			stand[i] = liftArg{fr, i}
		}
	}
	lift, want := get.lift, get.wantArr
	get.lift, get.wantArr = 0, f.arrays == liftPass
	defer func() { get.lift, get.wantArr = lift, want }()
	whole := f.call(stand, get)
	v := get.first(whole)
	var s shape
	var arrs []*Array
	for i, r := range fr.read {
		if r {
			a := get.arrayOf(fr.vals[i])
			s.fit(a)
			arrs = append(arrs, a)
		}
	}
	if len(arrs) == 0 {
		if f.arrays == liftPass {
			return whole // mapped over nothing: an array it returns stays whole
		}
		return v
	}
	s.data(arrs...)
	out, ok := s.newArray()
	if !ok {
		return value.ErrValue
	}
	for i := range s.drows {
		for j := range s.dcols {
			if i == 0 && j == 0 { // the call above
				out.Set(0, 0, v)
				continue
			}
			fr.r, fr.c = i, j
			out.Set(i, j, get.first(f.call(stand, get)))
		}
	}
	if s.drows < s.rows || s.dcols < s.cols {
		fr.fill = true
		out.Fill = get.first(f.call(stand, get))
	}
	return get.arrayValue(out)
}
