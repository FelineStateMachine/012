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

// NamedSheets returns the sheet names the formula at a names, as written
// and without repeats (in any case), or nil for anything else. Names of
// sheets that don't exist are included: exporters check them.
func (s *Sheet) NamedSheets(a Addr) []string {
	c := s.cells.get(a)
	if c == nil || len(c.xrefs) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(name string) {
		if k := formula.SheetKey(name); name != "" && !seen[k] {
			seen[k] = true
			out = append(out, name)
		}
	}
	formula.WalkRefs(c.expr, func(n string, _ Addr) { add(n) }, func(n string, _ Rect) { add(n) })
	return out
}

// ShiftEntry returns an entry as if it were typed in one cell and copied
// dc columns and dr rows away: a formula's relative references move and
// $absolute ones stay, as in a paste. Other entries come back unchanged.
// A macro recorded with relative references replays formulas this way.
func ShiftEntry(input string, dc, dr int) (string, error) {
	if dc == 0 && dr == 0 || !IsFormulaEntry(input) {
		return input, nil
	}
	n, err := Parse(input)
	if err != nil {
		return "", err
	}
	if moved, changed := formula.Rewrite(n, formula.Shift(dc, dr)); changed {
		return formula.Text(moved), nil
	}
	return input, nil
}
