package sheet

import "github.com/FelineStateMachine/012/internal/formula"

// References in a stored formula are rewritten (see formula.Rewrite) by
// building a new cell: stored cells are never changed in place.

// withFormula returns a copy of c holding expression n, printed as its
// new input. Other fields (formats, say) come along unchanged.
func (c *Cell) withFormula(n Node) *Cell {
	cp := c.clone()
	cp.Input = formula.Text(n)
	cp.setExpr(n)
	return cp
}

// rewritten returns c with its references rewritten, or c itself if none
// changed.
func (c *Cell) rewritten(rw formula.Rewriter) *Cell {
	if c.expr == nil || (len(c.refs) == 0 && len(c.ranges) == 0 && len(c.names) == 0 && len(c.xrefs) == 0) {
		return c
	}
	n, changed := formula.Rewrite(c.expr, rw)
	if !changed {
		return c
	}
	return c.withFormula(n)
}
