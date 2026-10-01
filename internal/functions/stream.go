package functions

import (
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/value"
)

// Calls over paged sources. A linked source (the engine's paged sheets:
// a Parquet file or a SQLite table read in place) is too big to hold,
// so a function given one of its ranges isn't evaluated where the
// formula is: the Reader describes the call, its source ranges and the
// values of its other arguments, as a StreamCall and asks the Book for
// the answer, which is worked out in the background by EvalStream over
// a Book that streams the source, and kept until the file changes.
// Until it is known the cell shows Loading…, as a JEV question does.
//
// The functions in streaming read any size in one pass and hold only
// what they compute; any other reads what its ranges hold, so it is
// given a source range only while that holds no more cells than the
// budget (max-cells), and past it answers #VALUE! saying why.

// StreamCall is a call of a function some of whose arguments are ranges
// of paged sheets. It isn't comparable: Key identifies it by content.
type StreamCall struct {
	Fn string
	// Dec is set when the call is computed in decimal (decimal.go).
	Dec  bool
	Args []StreamArg
}

// StreamArg is an argument of a StreamCall: a range of a paged sheet
// (Sheet set, as the engine names the source), or a value.
type StreamArg struct {
	Sheet string
	R     Rect
	V     Value
}

// StreamAnswer is what a StreamCall computes: a value, or an array of
// them, and for an error that needs one, why.
type StreamAnswer struct {
	V   Value
	A   *Array
	Why string
}

// Key identifies the call by its content.
func (c StreamCall) Key() string {
	var b strings.Builder
	b.WriteString(c.Fn)
	if c.Dec {
		b.WriteString("#dec")
	}
	for _, a := range c.Args {
		b.WriteByte('\x1f')
		if a.Sheet != "" {
			fmt.Fprintf(&b, "r%s\x1e%d:%d:%d:%d", a.Sheet, a.R.From.Col, a.R.From.Row, a.R.To.Col, a.R.To.Row)
			continue
		}
		fmt.Fprintf(&b, "v%d\x1e%v\x1e%s", a.V.Kind, a.V.Num, a.V.Str)
	}
	return b.String()
}

// streaming are the functions that read a source range of any size in
// one pass, holding only what they compute: aggregates, criteria
// functions and lookups.
var streaming = map[string]bool{
	"SUM": true, "AVERAGE": true, "COUNT": true, "COUNTA": true, "MIN": true, "MAX": true, "PRODUCT": true,
	"COUNTIF": true, "COUNTIFS": true, "SUMIF": true, "SUMIFS": true, "AVERAGEIF": true, "AVERAGEIFS": true,
	"COUNTBLANK": true, "SUMPRODUCT": true,
	"MATCH": true, "XLOOKUP": true, "VLOOKUP": true, "HLOOKUP": true, "INDEX": true, "ROWS": true, "COLUMNS": true,
}

// Streams reports whether the function named name reads a source of
// any size (see streaming).
func Streams(name string) bool { return streaming[name] }

// StreamingFuncs lists the functions that read a source of any size, in
// alphabetical order, for the docs and messages.
func StreamingFuncs() []string {
	var out []string
	for _, f := range Funcs() {
		if streaming[f.Name] {
			out = append(out, f.Name)
		}
	}
	return out
}

// streamed asks the Book for the call of f with args when one of its
// arguments is a range of a paged sheet, reporting whether it asked. A
// call whose other arguments aren't single values (a range of another
// sheet, an array) is evaluated as usual, reading the source through
// the Book's cells, which it holds only up to the budget.
func (rd *Reader) streamed(f *FuncDef, args []Node) (Value, bool) {
	if rd.stream || f.remote != nil || f.binds != formula.BindNone || !rd.pagedArg(args) {
		return Value{}, false
	}
	call := StreamCall{Fn: f.Name, Dec: funcs[f.Name] != f, Args: make([]StreamArg, len(args))}
	for i, a := range args {
		switch r := rd.refOf(a).(type) {
		case formula.Range:
			if r.Sheet == "" || !rd.book.Paged(r.Sheet) {
				return Value{}, false
			}
			call.Args[i] = StreamArg{Sheet: r.Sheet, R: r.Rect}
			continue
		}
		v := eval1(a, rd)
		if v.Kind == value.Array {
			return Value{}, false
		}
		call.Args[i] = StreamArg{V: v}
	}
	v, a := rd.book.Stream(call)
	if a != nil {
		return rd.arrayValue(a), true
	}
	return v, true
}

// pagedArg reports whether an argument is a range of a paged sheet.
func (rd *Reader) pagedArg(args []Node) bool {
	for _, a := range args {
		if r, ok := rd.refOf(a).(formula.Range); ok && r.Sheet != "" && rd.book.Paged(r.Sheet) {
			return true
		}
	}
	return false
}

// constArg is a StreamCall's value argument, as a node.
type constArg struct{ v Value }

// EvalStream computes c over book, a Book that reads the paged sheets
// its ranges name by streaming them: in the background, where reading
// takes as long as the source is big. A function not in streaming is
// given the source's ranges only up to budget cells.
func EvalStream(c StreamCall, book Book, budget int) StreamAnswer {
	f, ok := LookupFunc(c.Fn)
	if !ok {
		return StreamAnswer{V: value.ErrName}
	}
	if twin, ok := decFuncs()[c.Fn]; ok && c.Dec {
		f = twin
	}
	if why := overBudget(c, book, budget); why != "" {
		return StreamAnswer{V: value.ErrValue, Why: why}
	}
	args := make([]Node, len(c.Args))
	const fixed = formula.AbsCol | formula.AbsRow
	for i, a := range c.Args {
		if a.Sheet != "" {
			args[i] = formula.Range{Rect: a.R, Sheet: a.Sheet, Abs: [2]formula.Abs{fixed, fixed}}
		} else {
			args[i] = constArg{a.V}
		}
	}
	depth := 0
	rd := NewReader(book, &depth, false)
	rd.stream = true
	v := EvalCell(formula.Call{Fn: f, Args: args}, rd, Addr{})
	return StreamAnswer{V: v, A: rd.Spilled()}
}

// overBudget says why c can't be computed, when it names a function
// that doesn't stream over more cells of its sources than budget.
func overBudget(c StreamCall, book Book, budget int) string {
	if streaming[c.Fn] {
		return ""
	}
	cells, most := 0, 0
	biggest := ""
	for _, a := range c.Args {
		if a.Sheet == "" {
			continue
		}
		b, any, _ := book.Bounds(a.Sheet, a.R)
		if !any {
			continue
		}
		n := (b.To.Row - b.From.Row + 1) * (b.To.Col - b.From.Col + 1)
		if cells += n; n > most {
			most, biggest = n, a.Sheet
		}
	}
	if cells <= budget {
		return ""
	}
	return fmt.Sprintf("Raise max-cells to %s to %s %s. It is %s, and %s holds what it reads; SUMIFS, MEDIAN, XLOOKUP and the like stream a source of any size.",
		grouped(cells), c.Fn, biggest, grouped(budget), c.Fn)
}

// grouped is n with thousands separators: 12,000,000.
func grouped(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// Const is a node that evaluates to v: what the engine binds a name to
// while what it names can't be read yet (a source being opened shows
// Loading…).
func Const(v Value) Node { return constArg{v} }
