package theme

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Conditional formats draw in the named colors of sheet.Color, each an
// ANSI slot, so a rule looks right in the terminal's palette and in every
// color scheme: a text color is the slot as a foreground, a fill the slot
// as a background with text readable on it, and a color scale blends
// the slots of its points.

// ruleSlots are the ANSI colors of sheet.Color, by index.
var ruleSlots = [sheet.NumColors]ansi.BasicColor{0, lipgloss.Red, lipgloss.Yellow, lipgloss.Green,
	lipgloss.Cyan, lipgloss.Blue, lipgloss.Magenta}

// RuleSlot is the ANSI color index of a rule color, or -1 for none.
func RuleSlot(c sheet.Color) int {
	if c == sheet.ColorNone || int(c) >= sheet.NumColors {
		return -1
	}
	return int(ruleSlots[c])
}

// referenceANSI are the palettes the terminal theme's rule colors are
// checked against, the reference palettes of the e2e screens (common
// Ghostty defaults), dark and light.
var referenceANSI = [2][16]uint32{
	{0x1d1f21, 0xcc6666, 0xb5bd68, 0xf0c674, 0x81a2be, 0xb294bb, 0x8abeb7, 0xc5c8c6,
		0x666666, 0xd54e53, 0xb9ca4a, 0xe7c547, 0x7aa6da, 0xc397d8, 0x70c0b1, 0xeaeaea},
	{0x1d1f21, 0xc82829, 0x718c00, 0xb58900, 0x4271ae, 0x8959a8, 0x3e999f, 0xd6d6d6,
		0x8e908c, 0xc82829, 0x718c00, 0xb58900, 0x4271ae, 0x8959a8, 0x3e999f, 0xffffff},
}

func reference(dark bool, slot ansi.BasicColor) color.Color {
	p := referenceANSI[1]
	if dark {
		p = referenceANSI[0]
	}
	v := p[slot%16]
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

// ruleInk is the text color on a fill: black or bright white, whichever
// reads better on the reference palette.
func ruleInk(dark bool, fill ansi.BasicColor) ansi.BasicColor {
	bg := reference(dark, fill)
	if contrast(reference(dark, lipgloss.Black), bg) >= contrast(reference(dark, lipgloss.BrightWhite), bg) {
		return lipgloss.Black
	}
	return lipgloss.BrightWhite
}

// ruleRoles sets the roles of rule colors for the terminal theme.
func ruleRoles(t *Theme, dark bool) {
	n := sheet.NumColors
	for c := 1; c < n; c++ {
		slot := ruleSlots[c]
		t.RuleText[c] = lipgloss.NewStyle().Foreground(slot)
		t.RuleFill[c] = lipgloss.NewStyle().Background(slot).Foreground(ruleInk(dark, slot))
	}
	for f := 1; f < n; f++ {
		for c := 1; c < n; c++ {
			// A text color on a fill where it reads; the fill's own ink
			// where it wouldn't (red on red, yellow on green).
			fg := ruleSlots[c]
			if c == f || contrast(reference(dark, fg), reference(dark, ruleSlots[f])) < minText {
				fg = ruleInk(dark, ruleSlots[f])
			}
			t.RuleOn[f*n+c] = lipgloss.NewStyle().Background(ruleSlots[f]).Foreground(fg)
		}
	}
}

// Rule is the base style of a cell a single-color rule formats: its
// fill and text color, with text readable on the fill. Text styles are
// added by Text.
func (t *Theme) Rule(st sheet.RuleStyle) lipgloss.Style {
	switch {
	case st.Fill == sheet.ColorNone && st.Text == sheet.ColorNone:
		return t.Cell
	case st.Fill == sheet.ColorNone:
		return t.RuleText[st.Text]
	case st.Text == sheet.ColorNone:
		return t.RuleFill[st.Fill]
	}
	return t.RuleOn[int(st.Fill)*sheet.NumColors+int(st.Text)]
}

// scaleSteps is how many shades a color scale has between two points.
const scaleSteps = 32

type scaleKey struct {
	from, to color.RGBA
	step     int
}

// ScaleFill is the fill of a color scale's cell, pos (0 to 1) of the way
// from one point's color to the next's, with text readable on it. The
// colors are the scheme's, or for the terminal theme rgb's (the
// terminal's palette as it reports it). Shades are kept per theme.
func (t *Theme) ScaleFill(from, to sheet.Color, pos float64, rgb func(slot int) color.Color) lipgloss.Style {
	slotRGB := func(c sheet.Color) color.RGBA {
		var col color.Color = color.Black
		if s := RuleSlot(c); s >= 0 {
			if t.Palette != nil {
				col = t.Palette.ANSI[s]
			} else {
				col = rgb(s)
			}
		}
		r, g, b, _ := col.RGBA()
		return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 0xff}
	}
	step := int(math.Round(min(max(pos, 0), 1) * scaleSteps))
	k := scaleKey{slotRGB(from), slotRGB(to), step}
	if s, ok := t.scales[k]; ok {
		return s
	}
	bg := blend(k.from, k.to, float64(step)/scaleSteps)
	s := lipgloss.NewStyle().Background(bg).Foreground(t.scaleInk(bg, rgb))
	if t.scales != nil && len(t.scales) < 4096 {
		t.scales[k] = s
	}
	return s
}

// scaleInk is the text color on a scale's shade: the screen's text or
// background color, or failing both black or white.
func (t *Theme) scaleInk(bg color.Color, rgb func(int) color.Color) color.Color {
	fg, back := rgb(15), rgb(0)
	if t.Palette != nil {
		fg, back = t.Palette.Foreground, t.Palette.Background
	}
	best := fg
	if contrast(back, bg) > contrast(fg, bg) {
		best = back
	}
	if contrast(best, bg) >= minText {
		return best
	}
	if contrast(color.Black, bg) > contrast(color.White, bg) {
		return color.Black
	}
	return color.White
}
