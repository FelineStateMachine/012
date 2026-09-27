package theme

import (
	"image/color"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// In the terminal theme, rule fills and text colors on them read on
// both reference palettes: the fill's ink stands in where a text color
// wouldn't.
func TestRuleColorsReadOnReferencePalettes(t *testing.T) {
	for _, dark := range []bool{true, false} {
		th := New(dark)
		for _, c := range sheet.Colors()[1:] {
			for _, text := range sheet.Colors() {
				s := th.Rule(sheet.RuleStyle{Fill: c, Text: text})
				fg := reference(dark, s.GetForeground().(ansi.BasicColor))
				bg := reference(dark, s.GetBackground().(ansi.BasicColor))
				// The reference palettes' own colors fall a little short
				// of 4.5 in places: black on the light green is 4.29, as
				// for Traced.
				if r := contrast(fg, bg); r < 4.25 {
					t.Errorf("dark %v: %s text on %s fill: %.2f", dark, text.Title(), c.Title(), r)
				}
			}
		}
	}
}

// Every scheme's rule colors read: text colors on the screen and fills
// with their text (TestEveryThemeReadable checks the roles), and every
// shade of every color scale.
func TestColorScalesReadable(t *testing.T) {
	xterm := func(i int) color.Color { return reference(true, ansi.BasicColor(i)) }
	for _, p := range append(Builtins(), Palette{}) {
		th := New(true)
		if p.Background != nil {
			th = FromPalette(p)
		}
		for _, from := range sheet.Colors()[1:] {
			for _, to := range sheet.Colors()[1:] {
				for step := 0; step <= scaleSteps; step += 4 {
					s := th.ScaleFill(from, to, float64(step)/scaleSteps, xterm).Style
					requireContrast(t, p.Name, "color scale", s.GetForeground(), s.GetBackground(), minText)
				}
			}
		}
	}
}

func TestScaleFillBlends(t *testing.T) {
	p, _ := Lookup("Dracula", "")
	th := FromPalette(p)
	none := func(int) color.Color { return color.Black }
	lo := th.ScaleFill(sheet.ColorRed, sheet.ColorGreen, 0, none).Style.GetBackground()
	hi := th.ScaleFill(sheet.ColorRed, sheet.ColorGreen, 1, none).Style.GetBackground()
	mid := th.ScaleFill(sheet.ColorRed, sheet.ColorGreen, 0.5, none).Style.GetBackground()
	same := func(a, b color.Color) bool {
		ar, ag, ab, _ := a.RGBA()
		br, bg, bb, _ := b.RGBA()
		return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8
	}
	if !same(lo, p.ANSI[1]) || !same(hi, p.ANSI[2]) || same(mid, lo) || same(mid, hi) {
		t.Errorf("scale %v %v %v, scheme's red %v green %v", lo, mid, hi, p.ANSI[1], p.ANSI[2])
	}
	// A shade's codes draw what its style does.
	for _, s := range []Shade{th.ScaleFill(sheet.ColorRed, sheet.ColorGreen, 0.5, none), th.RuleShade(sheet.RuleStyle{Fill: sheet.ColorBlue, Text: sheet.ColorYellow})} {
		if s.Open == "" || s.Wrap(" 12 ") != s.Style.Render(" 12 ") {
			t.Errorf("wrap %q, render %q", s.Wrap(" 12 "), s.Style.Render(" 12 "))
		}
	}
	if len(th.scales) != 3 {
		t.Errorf("%d shades kept", len(th.scales))
	}
}
