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
		var line spanWriter
		for x := range cols {
			text, css, ok, err := cellHTML(vt, ghostty.Point{Tag: ghostty.PointTagActive, X: x, Y: y})
			if err != nil {
				return "", err
			}
			if ok {
				line.add(text, css)
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	return `<pre class="screen">` + strings.TrimRight(b.String(), "\n") + "</pre>", nil
}

// spanWriter writes a line of cells, one span per run of cells with the
// same CSS; unstyled runs are bare text.
type spanWriter struct {
	out, run strings.Builder
	style    string
}

// add appends a cell's text with its style.
func (w *spanWriter) add(text, css string) {
	if css != w.style {
		w.flush()
		w.style = css
	}
	w.run.WriteString(html.EscapeString(text))
}

func (w *spanWriter) flush() {
	if w.run.Len() == 0 {
		return
	}
	if w.style == "" {
		w.out.WriteString(w.run.String())
	} else {
		fmt.Fprintf(&w.out, `<span style="%s">%s</span>`, w.style, w.run.String())
	}
	w.run.Reset()
}

// String is the line so far.
func (w *spanWriter) String() string {
	w.flush()
	return w.out.String()
}

// cellHTML returns the text of the cell at p and its CSS; ok is false
// for the second half of a wide character, which draws nothing.
func cellHTML(vt *ghostty.Terminal, p ghostty.Point) (text, css string, ok bool, err error) {
	ref, err := vt.GridRef(p)
	if err != nil {
		return "", "", false, err
	}
	cell, err := ref.Cell()
	if err != nil {
		return "", "", false, err
	}
	if wide, _ := cell.Wide(); wide == ghostty.CellWideSpacerTail {
		return "", "", false, nil
	}
	text = " "
	if has, _ := cell.HasText(); has {
		cps, _ := ref.Graphemes()
		var sb strings.Builder
		for _, cp := range cps {
			sb.WriteRune(rune(cp))
		}
		text = sb.String()
	}
	if st, err := ref.Style(); err == nil && !st.IsDefault() {
		css = styleCSS(st, strings.TrimSpace(text) == "")
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
	return text, css, true, nil
}

// styleCSS converts a cell style to CSS. For blank cells only what shows
// is kept (background, lines), so snapshots don't depend on whether the
// renderer wrote spaces or erased cells.
func styleCSS(st *ghostty.Style, blank bool) string {
	fg, bg := cssColor(st.FgColor(), ""), cssColor(st.BgColor(), "")
	if st.Inverse() {
		fg, bg = cmp.Or(bg, "var(--bg)"), cmp.Or(fg, "var(--fg)")
	}
	var parts []string
	if fg != "" && !blank {
		parts = append(parts, "color:"+fg)
	}
	if bg != "" {
		parts = append(parts, "background:"+bg)
	}
	if st.Bold() && !blank {
		parts = append(parts, "font-weight:bold")
	}
	if st.Italic() && !blank {
		parts = append(parts, "font-style:italic")
	}
	if st.Faint() && !blank {
		parts = append(parts, "opacity:.6")
	}
	if u := st.Underline(); u != ghostty.UnderlineNone {
		line := map[ghostty.SGRUnderline]string{
			ghostty.UnderlineDouble: " double", ghostty.UnderlineCurly: " wavy",
			ghostty.UnderlineDotted: " dotted", ghostty.UnderlineDashed: " dashed",
		}[u]
		if c := cssColor(st.UnderlineColor(), ""); c != "" {
			line += " " + c
		}
		parts = append(parts, "text-decoration:underline"+line)
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
