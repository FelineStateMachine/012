package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Monochrome returns t with every color of every role replaced by one
// gray, as a terminal without color, or a reader who can't tell the
// colors apart, sees it: what's left to tell states apart is text,
// glyphs and attributes (bold, italic, underline styles, reverse video).
// Tests draw with it to check that every state reads without color (see
// docs/contributing/ux.md#visual-rules).
func Monochrome(t Theme) Theme {
	one := color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}
	eachRole(&t, func(_ string, s *lipgloss.Style) {
		if !isNoColor(s.GetForeground()) {
			*s = s.Foreground(one)
		}
		if !isNoColor(s.GetBackground()) {
			*s = s.Background(one)
		}
		if !isNoColor(s.GetUnderlineColor()) {
			*s = s.UnderlineColor(one)
		}
	})
	t.scales, t.shades = map[scaleKey]Shade{}, map[sheet.RuleStyle]Shade{}
	return t
}
