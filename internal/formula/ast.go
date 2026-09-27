// Package formula is the formula language: A1 references and ranges,
// sheet names in references, the lexer and Pratt parser, the syntax tree,
// the printer that writes trees back in Sheets' spelling, and the
// rewriting of references when cells are copied, moved, inserted or
// deleted. It knows nothing of cells or values: the engine evaluates the
// trees, and tells the parser which functions exist.
package formula

// Node is a parsed formula expression: one of the types below.
type Node any

type (
	Num  struct{ V float64 }
	Str  struct{ V string }
	Bool struct{ V bool }
	// Ref is a cell reference. Sheet is the sheet name as written before
	// the "!" (Sheet2!A1), or "" for the formula's own sheet.
	Ref struct {
		Addr  Addr
		Abs   Abs
		Sheet string
	}
	// Range is kept normalized (Rect.From is the top-left corner); Abs
	// holds the absolute markers of Rect.From and Rect.To.
	Range struct {
		Rect  Rect
		Abs   [2]Abs
		Sheet string
	}
	RefErr struct{}              // a reference to deleted cells: #REF!
	Name   struct{ Name string } // a named range as spelled in the formula
	Empty  struct{}              // an omitted argument, as in XLOOKUP(a, b, c, , 1)
	Unary  struct {
		Op string // "-", "+", "%" (postfix) or "#NOT#"
		X  Node
	}
	Binary struct {
		Op   string
		L, R Node
	}
	Call struct {
		Fn   Func
		Args []Node
	}
)

// Func is a function a formula can call. The parser checks calls against
// its signature; what a call computes is up to the engine, which
// recovers its own type from Call.Fn.
type Func interface {
	Signature() Signature
}

// Signature is how a function is called.
type Signature struct {
	Name string // canonical, upper case
	Args string // shown to users, e.g. "value1, [value2, ...]"
	Min  int
	Max  int // -1 for variadic
	// Step > 0 means arguments after Min come in groups of Step, like
	// SUMIFS' (range, criterion) pairs.
	Step int
}

// Funcs finds a function by its upper-case name or alias.
type Funcs func(name string) (Func, bool)

// WalkNames calls fn for every name in n.
func WalkNames(n Node, fn func(Name)) {
	switch n := n.(type) {
	case Name:
		fn(n)
	case Unary:
		WalkNames(n.X, fn)
	case Binary:
		WalkNames(n.L, fn)
		WalkNames(n.R, fn)
	case Call:
		for _, a := range n.Args {
			WalkNames(a, fn)
		}
	}
}

// WalkRefs calls fn for every single-cell reference and range in n, with
// the sheet it was qualified with ("" for the formula's own sheet).
func WalkRefs(n Node, ref func(string, Addr), rng func(string, Rect)) {
	switch n := n.(type) {
	case Ref:
		ref(n.Sheet, n.Addr)
	case Range:
		rng(n.Sheet, n.Rect)
	case Unary:
		WalkRefs(n.X, ref, rng)
	case Binary:
		WalkRefs(n.L, ref, rng)
		WalkRefs(n.R, ref, rng)
	case Call:
		for _, a := range n.Args {
			WalkRefs(a, ref, rng)
		}
	}
}
