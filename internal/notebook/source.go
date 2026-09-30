package notebook

import (
	"slices"
	"strings"
)

// How 012 reads a code cell's source, as nu does: its comments and
// strings apart from its code, and its statements. A statement is a
// line, or a part of one between `;`, at the top level (outside
// brackets); a pipeline goes on over the next line when that line
// starts with `|` (comment lines between allowed, a blank line not), or
// when its own line ends in `|` or in an assignment's `=`. A statement
// `name = pipeline` assigns name, as nu's `let name = pipeline` does.

// Source is a code cell's source as 012 reads it.
type Source struct {
	Text string
	// Stmts are its statements, in order.
	Stmts []Stmt
	// Comments are its comments' bytes, # to the end of the line, and
	// Strings its strings', quotes included.
	Comments, Strings [][2]int
	// Ranges are where it reads ranges of sheets, each $sheet.A1:C9
	// whole.
	Ranges [][2]int
	// code is Text with its comments blanked, and view is code with its
	// strings blanked too, but for the code interpolated into them:
	// where names are read.
	code, view []byte
}

// Stmt is a statement of a source, by byte offsets into it.
type Stmt struct {
	// From and To are its bytes, without the spaces and comments around.
	From, To int
	// Body is where what it runs starts: after `name =`, or From.
	Body int
	// Name is what `name = pipeline` assigns, or "".
	Name string
	// Local is the variable `let name = ...` or `mut name = ...` binds,
	// which only the cell's later statements read, or "".
	Local string
}

// Parse reads src.
func Parse(src string) Source {
	s := &scanner{src: src, view: []byte(src)}
	s.code(0, 0)
	code := []byte(src)
	for _, c := range s.comments {
		blank(code, c[0], c[1])
	}
	return Source{Text: src, Stmts: statements(src, s.view), Comments: s.comments, Strings: s.strs, Ranges: s.ranges, code: code, view: s.view}
}

// Name is what the source's last statement assigns: the cell's output's
// name, or "".
func (s Source) Name() string {
	if len(s.Stmts) == 0 {
		return ""
	}
	return s.Stmts[len(s.Stmts)-1].Name
}

// Assigned are the names the statements assign, in the order they
// first do.
func (s Source) Assigned() []string {
	var out []string
	for _, st := range s.Stmts {
		if st.Name != "" && !slices.Contains(out, st.Name) {
			out = append(out, st.Name)
		}
	}
	return out
}

// Refs are the names the source reads as $name from outside it, in the
// order it first names them: not a name an earlier statement assigns or
// binds with let, nor nushell's variables and the notebook's own
// ($selection, $sheet), nor one in a comment or a string.
func (s Source) Refs() []string {
	var out []string
	v := s.view
	for i := 0; i < len(v); i++ {
		if v[i] != '$' {
			continue
		}
		j := i + 1
		for j < len(v) && isWord(v[j]) {
			j++
		}
		name := string(v[i+1 : j])
		if name != "" && ValidName(name) == nil && !slices.Contains(out, name) && !s.local(name, i) {
			out = append(out, name)
		}
		i = j - 1
	}
	return out
}

// local reports whether name, read at offset at, is a variable of the
// cell's own: a statement before assigns or binds it.
func (s Source) local(name string, at int) bool {
	return slices.ContainsFunc(s.Stmts, func(st Stmt) bool {
		return st.To <= at && (st.Name == name || st.Local == name)
	})
}

// ReadsSelection reports whether the source reads $selection.
func (s Source) ReadsSelection() bool {
	v, want := string(s.view), "$"+Selection
	for i := 0; ; {
		k := strings.Index(v[i:], want)
		if k < 0 {
			return false
		}
		end := i + k + len(want)
		if end == len(v) || !isWord(v[end]) {
			return true
		}
		i = end
	}
}

// Heads are the bytes of each assignment's `name =`, up to what it runs.
func (s Source) Heads() [][2]int {
	var out [][2]int
	for _, st := range s.Stmts {
		if st.Name != "" {
			out = append(out, [2]int{st.From, st.Body})
		}
	}
	return out
}

// scanner tells a source's comments and strings from its code.
type scanner struct {
	src                    string
	view                   []byte
	comments, strs, ranges [][2]int
}

// code reads code from i up to the bracket close that ends it (0 for
// the end of the source), returning where it stopped.
func (s *scanner) code(i int, close byte) int {
	for i < len(s.src) {
		c := s.src[i]
		switch {
		case close != 0 && c == close:
			return i
		case c == '(' || c == '[' || c == '{':
			i = s.code(i+1, closer(c))
			if i < len(s.src) {
				i++
			}
		case c == '#' && s.tokenStart(i, close != 0):
			i = s.comment(i)
		case c == '"' || c == '\'' || c == '`':
			i = s.str(i, i+1, c)
		case c == '$':
			i = s.dollar(i)
		case c == 'r' && s.tokenStart(i, close != 0) && rawHashes(s.src[i+1:]) > 0:
			i = s.raw(i)
		default:
			i++
		}
	}
	return i
}

func closer(c byte) byte {
	switch c {
	case '(':
		return ')'
	case '[':
		return ']'
	}
	return '}'
}

// tokenStart reports whether a token may start at i: after a space, a
// pipe, a ; or an opening bracket, and inside brackets after a , or a
// : too, where nu reads a # as a comment.
func (s *scanner) tokenStart(i int, inside bool) bool {
	if i == 0 {
		return true
	}
	p := s.src[i-1]
	return strings.IndexByte(" \t\r\n|;([{", p) >= 0 || inside && (p == ',' || p == ':')
}

func (s *scanner) comment(i int) int {
	j := i
	for j < len(s.src) && s.src[j] != '\n' {
		j++
	}
	s.comments = append(s.comments, [2]int{i, j})
	blank(s.view, i, j)
	return j
}

// str reads a quoted string opened at start, its text from i: a double
// quoted one has escapes, a single quoted or backquoted one none.
func (s *scanner) str(start, i int, q byte) int {
	for i < len(s.src) && s.src[i] != q {
		if q == '"' && s.src[i] == '\\' {
			i++
		}
		i++
	}
	i = min(i+1, len(s.src))
	s.strs = append(s.strs, [2]int{start, i})
	blank(s.view, start, i)
	return i
}

// dollar reads what starts with $ at i: an interpolated string, a range
// of a sheet, or a variable.
func (s *scanner) dollar(i int) int {
	rest := s.src[i+1:]
	switch {
	case strings.HasPrefix(rest, `"`) || strings.HasPrefix(rest, "'"):
		return s.interpolated(i)
	case strings.HasPrefix(s.src[i:], sheetVar) && (i == 0 || !isWord(s.src[i-1])):
		from := i + len(sheetVar)
		if n := refLen(s.src[from:]); n > 0 {
			s.ranges = append(s.ranges, [2]int{i, from + n})
			blank(s.view, from, from+n) // its quotes and brackets aren't code
			return from + n
		}
	}
	return i + 1
}

// interpolated reads $"..." or $'...' at i: text, blanked, and the code
// in its parentheses, read as code.
func (s *scanner) interpolated(i int) int {
	q := s.src[i+1]
	j := i + 2
	blank(s.view, i, j)
	for j < len(s.src) && s.src[j] != q {
		switch {
		case q == '"' && s.src[j] == '\\':
			blank(s.view, j, min(j+2, len(s.src)))
			j += 2
		case s.src[j] == '(':
			j = s.code(j+1, ')')
			if j < len(s.src) {
				j++
			}
		default:
			blank(s.view, j, j+1)
			j++
		}
	}
	j = min(j+1, len(s.src))
	blank(s.view, j-1, j)
	s.strs = append(s.strs, [2]int{i, j})
	return j
}

// rawHashes is how many # open a raw string at the start of s, r#'...'#
// without its r, or 0.
func rawHashes(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n == len(s) || s[n] != '\'' {
		return 0
	}
	return n
}

// raw reads a raw string at i, to its ' and as many # as opened it.
func (s *scanner) raw(i int) int {
	n := rawHashes(s.src[i+1:])
	end := "'" + strings.Repeat("#", n)
	body := i + 2 + n
	j := len(s.src)
	if k := strings.Index(s.src[body:], end); k >= 0 {
		j = body + k + len(end)
	}
	s.strs = append(s.strs, [2]int{i, j})
	blank(s.view, i, j)
	return j
}

// blank makes b[from:to] spaces, but for its line breaks.
func blank(b []byte, from, to int) {
	for i := from; i < to && i < len(b); i++ {
		if b[i] != '\n' {
			b[i] = ' '
		}
	}
}
