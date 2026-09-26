package sheet

import (
	"fmt"
	"strconv"
	"strings"
)

// Node is a parsed formula expression.
type Node any

type (
	numLit    struct{ v float64 }
	strLit    struct{ v string }
	refNode   struct{ a Addr }
	rangeNode struct{ r Rect }
	unaryNode struct {
		op string
		x  Node
	}
	binaryNode struct {
		op   string
		l, r Node
	}
	callNode struct {
		name string
		args []Node
	}
)

type tokKind int

const (
	tokEOF tokKind = iota
	tokNum
	tokStr
	tokRef
	tokFunc
	tokOp
)

type token struct {
	kind tokKind
	text string
	pos  int
}

// ParseError describes a formula that could not be parsed. Pos is the byte
// offset where the problem was found, used to place the EDIT cursor.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s at position %d", e.Msg, e.Pos+1)
}

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
			for i < len(src) && (isDigit(src[i]) || src[i] == '.') {
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
			i++
			for i < len(src) && src[i] != '"' {
				i++
			}
			if i == len(src) {
				return nil, &ParseError{start, "unterminated string"}
			}
			i++
			toks = append(toks, token{tokStr, src[start+1 : i-1], start})
		case c == '@':
			i++
			for i < len(src) && isLetter(src[i]) {
				i++
			}
			if i == start+1 {
				return nil, &ParseError{start, "missing function name"}
			}
			toks = append(toks, token{tokFunc, strings.ToUpper(src[start+1 : i]), start})
		case c == '$' || isLetter(c):
			for i < len(src) && (src[i] == '$' || isLetter(src[i]) || isDigit(src[i])) {
				i++
			}
			if _, ok := ParseAddr(src[start:i]); !ok {
				return nil, &ParseError{start, fmt.Sprintf("invalid cell reference %q", src[start:i])}
			}
			toks = append(toks, token{tokRef, strings.ToUpper(src[start:i]), start})
		case c == '#':
			end := strings.IndexByte(src[i+1:], '#')
			if end < 0 {
				return nil, &ParseError{start, "unterminated logical operator"}
			}
			op := strings.ToUpper(src[i : i+end+2])
			if op != "#AND#" && op != "#OR#" && op != "#NOT#" {
				return nil, &ParseError{start, "unknown operator " + op}
			}
			i += end + 2
			toks = append(toks, token{tokOp, op, start})
		case strings.HasPrefix(src[i:], ".."), strings.HasPrefix(src[i:], "<="),
			strings.HasPrefix(src[i:], ">="), strings.HasPrefix(src[i:], "<>"):
			i += 2
			toks = append(toks, token{tokOp, src[start:i], start})
		case strings.IndexByte("+-*/^=<>&(),;:", c) >= 0:
			i++
			toks = append(toks, token{tokOp, string(c), start})
		default:
			return nil, &ParseError{start, fmt.Sprintf("unexpected %q", c)}
		}
	}
	return append(toks, token{tokEOF, "", len(src)}), nil
}

// Binding powers, lowest to highest, following 1-2-3's precedence table.
var infixPower = map[string]int{
	"#OR#": 10, "#AND#": 10,
	"=": 30, "<>": 30, "<": 30, ">": 30, "<=": 30, ">=": 30,
	"&": 35,
	"+": 40, "-": 40,
	"*": 50, "/": 50,
	"^": 70,
}

const (
	notPower   = 20
	unaryPower = 60
)

type parser struct {
	toks []token
	pos  int
}

// Parse parses a value entry. A leading '=' is accepted for users coming
// from modern spreadsheets; 1-2-3 itself starts formulas with '+'.
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
		err = &ParseError{p.peek().pos, "unexpected " + p.peek().text}
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

func (p *parser) expect(op string) error {
	if t := p.next(); t.kind != tokOp || t.text != op {
		return &ParseError{t.pos, "expected " + op}
	}
	return nil
}

func (p *parser) expr(minPower int) (Node, error) {
	left, err := p.prefix()
	if err != nil {
		return nil, err
	}
	for {
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
			return nil, &ParseError{t.pos, "invalid number " + t.text}
		}
		return numLit{v}, nil
	case tokStr:
		return strLit{t.text}, nil
	case tokRef:
		a, _ := ParseAddr(t.text)
		if n := p.peek(); n.kind == tokOp && (n.text == ".." || n.text == ":") {
			p.next()
			end := p.next()
			if end.kind != tokRef {
				return nil, &ParseError{end.pos, "expected cell reference after " + n.text}
			}
			b, _ := ParseAddr(end.text)
			return rangeNode{NewRect(a, b)}, nil
		}
		return refNode{a}, nil
	case tokFunc:
		return p.call(t)
	case tokOp:
		switch t.text {
		case "(":
			n, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			return n, p.expect(")")
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
		return nil, &ParseError{t.pos, "incomplete formula"}
	}
	return nil, &ParseError{t.pos, "unexpected " + t.text}
}

func (p *parser) call(fn token) (Node, error) {
	if _, ok := functions[fn.text]; !ok {
		return nil, &ParseError{fn.pos, "unknown function @" + fn.text}
	}
	n := callNode{name: fn.text}
	if t := p.peek(); t.kind != tokOp || t.text != "(" {
		return n, nil
	}
	p.next()
	if t := p.peek(); t.kind == tokOp && t.text == ")" {
		p.next()
		return n, nil
	}
	for {
		arg, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		n.args = append(n.args, arg)
		t := p.next()
		if t.kind == tokOp && t.text == ")" {
			return n, nil
		}
		if t.kind != tokOp || (t.text != "," && t.text != ";") {
			return nil, &ParseError{t.pos, "expected , or )"}
		}
	}
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
