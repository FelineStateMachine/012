package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Conditional formats and validation move with cut and paste on a sheet,
// as in Sheets: the cut cells take their rules to where they land, the
// cells they land on lose theirs, and formulas in the rules follow the
// cells they read.

// moveRules moves the sheet's rules with the cells in src, landing on
// dst; cell and rng are the move's, for the rules' formulas.
func (s *Sheet) moveRules(src, dst Rect, cell func(Addr) (Addr, bool), rng func(Rect) (Rect, bool)) {
	if s.rules.empty() {
		return
	}
	next := s.rules.moved(src, dst, formula.Relocate(s.onThis(s), cell, rng))
	if !next.equal(s.rules) {
		s.recordRules()
		s.rules = next
	}
}

// moved is the rules after the cells in src move to dst, with their
// formulas rewritten by rw. A rule whose cells are partly moved keeps one
// range list, or, when its formula is relative to its first cell, splits
// into a rule for the cells that stayed and one for those that moved,
// each with the formula written for its own first cell.
func (r rulesState) moved(src, dst Rect, rw formula.Rewriter) rulesState {
	d := Addr{Col: dst.From.Col - src.From.Col, Row: dst.From.Row - src.From.Row}
	var out rulesState
	for _, f := range r.formats {
		for _, p := range movedParts(f.Ranges, src, dst, d, f.Op == RuleFormula) {
			g := f
			g.Ranges = p.ranges
			if f.Op == RuleFormula {
				g.Args[0] = reanchor(f.Args[0], p.shift, rw)
			}
			out.formats = append(out.formats, g)
		}
	}
	for _, v := range r.validations {
		for _, p := range movedParts(v.Ranges, src, dst, d, v.Kind == ValidFormula) {
			w := v.clone()
			w.Ranges = p.ranges
			switch v.Kind {
			case ValidFormula:
				w.Args[0] = reanchor(v.Args[0], p.shift, rw)
			case ValidRange:
				w.Source = strings.TrimPrefix(shiftFormula("="+v.Source, rw), "=")
			}
			out.validations = append(out.validations, w)
		}
	}
	return out
}

// rulePart is the ranges of a rule after a move, and how far its
// formula's relative references shift: from the rule's first cell to
// where the part's first cell was before the move.
type rulePart struct {
	ranges []Rect
	shift  Addr
}

// movedParts splits a rule's ranges rs by the move of src to dst (d
// away): the cells that stay, outside both, and those that move. split
// keeps the two apart, for a rule with a relative formula.
func movedParts(rs []Rect, src, dst Rect, d Addr, split bool) []rulePart {
	var kept, moved []Rect
	for _, r := range rs {
		for _, k := range subtract(r, src) {
			kept = append(kept, subtract(k, dst)...)
		}
		if in, ok := overlap(r, src); ok {
			moved = append(moved, Rect{From: plus(in.From, d), To: plus(in.To, d)})
		}
	}
	from := anchor(rs)
	keptPart := func() rulePart { return rulePart{kept, minus(anchor(kept), from)} }
	movedPart := func() rulePart { return rulePart{moved, minus(minus(anchor(moved), d), from)} }
	switch {
	case len(moved) == 0 && len(kept) == 0:
		return nil
	case len(moved) == 0:
		return []rulePart{keptPart()}
	case len(kept) == 0:
		return []rulePart{movedPart()}
	case split:
		return []rulePart{keptPart(), movedPart()}
	}
	return []rulePart{{ranges: append(kept, moved...)}}
}

// reanchor rewrites a rule's formula (with its "=") for a first cell
// shift away, then for the move (rw).
func reanchor(src string, shift Addr, rw formula.Rewriter) string {
	if shift != (Addr{}) {
		src = shiftFormula(src, formula.Shift(shift.Col, shift.Row))
	}
	return shiftFormula(src, rw)
}

func plus(a, d Addr) Addr  { return Addr{Col: a.Col + d.Col, Row: a.Row + d.Row} }
func minus(a, d Addr) Addr { return Addr{Col: a.Col - d.Col, Row: a.Row - d.Row} }

// overlap is the cells a and b share, and whether they share any.
func overlap(a, b Rect) (Rect, bool) {
	r := Rect{
		From: Addr{Col: max(a.From.Col, b.From.Col), Row: max(a.From.Row, b.From.Row)},
		To:   Addr{Col: min(a.To.Col, b.To.Col), Row: min(a.To.Row, b.To.Row)},
	}
	return r, r.From.Col <= r.To.Col && r.From.Row <= r.To.Row
}
