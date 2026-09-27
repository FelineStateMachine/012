package sheet

import "github.com/FelineStateMachine/012/internal/formula"

// The formula language lives in internal/formula. The engine's API keeps
// its names for addresses, ranges and parsing, so callers see one package.

type (
	// Addr identifies a cell by zero-based column and row.
	Addr = formula.Addr
	// Rect is an inclusive rectangular range of cells.
	Rect = formula.Rect
	// Node is a parsed formula expression.
	Node = formula.Node
	// ParseError describes a formula that could not be parsed.
	ParseError = formula.ParseError
)

// Worksheet bounds, matching Lotus 1-2-3 Release 2 (A..IV, 1..8192).
const (
	MaxCols = formula.MaxCols
	MaxRows = formula.MaxRows
)

// ColName converts a zero-based column index to letters: 0 -> A, 26 -> AA.
func ColName(c int) string { return formula.ColName(c) }

// ParseCol converts column letters (case-insensitive) to a zero-based index.
func ParseCol(s string) (int, bool) { return formula.ParseCol(s) }

// ParseAddr parses an A1-style reference, ignoring absolute markers.
func ParseAddr(s string) (Addr, bool) { return formula.ParseAddr(s) }

// NewRect returns the normalized rectangle spanning a and b.
func NewRect(a, b Addr) Rect { return formula.NewRect(a, b) }

// ParseRange parses "A1", "A1:B3" or 1-2-3 style "A1..B3".
func ParseRange(s string) (Rect, bool) { return formula.ParseRange(s) }

// QuoteSheet writes a sheet name as a formula needs it, e.g. 'Q3 plan'.
func QuoteSheet(name string) string { return formula.QuoteSheet(name) }

// SplitSheet splits a reference such as "'Q3 plan'!B2" into the sheet
// name, unquoted, and the rest.
func SplitSheet(s string) (sheet, rest string) { return formula.SplitSheet(s) }

// Qualified writes r on sheet as a formula would: Sheet2!A1:B3.
func Qualified(sheet string, r Rect) string { return formula.Qualified(sheet, r) }

// Parse parses a formula with this engine's functions. src may start
// with "="; error positions are relative to src.
func Parse(src string) (Node, error) { return formula.Parse(src, parserFuncs) }

// parserFuncs is the function table as the parser sees it.
func parserFuncs(name string) (formula.Func, bool) {
	if f, ok := LookupFunc(name); ok {
		return f, true
	}
	return nil, false
}

// Signature is how the parser checks calls to f.
func (f *FuncDef) Signature() formula.Signature {
	return formula.Signature{Name: f.Name, Args: f.Args, Min: f.Min, Max: f.Max, Step: f.step}
}

// funcOf is the function a parsed call calls: always one of ours, since
// the parser only finds functions through parserFuncs.
func funcOf(c formula.Call) *FuncDef { return c.Fn.(*FuncDef) }

func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
