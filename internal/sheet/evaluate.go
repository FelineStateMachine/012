package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
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
	// Running aggregates shared by the recalculation (rangememo.go). A
	// cell put off abandons an extension part way: its checkpoints stop
	// at the last row it finished, and the next read goes on from there.
	memo := &aggMemo{w: w, m: map[aggKey]*runAgg{}}
	for _, s := range w.sheets {
		s.version++
		s.hidden.valid = false // values may have changed what the filter hides
		s.calcGet = w.recalcReader(s, e, memo)
		s.calcFmt = w.formatFrom(s)
	}
	for _, s := range w.sheets {
		for !e.sweep(s) {
			e.settle()
		}
	}
	for _, s := range w.sheets {
		s.calcGet.read, s.calcGet.memo = nil, nil
		s.calcGet.lib.Forget()
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
	case c.derived || c.spilled: // a pivot's or an array's result, set when it was computed
		s.calc[a] = done
		return c.Value
	case c.expr != nil && w.depth >= maxEvalDepth:
		s.calc[a] = deferred
		e.pending = append(e.pending, loc{s, a})
		panic(tooDeep{})
	}
	c.auto = Format{}
	switch {
	case c.Input == "":
		c.Value = Value{}
	case c.expr == nil && c.Format.Kind == FmtText:
		c.Value = Value{Kind: Text, Str: c.Input}
	case c.expr == nil:
		c.Value = Value{Kind: Text, Str: strings.TrimPrefix(c.Input, "'")}
	default:
		if n, lit := c.expr.(formula.Num); lit { // a number typed in
			c.Value = num(n.V)
		} else {
			e.formula(s, a, c)
		}
	}
	s.calc[a] = done
	s.cells.changed(a)
	return c.Value
}

// formula evaluates the formula in c, at a, which may read other cells:
// it's visiting meanwhile.
func (e *evaluator) formula(s *Sheet, a Addr, c *Cell) {
	w := e.w
	s.calc[a] = visiting
	w.depth++
	expr := s.bound(c)
	outer := w.evaluating
	w.evaluating = loc{s, a}
	c.Value = functions.EvalCell(w.arith(expr), s.calcGet.lib, a)
	if s.calcGet.lib.Spilled() != nil || s.spills != nil {
		w.noteSpill(s, a, c)
	}
	w.evaluating = outer
	c.auto = functions.InferFormat(expr, s.calcFmt)
	w.depth--
}

// sweep evaluates s's dirty cells, reporting false if it went too deep
// and put off a cell.
func (e *evaluator) sweep(s *Sheet) (finished bool) {
	defer e.abandon(&finished)
	for a, st := range s.calc {
		if st == dirty {
			e.compute(s, a)
		}
	}
	return true
}

// settle evaluates the cells put off, deepest first, putting off more on
// the way if need be.
func (e *evaluator) settle() {
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
	defer e.abandon(&finished)
	e.compute(l.s, l.a)
	return true
}

// abandon, deferred, recovers from an evaluation that went too deep:
// the cells it was evaluating go back to dirty, those put off stay
// deferred, and finished is set false.
func (e *evaluator) abandon(finished *bool) {
	r := recover()
	if r == nil {
		return
	}
	if _, deep := r.(tooDeep); !deep {
		panic(r)
	}
	for _, s := range e.w.sheets {
		for a, st := range s.calc {
			if st == visiting {
				s.calc[a] = dirty
			}
		}
		s.calcGet.lib.Reset()
	}
	for _, p := range e.pending {
		p.s.calc[p.a] = deferred
	}
	e.w.depth = 0
	e.w.evaluating = loc{}
	*finished = false
}
