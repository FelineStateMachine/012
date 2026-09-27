package theme

import "strings"

// HighContrast is the theme name for the high-contrast schemes: the dark
// one on a dark terminal, the light one on a light terminal, switching
// when the terminal does. HighContrastDark and HighContrastLight name
// one of them.
const (
	HighContrast      = "high-contrast"
	HighContrastDark  = "High Contrast Dark"
	HighContrastLight = "High Contrast Light"
)

// The high-contrast schemes: white on black and black on white, with
// ANSI colors picked so the roles defined on them (black text on the
// pointer's cyan, the found cells' yellow, the traced cells' green) meet
// WCAG AAA before correction, and a selection in saturated blue with
// white text. FromPalette then holds every role to AAA (aaa below).
const highContrastDark = `# High Contrast Dark: WCAG AAA (7:1) text.
background = #000000
foreground = #ffffff
selection-background = #0033cc
palette = 0=#000000
palette = 1=#ff8080
palette = 2=#40e040
palette = 3=#ffe040
palette = 4=#80a8ff
palette = 5=#ff80ff
palette = 6=#40e0ff
palette = 7=#e0e0e0
palette = 8=#3a3a3a
palette = 9=#ffa0a0
palette = 10=#80ff80
palette = 11=#ffff80
palette = 12=#a0c0ff
palette = 13=#ffa0ff
palette = 14=#a0f0ff
palette = 15=#ffffff
`

const highContrastLight = `# High Contrast Light: WCAG AAA (7:1) text.
background = #ffffff
foreground = #000000
selection-background = #0033cc
palette = 0=#000000
palette = 1=#a00000
palette = 2=#b0f0b0
palette = 3=#ffe070
palette = 4=#0033cc
palette = 5=#800080
palette = 6=#a0e8ff
palette = 7=#d0d0d0
palette = 8=#6a6a6a
palette = 9=#a00000
palette = 10=#005a00
palette = 11=#6a4a00
palette = 12=#0033cc
palette = 13=#800080
palette = 14=#005a6a
palette = 15=#ffffff
`

// highContrastPalettes are the two schemes, dark first.
func highContrastPalettes() []Palette {
	var out []Palette
	for _, s := range []struct{ name, text string }{{HighContrastDark, highContrastDark}, {HighContrastLight, highContrastLight}} {
		p, _, err := parseText(s.text)
		if err != nil {
			panic("theme: " + s.name + ": " + err.Error())
		}
		p.Name, p.HighContrast = s.name, true
		out = append(out, p)
	}
	return out
}

// isHighContrast reports whether name is HighContrast, loosely matched
// as Lookup matches names.
func isHighContrast(name string) bool {
	return squash(strings.TrimSpace(name)) == squash(HighContrast)
}

// highContrastFor is the high-contrast scheme for a dark or light
// terminal.
func highContrastFor(dark bool) Palette {
	ps := highContrastPalettes()
	if dark {
		return ps[0]
	}
	return ps[1]
}
