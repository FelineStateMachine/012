package nbview

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Note cells are Markdown, drawn as terminal text: headings bold (the
// first level underlined too), **bold**, *italic* and _italic_, `code`
// in the code's color, links as hyperlinks the terminal opens (OSC 8),
// list items with a bullet, quotes behind a bar, fenced code as it is,
// and paragraphs wrapped to the width. Each cue survives without color:
// the text's own attributes, bullets and bars.

// markdown draws src at width as lines.
func markdown(th *theme.Theme, src string, width int) []string {
	var out []string
	fenced := false
	for line := range strings.SplitSeq(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			out = append(out, wrapStyled(th.Code[theme.SyntaxString], line, width)...)
			continue
		}
		out = append(out, mdLine(th, line, trimmed, width)...)
	}
	return out
}

// mdLine draws one line of Markdown outside a fence.
func mdLine(th *theme.Theme, line, trimmed string, width int) []string {
	switch {
	case trimmed == "":
		return []string{""}
	case strings.Trim(trimmed, "-*_ ") == "" && len(strings.ReplaceAll(trimmed, " ", "")) >= 3:
		return []string{th.Muted.Render(strings.Repeat("─", max(width, 1)))}
	case strings.HasPrefix(trimmed, "#"):
		level := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
		text := strings.TrimSpace(trimmed[level:])
		style := th.Title
		if level == 1 {
			style = style.Underline(true)
		}
		return prefixed("", "", inline(th, style, text), width)
	case strings.HasPrefix(trimmed, "> "):
		return prefixed(th.Muted.Render("│ "), th.Muted.Render("│ "), inline(th, lipgloss.NewStyle(), trimmed[2:]), width)
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	if item, ok := listItem(trimmed); ok {
		lead := indent + "• "
		return prefixed(lead, strings.Repeat(" ", ansi.StringWidth(lead)), inline(th, lipgloss.NewStyle(), item), width)
	}
	if n, item, ok := numbered(trimmed); ok {
		lead := indent + n + " "
		return prefixed(lead, strings.Repeat(" ", ansi.StringWidth(lead)), inline(th, lipgloss.NewStyle(), item), width)
	}
	return prefixed("", "", inline(th, lipgloss.NewStyle(), trimmed), width)
}

func listItem(s string) (string, bool) {
	for _, m := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(s, m); ok {
			return rest, true
		}
	}
	return "", false
}

func numbered(s string) (string, string, bool) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i == 0 || i+1 >= len(s) || s[i] != '.' && s[i] != ')' || s[i+1] != ' ' {
		return "", "", false
	}
	return s[:i+1], s[i+2:], true
}

// prefixed wraps styled text to width after first on its first line and
// rest on the others.
func prefixed(first, rest, styled string, width int) []string {
	room := max(width-ansi.StringWidth(first), 4)
	lines := strings.Split(ansi.Wrap(styled, room, " -"), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = first + lines[i]
		} else {
			lines[i] = rest + lines[i]
		}
	}
	return lines
}

func wrapStyled(style lipgloss.Style, s string, width int) []string {
	var out []string
	for _, l := range wrapText(s, max(width, 4)) {
		out = append(out, style.Render(l))
	}
	return out
}

// inline draws a line's inline Markdown on base: emphasis, code spans
// and links.
func inline(th *theme.Theme, base lipgloss.Style, s string) string {
	var b strings.Builder
	plain := func(t string) {
		if t != "" {
			b.WriteString(base.Render(t))
		}
	}
	for s != "" {
		i := strings.IndexAny(s, "*_`[")
		if i < 0 {
			plain(s)
			break
		}
		plain(s[:i])
		s = s[i:]
		text, rest, style, link, ok := span(th, base, s)
		if !ok {
			plain(s[:1])
			s = s[1:]
			continue
		}
		if link != "" {
			style = style.Hyperlink(link)
		}
		b.WriteString(style.Render(text))
		s = rest
	}
	return b.String()
}

// span reads the inline span s starts with: its text, what follows it,
// its style and a link's URL.
func span(th *theme.Theme, base lipgloss.Style, s string) (text, rest string, style lipgloss.Style, link string, ok bool) {
	switch {
	case strings.HasPrefix(s, "**") || strings.HasPrefix(s, "__"):
		if j := strings.Index(s[2:], s[:2]); j > 0 {
			return s[2 : 2+j], s[4+j:], base.Bold(true), "", true
		}
	case s[0] == '*' || s[0] == '_':
		if j := strings.IndexByte(s[1:], s[0]); j > 0 {
			return s[1 : 1+j], s[2+j:], base.Italic(true), "", true
		}
	case s[0] == '`':
		if j := strings.IndexByte(s[1:], '`'); j >= 0 {
			return s[1 : 1+j], s[2+j:], th.Code[theme.SyntaxCommand], "", true
		}
	case s[0] == '[':
		if j := strings.Index(s, "]("); j > 0 {
			if k := strings.IndexByte(s[j:], ')'); k > 0 {
				return s[1:j], s[j+k+1:], base.Inherit(th.Link), s[j+2 : j+k], true
			}
		}
	}
	return "", "", base, "", false
}
