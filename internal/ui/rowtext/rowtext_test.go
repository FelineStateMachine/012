package rowtext

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// textCutter must cut exactly what ansi.Cut does, wide and combining
// characters included, for every way of splitting a line into columns.
func TestTextCutterMatchesCut(t *testing.T) {
	texts := []string{
		"plain ascii text",
		"Ça déjà vu, naïve café",
		"東京都の天気は晴れ",
		"é combining, 🙂🚀 emoji, 🇫🇷 flag, 👍🏽 tone",
		"a東b京c🙂d",
		"",
	}
	for _, s := range texts {
		tw := ansi.StringWidth(s)
		for step := 1; step <= 4; step++ {
			for off := range step {
				c := textCutter{s: s}
				for l := off; l < tw+step; l += step {
					if got, want := c.cut(l, l+step), ansi.Cut(s, l, l+step); got != want {
						t.Errorf("%q [%d,%d): got %q, want %q", s, l, l+step, got, want)
					}
				}
			}
		}
	}
}

// textWidth must measure what ansi.StringWidth does, its ASCII shortcut
// included.
func TestTextWidthMatchesStringWidth(t *testing.T) {
	for _, s := range []string{"", "1234.5", " ~plain ascii! ", "tab\there", "\x1b[1mbold\x1b[0m", "café", "東京", "🙂 a", "á"} {
		if got, want := textWidth(s), ansi.StringWidth(s); got != want {
			t.Errorf("%q: got %d, want %d", s, got, want)
		}
	}
}
