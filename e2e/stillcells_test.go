package e2e

import (
	"fmt"
	"html"
	"image/color"
	"strconv"
	"strings"

	"golang.org/x/text/width"
)

// A golden screen read back into cells, for drawing it as a still. The
// HTML is what renderHTML writes: one line of text per row, runs of
// cells with the same style in a span, colors as palette variables.

// ink is a color as the golden names it: the terminal's default
// foreground or background, a palette entry or an RGB value.
type ink struct {
	kind    inkKind
	palette int
	rgb     color.RGBA
}

type inkKind uint8

const (
	inkNone inkKind = iota // the default: fg or bg as the slot says
	inkFg
	inkBg
	inkPalette
	inkRGB
)

// underline styles, as the golden's text-decoration names them.
const (
	ulNone = iota
	ulSingle
	ulDouble
	ulWavy
	ulDotted
	ulDashed
)

// stillCell is one cell of the screen.
type stillCell struct {
	r                    rune // 0 for the second half of a wide character
	fg, bg, ulInk        ink
	bold, italic, faint  bool
	underline            int
	strike, wide, filled bool
}

// parseScreen reads a golden's HTML into rows of cols cells.
func parseScreen(src string, cols, rows int) ([][]stillCell, error) {
	src = strings.TrimPrefix(src, `<pre class="screen">`)
	src = strings.TrimSuffix(src, "</pre>")
	grid := make([][]stillCell, rows)
	for y := range grid {
		grid[y] = make([]stillCell, cols)
	}
	for y, line := range strings.Split(src, "\n") {
		if y >= rows {
			return nil, fmt.Errorf("more than %d rows", rows)
		}
		if err := parseLine(grid[y], line); err != nil {
			return nil, fmt.Errorf("row %d: %w", y, err)
		}
	}
	return grid, nil
}

// parseLine fills row from one line of the golden.
func parseLine(row []stillCell, line string) error {
	x := 0
	var st stillCell
	for line != "" {
		switch {
		case strings.HasPrefix(line, `<span style="`):
			end := strings.Index(line, `">`)
			if end < 0 {
				return fmt.Errorf("unclosed span")
			}
			st = styleOf(line[len(`<span style="`):end])
			line = line[end+2:]
		case strings.HasPrefix(line, "</span>"):
			st = stillCell{}
			line = line[len("</span>"):]
		default:
			end := strings.IndexByte(line, '<')
			if end < 0 {
				end = len(line)
			}
			var err error
			if x, err = putText(row, x, html.UnescapeString(line[:end]), st); err != nil {
				return err
			}
			line = line[end:]
		}
	}
	return nil
}

// putText writes text into row from column x in style st and returns
// the column after it.
func putText(row []stillCell, x int, text string, st stillCell) (int, error) {
	for _, r := range text {
		w := 1
		if k := width.LookupRune(r).Kind(); k == width.EastAsianWide || k == width.EastAsianFullwidth {
			w = 2
		}
		if x+w > len(row) {
			return x, fmt.Errorf("wider than %d columns", len(row))
		}
		c := st
		c.r, c.wide, c.filled = r, w == 2, true
		row[x] = c
		if w == 2 {
			row[x+1] = st
			row[x+1].filled = true
		}
		x += w
	}
	return x, nil
}

// styleOf reads a span's CSS.
func styleOf(css string) stillCell {
	var c stillCell
	for _, decl := range strings.Split(css, ";") {
		prop, val, _ := strings.Cut(decl, ":")
		switch prop {
		case "color":
			c.fg = inkOf(val)
		case "background":
			c.bg = inkOf(val)
		case "font-weight":
			c.bold = val == "bold"
		case "font-style":
			c.italic = val == "italic"
		case "opacity":
			c.faint = true
		case "text-decoration":
			decorate(&c, val)
		}
	}
	return c
}

// decorate reads a text-decoration: underline with its style and color,
// or line-through.
func decorate(c *stillCell, val string) {
	f := strings.Fields(val)
	if len(f) == 0 {
		return
	}
	if f[0] == "line-through" {
		c.strike = true
		return
	}
	c.underline = ulSingle
	for _, w := range f[1:] {
		switch w {
		case "double":
			c.underline = ulDouble
		case "wavy":
			c.underline = ulWavy
		case "dotted":
			c.underline = ulDotted
		case "dashed":
			c.underline = ulDashed
		default:
			c.ulInk = inkOf(w)
		}
	}
}

// inkOf reads a CSS color as the goldens write them.
func inkOf(v string) ink {
	switch {
	case v == "var(--fg)":
		return ink{kind: inkFg}
	case v == "var(--bg)":
		return ink{kind: inkBg}
	case strings.HasPrefix(v, "var(--vt-palette-"):
		n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(v, "var(--vt-palette-"), ")"))
		return ink{kind: inkPalette, palette: n}
	case strings.HasPrefix(v, "#") && len(v) == 7:
		n, _ := strconv.ParseUint(v[1:], 16, 32)
		return ink{kind: inkRGB, rgb: rgba(uint32(n))}
	}
	return ink{}
}

func rgba(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}
