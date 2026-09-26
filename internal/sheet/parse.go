package sheet

import (
	"fmt"
	"strconv"
	"strings"
)

// Node is a parsed formula expression.
type Node any

type (
	numLit  struct{ v float64 }
	strLit  struct{ v string }
	boolLit struct{ v bool }
	refNode struct {
		a   Addr
		abs absFlags
	}
	// rangeNode is kept normalized (r.From is the top-left corner); abs
	// holds the absolute markers of r.From and r.To.
	rangeNode struct {
		r   Rect
		abs [2]absFlags
	}
	refErrNode struct{}              // a reference to deleted cells: #REF!
	nameNode   struct{ name string } // an identifier that isn't a cell; #NAME? until names exist
	emptyArg   struct{}              // an omitted argument, as in XLOOKUP(a, b, c, , 1)
	unaryNode  struct {
		op string
		x  Node
	}
	binaryNode struct {
		op   string
		l, r Node
	}
	callNode struct {
		fn   *FuncDef
		args []Node
	}
)

type tokKind int

const (
	tokEOF tokKind = iota
	tokNum
	tokStr
	tokIdent  // cell reference, TRUE/FALSE or a name
	tokFunc   // identifier directly followed by "("
	tokOp     // operators and punctuation
	tokRefErr // #REF!, left behind when a referenced cell is deleted
)

type token struct {
	kind tokKind
	text string
	pos  int
}

// ParseError describes a formula that could not be parsed. Pos is the byte
// offset in the entry where the problem was found, used to place the edit
// cursor.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string { return e.Msg }

func isIdentStart(c byte) bool { return isLetter(c) || c == '_' || c == '$' }

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) || c == '.' }

func lex(src string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		start := i
		switch {
		case c == ' ':
			i++
			continue
		case isDigit(c) || (c == '.' && i+1 < len(src) && isDigit(src[i+1])):
			for i < len(src) && (isDigit(src[i]) || src[i] == '.' && !strings.HasPrefix(src[i:], "..")) {
				i++
			}
			if i < len(src) && (src[i] == 'e' || src[i] == 'E') {
				j := i + 1
				if j < len(src) && (src[j] == '+' || src[j] == '-') {
					j++
				}
				if j < len(src) && isDigit(src[j]) {
					i = j
					for i < len(src) && isDigit(src[i]) {
						i++
					}
				}
			}
			toks = append(toks, token{tokNum, src[start:i], start})
		case c == '"':
			// Strings use "" to escape a quote, as in Sheets.
			var sb strings.Builder
			i++
			for {
				if i >= len(src) {
					return nil, &ParseError{start, "Missing closing quote"}
				}
				if src[i] == '"' {
					if i+1 < len(src) && src[i+1] == '"' {
						sb.WriteByte('"')
						i += 2
						continue
					}
					i++
					break
				}
				sb.WriteByte(src[i])
				i++
			}
			toks = append(toks, token{tokStr, sb.String(), start})
		case c == '@' || isIdentStart(c):
			if c == '@' { // 1-2-3 style @SUM
				i++
			}
			for i < len(src) && isIdentPart(src[i]) && !strings.HasPrefix(src[i:], "..") {
				i++
			}
			name := strings.ToUpper(strings.TrimPrefix(src[start:i], "@"))
			if name == "" {
				return nil, &ParseError{start, "Missing function name after @"}
			}
			j := i
			for j < len(src) && src[j] == ' ' {
				j++
			}
			kind := tokIdent
			if j < len(src) && src[j] == '(' {
				kind = tokFunc
			} else if c == '@' {
				kind = tokFunc // @PI without parentheses
			}
			toks = append(toks, token{kind, name, start})
		case c == '#' && strings.HasPrefix(strings.ToUpper(src[i:]), "#REF!"):
			i += len("#REF!")
			toks = append(toks, token{tokRefErr, "#REF!", start})
		case c == '#':
			end := strings.IndexByte(src[i+1:], '#')
			if end < 0 {
				return nil, &ParseError{start, "Unexpected #"}
			}
			op := strings.ToUpper(src[i : i+end+2])
			if op != "#AND#" && op != "#OR#" && op != "#NOT#" {
				return nil, &ParseError{start, "Unknown operator " + op}
			}
			i += end + 2
			toks = append(toks, token{tokOp, op, start})
		case strings.HasPrefix(src[i:], ".."), strings.HasPrefix(src[i:], "<="),
			strings.HasPrefix(src[i:], ">="), strings.HasPrefix(src[i:], "<>"):
			i += 2
			toks = append(toks, token{tokOp, src[start:i], start})
		case strings.IndexByte("+-*/^=<>&(),;:%", c) >= 0:
			i++
			toks = append(toks, token{tokOp, string(c), start})
		default:
			return nil, &ParseError{start, fmt.Sprintf("Unexpected %q", c)}
		}
	}
	return append(toks, token{tokEOF, "", len(src)}), nil
}

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

type parser struct {
	toks []token
	pos  int
}

// Parse parses a formula. src may start with "=", as typed in a cell;
// error positions are relative to src.
func Parse(src string) (Node, error) {
	body := strings.TrimPrefix(src, "=")
	offset := len(src) - len(body)
	toks, err := lex(body)
	if err != nil {
		return nil, shift(err, offset)
	}
	p := &parser{toks: toks}
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

func (p *parser) expr(minPower int) (Node, error) {
	left, err := p.prefix()
	if err != nil {
		return nil, err
	}
	for {
		if p.isOp("%") && percentPower > minPower {
			p.next()
			left = unaryNode{op: "%", x: left}
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
		left = binaryNode{op: t.text, l: left, r: right}
	}
}

func (p *parser) prefix() (Node, error) {
	t := p.next()
	switch t.kind {
	case tokNum:
		v, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, &ParseError{t.pos, "Invalid number " + t.text}
		}
		return numLit{v}, nil
	case tokStr:
		return strLit{t.text}, nil
	case tokIdent:
		return p.ident(t)
	case tokFunc:
		return p.call(t)
	case tokRefErr:
		return refErrNode{}, nil
	case tokOp:
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
		case "+", "-":
			x, err := p.expr(unaryPower)
			if err != nil {
				return nil, err
			}
			return unaryNode{op: t.text, x: x}, nil
		case "#NOT#":
			x, err := p.expr(notPower)
			if err != nil {
				return nil, err
			}
			return unaryNode{op: t.text, x: x}, nil
		}
	case tokEOF:
		return nil, &ParseError{t.pos, "Formula is incomplete"}
	}
	return nil, &ParseError{t.pos, "Unexpected " + t.text}
}

func (p *parser) ident(t token) (Node, error) {
	a, abs, isRef := parseRef(t.text)
	if !isRef {
		switch t.text {
		case "TRUE":
			return boolLit{true}, nil
		case "FALSE":
			return boolLit{false}, nil
		}
		return nameNode{t.text}, nil
	}
	if p.isOp(":") || p.isOp("..") {
		sep := p.next()
		end := p.next()
		b, bAbs, ok := parseRef(end.text)
		if end.kind != tokIdent || !ok {
			return nil, &ParseError{end.pos, "Expected a cell after " + sep.text}
		}
		return newRange(a, b, abs, bAbs), nil
	}
	return refNode{a, abs}, nil
}

// newRange builds a normalized range from two corners as written. Each
// absolute marker stays with its column or row, so $B1:A$2 becomes A1:$B$2.
func newRange(a, b Addr, aAbs, bAbs absFlags) rangeNode {
	swap := func(bit absFlags) {
		aAbs, bAbs = aAbs&^bit|bAbs&bit, bAbs&^bit|aAbs&bit
	}
	if a.Col > b.Col {
		a.Col, b.Col = b.Col, a.Col
		swap(absCol)
	}
	if a.Row > b.Row {
		a.Row, b.Row = b.Row, a.Row
		swap(absRow)
	}
	return rangeNode{Rect{a, b}, [2]absFlags{aAbs, bAbs}}
}

func (p *parser) call(t token) (Node, error) {
	fn, ok := LookupFunc(t.text)
	if !ok {
		return nil, &ParseError{t.pos, "Unknown function " + t.text}
	}
	n := callNode{fn: fn}
	if p.isOp("(") {
		p.next()
		if p.isOp(")") {
			p.next()
		} else {
			for {
				// An argument may be left out: F(a, , c) or F(a, ).
				if p.isOp(",") || p.isOp(";") || p.isOp(")") {
					n.args = append(n.args, emptyArg{})
				} else {
					arg, err := p.expr(0)
					if err != nil {
						return nil, err
					}
					n.args = append(n.args, arg)
				}
				sep := p.next()
				if sep.kind == tokOp && sep.text == ")" {
					break
				}
				if sep.kind != tokOp || (sep.text != "," && sep.text != ";") {
					return nil, &ParseError{sep.pos, "Expected , or ) in " + fn.Name}
				}
			}
		}
	}
	if len(n.args) < fn.Min || (fn.Max >= 0 && len(n.args) > fn.Max) ||
		(fn.step > 0 && (len(n.args)-fn.Min)%fn.step != 0) {
		return nil, &ParseError{t.pos, fmt.Sprintf("Wrong number of arguments to %s(%s)", fn.Name, fn.Args)}
	}
	return n, nil
}

// walkRefs calls fn for every single-cell reference and range in n.
func walkRefs(n Node, ref func(Addr), rng func(Rect)) {
	switch n := n.(type) {
	case refNode:
		ref(n.a)
	case rangeNode:
		rng(n.r)
	case unaryNode:
		walkRefs(n.x, ref, rng)
	case binaryNode:
		walkRefs(n.l, ref, rng)
		walkRefs(n.r, ref, rng)
	case callNode:
		for _, a := range n.args {
			walkRefs(a, ref, rng)
		}
	}
}
