package sheet

import (
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// ExplainError says why the cell at a shows an error, for the context
// line: where the error starts and what went wrong, e.g. "Division by
// zero in B3/0", or "From B5: division by zero in B3/0" when it comes from
// another cell. It is empty for cells without errors.
func (s *Sheet) ExplainError(a Addr) string {
	return s.explain(a, nil)
}

// label names a cell for an explanation, with its sheet when it isn't on
// the sheet being explained: B3 or Sheet2!B3.
func (l loc) label(from *Sheet) string {
	if l.s == from {
		return l.a.String()
	}
	return formula.QuoteSheet(l.s.name) + "!" + l.a.String()
}

// maxExplainExpr caps how much of a formula an explanation quotes.
const maxExplainExpr = 40

func (s *Sheet) explain(a Addr, path []loc) string {
	here := loc{s, a}
	c := s.cells.get(a)
	v := s.Value(a)
	if c == nil || v.Kind != Error || !c.IsFormula() {
		return ""
	}
	// Cells are named relative to the sheet the explanation starts on.
	home := s
	if len(path) > 0 {
		home = path[0].s
	}
	if i := slices.Index(path, here); i >= 0 {
		names := make([]string, 0, len(path)-i+1)
		for _, p := range path[i:] {
			names = append(names, p.label(home))
		}
		return "Circular reference: " + strings.Join(append(names, here.label(home)), " → ")
	}
	path = append(path, here)
	n, from := s.errorOrigin(c.expr, v)
	if from != nil {
		why := from.s.explain(from.a, path)
		name := from.label(home)
		if why == "" {
			return "From " + name
		}
		if strings.HasPrefix(why, "Circular") || strings.HasPrefix(why, "From ") {
			return why
		}
		return "From " + name + ": " + strings.ToLower(why[:1]) + why[1:]
	}
	if r, ok := n.(formula.Ref); ok && s.wb.resolve(s, r.Sheet) == nil {
		return "Unresolved sheet name " + formula.QuoteSheet(r.Sheet)
	}
	if r, ok := n.(formula.Range); ok && s.wb.resolve(s, r.Sheet) == nil {
		return "Unresolved sheet name " + formula.QuoteSheet(r.Sheet)
	}
	return describeError(v, n)
}

// errorOrigin finds the innermost part of n that produces the error want
// itself rather than passing it on. When the error comes from a
// referenced cell, it returns that cell.
func (s *Sheet) errorOrigin(n Node, want Value) (Node, *loc) {
	same := func(v Value) bool { return v.Kind == Error && v.Str == want.Str }
	get := s.wb.values(s)
	at := func(sheet string, a Addr) *loc {
		if t := s.wb.resolve(s, sheet); t != nil {
			return &loc{t, a}
		}
		return nil // an unresolved sheet name is the error itself
	}
	switch n := n.(type) {
	case formula.Ref:
		if same(get(n.Sheet, n.Addr)) {
			return n, at(n.Sheet, n.Addr)
		}
	case formula.Range:
		if n.Rect.From == n.Rect.To && same(get(n.Sheet, n.Rect.From)) {
			return n, at(n.Sheet, n.Rect.From)
		}
	case formula.Unary:
		if same(eval(n.X, get)) {
			return s.errorOrigin(n.X, want)
		}
	case formula.Binary:
		for _, x := range []Node{n.L, n.R} {
			if same(eval(x, get)) {
				return s.errorOrigin(x, want)
			}
		}
	case formula.Call:
		if funcOf(n).remote != nil {
			return n, nil // JEV explains its own answers
		}
		for _, arg := range n.Args {
			if r, ok := arg.(formula.Range); ok && r.Rect.From != r.Rect.To {
				t := s.wb.resolve(s, r.Sheet)
				if t == nil {
					return arg, nil
				}
				cells := t.cellsIn(r.Rect)
				sortAddrs(cells)
				for _, a := range cells {
					if same(t.Value(a)) {
						return arg, &loc{t, a}
					}
				}
				continue
			}
			if same(eval(arg, get)) {
				return s.errorOrigin(arg, want)
			}
		}
	}
	return n, nil
}

// lookupFuncs report #N/A when they find no match.
var lookupFuncs = []string{"VLOOKUP", "HLOOKUP", "XLOOKUP", "LOOKUP", "MATCH", "XMATCH"}

// describeError explains an error code produced by n.
func describeError(v Value, n Node) string {
	expr := formula.Expr(n)
	if r := []rune(expr); len(r) > maxExplainExpr {
		expr = string(r[:maxExplainExpr-1]) + "…"
	}
	switch v.Str {
	case ErrDiv0.Str:
		return "Division by zero in " + expr
	case ErrValue.Str:
		if _, ok := n.(formula.Range); ok {
			return "A range where one value is expected: " + expr
		}
		return "Wrong type of value in " + expr
	case ErrName.Str:
		if nm, ok := n.(formula.Name); ok {
			return "Unknown name " + nm.Name
		}
		return "Unknown name in " + expr
	case ErrNA.Str:
		if call, ok := n.(formula.Call); ok && slices.Contains(lookupFuncs, funcOf(call).Name) {
			return "No match found by " + expr
		}
		return "Value not available in " + expr
	case ErrNum.Str:
		return "Number out of range in " + expr
	case ErrRef.Str:
		if _, ok := n.(formula.RefErr); ok {
			return "Reference to deleted cells"
		}
		return "Invalid reference in " + expr
	}
	return "Error in " + expr
}
