package sheet

import (
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
)

// Steps is a formula evaluated one part at a time, as Excel's Evaluate
// Formula: the part computed next is marked in the formula, computing it
// puts its value in its place, and a reference to another formula can be
// stepped into. The values come from one evaluation by the function
// library with each part recorded where it stands (functions.EvalParts),
// so they are what the formula computes, context and all, and a branch
// IF didn't take is never computed. References are read as the formula
// reads them: the cell's value.
type Steps struct {
	sheet *Sheet
	at    Addr
	expr  Node   // the formula as written, which Text prints
	parts []part // in the order they're computed: operands first, left to right
	next  int    // the part computed next; len(parts) once done
	top   functions.Part
	index map[string]int // parts by pathKey, while references are read
}

// part is one part of the formula: an operator, a call or a reference.
type part struct {
	path []int
	node Node // as written
	functions.Part
	into loc // a reference to a formula's cell, which can be stepped into
}

// EvaluateSteps starts evaluating the formula at a step by step, or
// returns nil when a holds no formula.
func (s *Sheet) EvaluateSteps(a Addr) *Steps {
	c, _, _ := s.cells.peek(a)
	if c == nil || !c.IsFormula() {
		return nil
	}
	st := &Steps{sheet: s, at: a, expr: c.expr}
	var paths [][]int
	st.collect(c.expr, nil, func(p []int, n Node) {
		st.parts = append(st.parts, part{path: p, node: n})
		paths = append(paths, p)
	})
	run := s.wb.arith(s.bound(a, c))
	var parts []functions.Part
	st.top, parts = functions.EvalParts(run, paths, s.wb.values(s).lib, a)
	for i := range st.parts {
		st.parts[i].Part = parts[i]
	}
	st.index = make(map[string]int, len(st.parts))
	for i, p := range st.parts {
		st.index[pathKey(p.path)] = i
	}
	st.readRefs()
	st.index = nil
	st.parts = slices.DeleteFunc(st.parts, func(p part) bool { return !p.Reached })
	return st
}

// collect calls fn with the path of each part of n, operands before
// what they're operands of: operators, calls, LAMBDA calls, and
// references to one cell, written or named.
func (st *Steps) collect(n Node, path []int, fn func([]int, Node)) {
	i := 0
	formula.EachChild(n, func(k Node) {
		st.collect(k, append(slices.Clip(path), i), fn)
		i++
	})
	switch n := n.(type) {
	case formula.Unary, formula.Binary, formula.Call, formula.Invoke, formula.Ref:
		fn(path, n)
	case formula.Name:
		if nm, ok := st.sheet.wb.names[nameKey(n.Name)]; ok && !nm.Gone() && nm.Range.From == nm.Range.To {
			fn(path, n)
		}
	case formula.TableRef:
		if _, ok := st.sheet.bindTable(st.at, n, 0).(formula.Ref); ok {
			fn(path, n) // one cell, as Sales[@Amount] is
		}
	}
}

// readRefs reads the references among the parts, as the formula reads
// them, where the formula reached them.
func (st *Steps) readRefs() {
	for i := range st.parts {
		p := &st.parts[i]
		t, a, ok := st.refCell(p.node)
		if !ok || !st.reached(p.path) {
			continue
		}
		p.Reached = true
		if t == nil {
			p.Value = ErrRef
			continue
		}
		p.Value = t.Value(a)
		if c, _, _ := t.cells.peek(a); c != nil && c.IsFormula() {
			p.into = loc{t, a}
		}
	}
}

// refCell is the cell a reference, single-cell name or table reference
// points at, and
// its sheet (nil for a sheet no sheet has the name of).
func (st *Steps) refCell(n Node) (*Sheet, Addr, bool) {
	s := st.sheet
	switch n := n.(type) {
	case formula.Ref:
		return s.wb.resolve(s, n.Sheet), n.Addr, true
	case formula.Name:
		nm := s.wb.names[nameKey(n.Name)]
		return nm.Sheet, nm.Range.From, true
	case formula.TableRef:
		if r, ok := s.bindTable(st.at, n, 0).(formula.Ref); ok {
			return s.wb.resolve(s, r.Sheet), r.Addr, true
		}
	}
	return nil, Addr{}, false
}

// reached reports whether the formula computed what's at path, a
// reference: when the part it's in was computed, and, for an argument of
// IF, IFERROR, IFNA or CHOOSE, when that call took it.
func (st *Steps) reached(path []int) bool {
	for up := len(path) - 1; up >= 0; up-- {
		i, ok := st.index[pathKey(path[:up])]
		if !ok {
			continue // an array literal: go on out
		}
		o := &st.parts[i]
		if !o.Reached {
			return false
		}
		if up == len(path)-1 {
			return st.taken(o, path[up])
		}
		return true
	}
	return true
}

// taken reports whether the call o, computed, took its argument arg.
func (st *Steps) taken(o *part, arg int) bool {
	call, ok := o.node.(formula.Call)
	if !ok || arg == 0 {
		return true
	}
	first := st.valueAt(append(slices.Clip(o.path), 0), call.Args[0])
	switch call.Fn.Signature().Name {
	case "IF":
		if first.Kind == Error || first.Kind == Text {
			return false
		}
		return (first.Num != 0) == (arg == 1)
	case "IFERROR":
		return first.Kind == Error
	case "IFNA":
		return first == ErrNA
	case "CHOOSE":
		return first.Kind == Number && int(first.Num) == arg
	}
	return true
}

// valueAt is what the node n at path computed: a part's recorded value,
// or a literal's own.
func (st *Steps) valueAt(path []int, n Node) Value {
	if i, ok := st.index[pathKey(path)]; ok && st.parts[i].Reached {
		return st.parts[i].Value
	}
	if t, a, ok := st.refCell(n); ok && t != nil {
		return t.Value(a)
	}
	return functions.EvalAt(n, st.sheet.wb.values(st.sheet).lib, st.at)
}

// Cell is the formula's cell.
func (st *Steps) Cell() (*Sheet, Addr) { return st.sheet, st.at }

// Done reports whether every part is computed.
func (st *Steps) Done() bool { return st.next >= len(st.parts) }

// Progress is how many parts are computed, of how many.
func (st *Steps) Progress() (done, total int) { return st.next, len(st.parts) }

// Step computes the next part.
func (st *Steps) Step() {
	if !st.Done() {
		st.next++
	}
}

// Restart goes back to the formula as written.
func (st *Steps) Restart() { st.next = 0 }

// Next is the part computed next, as written, and its value as the
// formula would write it; ok is false once done.
func (st *Steps) Next() (expr, value string, ok bool) {
	if st.Done() {
		return "", "", false
	}
	p := st.parts[st.next]
	return formula.Expr(p.node), partText(p.Part), true
}

// Result is the formula's value, an array it spills written whole.
func (st *Steps) Result() string { return partText(st.top) }

// CanStepIn reports whether the next part is a reference to a formula's
// cell, which Into steps into.
func (st *Steps) CanStepIn() bool { return !st.Done() && st.parts[st.next].into.s != nil }

// Into starts stepping through the formula the next part refers to,
// when it is a reference to a formula's cell; nil otherwise.
func (st *Steps) Into() *Steps {
	if !st.CanStepIn() {
		return nil
	}
	l := st.parts[st.next].into
	return l.s.EvaluateSteps(l.a)
}
