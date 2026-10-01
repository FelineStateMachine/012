package e2e

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Stills are drawn in a terminal color scheme: the golden's palette
// indexes in the scheme's colors, so a still stays tied to its golden
// whatever scheme shows it. The schemes are 012's built-in ones
// (internal/ui/theme/themes.json, the file VHS's Set Theme reads too),
// and the families the docs use are the table in
// docs/contributing/site.md, which the tapes follow as well.

const (
	schemesFile = "../internal/ui/theme/themes.json"
	familiesDoc = "../docs/contributing/site.md"
)

// stillPalette is a terminal's colors: a scheme's 16 ANSI colors, text
// and background.
type stillPalette struct {
	name   string
	ansi   [16]uint32
	fg, bg uint32
}

// family is a row of the families table: a dark scheme and a light one.
type family struct{ dark, light string }

// familyRow is a row of the table: | Family | `dark` | `light` | pages |.
var familyRow = regexp.MustCompile("(?m)^\\| ([^|`]+?) \\| `([^`]+)` \\| `([^`]+)` \\|")

// pictureFamilies reads the families table from the docs.
func pictureFamilies() (map[string]family, error) {
	src, err := os.ReadFile(familiesDoc)
	if err != nil {
		return nil, err
	}
	fams := map[string]family{}
	for _, m := range familyRow.FindAllStringSubmatch(string(src), -1) {
		fams[m[1]] = family{dark: m[2], light: m[3]}
	}
	if len(fams) == 0 {
		return nil, fmt.Errorf("%s: no families table", familiesDoc)
	}
	return fams, nil
}

// loadSchemes reads the built-in schemes by name.
func loadSchemes() (map[string]stillPalette, error) {
	src, err := os.ReadFile(schemesFile)
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(src, &raw); err != nil {
		return nil, err
	}
	slots := [16]string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"brightBlack", "brightRed", "brightGreen", "brightYellow", "brightBlue", "brightMagenta", "brightCyan", "brightWhite"}
	out := map[string]stillPalette{}
	for _, s := range raw {
		name, _ := s["name"].(string)
		p := stillPalette{name: name}
		ok := hexInto(s["foreground"], &p.fg) && hexInto(s["background"], &p.bg)
		for i, k := range slots {
			ok = ok && hexInto(s[k], &p.ansi[i])
		}
		if ok {
			out[name] = p
		}
	}
	return out, nil
}

func hexInto(v any, dst *uint32) bool {
	s, _ := v.(string)
	n, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil || len(s) != 7 {
		return false
	}
	*dst = uint32(n)
	return true
}

// color resolves an ink; def is what inkNone means in its slot.
func (p stillPalette) color(k ink, def inkKind) color.RGBA {
	if k.kind == inkNone {
		k.kind = def
	}
	switch k.kind {
	case inkFg:
		return rgba(p.fg)
	case inkPalette:
		return rgba(p.xterm(k.palette))
	case inkRGB:
		return k.rgb
	}
	return rgba(p.bg)
}

// xterm is palette entry n: the 16 ANSI colors, then xterm's 6x6x6 cube
// and gray ramp.
func (p stillPalette) xterm(n int) uint32 {
	switch {
	case n < 16:
		return p.ansi[n]
	case n < 232:
		n -= 16
		level := func(v int) uint32 {
			if v == 0 {
				return 0
			}
			return uint32(55 + 40*v)
		}
		return level(n/36)<<16 | level(n/6%6)<<8 | level(n%6)
	}
	g := uint32(8 + 10*(n-232))
	return g<<16 | g<<8 | g
}

// inks are a cell's colors in p, held to the contrast 012's schemes hold
// their roles to (internal/ui/theme's FromPalette), since a scheme's
// colors weren't chosen for 012's: a background from the palette stands
// apart from the screen, and text moves toward the scheme's text color,
// or black or white, until it reaches its minimum. Colors the user chose
// as RGB are drawn as they are.
func (p stillPalette) inks(c stillCell) (fg, bg color.RGBA) {
	text, screen := rgba(p.fg), rgba(p.bg)
	bg = p.color(c.bg, inkBg)
	if c.bg.kind == inkPalette {
		bg = distinct(bg, screen, text)
	}
	fg = p.color(c.fg, inkFg)
	if c.faint {
		fg = blend(bg, fg, 0.6)
	}
	if c.fg.kind != inkRGB && c.bg.kind != inkRGB {
		fg = readable(fg, bg, text, minContrast(c))
	}
	return fg, bg
}

// minContrast is a cell's minimum, as theme's: 4.5:1 for text, 3:1 for
// lines and 012's muted gray (hints, borders), 2:1 for faint text.
func minContrast(c stillCell) float64 {
	switch {
	case c.faint:
		return 2
	case c.r >= 0x2500 && c.r <= 0x28ff, c.fg.kind == inkPalette && c.fg.palette == 8:
		return 3
	}
	return 4.5
}

// The contrast arithmetic is internal/ui/theme's (contrast.go), which
// this module can't import: WCAG 2 contrast, and a color that falls
// short moved just far enough.

func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 0xff
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func contrast(a, b color.RGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// readable is fg, or fg moved toward toward, or failing that black or
// white, until it has min contrast with bg.
func readable(fg, bg, toward color.RGBA, min float64) color.RGBA {
	if contrast(fg, bg) >= min {
		return fg
	}
	if contrast(toward, bg) >= min {
		return blendUntil(fg, toward, func(c color.RGBA) bool { return contrast(c, bg) >= min })
	}
	end := color.RGBA{0xff, 0xff, 0xff, 0xff}
	if luminance(fg) <= luminance(bg) {
		end = color.RGBA{0, 0, 0, 0xff}
	}
	if contrast(end, bg) < min {
		end = color.RGBA{0xff - end.R, 0xff - end.G, 0xff - end.B, 0xff}
	}
	return blendUntil(fg, end, func(c color.RGBA) bool { return contrast(c, bg) >= min })
}

// distinct is bg, or bg moved toward toward until it stands apart from
// the screen (1.2:1).
func distinct(bg, screen, toward color.RGBA) color.RGBA {
	if contrast(bg, screen) >= 1.2 {
		return bg
	}
	return blendUntil(bg, toward, func(c color.RGBA) bool { return contrast(c, screen) >= 1.2 })
}

// blendUntil is the color nearest from on the way to end for which ok
// holds; ok(end) must.
func blendUntil(from, end color.RGBA, ok func(color.RGBA) bool) color.RGBA {
	lo, hi := 0.0, 1.0
	for range 24 {
		mid := (lo + hi) / 2
		if ok(blend(from, end, mid)) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return blend(from, end, hi)
}
