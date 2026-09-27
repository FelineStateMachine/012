package formula

import (
	"fmt"
	"strings"
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
	tokSheet  // a sheet name and its "!", as in Sheet2!A1 or 'My Sheet'!A1; text is the name
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

// Operators: two-character ones are matched first, then single characters.
// #AND#, #OR# and #NOT# are 1-2-3's logical operators.
var (
	twoCharOps   = [...]string{"..", "<=", ">=", "<>"}
	hashOps      = [...]string{"#AND#", "#OR#", "#NOT#"}
	oneCharOps   = "+-*/^=<>&(),;:%"
	refErrorText = "#REF!"
)

func isIdentStart(c byte) bool { return isLetter(c) || c == '_' || c == '$' }

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) || c == '.' }

// lexer splits a formula (without its "=") into tokens.
type lexer struct {
	src  string
	i    int
	toks []token
}

func lex(src string) ([]token, error) {
	lx := lexer{src: src}
	for lx.i < len(src) {
		if err := lx.next(); err != nil {
			return nil, err
		}
	}
	return append(lx.toks, token{tokEOF, "", len(src)}), nil
}

// emit adds a token that started at start and ends at the lexer's position.
func (lx *lexer) emit(kind tokKind, text string, start int) {
	lx.toks = append(lx.toks, token{kind, text, start})
}

func (lx *lexer) next() error {
	c := lx.src[lx.i]
	switch {
	case c == ' ':
		lx.i++
	case isDigit(c) || c == '.' && lx.i+1 < len(lx.src) && isDigit(lx.src[lx.i+1]):
		lx.number()
	case c == '"':
		return lx.str()
	case c == '\'':
		return lx.quotedSheet()
	case c == '@' || isIdentStart(c):
		return lx.ident()
	case c == '#':
		return lx.hash()
	default:
		return lx.operator()
	}
	return nil
}

// number reads 12, 1.5, .5 or 1.5E-3. A ".." ends it: A1..B3's range
// operator follows digits.
func (lx *lexer) number() {
	src, start := lx.src, lx.i
	for lx.i < len(src) && (isDigit(src[lx.i]) || src[lx.i] == '.' && !strings.HasPrefix(src[lx.i:], "..")) {
		lx.i++
	}
	lx.exponent()
	lx.emit(tokNum, src[start:lx.i], start)
}

// exponent reads E3, E+3 or E-3 after a number's digits, if there is one.
func (lx *lexer) exponent() {
	src, j := lx.src, lx.i
	if j >= len(src) || src[j] != 'e' && src[j] != 'E' {
		return
	}
	j++
	if j < len(src) && (src[j] == '+' || src[j] == '-') {
		j++
	}
	if j >= len(src) || !isDigit(src[j]) {
		return
	}
	for j < len(src) && isDigit(src[j]) {
		j++
	}
	lx.i = j
}

// str reads a string literal. Strings use "" to escape a quote, as in
// Sheets.
func (lx *lexer) str() error {
	src, start := lx.src, lx.i
	var escaped strings.Builder // the text so far, once it has an escape
	from := start + 1           // the start of the text not yet copied
	for i := from; i < len(src); i++ {
		if src[i] != '"' {
			continue
		}
		if i+1 < len(src) && src[i+1] == '"' {
			escaped.WriteString(src[from : i+1])
			i++
			from = i + 1
			continue
		}
		text := src[from:i]
		if escaped.Len() > 0 {
			escaped.WriteString(text)
			text = escaped.String()
		}
		lx.i = i + 1
		lx.emit(tokStr, text, start)
		return nil
	}
	return &ParseError{start, "Missing closing quote"}
}

// quotedSheet reads a quoted sheet name and its "!", with ” for a
// quote: 'Q3 ”26'!A1.
func (lx *lexer) quotedSheet() error {
	start := lx.i
	name, end, ok := quotedName(lx.src, start)
	if !ok || end >= len(lx.src) || lx.src[end] != '!' {
		return &ParseError{start, "Expected ! after a quoted sheet name"}
	}
	lx.i = end + 1
	lx.emit(tokSheet, name, start)
	return nil
}

// ident reads a reference, a name, a function name, a sheet name before
// "!", or a 1-2-3 style @function, which may omit its parentheses (@PI).
func (lx *lexer) ident() error {
	src, start := lx.src, lx.i
	at := src[start] == '@'
	if at {
		lx.i++
	}
	for lx.i < len(src) && isIdentPart(src[lx.i]) && !strings.HasPrefix(src[lx.i:], "..") {
		lx.i++
	}
	if !at && lx.i < len(src) && src[lx.i] == '!' {
		lx.emit(tokSheet, src[start:lx.i], start)
		lx.i++
		return nil
	}
	name := src[start:lx.i]
	if at {
		name = name[1:]
	}
	if name == "" {
		return &ParseError{start, "Missing function name after @"}
	}
	kind := tokIdent
	if at || lx.parenFollows() {
		kind = tokFunc
	}
	lx.emit(kind, strings.ToUpper(name), start)
	return nil
}

// parenFollows reports whether "(" comes next, after any spaces.
func (lx *lexer) parenFollows() bool {
	j := lx.i
	for j < len(lx.src) && lx.src[j] == ' ' {
		j++
	}
	return j < len(lx.src) && lx.src[j] == '('
}

// hash reads #REF! or one of 1-2-3's #AND#, #OR# and #NOT#.
func (lx *lexer) hash() error {
	start, rest := lx.i, lx.src[lx.i:]
	if len(rest) >= len(refErrorText) && strings.EqualFold(rest[:len(refErrorText)], refErrorText) {
		lx.i += len(refErrorText)
		lx.emit(tokRefErr, refErrorText, start)
		return nil
	}
	end := strings.IndexByte(rest[1:], '#')
	if end < 0 {
		return &ParseError{start, "Unexpected #"}
	}
	op := strings.ToUpper(rest[:end+2])
	for _, known := range hashOps {
		if op == known {
			lx.i += end + 2
			lx.emit(tokOp, known, start)
			return nil
		}
	}
	return &ParseError{start, "Unknown operator " + op}
}

func (lx *lexer) operator() error {
	start, rest := lx.i, lx.src[lx.i:]
	for _, op := range twoCharOps {
		if strings.HasPrefix(rest, op) {
			lx.i += 2
			lx.emit(tokOp, op, start)
			return nil
		}
	}
	if strings.IndexByte(oneCharOps, rest[0]) >= 0 {
		lx.i++
		lx.emit(tokOp, rest[:1], start)
		return nil
	}
	return &ParseError{start, fmt.Sprintf("Unexpected %q", rest[0])}
}
