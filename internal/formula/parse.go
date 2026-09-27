package formula

import (
	"fmt"
	"strconv"
	"strings"
)

// Binding powers, lowest to highest, following Sheets' precedence.
var infixPower = map[string]int{
	"#OR#": 10, "#AND#": 10,
	"=": 30, "<>": 30, "<": 30, ">": 30, "<=": 30, ">=": 30,
	"&": 35,
	"+": 40, "-": 40,
	"*": 50, "/": 50,
	"^": 70,
}

// Negation binds tighter than ^, so -2^2 is 4 as in Sheets and Excel.
const (
	notPower     = 20
	unaryPower   = 75
	percentPower = 80
)

// MaxDepth is how deeply a formula may nest: parentheses, function calls
// and prefix operators each open a level (an operator of higher
// precedence may add one or two). Excel allows 64 nested functions; this
// is far more than a formula written by hand needs, and it bounds the
// recursion of everything that walks a formula (the parser, printer,
// evaluator and reference rewriting), so a pathological file fails to
// parse rather than exhausting the stack.
const MaxDepth = 1024

type parser struct {
	src   string // the formula without its "=", for names' original spelling
	toks  []token
	pos   int
	funcs Funcs
	depth int // of expr calls, see MaxDepth
}

// Parse parses a formula, finding the functions it calls with funcs. src
// may start with "=", as typed in a cell; error positions are relative to
// src.
func Parse(src string, funcs Funcs) (Node, error) {
	body := strings.TrimPrefix(src, "=")
	offset := len(src) - len(body)
	toks, err := lex(body)
	if err != nil {
		return nil, shift(err, offset)
	}
	p := &parser{src: body, toks: toks, funcs: funcs}
	n, err := p.expr(0)
	if err == nil && p.peek().kind != tokEOF {
		err = &ParseError{p.peek().pos, "Unexpected " + p.peek().text}
	}
	if err != nil {
		return nil, shift(err, offset)
	}
	return n, nil
}

func shift(err error, offset int) error {
	if pe, ok := err.(*ParseError); ok {
		pe.Pos += offset
	}
	return err
}

func (p *parser) peek() token { return p.toks[p.pos] }

func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *parser) isOp(op string) bool {
	t := p.peek()
	return t.kind == tokOp && t.text == op
}

// expr parses an expression whose operators bind tighter than minPower.
// Every level of nesting passes through here, so here it's counted.
func (p *parser) expr(minPower int) (Node, error) {
	if p.depth >= MaxDepth {
		return nil, &ParseError{p.peek().pos, fmt.Sprintf("Formula is nested too deeply (more than %d levels)", MaxDepth)}
	}
	p.depth++
	n, err := p.climb(minPower)
	p.depth--
	return n, err
}

// climb parses by precedence climbing: a prefix, then infix and postfix
// operators binding tighter than minPower.
func (p *parser) climb(minPower int) (Node, error) {
	left, err := p.prefix()
	if err != nil {
		return nil, err
	}
	for {
		if p.isOp("%") && percentPower > minPower {
			p.next()
			left = Unary{Op: "%", X: left}
			continue
		}
		t := p.peek()
		power, ok := infixPower[t.text]
		if t.kind != tokOp || !ok || power <= minPower {
			return left, nil
		}
		p.next()
		right, err := p.expr(power)
		if err != nil {
			return nil, err
		}
		left = Binary{Op: t.text, L: left, R: right}
	}
}

func (p *parser) prefix() (Node, error) {
	t := p.next()
	switch t.kind {
	case tokNum:
		if r, ok := p.lines(t, ""); ok {
			return r, nil
		}
		v, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, &ParseError{t.pos, "Invalid number " + t.text}
		}
		return Num{v}, nil
	case tokStr:
		return Str{t.text}, nil
	case tokIdent:
		return p.ident(t, "")
	case tokSheet:
		ref := p.next()
		if r, ok := p.lines(ref, t.text); ok {
			return r, nil
		}
		if ref.kind != tokIdent {
			return nil, &ParseError{ref.pos, "Expected a cell after " + QuoteSheet(t.text) + "!"}
		}
		return p.ident(ref, t.text)
	case tokFunc:
		return p.call(t)
	case tokRefErr:
		return RefErr{}, nil
	case tokOp:
		return p.prefixOp(t)
	case tokEOF:
		return nil, &ParseError{t.pos, "Formula is incomplete"}
	}
	return nil, &ParseError{t.pos, "Unexpected " + t.text}
}

// prefixOp parses what starts with an operator: a parenthesized
// expression, or a prefix + - or #NOT#.
func (p *parser) prefixOp(t token) (Node, error) {
	switch t.text {
	case "(":
		n, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		if !p.isOp(")") {
			return nil, &ParseError{p.peek().pos, "Missing )"}
		}
		p.next()
		return n, nil
	case "+", "-", "#NOT#":
		power := unaryPower
		if t.text == "#NOT#" {
			power = notPower
		}
		x, err := p.expr(power)
		if err != nil {
			return nil, err
		}
		return Unary{Op: t.text, X: x}, nil
	}
	return nil, &ParseError{t.pos, "Unexpected " + t.text}
}

// ident parses a reference, a range, a boolean or a name. sheet is the
// sheet the reference was qualified with, if any.
func (p *parser) ident(t token, sheet string) (Node, error) {
	if r, ok := p.lines(t, sheet); ok {
		return r, nil
	}
	a, abs, isRef := ParseRef(t.text)
	if !isRef {
		if sheet != "" {
			return nil, &ParseError{t.pos, "Expected a cell after " + QuoteSheet(sheet) + "!"}
		}
		switch t.text {
		case "TRUE":
			return Bool{true}, nil
		case "FALSE":
			return Bool{false}, nil
		}
		// Names match case-insensitively but print as written.
		return Name{p.src[t.pos : t.pos+len(t.text)]}, nil
	}
	if !p.isOp(":") && !p.isOp("..") {
		return Ref{a, abs, sheet}, nil
	}
	sep := p.next()
	end := p.next()
	// The second corner may repeat the sheet: Sheet2!A1:Sheet2!B3.
	if end.kind == tokSheet {
		if SheetKey(end.text) != SheetKey(sheet) {
			return nil, &ParseError{end.pos, "A range can't span sheets"}
		}
		end = p.next()
	}
	b, bAbs, ok := ParseRef(end.text)
	if end.kind != tokIdent || !ok {
		return nil, &ParseError{end.pos, "Expected a cell after " + sep.text}
	}
	r := NewRange(a, b, abs, bAbs)
	r.Sheet = sheet
	return r, nil
}

// lines parses whole columns (A:C, $A:$A) or rows (2:5), starting at t,
// if that's what comes.
func (p *parser) lines(t token, sheet string) (Range, bool) {
	if !p.isOp(":") || t.kind != tokIdent && t.kind != tokNum {
		return Range{}, false
	}
	end := p.toks[p.pos+1]
	if end.kind == tokSheet && SheetKey(end.text) == SheetKey(sheet) && p.pos+2 < len(p.toks) {
		end = p.toks[p.pos+2]
	}
	if end.kind != tokIdent && end.kind != tokNum {
		return Range{}, false
	}
	r, abs, ok := ParseLines(t.text, end.text)
	if !ok {
		return Range{}, false
	}
	for p.next() != end {
	}
	return Range{r, abs, sheet}, true
}

// NewRange builds a normalized range from two corners as written. Each
// absolute marker stays with its column or row, so $B1:A$2 becomes A1:$B$2.
func NewRange(a, b Addr, aAbs, bAbs Abs) Range {
	swap := func(bit Abs) {
		aAbs, bAbs = aAbs&^bit|bAbs&bit, bAbs&^bit|aAbs&bit
	}
	if a.Col > b.Col {
		a.Col, b.Col = b.Col, a.Col
		swap(AbsCol)
	}
	if a.Row > b.Row {
		a.Row, b.Row = b.Row, a.Row
		swap(AbsRow)
	}
	return Range{Rect{a, b}, [2]Abs{aAbs, bAbs}, ""}
}

// call parses a function call, with or without parentheses (@PI), and
// checks its number of arguments.
func (p *parser) call(t token) (Node, error) {
	fn, ok := p.funcs(t.text)
	if !ok {
		return nil, &ParseError{t.pos, "Unknown function " + t.text}
	}
	sig := fn.Signature()
	n := Call{Fn: fn}
	if p.isOp("(") {
		p.next()
		args, err := p.args(sig.Name)
		if err != nil {
			return nil, err
		}
		n.Args = args
	}
	if !sig.accepts(len(n.Args)) {
		return nil, &ParseError{t.pos, fmt.Sprintf("Wrong number of arguments to %s(%s)", sig.Name, sig.Args)}
	}
	return n, nil
}

// args parses a call's arguments after its "(", through the ")". An
// argument may be left out: F(a, , c) or F(a, ).
func (p *parser) args(name string) ([]Node, error) {
	if p.isOp(")") {
		p.next()
		return nil, nil
	}
	var args []Node
	for {
		if p.isOp(",") || p.isOp(";") || p.isOp(")") {
			args = append(args, Empty{})
		} else {
			arg, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		sep := p.next()
		switch {
		case sep.kind == tokOp && sep.text == ")":
			return args, nil
		case sep.kind != tokOp || (sep.text != "," && sep.text != ";"):
			return nil, &ParseError{sep.pos, "Expected , or ) in " + name}
		}
	}
}

// accepts reports whether a call may pass n arguments.
func (s Signature) accepts(n int) bool {
	return n >= s.Min && (s.Max < 0 || n <= s.Max) && (s.Step <= 0 || (n-s.Min)%s.Step == 0)
}
