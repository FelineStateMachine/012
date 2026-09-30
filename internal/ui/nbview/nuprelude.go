package nbview

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// What nu is asked about a cell, and how its answers map back to the
// cell's source.

// prepared is the text nu is asked about: a prelude declaring the
// notebook's variables, then the source masked, starting at byte at.
// Masking keeps every byte of the source in its place, so what nu says
// of the text is where it says it in the source.
type prepared struct {
	text  string
	at    int
	heads [][2]int // the bytes of the source that are its `name =`s
}

// prepare masks src and declares what it reads: `sales = ls` is asked
// about as `        ls`, $sheet.A1:C9 as $__s1_______, and the prelude
// declares $__s1, $selection, $sheet, names and the names the cell
// assigns, each `let x: any = []` so nu takes it for whatever it holds.
func prepare(src string, names []string) prepared {
	b := []byte(src)
	parsed := notebook.Parse(src)
	heads := parsed.Heads()
	for _, h := range heads {
		for i := h[0]; i < h[1]; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}
	names = append(parsed.Assigned(), names...)
	var pre strings.Builder
	declared := map[string]bool{}
	declare := func(name, value string) {
		if !declared[name] {
			declared[name] = true
			pre.WriteString("let " + name + ": any = " + value + "\n")
		}
	}
	for i, s := range parsed.Ranges {
		v := "__s" + strconv.Itoa(i+1)
		if len(v)+1 > s[1]-s[0] {
			continue
		}
		v += strings.Repeat("_", s[1]-s[0]-len(v)-1)
		copy(b[s[0]:s[1]], "$"+v)
		declare(v, "[]")
	}
	declare(notebook.Selection, "[]")
	declare("sheet", "{}")
	for _, nm := range names {
		if notebook.ValidName(nm) == nil {
			declare(nm, "[]")
		}
	}
	return prepared{text: pre.String() + string(b), at: pre.Len(), heads: heads}
}

// inHead reports whether src[from:to] is part of an assignment's head.
func (p prepared) inHead(from, to int) bool {
	return slices.ContainsFunc(p.heads, func(h [2]int) bool { return from < h[1] && to > h[0] })
}

// spans are src's syntax roles from nu's shapes of p's text: the
// tokenizer's for the names the cell assigns and its comments, which nu
// doesn't report, and a number's unit (the kb of 1kb) as part of the
// number.
func (p prepared) spans(src string, shapes []nushell.Shape) []Span {
	var out []Span
	for _, s := range (Tokens{}).Highlight(context.Background(), src) {
		if p.inHead(s.From, s.To) || s.Kind == theme.SyntaxComment {
			out = append(out, s)
		}
	}
	for i, s := range shapes {
		from, to := s.From-p.at, s.To-p.at
		if from < 0 || to > len(src) || from >= to || p.inHead(from, to) {
			continue
		}
		kind, ok := shapeKind(s.Shape, src[from:to])
		if s.Shape == "shape_string" && i > 0 && shapes[i-1].To == s.From && isNumberShape(shapes[i-1].Shape) {
			kind, ok = theme.SyntaxNumber, true
		}
		if ok {
			out = append(out, Span{From: from, To: to, Kind: kind})
		}
	}
	return out
}

// shapeRoles are the syntax roles of nu's shapes; the rest (flags,
// brackets, garbage, which the checker underlines) show plain.
var shapeRoles = map[string]theme.Syntax{
	"shape_internalcall": theme.SyntaxCommand, "shape_external": theme.SyntaxCommand, "shape_external_resolved": theme.SyntaxCommand,
	"shape_keyword": theme.SyntaxKeyword, "shape_bool": theme.SyntaxKeyword, "shape_nothing": theme.SyntaxKeyword,
	"shape_string": theme.SyntaxString, "shape_string_interpolation": theme.SyntaxString, "shape_raw_string": theme.SyntaxString,
	"shape_glob_interpolation": theme.SyntaxString, "shape_globpattern": theme.SyntaxString, "shape_filepath": theme.SyntaxString,
	"shape_directory": theme.SyntaxString, "shape_externalarg": theme.SyntaxString,
	"shape_variable": theme.SyntaxVariable, "shape_vardecl": theme.SyntaxVariable,
	"shape_int": theme.SyntaxNumber, "shape_float": theme.SyntaxNumber, "shape_filesize": theme.SyntaxNumber,
	"shape_duration": theme.SyntaxNumber, "shape_datetime": theme.SyntaxNumber, "shape_binary": theme.SyntaxNumber,
	"shape_operator": theme.SyntaxOperator, "shape_pipe": theme.SyntaxOperator, "shape_redirection": theme.SyntaxOperator,
	"shape_range": theme.SyntaxOperator,
}

// shapeKind is the role of text nu shaped so: let, if and def are
// keywords to 012, as the tokenizer has them, though nu calls them.
func shapeKind(shape, text string) (theme.Syntax, bool) {
	k, ok := shapeRoles[shape]
	if k == theme.SyntaxCommand && slices.Contains(keywords, text) {
		k = theme.SyntaxKeyword
	}
	return k, ok
}

func isNumberShape(shape string) bool { return shape == "shape_int" || shape == "shape_float" }

// replaced is where completion w starts replacing src before offset:
// the longest stretch ending there, from a word's start or after a . or
// /, that w starts with (ignoring case), since nu doesn't say. When none
// is, the word at the caret.
func replaced(src string, offset int, w string) int {
	start := offset
	for start > 0 && !strings.ContainsRune("|;({[\n", rune(src[start-1])) {
		start--
	}
	lw := strings.ToLower(w)
	for i := start; i < offset; i++ {
		if (i == start || strings.ContainsRune(" \t./", rune(src[i-1]))) && strings.HasPrefix(lw, strings.ToLower(src[i:offset])) {
			return i
		}
	}
	if w != "" && strings.ContainsRune("'\"`", rune(w[0])) { // a quoted path: from its quote
		if q := strings.LastIndexAny(src[start:offset], "'\"`"); q >= 0 {
			return start + q
		}
	}
	i := offset
	for i > start && !isSpace(src[i-1]) {
		i--
	}
	return i
}
