package functions

import (
	"regexp"
	"strings"
	"sync"

	"github.com/FelineStateMachine/012/internal/value"
)

// Splitting text and regular expressions. Sheets' regular expressions
// are RE2, as Go's regexp package is, so patterns mean the same.

func init() {
	define(
		&FuncDef{Name: "SPLIT", Args: "text, delimiter, [split_by_each], [remove_empty_text]", Desc: "Text split at a delimiter into cells across (each character of it by default)", Min: 2, Max: 4,
			eval: split, arrays: takesArrays},
		&FuncDef{Name: "REGEXMATCH", Args: "text, regular_expression", Desc: "TRUE if text matches a regular expression", Min: 2, Max: 2,
			eval: regexMatch},
		&FuncDef{Name: "REGEXEXTRACT", Args: "text, regular_expression", Desc: "The first match of a regular expression, or its capture groups across", Min: 2, Max: 2,
			eval: regexExtract, arrays: liftPass},
		&FuncDef{Name: "REGEXREPLACE", Args: "text, regular_expression, replacement", Desc: "Text with every match replaced; $1 in the replacement is a capture group", Min: 3, Max: 3,
			eval: regexReplace},
	)
}

// Evaluators for the table above, in its order.

// split splits text at a delimiter: at each of its characters unless
// split_by_each is FALSE, leaving out empty pieces unless
// remove_empty_text is FALSE. Pieces that read as numbers are numbers,
// as in Sheets. In an array context each entry of text is split into a
// row of its own.
func split(args []Node, get lookup) Value {
	delim, err := textArg(args[1], get)
	if err != nil {
		return *err
	}
	each, err := boolArg(args, 2, true, get)
	if err != nil {
		return *err
	}
	drop, err := boolArg(args, 3, true, get)
	if err != nil {
		return *err
	}
	if delim == "" {
		return value.ErrValue
	}
	cut := func(s string) []Value { return splitText(s, delim, each, drop) }
	if get.lift == 0 {
		s, err := textArg(args[0], get)
		if err != nil {
			return *err
		}
		row := cut(s)
		out := NewArray(1, len(row))
		copy(out.V, row)
		return get.arrayValue(out)
	}
	texts, err := arrayArg(args[0], get)
	if err != nil {
		return *err
	}
	return get.arrayValue(splitRows(texts, cut))
}

// splitRows splits each entry of texts (row by row) into a row of its
// own, the rows as wide as the widest, blank past their pieces.
func splitRows(texts *Array, cut func(string) []Value) *Array {
	var rows [][]Value
	width := 0
	for r := range texts.DRows {
		for c := range texts.DCols {
			v := texts.At(r, c)
			var row []Value
			if v.Kind == value.Error {
				row = []Value{v}
			} else if v.Kind != value.Empty {
				row = cut(text(v))
			}
			rows = append(rows, row)
			width = max(width, len(row))
		}
	}
	width = max(width, 1)
	if tooBig(len(rows), width) {
		return &Array{Rows: 1, Cols: 1, DRows: 1, DCols: 1, V: []Value{value.ErrValue}}
	}
	out := NewArray(max(len(rows), 1), width)
	for i, row := range rows {
		copy(out.V[i*width:], row)
	}
	return out
}

// splitText is the pieces of s between delimiters, as values.
func splitText(s, delim string, each, drop bool) []Value {
	var parts []string
	switch {
	case each && drop:
		parts = strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(delim, r) })
	case each:
		parts = splitEach(s, delim)
	default:
		parts = strings.Split(s, delim)
	}
	out := make([]Value, 0, len(parts))
	for _, p := range parts {
		if p == "" && drop {
			continue
		}
		if n, _, ok := value.ParseValue(p); ok && strings.TrimSpace(p) == p {
			out = append(out, num(n))
			continue
		}
		out = append(out, str(p))
	}
	if len(out) == 0 {
		out = append(out, Value{})
	}
	return out
}

// splitEach splits s at every character of delim, keeping empty pieces.
func splitEach(s, delim string) []string {
	var parts []string
	start := 0
	for i, r := range s {
		if strings.ContainsRune(delim, r) {
			parts = append(parts, s[start:i])
			start = i + len(string(r))
		}
	}
	return append(parts, s[start:])
}

func regexMatch(args []Node, get lookup) Value {
	s, re, err := regexArgs(args, get)
	if err != nil {
		return *err
	}
	return boolean(re.MatchString(s))
}

func regexExtract(args []Node, get lookup) Value {
	s, re, err := regexArgs(args, get)
	if err != nil {
		return *err
	}
	m := re.FindStringSubmatch(s)
	switch {
	case m == nil:
		return value.ErrNA
	case len(m) == 1:
		return str(m[0])
	case len(m) == 2:
		return str(m[1])
	}
	out := NewArray(1, len(m)-1)
	for i, g := range m[1:] {
		out.V[i] = str(g)
	}
	return get.arrayValue(out)
}

func regexReplace(args []Node, get lookup) Value {
	s, re, err := regexArgs(args, get)
	if err != nil {
		return *err
	}
	with, err := regexText(args[2], get)
	if err != nil {
		return *err
	}
	return capText(re.ReplaceAllString(s, with))
}

// regexArgs reads the text and the regular expression of a REGEX
// function: text only, as Sheets takes (a number is #VALUE!), and a
// pattern that compiles.
func regexArgs(args []Node, get lookup) (string, *regexp.Regexp, *Value) {
	s, err := regexText(args[0], get)
	if err != nil {
		return "", nil, err
	}
	pattern, err := textArg(args[1], get)
	if err != nil {
		return "", nil, err
	}
	re, ok := compiled(pattern)
	if !ok {
		return "", nil, &value.ErrValue
	}
	return s, re, nil
}

// regexText reads an argument that must be text.
func regexText(n Node, get lookup) (string, *Value) {
	v := eval1(n, get)
	switch v.Kind {
	case value.Error:
		return "", errOf(v)
	case value.Text, value.Empty:
		return v.Str, nil
	}
	return "", &value.ErrValue
}

// regexCache keeps compiled patterns, as a column of REGEXMATCH or an
// ARRAYFORMULA of one uses the same pattern for every row. It is shared
// by every workbook (and every session of 012 serve), so it's locked,
// and emptied when full.
var regexCache = struct {
	sync.Mutex
	m map[string]*regexp.Regexp
}{m: map[string]*regexp.Regexp{}}

const maxRegexCache = 256

// compiled is pattern compiled, or false when it isn't a valid RE2
// pattern.
func compiled(pattern string) (*regexp.Regexp, bool) {
	regexCache.Lock()
	defer regexCache.Unlock()
	if re, ok := regexCache.m[pattern]; ok {
		return re, re != nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		re = nil
	}
	if len(regexCache.m) >= maxRegexCache {
		clear(regexCache.m)
	}
	regexCache.m[pattern] = re
	return re, re != nil
}
