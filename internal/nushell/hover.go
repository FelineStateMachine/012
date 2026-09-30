package nushell

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// What nu says of the word at a byte of a script, through
//
//	nu --ide-hover 7 file       {"hover":"Sort by the given cell path or closure.\n### Usage\n...","span":{"start":5,"end":12}}
//
// A command's hover is its help in Markdown (Help reads it), a
// variable's is its type ("int"), a flag's is just "flag", and a byte
// on no word prints nothing. nu --lsp answers the same through the
// language server's protocol; a process per question, as the other
// questions are, needs no session with it.

// Hover is nu's text about the word at a byte, and the word's bytes.
type Hover struct {
	From, To int
	Text     string
}

// HoverAt is what nu says of the word at offset, a byte of script;
// false when it's on none.
func HoverAt(ctx context.Context, r Runner, script string, offset int) (Hover, bool, error) {
	out, err := ask(ctx, r, script, "--ide-hover", strconv.Itoa(offset))
	if err != nil {
		return Hover{}, false, err
	}
	return ParseHover(out)
}

// ParseHover reads what --ide-hover printed.
func ParseHover(out []byte) (Hover, bool, error) {
	if len(out) == 0 {
		return Hover{}, false, nil
	}
	var h struct {
		Hover *string
		Span  *span
	}
	if err := json.Unmarshal(out, &h); err != nil || h.Hover == nil || h.Span == nil {
		return Hover{}, false, fmt.Errorf("%w: %s", ErrOld, firstLine(out))
	}
	return Hover{From: h.Span.Start, To: h.Span.End, Text: *h.Hover}, true, nil
}

// Help is a command's help, as its hover has it.
type Help struct {
	Name  string // sort-by, str join
	Desc  string // what it does, its paragraphs
	Usage string // sort-by {flags} <...comparator>
	// Flags are its flags but --help; Params its positional
	// parameters, in order.
	Flags  []Flag
	Params []Param
	// Types are its input and output types, "table | table".
	Types    []string
	Examples []Example
}

// Flag is a command's flag: -r, --reverse, the type of its value if it
// takes one, and what it does.
type Flag struct {
	Short, Long, Arg, Desc string
}

// Param is a positional parameter: comparator, its type as nu writes it
// (oneof<cell-path, closure>), and what it is.
type Param struct {
	Name, Type, Desc string
}

// Example is one of a command's examples: what it does, and the code.
type Example struct {
	Desc, Code string
}

// ParseHelp reads a command's hover; false when it isn't one (a
// variable's type, a flag's "flag").
func ParseHelp(md string) (Help, bool) {
	desc, rest, ok := strings.Cut(md, "### Usage")
	if !ok {
		return Help{}, false
	}
	h := Help{Desc: strings.TrimSpace(desc)}
	for _, sec := range strings.Split("### Usage"+rest, "\n### ") {
		head, body, _ := strings.Cut(strings.TrimPrefix(sec, "### "), "\n")
		switch strings.TrimSpace(head) {
		case "Usage":
			h.Usage = strings.TrimSpace(strings.Trim(strings.TrimSpace(body), "`"))
			h.Name = commandName(h.Usage)
		case "Flags":
			h.Flags = parseFlags(body)
		case "Parameters":
			h.Params = parseParams(body)
		case "Input/output types":
			h.Types = fenced(body)
		case "Example(s)":
			h.Examples = parseExamples(body)
		}
	}
	return h, h.Name != ""
}

// commandName is the words of usage before its flags and parameters.
func commandName(usage string) string {
	var words []string
	for w := range strings.FieldsSeq(usage) {
		if strings.ContainsAny(w[:1], "{<(.[") {
			break
		}
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

// fenced are the lines of body's first code fence, trimmed.
func fenced(body string) []string {
	var out []string
	in := false
	for line := range strings.SplitSeq(body, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "```") && in:
			return out
		case strings.HasPrefix(t, "```"):
			in = true
		case in && t != "":
			out = append(out, t)
		}
	}
	return out
}

// ticked splits `-r`, `--reverse` `<int>` - Sort in reverse order.\
// into its backquoted parts and what follows " - ".
func ticked(line string) ([]string, string) {
	line = strings.TrimSuffix(strings.TrimSpace(line), "\\")
	var parts []string
	for strings.HasPrefix(line, "`") {
		end := strings.Index(line[1:], "`")
		if end < 0 {
			break
		}
		parts = append(parts, line[1:end+1])
		line = strings.TrimLeft(line[end+2:], ", ")
	}
	return parts, strings.TrimSpace(strings.TrimPrefix(line, "- "))
}

func parseFlags(body string) []Flag {
	var out []Flag
	for line := range strings.SplitSeq(body, "\n") {
		parts, desc := ticked(line)
		var f Flag
		for _, p := range parts {
			switch {
			case strings.HasPrefix(p, "--"):
				f.Long = p
			case strings.HasPrefix(p, "-"):
				f.Short = p
			case strings.HasPrefix(p, "<"):
				f.Arg = strings.Trim(p, "<>")
			}
		}
		if (f.Long != "" || f.Short != "") && f.Long != "--help" {
			f.Desc = desc
			out = append(out, f)
		}
	}
	return out
}

func parseParams(body string) []Param {
	var out []Param
	for line := range strings.SplitSeq(body, "\n") {
		parts, desc := ticked(line)
		if len(parts) == 0 {
			continue
		}
		name, typ, _ := strings.Cut(parts[0], ":")
		out = append(out, Param{Name: strings.TrimSpace(name), Type: strings.TrimSpace(typ), Desc: desc})
	}
	return out
}

// parseExamples reads the examples: an empty fence, then each
// example's description outside a fence and its code inside one.
func parseExamples(body string) []Example {
	segs := strings.Split(body, "```")
	var out []Example
	for i := 2; i+1 < len(segs); i += 2 {
		desc := strings.TrimSpace(segs[i])
		var code []string
		for line := range strings.SplitSeq(strings.Trim(segs[i+1], "\n"), "\n") {
			code = append(code, strings.TrimPrefix(line, "  "))
		}
		c := strings.TrimSpace(strings.Join(code, "\n"))
		if desc != "" || c != "" {
			out = append(out, Example{Desc: desc, Code: c})
		}
	}
	return out
}

// Summary is the first paragraph of the description.
func (h Help) Summary() string {
	first, _, _ := strings.Cut(h.Desc, "\n\n")
	return strings.Join(strings.Fields(first), " ")
}

// Signature is the command's usage with its parameters' types and its
// flags by name: sort-by <...comparator: cell-path|closure> --reverse.
func (h Help) Signature() string {
	sig := []string{h.Name}
	rest := strings.TrimSpace(strings.TrimPrefix(h.Usage, h.Name))
	for w := range strings.FieldsSeq(rest) {
		if w == "{flags}" {
			continue
		}
		if p, ok := h.param(w); ok && p.Type != "" {
			w = w[:len(w)-1] + ": " + typeWords(p.Type) + w[len(w)-1:]
		}
		sig = append(sig, w)
	}
	for _, f := range h.Flags {
		name := cmp.Or(f.Long, f.Short)
		if f.Arg != "" {
			name += " <" + f.Arg + ">"
		}
		sig = append(sig, name)
	}
	return strings.Join(sig, " ")
}

// param is the parameter usage's word w names: <...comparator>,
// <rows?>.
func (h Help) param(w string) (Param, bool) {
	name := strings.Trim(w, "<>()[]?.")
	for _, p := range h.Params {
		if strings.Trim(p.Name, ".?") == name {
			return p, true
		}
	}
	return Param{}, false
}

// typeWords writes oneof<int, filesize> as int|filesize.
func typeWords(t string) string {
	if in, ok := strings.CutPrefix(t, "oneof<"); ok {
		return strings.ReplaceAll(strings.TrimSuffix(in, ">"), ", ", "|")
	}
	return t
}

// Flag is the command's flag named w, long (--reverse) or short (-r).
func (h Help) Flag(w string) (Flag, bool) {
	for _, f := range h.Flags {
		if w != "" && (f.Long == w || f.Short == w) {
			return f, true
		}
	}
	return Flag{}, false
}

// DocsURL is the page of nushell's documentation for command name.
func DocsURL(name string) string {
	return "https://www.nushell.sh/commands/docs/" + strings.ReplaceAll(name, " ", "_") + ".html"
}
