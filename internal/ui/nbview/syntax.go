package nbview

import (
	"context"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// What the code editor asks of the language, behind small interfaces so
// better answers (nushell's own --ide-ast, --ide-complete and
// --ide-check) can replace the built-in ones: highlighting, completion
// and diagnostics. Each is asked in the background, after typing
// pauses, never while a key is being answered; until it answers, the
// code shows plain, or as it last did.

// Span is a stretch of a cell's source, by byte offsets, and what it is.
type Span struct {
	From, To int
	Kind     theme.Syntax
}

// Highlighter says what each part of a pipeline is.
type Highlighter interface {
	Highlight(ctx context.Context, src string) []Span
}

// Completion is a word Tab can put at the caret: Text in place of
// src[From:To].
type Completion struct {
	Text     string
	Desc     string
	From, To int
}

// Completer lists what can go at the caret, a byte offset into src.
type Completer interface {
	Complete(ctx context.Context, src string, offset int) []Completion
}

// Diagnostic is a problem with part of a pipeline, by byte offsets.
type Diagnostic struct {
	From, To int
	Msg      string
}

// Checker finds the problems in a pipeline.
type Checker interface {
	Check(ctx context.Context, src string) []Diagnostic
}

// Providers are the language's answers for a notebook; any may be nil.
type Providers struct {
	Highlighter Highlighter
	Completer   Completer
	Checker     Checker
}

// Tokens is the built-in highlighter: a small tokenizer of nushell's
// shapes, which knows commands by where they stand rather than by name.
type Tokens struct{}

// keywords are nushell's words that aren't commands' names.
var keywords = []string{"let", "mut", "const", "def", "if", "else", "for", "while", "loop", "match", "try", "catch",
	"return", "break", "continue", "true", "false", "null", "use", "export", "module", "alias"}

// operators are the words nushell reads as operators.
var operators = []string{"and", "or", "not", "xor", "in", "not-in", "like", "not-like", "mod", "starts-with", "ends-with", "bit-and", "bit-or"}

// Highlight implements Highlighter.
func (Tokens) Highlight(_ context.Context, src string) []Span {
	t := tokenizer{src: src, command: true}
	if name, _ := notebook.SplitName(src); name != "" {
		t.i = strings.Index(src, name) + len(name)
		t.add(t.i-len(name), theme.SyntaxVariable) // the cell's name, as $name reads it
	}
	for t.i < len(t.src) {
		if !t.punct() {
			t.word()
		}
	}
	return t.out
}

// tokenizer reads a pipeline's tokens, knowing whether a word where it
// stands is a command's name.
type tokenizer struct {
	src     string
	i       int
	command bool
	out     []Span
}

func (t *tokenizer) add(from int, k theme.Syntax) { t.out = append(t.out, Span{from, t.i, k}) }

// punct reads what isn't a bare word, reporting whether there was some.
func (t *tokenizer) punct() bool {
	src, i := t.src, t.i
	c := src[i]
	start := i
	switch {
	case c == '\n' || c == ';' || c == '(' || c == '{' || c == '|' && !strings.HasPrefix(src[i:], "||"):
		t.i++
		if c == '|' {
			t.add(start, theme.SyntaxOperator)
		}
		t.command = true
	case c == ' ' || c == '\t' || c == '\r' || c == ')' || c == '}' || c == '[' || c == ']' || c == ',':
		t.i++
	case c == '#' && (i == 0 || isSpace(src[i-1])):
		t.i = lineEnd(src, i)
		t.add(start, theme.SyntaxComment)
	case c == '"' || c == '\'' || c == '`':
		t.i = quoted(src, i)
		t.add(start, theme.SyntaxString)
		t.command = false
	case c == '$':
		t.i = wordEnd(src, i+1)
		t.add(start, theme.SyntaxVariable)
		t.command = false
	case strings.ContainsRune("=!<>+-*/~", rune(c)) && !isWordStart(src, i):
		for t.i < len(src) && strings.ContainsRune("=!<>+-*/~", rune(src[t.i])) {
			t.i++
		}
		t.add(start, theme.SyntaxOperator)
		t.command = src[start:t.i] == "="
	default:
		return false
	}
	return true
}

// word reads a bare word: a number, a keyword, an operator's name, a
// command's name where one stands, or an argument.
func (t *tokenizer) word() {
	start := t.i
	t.i = wordEnd(t.src, t.i)
	if t.i == start {
		t.i++
		return
	}
	w := t.src[start:t.i]
	switch {
	case isDigit(w[0]) || w[0] == '-' && len(w) > 1 && isDigit(w[1]):
		t.add(start, theme.SyntaxNumber)
	case slices.Contains(keywords, w):
		t.add(start, theme.SyntaxKeyword)
		t.command = w == "else" || w == "try"
		return
	case slices.Contains(operators, w) && !t.command:
		t.add(start, theme.SyntaxOperator)
		return
	case t.command && !strings.HasPrefix(w, "-") && !strings.HasSuffix(w, ":"): // a record's key isn't a command
		t.add(start, theme.SyntaxCommand)
	}
	t.command = false
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isWordStart reports whether the - or + at i starts a word: a flag or
// a negative number.
func isWordStart(src string, i int) bool {
	return (src[i] == '-' || src[i] == '+') && i+1 < len(src) && !isSpace(src[i+1]) && !strings.ContainsRune("=<>", rune(src[i+1])) &&
		(i == 0 || isSpace(src[i-1]) || src[i-1] == '(' || src[i-1] == '[')
}

// wordEnd is where a bare word from i ends.
func wordEnd(src string, i int) int {
	for i < len(src) && !isSpace(src[i]) && !strings.ContainsRune("|;(){}[],\"'`", rune(src[i])) {
		i++
	}
	return i
}

func lineEnd(src string, i int) int {
	for i < len(src) && src[i] != '\n' {
		i++
	}
	return i
}

// quoted is where a string starting at i ends: its closing quote, a
// double-quoted string's escapes skipped, or the end.
func quoted(src string, i int) int {
	q := src[i]
	for i++; i < len(src); i++ {
		switch {
		case q == '"' && src[i] == '\\':
			i++
		case src[i] == q:
			return i + 1
		}
	}
	return len(src)
}

// Word is a completion the built-in completer offers.
type Word struct {
	Text string // $sales, or a command: sort-by
	Desc string
}

// Words is the built-in completer: the notebook's names at a $, nu's
// commands where a command goes.
type Words func() []Word

// Complete implements Completer.
func (ws Words) Complete(_ context.Context, src string, offset int) []Completion {
	offset = min(offset, len(src))
	start := offset
	for start > 0 && !isSpace(src[start-1]) && !strings.ContainsRune("|(;{[", rune(src[start-1])) {
		start--
	}
	w := src[start:offset]
	before := strings.TrimRight(src[:start], " \t")
	command := before == "" || strings.ContainsAny(before[len(before)-1:], "|(;{\n") ||
		strings.HasSuffix(before, "=") && !strings.HasSuffix(before, "==")
	if !strings.HasPrefix(w, "$") && (!command || w == "") {
		return nil
	}
	lw := strings.ToLower(w)
	var out, contains []Completion
	for _, it := range ws() {
		if strings.HasPrefix(it.Text, "$") != strings.HasPrefix(w, "$") {
			continue
		}
		c := Completion{Text: it.Text, Desc: it.Desc, From: start, To: offset}
		switch t := strings.ToLower(it.Text); {
		case t == lw:
		case strings.HasPrefix(t, lw):
			out = append(out, c)
		case len(lw) > 1 && strings.Contains(t, lw):
			contains = append(contains, c)
		}
	}
	return append(out, contains...)
}
