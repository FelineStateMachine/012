package functions

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// IsVolatile reports whether n calls a function whose result changes
// without its inputs changing (TODAY, NOW, RAND).
func IsVolatile(n Node) bool {
	switch n := n.(type) {
	case formula.Unary:
		return IsVolatile(n.X)
	case formula.Binary:
		return IsVolatile(n.L) || IsVolatile(n.R)
	case formula.Call:
		if funcOf(n).Volatile {
			return true
		}
		for _, a := range n.Args {
			if IsVolatile(a) {
				return true
			}
		}
	case formula.Array, formula.Invoke:
		volatile := false
		formula.EachChild(n, func(k Node) { volatile = volatile || IsVolatile(k) })
		return volatile
	}
	return false
}

// InferFormat picks the format Sheets shows a formula's result in when
// the cell is Automatic: dates from date functions, and otherwise the
// format of the first formatted input, so =B2+B3 of currency shows
// currency and a date plus days shows a date. at is the format a cell
// shows in, a linked source's column the one its type says; paged
// reports whether a sheet is a linked source's tab, whose counts show as
// whole numbers with thousands separators.
func InferFormat(n Node, at func(string, Addr) Format, paged func(string) bool) Format {
	infer := func(n Node) Format { return InferFormat(n, at, paged) }
	switch n := n.(type) {
	case formula.Ref:
		return at(n.Sheet, n.Addr)
	case formula.Range:
		return at(n.Sheet, n.Rect.From)
	case formula.Unary:
		if n.Op == "-" || n.Op == "+" {
			return infer(n.X)
		}
	case formula.Binary:
		l, r := infer(n.L), infer(n.R)
		switch n.Op {
		case "+", "-":
			if n.Op == "-" && l.Kind.IsTime() && r.Kind.IsTime() {
				return Format{} // days between two dates
			}
			return firstFormat(l, r)
		case "*", "/":
			if l.Kind.IsTime() || r.Kind.IsTime() {
				return Format{}
			}
			return firstFormat(l, r)
		}
	case formula.Call:
		f := funcOf(n)
		if counting[f.Name] && paged != nil && countsPaged(n.Args, paged) {
			return wholeNumber
		}
		if f.format != nil {
			return f.format(n.Args, infer)
		}
	case formula.Array:
		return infer(n.Rows[0][0])
	}
	return Format{}
}

// counting are the functions that count cells or rows.
var counting = map[string]bool{"COUNT": true, "COUNTA": true, "COUNTIF": true, "COUNTIFS": true,
	"COUNTBLANK": true, "ROWS": true, "COLUMNS": true}

// wholeNumber is a count's format over a linked source: the locale's
// thousands separators and no decimals, so two million rows read
// 2,000,000.
var wholeNumber = Format{Kind: value.FmtNumber}

// countsPaged reports whether a count's arguments name a range of a
// linked source.
func countsPaged(args []Node, paged func(string) bool) bool {
	for _, a := range args {
		sheet := ""
		switch r := a.(type) {
		case formula.Range:
			sheet = r.Sheet
		case formula.Ref:
			sheet = r.Sheet
		}
		if sheet != "" && paged(sheet) {
			return true
		}
	}
	return false
}

func firstFormat(fs ...Format) Format {
	for _, f := range fs {
		if !f.IsZero() && f.Kind != value.FmtText {
			return f
		}
	}
	return Format{}
}

// Result formats for the function table.

// returns makes a function's result show in format f.
func returns(f Format) func([]Node, func(Node) Format) Format {
	return func([]Node, func(Node) Format) Format { return f }
}

// inherit makes a function's result take the format of its first
// formatted argument (SUM, MIN, ROUND...).
func inherit(args []Node, infer func(Node) Format) Format {
	for _, a := range args {
		if f := infer(a); !f.IsZero() && f.Kind != value.FmtText {
			return f
		}
	}
	return Format{}
}

// inheritFrom takes the format of argument i only.
func inheritFrom(i int) func([]Node, func(Node) Format) Format {
	return func(args []Node, infer func(Node) Format) Format {
		if i < len(args) {
			return infer(args[i])
		}
		return Format{}
	}
}
