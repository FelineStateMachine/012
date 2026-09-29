// Package formula reads the formula being typed in the UI: the
// reference F4 cycles through absolute markers, and the word and function
// call around the caret that formula assistance follows. It works on the
// entry's runes alone.
package formula

import (
	"slices"
	"strings"
	"unicode"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// CycleRef finds the reference (or range) at or just before pos in a
// formula and returns the text with its absolute markers cycled, and the
// caret moved to the reference's end.
func CycleRef(buf []rune, pos int) ([]rune, int, bool) {
	corners, ok := newRefScan(buf).corners(pos)
	if !ok {
		return nil, 0, false
	}
	next := nextMarkers(string(buf[corners[0][0]:corners[0][1]]))
	out := slices.Clone(buf[:corners[0][0]])
	for i, c := range corners {
		if i > 0 {
			out = append(out, buf[corners[i-1][1]:c[0]]...)
		}
		out = append(out, []rune(withMarkers(string(buf[c[0]:c[1]]), next))...)
	}
	caret := len(out)
	out = append(out, buf[corners[len(corners)-1][1]:]...)
	return out, caret, true
}

// refScan finds cell references in a formula's text, outside strings.
type refScan struct {
	buf    []rune
	quoted []bool // inside a string, quotes included
}

func newRefScan(buf []rune) refScan {
	quoted := make([]bool, len(buf))
	in := false
	for i, r := range buf {
		if r == '"' {
			in = !in
		}
		quoted[i] = in || r == '"'
	}
	return refScan{buf: buf, quoted: quoted}
}

func (s refScan) isRefRune(i int) bool {
	r := s.buf[i]
	return !s.quoted[i] && (r == '$' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
}

// word returns the bounds of the run of reference characters around i.
func (s refScan) word(i int) (int, int) {
	start, end := i, i
	for start > 0 && s.isRefRune(start-1) {
		start--
	}
	for end < len(s.buf) && s.isRefRune(end) {
		end++
	}
	return start, end
}

// isRef reports whether buf[start:end] is a cell reference, not a name
// (@x) or a function (x().
func (s refScan) isRef(start, end int) bool {
	buf := s.buf
	if start == end || start > 0 && buf[start-1] == '@' || end < len(buf) && buf[end] == '(' {
		return false
	}
	_, ok := sheet.ParseAddr(string(buf[start:end]))
	return ok
}

// sep returns the length of a range separator at i, ":" or "..", or 0.
func (s refScan) sep(i int) int {
	switch {
	case i < len(s.buf) && s.buf[i] == ':':
		return 1
	case i+1 < len(s.buf) && s.buf[i] == '.' && s.buf[i+1] == '.':
		return 2
	}
	return 0
}

// corners returns the bounds of the reference at pos, widened to both
// corners of a range written A1:B2 or A1..B2.
func (s refScan) corners(pos int) ([][2]int, bool) {
	start, end := s.word(pos)
	if !s.isRef(start, end) {
		return nil, false
	}
	if n := s.sep(end); n > 0 {
		if s2, e2 := s.word(end + n); s2 == end+n && s.isRef(s2, e2) {
			return [][2]int{{start, end}, {s2, e2}}, true
		}
		return [][2]int{{start, end}}, true
	}
	for n := 1; n <= 2 && start-n > 0; n++ {
		if s.sep(start-n) == n {
			if s0, e0 := s.word(start - n - 1); e0 == start-n && s.isRef(s0, e0) {
				return [][2]int{{s0, e0}, {start, end}}, true
			}
			break
		}
	}
	return [][2]int{{start, end}}, true
}

// Caret describes the formula text before the caret: the word being
// typed, if it may be a function, name or sheet, and the innermost
// function call the caret is in. A sheet name being typed in quotes is a
// word starting with the quote, e.g. "'Q3 p". In a structured
// reference's brackets, Sales[Am, the word is the column or item being
// typed and Table the table's name.
type Caret struct {
	Word      string
	WordStart int
	Fn        string // e.g. "SUM", or "" outside any function's parentheses
	Arg       int    // index of the argument the caret is in
	Table     string // the table whose brackets the caret is in, as written, or ""
}

func isWordRune(r rune) bool {
	return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) || r == '_' || r == '.' || r == '$' || r == '@'
}

// ScanCaret reads a formula up to pos the way the lexer does: strings are
// skipped, parentheses nest, and commas or semicolons at the innermost
// level separate arguments.
func ScanCaret(buf []rune, pos int) Caret {
	s := caretScan{start: -1, quote: -1}
	for i := 1; i < pos; i++ { // buf[0] is "=", "+" or "-"
		s.step(buf, i)
	}
	if s.inStr {
		return Caret{}
	}
	var c Caret
	if s.depth > 0 {
		c.Table, c.WordStart = s.table, s.item
		c.Word = string(buf[s.item:pos])
		s.quote = -1
	}
	if s.quote >= 0 {
		c.Word, c.WordStart = string(buf[s.quote:pos]), s.quote
	}
	for i := len(s.stack) - 1; i >= 0; i-- {
		if s.stack[i].fn != "" {
			c.Fn, c.Arg = s.stack[i].fn, s.stack[i].arg
			break
		}
	}
	if s.depth == 0 && s.quote < 0 && s.start >= 0 && (pos == len(buf) || !isWordRune(buf[pos])) {
		w := []rune(strings.TrimPrefix(string(buf[s.start:pos]), "@"))
		if len(w) > 0 && (unicode.IsLetter(w[0]) || w[0] == '_') {
			c.Word, c.WordStart = string(w), pos-len(w)
		}
	}
	return c
}

// caretScan is ScanCaret's state as it reads.
type caretScan struct {
	stack []callFrame // the calls open at this point, innermost last
	inStr bool
	quote int    // start of a quoted sheet name being read, or -1
	start int    // start of the word being read, or -1
	prev  string // the word just before a space or "(" (SUM ( is allowed)
	// In a structured reference's brackets: how deep, the table's name,
	// where the item being typed starts, and whether the next character
	// is escaped by '.
	depth int
	table string
	item  int
	esc   bool
}

// callFrame is a function call the caret may be in.
type callFrame struct {
	fn  string
	arg int
}

// step reads buf[i].
func (s *caretScan) step(buf []rune, i int) {
	r := buf[i]
	if s.depth > 0 {
		s.bracket(r, i)
		return
	}
	if s.inStr {
		s.inStr = r != '"'
		return
	}
	if s.quote >= 0 {
		if r == '\'' {
			s.quote = -1
		}
		return
	}
	if isWordRune(r) {
		if s.start < 0 {
			s.start = i
		}
		return
	}
	if s.start >= 0 {
		s.prev, s.start = string(buf[s.start:i]), -1
	}
	switch r {
	case ' ':
		return // keep prev for "SUM ("
	case '"':
		s.inStr = true
	case '\'':
		s.quote = i
	case '[':
		s.table, s.depth, s.item = s.prev, 1, i+1
	case '(':
		s.stack = append(s.stack, callFrame{fn: strings.ToUpper(strings.TrimPrefix(s.prev, "@"))})
	case ')':
		if len(s.stack) > 0 {
			s.stack = s.stack[:len(s.stack)-1]
		}
	case ',', ';':
		if len(s.stack) > 0 {
			s.stack[len(s.stack)-1].arg++
		}
	}
	s.prev = ""
}

// bracket reads r, at i, inside a structured reference's brackets:
// Sales[[#Headers],[Amount]]. Each [ or item separator starts an item;
// ' escapes the character after it.
func (s *caretScan) bracket(r rune, i int) {
	switch {
	case s.esc:
		s.esc = false
	case r == '\'':
		s.esc = true
	case r == '[':
		s.depth++
		s.item = i + 1
	case r == ']':
		if s.depth--; s.depth == 0 {
			s.table = ""
		}
		s.item = i + 1
	case (r == ',' || r == ';') && s.depth == 1:
		s.item = i + 1
	}
}

// SplitArgs splits a signature's arguments at the top-level commas, so
// "[value2, ...]" stays one part.
func SplitArgs(args string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range args {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(args[start:i]))
				start = i + 1
			}
		}
	}
	if s := strings.TrimSpace(args[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// ArgPart maps an argument index to the part of the signature that
// describes it: a repeating part like "[value2, ...]" covers every
// argument from its position on. It returns -1 past the last argument.
func ArgPart(parts []string, arg int, variadic bool) int {
	rep := slices.IndexFunc(parts, func(p string) bool { return strings.Contains(p, "...") })
	switch {
	case rep >= 0 && variadic && arg >= rep:
		return rep
	case arg < len(parts):
		return arg
	}
	return -1
}

// nextMarkers returns the absolute markers that follow ref's in F4's
// cycle: A1 -> $A$1 -> A$1 -> $A1 -> A1.
func nextMarkers(ref string) markers {
	col := strings.HasPrefix(ref, "$")
	row := strings.Contains(ref[1:], "$")
	switch {
	case !col && !row:
		return markers{true, true}
	case col && row:
		return markers{false, true}
	case row:
		return markers{true, false}
	}
	return markers{}
}

// markers says which parts of a reference are absolute.
type markers struct{ col, row bool }

// withMarkers rewrites ref with the given absolute markers.
func withMarkers(ref string, marks markers) string {
	ref = strings.ToUpper(strings.ReplaceAll(ref, "$", ""))
	i := strings.IndexAny(ref, "0123456789")
	var b strings.Builder
	if marks.col {
		b.WriteByte('$')
	}
	b.WriteString(ref[:i])
	if marks.row {
		b.WriteByte('$')
	}
	b.WriteString(ref[i:])
	return b.String()
}
