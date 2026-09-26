package e2e

import (
	"cmp"
	"fmt"
	"html"
	"strings"

	ghostty "go.mitchellh.com/libghostty"
)

// renderHTML draws the screen as HTML from libghostty's cell grid, one
// span per run of identically styled cells. Colors are palette CSS
// variables so a gallery can show the same snapshot under any theme.
//
// We don't use libghostty's HTML formatter because it paints erased cells
// with the style of the text before them, which misrepresents what a
// terminal shows.
func renderHTML(vt *ghostty.Terminal) (string, error) {
	cols, err := vt.Cols()
	if err != nil {
		return "", err
	}
	rows, err := vt.Rows()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for y := range uint32(rows) {
		var line strings.Builder
		var run strings.Builder
		style := ""
		flush := func() {
			if run.Len() == 0 {
				return
			}
			if style == "" {
				line.WriteString(run.String())
			} else {
				fmt.Fprintf(&line, `<span style="%s">%s</span>`, style, run.String())
			}
			run.Reset()
		}
		for x := range cols {
			ref, err := vt.GridRef(ghostty.Point{Tag: ghostty.PointTagActive, X: x, Y: y})
			if err != nil {
				return "", err
			}
			cell, err := ref.Cell()
			if err != nil {
				return "", err
			}
			if wide, _ := cell.Wide(); wide == ghostty.CellWideSpacerTail {
				continue
			}
			text := " "
			if has, _ := cell.HasText(); has {
				cps, _ := ref.Graphemes()
				var sb strings.Builder
				for _, cp := range cps {
					sb.WriteRune(rune(cp))
				}
				text = sb.String()
			}
			css := ""
			if st, err := ref.Style(); err == nil && !st.IsDefault() {
				css = styleCSS(st)
			}
			// Cells erased with a background color hold just that color.
			switch tag, _ := cell.ContentTag(); tag {
			case ghostty.CellContentBgColorPalette:
				p, _ := cell.ColorPalette()
				css = fmt.Sprintf("background:var(--vt-palette-%d)", p)
			case ghostty.CellContentBgColorRGB:
				c, _ := cell.ColorRGB()
				css = fmt.Sprintf("background:#%02x%02x%02x", c.R, c.G, c.B)
			}
			if css != style {
				flush()
				style = css
			}
			run.WriteString(html.EscapeString(text))
		}
		flush()
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	return `<pre class="screen">` + strings.TrimRight(b.String(), "\n") + "</pre>", nil
}

func styleCSS(st *ghostty.Style) string {
	fg, bg := cssColor(st.FgColor(), ""), cssColor(st.BgColor(), "")
	if st.Inverse() {
		fg, bg = cmp.Or(bg, "var(--bg)"), cmp.Or(fg, "var(--fg)")
	}
	var parts []string
	if fg != "" {
		parts = append(parts, "color:"+fg)
	}
	if bg != "" {
		parts = append(parts, "background:"+bg)
	}
	if st.Bold() {
		parts = append(parts, "font-weight:bold")
	}
	if st.Italic() {
		parts = append(parts, "font-style:italic")
	}
	if st.Faint() {
		parts = append(parts, "opacity:.6")
	}
	if st.Underline() != 0 {
		parts = append(parts, "text-decoration:underline")
	}
	if st.Strikethrough() {
		parts = append(parts, "text-decoration:line-through")
	}
	return strings.Join(parts, ";")
}

func cssColor(c ghostty.StyleColor, none string) string {
	switch c.Tag {
	case ghostty.StyleColorPalette:
		return fmt.Sprintf("var(--vt-palette-%d)", c.Palette)
	case ghostty.StyleColorRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.RGB.R, c.RGB.G, c.RGB.B)
	}
	return none
}
