package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Conditional formats and validation move with cut and paste, as in
// Sheets: the cut cells take their rules to where they land, on the same
// sheet or another, the cells they land on lose theirs, and formulas in
// the rules follow the cells they read.

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
	keptPart, movedPart := splitMoved(rs, src, dst, d)
	kept, moved := keptPart.ranges, movedPart.ranges
	switch {
	case len(moved) == 0 && len(kept) == 0:
		return nil
	case len(moved) == 0:
		return []rulePart{keptPart}
	case len(kept) == 0:
		return []rulePart{movedPart}
	case split:
		return []rulePart{keptPart, movedPart}
	}
	return []rulePart{{ranges: append(kept, moved...)}}
}

// splitMoved is the part of ranges rs that stays when src moves to dst
// (d away), outside both, and the part that moves, each with the shift
// from rs's first cell to where the part's first cell was.
func splitMoved(rs []Rect, src, dst Rect, d Addr) (kept, moved rulePart) {
	for _, r := range rs {
		for _, k := range subtract(r, src) {
			kept.ranges = append(kept.ranges, subtract(k, dst)...)
		}
		if in, ok := overlap(r, src); ok {
			moved.ranges = append(moved.ranges, Rect{From: plus(in.From, d), To: plus(in.To, d)})
		}
	}
	from := anchor(rs)
	kept.shift = minus(anchor(kept.ranges), from)
	moved.shift = minus(minus(anchor(moved.ranges), d), from)
	return kept, moved
}

// moveRules moves the rules of the cells in src to the sheet they land
// on: they leave the source sheet's rules, the cells they land on leave
// the destination's, and their parts of the source's rules join the
// destination's, each rule after the destination's own, as a copy's
// rules would. Formulas in every sheet's rules follow the cells they
// read, as the formulas in cells do, and the moved rules' name the
// source sheet for cells that stayed behind.
func (mv sheetMove) moveRules() {
	var landed rulesState
	for _, t := range mv.from.wb.sheets {
		if t.rules.empty() {
			continue
		}
		next := mv.splitRules(t, &landed)
		if t == mv.to {
			next.formats = append(next.formats, landed.formats...)
			next.validations = append(next.validations, landed.validations...)
			landed = rulesState{}
		}
		if !next.equal(t.rules) {
			t.recordRules()
			t.rules = next
		}
	}
	if !landed.empty() { // the destination had no rules of its own
		mv.to.recordRules()
		mv.to.rules = landed
	}
}

// splitRules is t's rules after the move: what stays on t, rewritten for
// the cells that moved, with the parts that move added to landed.
func (mv sheetMove) splitRules(t *Sheet, landed *rulesState) rulesState {
	d := minus(mv.d.From, mv.src.From)
	nowhere := Rect{From: Addr{Col: -1, Row: -1}, To: Addr{Col: -1, Row: -1}}
	src, dst := nowhere, nowhere
	switch t {
	case mv.from:
		src = mv.src
	case mv.to:
		dst = mv.d
	}
	rw, away := mv.rewrite(t, t), mv.rewrite(mv.from, mv.to)
	var next rulesState
	for _, f := range t.rules.formats {
		kept, moved := splitMoved(f.Ranges, src, dst, d)
		if len(kept.ranges) > 0 {
			next.formats = append(next.formats, movedFormat(f, kept, rw))
		}
		if len(moved.ranges) > 0 {
			landed.formats = append(landed.formats, movedFormat(f, moved, away))
		}
	}
	for _, v := range t.rules.validations {
		kept, moved := splitMoved(v.Ranges, src, dst, d)
		if len(kept.ranges) > 0 {
			next.validations = append(next.validations, movedValidation(v, kept, rw))
		}
		if len(moved.ranges) > 0 {
			landed.validations = append(landed.validations, movedValidation(v, moved, away))
		}
	}
	return next
}

// movedFormat is f over part p's ranges, its formula written for their
// first cell and rewritten by rw.
func movedFormat(f CondFormat, p rulePart, rw formula.Rewriter) CondFormat {
	g := f.clone()
	g.Ranges = p.ranges
	if f.Op == RuleFormula {
		g.Args[0] = reanchor(f.Args[0], p.shift, rw)
	} else {
		for k := range f.Op.Args() {
			if IsFormulaEntry(strings.TrimSpace(f.Args[k])) {
				g.Args[k] = reanchor(f.Args[k], p.shift, rw)
			}
		}
	}
	return g
}

// movedValidation is movedFormat for a validation rule.
func movedValidation(v Validation, p rulePart, rw formula.Rewriter) Validation {
	w := v.clone()
	w.Ranges = p.ranges
	switch v.Kind {
	case ValidFormula:
		w.Args[0] = reanchor(v.Args[0], p.shift, rw)
	case ValidRange:
		w.Source = strings.TrimPrefix(shiftFormula("="+v.Source, rw), "=")
	}
	return w
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
