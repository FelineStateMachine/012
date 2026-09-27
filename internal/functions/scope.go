package functions

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Names LET and LAMBDA bind (Local nodes), and calling a LAMBDA: the
// scope a formula is evaluated in. The functions themselves are in
// lambda.go.

// binding is a name LET or LAMBDA bound: to a reference or range (ref),
// which then reads as the range does wherever it's used, or to a value.
type binding struct {
	name string
	ref  Node
	val  Value
}

// lookupName finds the innermost binding of name.
func (rd *Reader) lookupName(name string) (binding, bool) {
	for i := len(rd.scope) - 1; i >= 0; i-- {
		if strings.EqualFold(rd.scope[i].name, name) {
			return rd.scope[i], true
		}
	}
	return binding{}, false
}

// local is the value of a bound name.
func (rd *Reader) local(name string) Value {
	b, ok := rd.lookupName(name)
	switch {
	case !ok:
		return value.ErrName
	case b.ref != nil:
		return eval(b.ref, rd)
	}
	return b.val
}

// refOf is the reference or range n stands for when a function reads it
// whole: n itself, a name bound to one, or the argument an entry being
// mapped over stands in for.
func (rd *Reader) refOf(n Node) Node {
	switch l := n.(type) {
	case formula.Local:
		if b, ok := rd.lookupName(l.Name); ok && b.ref != nil {
			return b.ref
		}
	case liftArg:
		return unlift(n)
	}
	return n
}

// bind evaluates what a name is bound to: a reference or range stays
// one, read as it would be where the name is used, except in an array
// context, which reads a range whole.
func (rd *Reader) bind(name string, n Node) binding {
	if rd.lift == 0 {
		switch r := rd.refOf(n).(type) {
		case formula.Ref, formula.Range:
			return binding{name: name, ref: r}
		}
	}
	want := rd.wantArr
	rd.wantArr = true
	v := eval(n, rd)
	rd.wantArr = want
	return binding{name: name, val: v}
}

// invoke evaluates a LAMBDA call written in the formula: LAMBDA(...)(...)
// or a name bound to a LAMBDA with arguments.
func (rd *Reader) invoke(n formula.Invoke) Value {
	want := rd.wantArr
	rd.wantArr = true
	fn := eval(n.Fn, rd)
	rd.wantArr = want
	l := rd.lambdaOf(fn)
	switch {
	case fn.Kind == value.Error:
		return fn
	case l == nil:
		return value.ErrValue
	case len(n.Args) != len(l.params):
		return value.ErrNA // Sheets: wrong number of arguments
	}
	vals := make([]binding, len(n.Args))
	for i, a := range n.Args {
		vals[i] = rd.bind(l.params[i], a)
	}
	return rd.apply(l, vals)
}

// apply evaluates a LAMBDA's expression with its parameters bound, in
// the scope it was written in.
func (rd *Reader) apply(l *lambda, args []binding) Value {
	saved := rd.scope
	rd.scope = append(append([]binding(nil), l.scope...), args...)
	want := rd.wantArr
	rd.wantArr = true
	v := eval(l.body, rd)
	rd.wantArr = want
	rd.scope = saved
	return v
}

// call calls a LAMBDA with values, for the functions that take one,
// outside any array context: its expression sees the values it's given.
func (rd *Reader) call(l *lambda, vals ...Value) Value {
	args := make([]binding, len(vals))
	for i, v := range vals {
		args[i] = binding{name: l.params[i], val: v}
	}
	lift := rd.lift
	rd.lift = 0
	v := rd.apply(l, args)
	rd.lift = lift
	return v
}

// lambdaArg reads a LAMBDA argument taking n values: #N/A when it takes
// another number, as Sheets says.
func lambdaArg(n Node, get lookup, params int) (*lambda, *Value) {
	v := wholeArg(n, get)
	if v.Kind == value.Error {
		return nil, errOf(v)
	}
	l := get.lambdaOf(v)
	switch {
	case l == nil:
		return nil, &value.ErrValue
	case len(l.params) != params:
		return nil, &value.ErrNA
	}
	return l, nil
}

// single is what a LAMBDA called for one entry returns: an array there
// is #VALUE!, as Sheets says a LAMBDA must return one value.
func (rd *Reader) single(v Value) Value {
	if v.Kind == value.Array {
		return value.ErrValue
	}
	return v
}
