package sheet

import (
	"slices"
	"strings"
)

// ExplainError says why the cell at a shows an error, for the context
// line: where the error starts and what went wrong, e.g. "Division by
// zero in B3/0", or "From B5: division by zero in B3/0" when it comes from
// another cell. It is empty for cells without errors.
func (s *Sheet) ExplainError(a Addr) string {
	return s.explain(a, nil)
}

// maxExplainExpr caps how much of a formula an explanation quotes.
const maxExplainExpr = 40

func (s *Sheet) explain(a Addr, path []Addr) string {
	c := s.cells[a]
	v := s.Value(a)
	if c == nil || v.Kind != Error || !c.IsFormula() {
		return ""
	}
	if i := slices.Index(path, a); i >= 0 {
		names := make([]string, 0, len(path)-i+1)
		for _, p := range path[i:] {
			names = append(names, p.String())
		}
		return "Circular reference: " + strings.Join(append(names, a.String()), " → ")
	}
	path = append(path, a)
	n, from := s.errorOrigin(c.expr, v)
	if from != nil {
		why := s.explain(*from, path)
		if why == "" {
			return "From " + from.String()
		}
		if strings.HasPrefix(why, "Circular") || strings.HasPrefix(why, "From ") {
			return why
		}
		return "From " + from.String() + ": " + strings.ToLower(why[:1]) + why[1:]
	}
	return describeError(v, n)
}

// errorOrigin finds the innermost part of n that produces the error want
// itself rather than passing it on. When the error comes from a
// referenced cell, it returns that cell.
func (s *Sheet) errorOrigin(n Node, want Value) (Node, *Addr) {
	same := func(v Value) bool { return v.Kind == Error && v.Str == want.Str }
	get := s.Value
	switch n := n.(type) {
	case refNode:
		if same(get(n.a)) {
			return n, &n.a
		}
	case rangeNode:
		if n.r.From == n.r.To && same(get(n.r.From)) {
			return n, &n.r.From
		}
	case unaryNode:
		if same(eval(n.x, get)) {
			return s.errorOrigin(n.x, want)
		}
	case binaryNode:
		for _, x := range []Node{n.l, n.r} {
			if same(eval(x, get)) {
				return s.errorOrigin(x, want)
			}
		}
	case callNode:
		if n.fn.remote != nil {
			return n, nil // JEV explains its own answers
		}
		for _, arg := range n.args {
			if r, ok := arg.(rangeNode); ok && r.r.From != r.r.To {
				cells := s.cellsIn(r.r)
				sortAddrs(cells)
				for _, a := range cells {
					if same(get(a)) {
						return arg, &a
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
	var b strings.Builder
	printNode(&b, n)
	expr := b.String()
	if r := []rune(expr); len(r) > maxExplainExpr {
		expr = string(r[:maxExplainExpr-1]) + "…"
	}
	switch v.Str {
	case ErrDiv0.Str:
		return "Division by zero in " + expr
	case ErrValue.Str:
		if _, ok := n.(rangeNode); ok {
			return "A range where one value is expected: " + expr
		}
		return "Wrong type of value in " + expr
	case ErrName.Str:
		if nm, ok := n.(nameNode); ok {
			return "Unknown name " + nm.name
		}
		return "Unknown name in " + expr
	case ErrNA.Str:
		if call, ok := n.(callNode); ok && slices.Contains(lookupFuncs, call.fn.Name) {
			return "No match found by " + expr
		}
		return "Value not available in " + expr
	case ErrNum.Str:
		return "Number out of range in " + expr
	case ErrRef.Str:
		if _, ok := n.(refErrNode); ok {
			return "Reference to deleted cells"
		}
		return "Invalid reference in " + expr
	}
	return "Error in " + expr
}
