package theme

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A box is framed with light box-drawing lines, which keep the crisp
// character-grid look. Rows are exactly the inner width; separator rows
// become a line joined to the frame.

// SepRow marks a separator in the rows passed to Frame.
const SepRow = "\x00"

// Frame draws a border around rows. A title sits in the top border and a
// footer at the right of the bottom border.
func (t *Theme) Frame(inner int, title, footer string, rows []string) []string {
	inner = max(inner, 2) // screens smaller than the box get a clipped box
	b := t.Border
	top := "┌" + strings.Repeat("─", inner) + "┐"
	if title != "" {
		tt := ansi.Truncate(" "+title+" ", inner-1, "…")
		top = b.Render("┌─") + t.Title.Render(tt) + b.Render(strings.Repeat("─", inner-1-ansi.StringWidth(tt))+"┐")
	} else {
		top = b.Render(top)
	}
	bottom := b.Render("└" + strings.Repeat("─", inner) + "┘")
	if footer != "" && ansi.StringWidth(footer)+4 <= inner {
		f := " " + footer + " "
		bottom = b.Render("└"+strings.Repeat("─", inner-1-ansi.StringWidth(f))) + t.Muted.Render(f) + b.Render("─┘")
	}
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, top)
	for _, r := range rows {
		if r == SepRow {
			lines = append(lines, b.Render("├"+strings.Repeat("─", inner)+"┤"))
			continue
		}
		lines = append(lines, b.Render("│")+r+b.Render("│"))
	}
	return append(lines, bottom)
}

// Chip draws a key name as a key cap, e.g. "Ctrl+S".
func (t *Theme) Chip(label string) string {
	return t.KeyChip.Render(" " + label + " ")
}

// Chips renders keys as key chips, e.g. [Ctrl+/] [F1].
func (t *Theme) Chips(keys []string) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = t.Chip(k)
	}
	return strings.Join(parts, " ")
}

// KeyHints renders key and description pairs, e.g. "Enter accept", each
// key as a chip.
func (t *Theme) KeyHints(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, t.Chip(pairs[i])+" "+t.Muted.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

// Cells pads or truncates s to exactly w columns, then styles it.
func Cells(style lipgloss.Style, s string, w int) string {
	return style.Render(PadRight(ansi.Truncate(s, w, "…"), w))
}

// HighlightMatches renders s in w columns with the bytes at idx in the hl
// style, truncating with an ellipsis.
func HighlightMatches(s string, idx []int, w int, base, hl lipgloss.Style) string {
	s = ansi.Truncate(s, w, "…")
	var b strings.Builder
	run, lit := "", false
	flush := func() {
		if lit {
			b.WriteString(hl.Render(run))
		} else {
			b.WriteString(base.Render(run))
		}
		run = ""
	}
	for i, r := range s {
		if on := slices.Contains(idx, i); on != lit {
			flush()
			lit = on
		}
		run += string(r)
	}
	flush()
	return b.String() + base.Render(strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)))
}

// KeyLabel formats a key binding for display, e.g. "ctrl+s" -> "Ctrl+S".
func KeyLabel(k string) string {
	switch k {
	case "delete":
		return "Del"
	case "backspace":
		return "Backspace"
	case "esc":
		return "Esc"
	}
	k = strings.NewReplacer("pgdown", "PgDn", "pgup", "PgUp").Replace(k)
	parts := strings.Split(k, "+")
	for i, p := range parts {
		switch {
		case len(p) == 1:
			parts[i] = strings.ToUpper(p)
		case p[0] == 'f' && len(p) <= 3 && p[1] >= '0' && p[1] <= '9':
			parts[i] = strings.ToUpper(p)
		default:
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "+")
}

// PadRight pads s with spaces to w columns.
func PadRight(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// PadLeft right-aligns s in w columns.
func PadLeft(s string, w int) string {
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}

// Center centers s in w columns, any odd space going right.
func Center(s string, w int) string {
	pad := max(w-ansi.StringWidth(s), 0)
	return strings.Repeat(" ", pad/2) + s + strings.Repeat(" ", pad-pad/2)
}
