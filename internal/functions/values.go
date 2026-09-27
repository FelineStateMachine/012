// Package functions is 012's function library: the FuncDef table, the
// evaluation of formulas (operators and calls), the argument, range and
// criteria helpers functions share, decimal arithmetic, and the
// questions JEV functions ask.
//
// It knows nothing of sheets or storage. A formula reads cells through a
// Reader over a Book, which the engine (internal/sheet) implements, and
// the engine finds functions through the table (LookupFunc) when it
// parses formulas. Dependencies point one way: formula and value below,
// the engine above.
package functions

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// The library works in the formula and value packages' terms; these
// names keep its tables short, as the engine's do.
type (
	// Value is the computed contents of a cell.
	Value = value.Value
	// Format is a cell's number format.
	Format = value.Format
	// Node is a parsed formula expression.
	Node = formula.Node
	// Addr identifies a cell by zero-based column and row.
	Addr = formula.Addr
	// Rect is an inclusive rectangular range of cells.
	Rect = formula.Rect
)

func num(v float64) Value             { return value.Num(v) }
func boolean(b bool) Value            { return value.Boolean(b) }
func str(s string) Value              { return value.Str(s) }
func errOf(v Value) *Value            { return value.ErrOf(v) }
func toNum(v Value) (float64, *Value) { return value.ToNum(v) }
func text(v Value) string             { return value.AsText(v) }
func compare(l, r Value) int          { return value.Compare(l, r) }

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }
