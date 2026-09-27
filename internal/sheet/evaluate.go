package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Evaluation is lazy and recursive: a formula reading a dirty cell
// evaluates it first, so a chain of formulas (each row adding to the row
// above) recurses once per link. The parser caps how deeply one formula
// nests (formula.MaxDepth), but a chain can run through every cell of a
// sheet, two million links, which would take more than the goroutine's
// stack. So evaluation counts how deep it is (w.depth: cells and
// operators, nested), and a cell it reaches past maxEvalDepth is put off:
// the evaluation in progress is abandoned (its cells go back to dirty),
// the cell is evaluated on its own, from the top of the stack, and then
// what was abandoned is tried again, finding it done. Values come out as
// if evaluated in one go; a long chain costs about twice the work.
//
// Cells put off are "deferred" until they're evaluated: each depends on
// the one put off after it, so reaching one again is a cycle, as reaching
// a cell being evaluated is.

// maxEvalDepth bounds evaluation's nesting, in cells and operators: about
// 30 MB of stack. A var so tests can lower it.
var maxEvalDepth = 1 << 16

// tooDeep abandons an evaluation that went past maxEvalDepth.
type tooDeep struct{}

// evaluator is the state of one evaluate.
type evaluator struct {
	w       *Workbook
	stack   []loc // cells being evaluated, outermost first
	pending []loc // cells put off, each needed by the one before
}

// evaluate computes the cells marked dirty in each sheet's calc. Cells
// are evaluated lazily in dependency order: reading a dirty cell
// evaluates it first. A cell that is reached again while it is still
// being evaluated is part of a cycle and becomes ERR. The state lives on
// the sheets, keyed by address as on one sheet, and each sheet's lookup
// is made once, so evaluating a formula allocates nothing for sheets.
func (w *Workbook) evaluate() {
	w.Circular = false
	e := &evaluator{w: w}
	for _, s := range w.sheets {
		s.version++
		s.hidden.valid = false // values may have changed what the filter hides
		s.calcGet = w.lookupOn(s, e.compute)
		s.calcFmt = w.formatFrom(s)
	}
	for _, s := range w.sheets {
		for a, st := range s.calc {
			if st == dirty {
				e.settle(loc{s, a})
			}
		}
	}
	for _, s := range w.sheets {
		s.calc, s.calcGet, s.calcFmt = nil, nil, nil
	}
}

// compute returns the value of the cell at a, evaluating it first if it's
// dirty. Every cell a formula reads comes through here, so it looks each
// map up once: a SUM over 8192 cells makes 8192 calls.
func (e *evaluator) compute(s *Sheet, a Addr) Value {
	w := e.w
	c := s.cells.get(a)
	switch st := s.calc[a]; {
	case c == nil:
		return Value{}
	case st == visiting, st == deferred:
		w.Circular = true
		return ErrRef
	case st != dirty:
		return c.Value
	case c.derived: // a pivot's result, set when the pivot was computed
		s.calc[a] = done
		return c.Value
	case w.depth >= maxEvalDepth:
		s.calc[a] = deferred
		e.pending = append(e.pending, loc{s, a})
		panic(tooDeep{})
	}
	s.calc[a] = visiting
	e.stack = append(e.stack, loc{s, a})
	w.depth++
	c.auto = Format{}
	switch {
	case c.Input == "":
		c.Value = Value{}
	case c.expr == nil && c.Format.Kind == FmtText:
		c.Value = Value{Kind: Text, Str: c.Input}
	case c.expr == nil:
		c.Value = Value{Kind: Text, Str: strings.TrimPrefix(c.Input, "'")}
	default:
		expr := s.bound(c)
		outer := w.evaluating
		w.evaluating = loc{s, a}
		c.Value = eval(w.arith(expr), s.calcGet)
		w.evaluating = outer
		if _, lit := expr.(formula.Num); !lit {
			c.auto = inferFormat(expr, s.calcFmt)
		}
	}
	w.depth--
	e.stack = e.stack[:len(e.stack)-1]
	s.calc[a] = done
	s.cells.changed(a)
	return c.Value
}

// settle evaluates the dirty cell at l, and every cell put off on the
// way, deepest first.
func (e *evaluator) settle(l loc) {
	e.pending = append(e.pending[:0], l)
	for len(e.pending) > 0 {
		top := e.pending[len(e.pending)-1]
		if top.s.calc[top.a] == deferred {
			top.s.calc[top.a] = dirty
		}
		if e.try(top) {
			e.pending = e.pending[:len(e.pending)-1]
		}
	}
}

// try evaluates the cell at l, reporting false if it went too deep and
// put off a cell, having undone what it had started.
func (e *evaluator) try(l loc) (finished bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, deep := r.(tooDeep); !deep {
				panic(r)
			}
			for _, v := range e.stack {
				v.s.calc[v.a] = dirty
			}
			for _, p := range e.pending {
				p.s.calc[p.a] = deferred
			}
			e.stack = e.stack[:0]
			e.w.depth = 0
			e.w.evaluating = loc{}
			finished = false
		}
	}()
	e.compute(l.s, l.a)
	return true
}
