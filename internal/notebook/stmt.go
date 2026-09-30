package notebook

import (
	"slices"
	"strconv"
	"strings"
)

// statements splits a source into its statements, reading view, the
// source with its comments and strings blanked.
func statements(src string, view []byte) []Stmt {
	var out []Stmt
	depth, from := 0, 0
	for i := 0; i <= len(view); i++ {
		if i < len(view) {
			switch c := view[i]; {
			case c == '(' || c == '[' || c == '{':
				depth++
				continue
			case c == ')' || c == ']' || c == '}':
				depth = max(depth-1, 0)
				continue
			case c != ';' && c != '\n', depth > 0, c == '\n' && continues(src, view, from, i):
				continue
			}
		}
		if st, ok := statement(view, from, i); ok {
			out = append(out, st)
		}
		from = i + 1
	}
	return out
}

// continues reports whether the statement from from goes on past the
// line break at i: its line ends in | or in an assignment's =, or the
// next line, past comment lines, starts with |.
func continues(src string, view []byte, from, i int) bool {
	before := strings.TrimRight(string(view[from:i]), " \t\r")
	if strings.HasSuffix(before, "|") || strings.HasSuffix(before, "=") {
		return true
	}
	for j := i + 1; j < len(view); {
		end := j
		for end < len(view) && view[end] != '\n' {
			end++
		}
		line := strings.TrimSpace(string(view[j:end]))
		switch {
		case strings.HasPrefix(line, "|"):
			return true
		case line != "" || strings.TrimSpace(src[j:end]) == "":
			return false // code, or a blank line
		}
		j = end + 1 // a comment line
	}
	return false
}

// statement is the statement in view[from:to], if there's one.
func statement(view []byte, from, to int) (Stmt, bool) {
	for from < to && isSpace(view[from]) {
		from++
	}
	for to > from && isSpace(view[to-1]) {
		to--
	}
	if from == to {
		return Stmt{}, false
	}
	st := Stmt{From: from, To: to, Body: from}
	name, rest := word(view, from, to)
	rest = skipSpaces(view, rest, to, " \t")
	switch {
	case rest < to && view[rest] == '=' && ValidName(name) == nil && !opAt(view, rest+1, to):
		st.Name, st.Body = name, skipSpaces(view, rest+1, to, " \t\r\n")
	case name == "let" || name == "mut" || name == "const":
		st.Local, _ = word(view, skipSpaces(view, rest, to, " \t"), to)
	}
	return st, true
}

// opAt reports whether an = ending at i is part of an operator: ==, =~.
func opAt(view []byte, i, to int) bool { return i < to && (view[i] == '=' || view[i] == '~') }

// word is the word of letters, digits and _ at from, and where it ends.
func word(view []byte, from, to int) (string, int) {
	i := from
	for i < to && isWord(view[i]) {
		i++
	}
	return string(view[from:i]), i
}

func skipSpaces(view []byte, i, to int, spaces string) int {
	for i < to && strings.IndexByte(spaces, view[i]) >= 0 {
		i++
	}
	return i
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// OutputVar is the name of the variable that holds a cell's output in
// the record a run hands back when it also hands back variables
// (nushell.ExecVars).
const OutputVar = "__out"

// Command is what nu runs for the source: each statement but the last
// that assigns a name as `let name = ...`, the last without its `name =`
// (its value is the output), the ranges of sheets renamed to variables
// of their own ($__sheet1), and the comments blanked, so what's left
// keeps its place. When statements before the last assign names the
// last doesn't, those are exports, and the command ends in a record of
// the output (as __out) and each of them.
func (s Source) Command() (cmd string, exports []string, ranges []SheetRef) {
	return s.command(true)
}

// StreamCommand is what nu runs for the source as a stream: Command
// without the record, so the last statement's values stream as they
// come, and the names the statements before it assign stay the run's.
func (s Source) StreamCommand() string {
	cmd, _, _ := s.command(false)
	return cmd
}

func (s Source) command(record bool) (cmd string, exports []string, ranges []SheetRef) {
	var edits []edit
	for _, r := range s.Ranges {
		ref := s.Text[r[0]+len(sheetVar) : r[1]]
		i := slices.IndexFunc(ranges, func(x SheetRef) bool { return x.Ref == ref })
		if i < 0 {
			ranges = append(ranges, SheetRef{Ref: ref, Var: "__sheet" + strconv.Itoa(len(ranges)+1)})
			i = len(ranges) - 1
		}
		edits = append(edits, edit{r[0], r[1], "$" + ranges[i].Var})
	}
	if len(s.Stmts) == 0 {
		return apply(s.code, edits), nil, ranges
	}
	last := s.Stmts[len(s.Stmts)-1]
	for _, st := range s.Stmts[:len(s.Stmts)-1] {
		if st.Name == "" {
			continue
		}
		edits = append(edits, edit{st.From, st.Body, "let " + st.Name + " = "})
		if st.Name != last.Name && !slices.Contains(exports, st.Name) {
			exports = append(exports, st.Name)
		}
	}
	if last.Name != "" {
		edits = append(edits, edit{last.From, last.Body, ""})
	}
	if !record {
		exports = nil
	}
	if len(exports) > 0 {
		var rec strings.Builder
		rec.WriteString(")")
		for _, e := range exports {
			rec.WriteString(", " + e + ": $" + e)
		}
		rec.WriteString("}")
		edits = append(edits, edit{last.Body, last.Body, "{" + OutputVar + ": ("}, edit{last.To, last.To, rec.String()})
	}
	return apply(s.code, edits), exports, ranges
}

// edit replaces b[from:to] with text.
type edit struct {
	from, to int
	text     string
}

// apply makes edits to b, which don't overlap.
func apply(b []byte, edits []edit) string {
	slices.SortStableFunc(edits, func(x, y edit) int {
		if x.from != y.from {
			return x.from - y.from
		}
		return x.to - y.to
	})
	var out strings.Builder
	at := 0
	for _, e := range edits {
		out.Write(b[at:e.from])
		out.WriteString(e.text)
		at = e.to
	}
	out.Write(b[at:])
	return out.String()
}
