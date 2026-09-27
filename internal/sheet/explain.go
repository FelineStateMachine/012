package sheet

import (
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
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
	if c != nil && c.derived && v.Kind == Error && s.pivot.err != "" {
		return s.pivot.err
	}
	if why := s.spillError(a); why != "" && v == ErrRef {
		return why
	}
	if anchor, ok := s.SpillAnchor(a); ok && v.Kind == Error {
		why := s.explain(anchor, path)
		if why == "" {
			return "Spilled from " + anchor.String()
		}
		return "Spilled from " + anchor.String() + ": " + strings.ToLower(why[:1]) + why[1:]
	}
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
	n, from := s.errorOrigin(c.expr, a, v)
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
// referenced cell, it returns that cell. The formula is the one at here.
func (s *Sheet) errorOrigin(n Node, here Addr, want Value) (Node, *loc) {
	return errorSearch{s: s, here: here, want: want, get: s.wb.values(s)}.find(n)
}

// errorSearch follows an error through a formula on s to where it starts.
type errorSearch struct {
	s    *Sheet
	here Addr // the formula's cell, for ranges read as one of their cells
	want Value
	get  *reader
}

func (e errorSearch) same(v Value) bool { return v.Kind == Error && v.Str == e.want.Str }

// at is the cell a reference points at, or nil when its sheet name is
// unresolved: then the reference is the error itself.
func (e errorSearch) at(sheet string, a Addr) *loc {
	if t := e.s.wb.resolve(e.s, sheet); t != nil {
		return &loc{t, a}
	}
	return nil
}

func (e errorSearch) find(n Node) (Node, *loc) {
	switch n := n.(type) {
	case formula.Ref:
		if e.same(e.get.Cell(n.Sheet, n.Addr)) {
			return n, e.at(n.Sheet, n.Addr)
		}
	case formula.Range:
		if a, ok := functions.Intersect(n.Rect, e.here); ok && e.same(e.get.Cell(n.Sheet, a)) {
			return n, e.at(n.Sheet, a)
		}
	case formula.Unary:
		return e.operand(n, n.X)
	case formula.Binary:
		return e.operand(n, n.L, n.R)
	case formula.Call:
		return e.inCall(n)
	}
	return n, nil
}

// operand follows the first of n's operands that has the error, or stops
// at n when none has.
func (e errorSearch) operand(n Node, operands ...Node) (Node, *loc) {
	for _, x := range operands {
		if e.same(functions.EvalAt(x, e.get.lib, e.here)) {
			return e.find(x)
		}
	}
	return n, nil
}

// inCall follows the first argument of a call that has the error: for a
// range, its first cell with it.
func (e errorSearch) inCall(n formula.Call) (Node, *loc) {
	if funcOf(n).Remote() {
		return n, nil // JEV explains its own answers
	}
	for _, arg := range n.Args {
		if r, ok := arg.(formula.Range); ok && r.Rect.From != r.Rect.To {
			if l, found := e.inRange(r); found {
				return arg, l
			}
			continue
		}
		if e.same(functions.EvalAt(arg, e.get.lib, e.here)) {
			return e.find(arg)
		}
	}
	return n, nil
}

// inRange finds the first cell of r, row by row, with the error. An
// unresolved sheet name is found, as the error itself, with no cell.
func (e errorSearch) inRange(r formula.Range) (*loc, bool) {
	t := e.s.wb.resolve(e.s, r.Sheet)
	if t == nil {
		return nil, true
	}
	cells := t.cellsIn(r.Rect)
	sortAddrs(cells)
	for _, a := range cells {
		if e.same(t.Value(a)) {
			return &loc{t, a}, true
		}
	}
	return nil, false
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
