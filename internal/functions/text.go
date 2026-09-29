package functions

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
)

// maxText is the longest text a function may build, as in Sheets.
const maxText = 50000

func init() {
	define(
		&FuncDef{Name: "CONCATENATE", Args: "string1, [string2, ...]", Desc: "Join text, including every cell of ranges", Min: 1, Max: -1,
			eval: concatenate},
		&FuncDef{Name: "CONCAT", Args: "value1, value2", Desc: "Join two values as text", Min: 2, Max: 2,
			eval: concat},
		&FuncDef{Name: "TEXTJOIN", Args: "delimiter, ignore_empty, text1, [text2, ...]", Desc: "Join text with a delimiter", Min: 3, Max: -1,
			eval: textJoin},
		&FuncDef{Name: "LEFT", Args: "string, [number_of_characters]", Desc: "The first characters of text", Min: 1, Max: 2,
			eval: textSlice(func(r []rune, n int) string { return string(r[:min(n, len(r))]) })},
		&FuncDef{Name: "RIGHT", Args: "string, [number_of_characters]", Desc: "The last characters of text", Min: 1, Max: 2,
			eval: textSlice(func(r []rune, n int) string { return string(r[len(r)-min(n, len(r)):]) })},
		&FuncDef{Name: "MID", Args: "string, starting_at, extract_length", Desc: "Characters from the middle of text", Min: 3, Max: 3,
			eval: mid},
		&FuncDef{Name: "LEN", Args: "text", Desc: "Number of characters in text: an emoji is one, where Sheets counts two", Min: 1, Max: 1,
			eval: textFn(func(s string) Value { return num(float64(runeLen(s))) })},
		&FuncDef{Name: "UPPER", Args: "text", Desc: "Text in upper case: ß is SS", Min: 1, Max: 1,
			eval: textFn(func(s string) Value { return str(upper(s)) })},
		&FuncDef{Name: "LOWER", Args: "text", Desc: "Text in lower case", Min: 1, Max: 1,
			eval: textFn(func(s string) Value { return str(strings.ToLower(s)) })},
		&FuncDef{Name: "PROPER", Args: "text", Desc: "Text with each word capitalized", Min: 1, Max: 1,
			eval: textFn(func(s string) Value { return str(proper(s)) })},
		&FuncDef{Name: "TRIM", Args: "text", Desc: "Text without leading, trailing and repeated spaces", Min: 1, Max: 1,
			eval: textFn(trimSpaces)},
		&FuncDef{Name: "SUBSTITUTE", Args: "text, search_for, replace_with, [occurrence_number]", Desc: "Replace occurrences of text", Min: 3, Max: 4,
			eval: substitute},
		&FuncDef{Name: "REPLACE", Args: "text, position, length, new_text", Desc: "Replace characters at a position", Min: 4, Max: 4,
			eval: replaceAt},
		&FuncDef{Name: "FIND", Args: "search_for, text_to_search, [starting_at]", Desc: "Position of text, case-sensitive", Min: 2, Max: 3,
			eval: textFinder(false)},
		&FuncDef{Name: "SEARCH", Args: "search_for, text_to_search, [starting_at]", Desc: "Position of text, ignoring case, with * and ? wildcards", Min: 2, Max: 3,
			eval: textFinder(true)},
		&FuncDef{Name: "TEXT", Args: "number, format", Desc: `A number as text in a format, e.g. "$#,##0.00" or "yyyy-mm-dd"`, Min: 2, Max: 2,
			eval: textFormat},
		&FuncDef{Name: "VALUE", Args: "text", Desc: "Text as a number; dates and times too", Min: 1, Max: 1,
			eval: valueOf},
		&FuncDef{Name: "REPT", Args: "text, number_of_repetitions", Desc: "Text repeated a number of times", Min: 2, Max: 2,
			eval: rept},
		&FuncDef{Name: "EXACT", Args: "string1, string2", Desc: "TRUE if two texts are identical, case included", Min: 2, Max: 2,
			eval: exact},
	)
}

// Evaluators for the table above, in its order.

func concatenate(args []Node, get lookup) Value {
	parts, err := texts(args, get)
	if err != nil {
		return *err
	}
	return str(strings.Join(parts, ""))
}

func concat(args []Node, get lookup) Value {
	a, err := textArg(args[0], get)
	if err != nil {
		return *err
	}
	b, err := textArg(args[1], get)
	if err != nil {
		return *err
	}
	return str(a + b)
}

func textJoin(args []Node, get lookup) Value {
	delim, err := textArg(args[0], get)
	if err != nil {
		return *err
	}
	skip, err := boolArg(args, 1, true, get)
	if err != nil {
		return *err
	}
	var parts []string
	if skip || delim == "" {
		parts, err = texts(args[2:], get)
	} else {
		parts, err = textsWithBlanks(args[2:], get, maxText+2)
	}
	if err != nil {
		return *err
	}
	if skip {
		parts = slicesDeleteEmpty(parts)
	}
	return capText(strings.Join(parts, delim))
}

func mid(args []Node, get lookup) Value {
	s, err := textArg(args[0], get)
	if err != nil {
		return *err
	}
	start, err := intArg(args, 1, 1, get)
	if err != nil {
		return *err
	}
	n, err := intArg(args, 2, 0, get)
	if err != nil {
		return *err
	}
	switch {
	case start < 1:
		return value.ErrNum // as Sheets; Excel says #VALUE!
	case n < 0:
		return value.ErrValue
	}
	r := []rune(s)
	if start > len(r) {
		return str("")
	}
	return str(string(r[start-1 : min(start-1+n, len(r))]))
}

// upper is UPPER's full case mapping, which Sheets uses: "straße" is
// "STRASSE", where mapping each letter alone would keep the ß. A Caser
// keeps state, so each call makes its own, and ASCII needs none.
func upper(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return cases.Upper(language.Und).String(s)
		}
	}
	return strings.ToUpper(s)
}

func trimSpaces(s string) Value {
	return str(strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' }), " "))
}

func replaceAt(args []Node, get lookup) Value {
	s, err := textArg(args[0], get)
	if err != nil {
		return *err
	}
	pos, err := intArg(args, 1, 1, get)
	if err != nil {
		return *err
	}
	n, err := intArg(args, 2, 0, get)
	if err != nil {
		return *err
	}
	repl, err := textArg(args[3], get)
	if err != nil {
		return *err
	}
	if pos < 1 || n < 0 {
		return value.ErrValue
	}
	r := []rune(s)
	from := min(pos-1, len(r))
	to := min(from+n, len(r))
	return capText(string(r[:from]) + repl + string(r[to:]))
}

func textFormat(args []Node, get lookup) Value {
	v := eval(args[0], get)
	if v.Kind == value.Error {
		return v
	}
	pat, err := textArg(args[1], get)
	if err != nil {
		return *err
	}
	if v.Kind == value.Bool {
		return str(v.String()) // TRUE stays TRUE whatever the format, as in Sheets
	}
	if v.Kind == value.Text {
		n, _, ok := value.ParseValue(v.Str)
		if !ok {
			return v
		}
		v = num(n)
	}
	return str(numfmt.Format(v.Num, pat))
}

func valueOf(args []Node, get lookup) Value {
	v := eval(args[0], get)
	switch v.Kind {
	case value.Error, value.Number:
		return v
	case value.Empty:
		return num(0)
	case value.Bool:
		return value.ErrValue
	}
	if n, _, ok := value.ParseValue(strings.TrimSpace(v.Str)); ok {
		return num(n)
	}
	if strings.TrimSpace(v.Str) == "" {
		return num(0)
	}
	return value.ErrValue
}

func rept(args []Node, get lookup) Value {
	s, err := textArg(args[0], get)
	if err != nil {
		return *err
	}
	n, err := intArg(args, 1, 0, get)
	if err != nil {
		return *err
	}
	if n < 0 || n*len(s) > maxText {
		return value.ErrValue
	}
	return str(strings.Repeat(s, n))
}

func exact(args []Node, get lookup) Value {
	a, err := textArg(args[0], get)
	if err != nil {
		return *err
	}
	b, err := textArg(args[1], get)
	if err != nil {
		return *err
	}
	return boolean(a == b)
}

func capText(s string) Value {
	if len(s) > maxText {
		return value.ErrValue
	}
	return str(s)
}

func slicesDeleteEmpty(parts []string) []string {
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func textFn(f func(string) Value) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		s, err := textArg(args[0], get)
		if err != nil {
			return *err
		}
		return f(s)
	}
}

// textSlice builds LEFT and RIGHT: n defaults to 1, negative is #VALUE!.
func textSlice(f func(r []rune, n int) string) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		s, err := textArg(args[0], get)
		if err != nil {
			return *err
		}
		n, err := intArg(args, 1, 1, get)
		if err != nil {
			return *err
		}
		if n < 0 {
			return value.ErrValue
		}
		return str(f([]rune(s), n))
	}
}

// proper capitalizes the first letter of every run of letters, as Sheets
// does: "it's" becomes "It'S".
func proper(s string) string {
	r := []rune(s)
	prevLetter := false
	for i, c := range r {
		if prevLetter {
			r[i] = unicode.ToLower(c)
		} else {
			r[i] = unicode.ToUpper(c)
		}
		prevLetter = unicode.IsLetter(c)
	}
	return string(r)
}

func substitute(args []Node, get lookup) Value {
	s, err := textArg(args[0], get)
	if err != nil {
		return *err
	}
	old, err := textArg(args[1], get)
	if err != nil {
		return *err
	}
	repl, err := textArg(args[2], get)
	if err != nil {
		return *err
	}
	if len(args) < 4 {
		if old == "" {
			return str(s)
		}
		return capText(strings.ReplaceAll(s, old, repl))
	}
	nth, err := intArg(args, 3, 0, get)
	switch {
	case err != nil:
		return *err
	case nth < 1:
		return value.ErrValue
	case old == "":
		return str(s)
	}
	idx := 0
	for i := 1; ; i++ {
		j := strings.Index(s[idx:], old)
		if j < 0 {
			return str(s)
		}
		idx += j
		if i == nth {
			return capText(s[:idx] + repl + s[idx+len(old):])
		}
		idx += len(old)
	}
}

// textFinder builds FIND (exact) and SEARCH (case-insensitive, wildcards).
// Positions count characters from 1; not found is #VALUE!.
func textFinder(search bool) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		needle, err := textArg(args[0], get)
		if err != nil {
			return *err
		}
		hay, err := textArg(args[1], get)
		if err != nil {
			return *err
		}
		start, err := intArg(args, 2, 1, get)
		if err != nil {
			return *err
		}
		r := []rune(hay)
		if start < 1 || start > len(r)+1 {
			return value.ErrValue
		}
		if needle == "" {
			return num(float64(start))
		}
		for i := start - 1; i < len(r); i++ {
			rest := string(r[i:])
			switch {
			case !search && strings.HasPrefix(rest, needle):
				return num(float64(i + 1))
			case search && hasWildcards(needle) && prefixWild(needle, r[i:]):
				return num(float64(i + 1))
			case search && !hasWildcards(needle) && strings.HasPrefix(strings.ToLower(rest), strings.ToLower(needle)):
				return num(float64(i + 1))
			}
		}
		return value.ErrValue
	}
}

// prefixWild reports whether some prefix of r matches the pattern.
func prefixWild(pattern string, r []rune) bool {
	return wildMatch(pattern+"*", string(r))
}
