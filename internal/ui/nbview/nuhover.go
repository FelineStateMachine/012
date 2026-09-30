package nbview

import (
	"context"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
)

// Hover implements Hoverer: for a command, its signature and help from
// nu's --ide-hover; for a flag, the flag in its command's help; for a
// variable, the notebook's word for it ($selection, a linked file), or
// the type nu gives a variable the cell binds itself.
func (n *Nu) Hover(ctx context.Context, src string, at int) (Hover, bool) {
	names, words := n.known()
	from, to := wordBounds(src, at)
	if h, ok := wordHover(words, src, from, to); ok {
		return h, true
	}
	p := prepare(src, names)
	nh, ok := n.hoverAt(ctx, p, from) // the word's start, whichever byte of it the caret is at
	if !ok {
		return Hover{}, false
	}
	from, to = nh.From-p.at, nh.To-p.at
	if from < 0 || to > len(src) || from >= to || p.inHead(from, to) {
		return Hover{}, false
	}
	if nh.Text == "flag" {
		return n.flagHover(ctx, src, p, from, to)
	}
	if help, ok := nushell.ParseHelp(nh.Text); ok {
		return Hover{From: from, To: to, Text: help.Signature(), Brief: help.Brief(), Desc: help.Summary(), Help: &help}, true
	}
	word := src[from:to]
	if strings.HasPrefix(word, "$") && nh.Text != "any" && !strings.ContainsAny(nh.Text, "\n ") {
		return Hover{From: from, To: to, Text: word + ": " + nh.Text}, true
	}
	return Hover{}, false
}

// hoverAt is nu's hover at byte at of the source p was prepared from.
func (n *Nu) hoverAt(ctx context.Context, p prepared, at int) (h nushell.Hover, ok bool) {
	err := n.s.ask(ctx, func(ctx context.Context) (err error) {
		h, ok, err = nushell.HoverAt(ctx, n.s.Runner, p.text, p.at+at)
		return err
	})
	return h, err == nil && ok
}

// flagHover is what the help of the command a flag belongs to says of
// the flag at src[from:to].
func (n *Nu) flagHover(ctx context.Context, src string, p prepared, from, to int) (Hover, bool) {
	cmd, ok := commandStart(p.text[p.at:], notebook.Parse(src).Strings, from)
	if !ok {
		return Hover{}, false
	}
	nh, ok := n.hoverAt(ctx, p, cmd)
	if !ok {
		return Hover{}, false
	}
	help, ok := nushell.ParseHelp(nh.Text)
	if !ok {
		return Hover{}, false
	}
	f, ok := help.Flag(src[from:to])
	if !ok {
		return Hover{}, false
	}
	text := help.Name + " " + f.Long
	if f.Arg != "" {
		text += " <" + f.Arg + ">"
	}
	if f.Short != "" {
		text += ", " + f.Short
	}
	return Hover{From: from, To: to, Text: strings.TrimSpace(text), Desc: f.Desc, Help: &help}, true
}

// commandStart is the first byte of the command whose argument begins
// at byte at of src: after the |, ;, line or bracket opening the
// pipeline's element it's in, strings in strs left alone, and after a
// let's =.
func commandStart(src string, strs [][2]int, at int) (int, bool) {
	depth, i := 0, at
scan:
	for i > 0 {
		i--
		if s, ok := stringAround(strs, i); ok {
			i = s
			continue
		}
		switch src[i] {
		case ')', '}', ']':
			depth++
		case '(', '{', '[':
			if depth == 0 {
				i++
				break scan
			}
			depth--
		case '|', ';', '\n':
			if depth == 0 {
				i++
				break scan
			}
		}
	}
	for i < at && (src[i] == ' ' || src[i] == '\t' || src[i] == '^') {
		i++
	}
	if _, end := wordBounds(src, i); slices.Contains([]string{"let", "mut", "const"}, src[i:end]) {
		if eq := strings.IndexByte(src[end:at], '='); eq >= 0 {
			i = end + eq + 1
			for i < at && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
		}
	}
	return i, i < at
}

// stringAround is where the string holding byte i starts.
func stringAround(strs [][2]int, i int) (int, bool) {
	for _, s := range strs {
		if i >= s[0] && i < s[1] {
			return s[0], true
		}
	}
	return 0, false
}

// wordHover is the notebook's own word for the $name at src[from:to]:
// $selection, a linked file, a range of a sheet.
func wordHover(words []Word, src string, from, to int) (Hover, bool) {
	name, end := varName(src, from)
	if name == "" {
		return Hover{}, false
	}
	for _, w := range words {
		switch {
		case w.Text == "$"+name:
			return Hover{From: from, To: end, Text: w.Text + ": " + w.Desc}, true
		case strings.HasSuffix(w.Text, ".") && strings.HasPrefix(src[from:to], w.Text):
			desc, _, _ := strings.Cut(w.Desc, ":") // a range of a sheet: $sheet.A1:C9
			return Hover{From: from, To: to, Text: src[from:to] + ": " + desc}, true
		}
	}
	return Hover{}, false
}
