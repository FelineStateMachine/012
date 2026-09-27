package theme

import (
	"image/color"
	"math"
)

// Contrast correction for color schemes, adapted from puzzletea's theme
// package (same author): WCAG 2 contrast, and a readable replacement for
// a foreground that falls short, taken from the scheme's own colors when
// one qualifies.

// Contrast minimums.
const (
	minText      = 4.5 // text: WCAG AA
	minSecondary = 3.0 // hints, muted text, borders, chart axes
	minDisabled  = 2.0 // unavailable items stay visible but recede
	minDistinct  = 1.2 // a background role against the screen, so bars and highlights show
)

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		f := float64(v) / 0xffff
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrast is the WCAG contrast ratio of two colors, from 1 to 21.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// readable returns fg, or when it has less than min contrast with bg,
// the scheme color closest to it in lightness that has enough, or failing
// that the nearest color toward black or white that does.
func readable(fg, bg color.Color, p *Palette, min float64) color.Color {
	if contrast(fg, bg) >= min {
		return fg
	}
	target := luminance(fg)
	var best color.Color
	bestDist := math.Inf(1)
	for _, c := range append(p.ANSI[:], p.Foreground, p.Background) {
		if d := math.Abs(luminance(c) - target); contrast(c, bg) >= min && d < bestDist {
			best, bestDist = c, d
		}
	}
	if best != nil {
		return best
	}
	return nudge(fg, bg, min)
}

// nudge blends fg toward white or black, whichever moves it away from
// bg, just far enough to reach min contrast.
func nudge(fg, bg color.Color, min float64) color.Color {
	var end color.Color = color.White
	if luminance(fg) <= luminance(bg) {
		end = color.Black
	}
	if contrast(end, bg) < min {
		if end == color.White {
			end = color.Black
		} else {
			end = color.White
		}
	}
	lo, hi := 0.0, 1.0
	for range 24 {
		mid := (lo + hi) / 2
		if contrast(blend(fg, end, mid), bg) >= min {
			hi = mid
		} else {
			lo = mid
		}
	}
	return blend(fg, end, hi)
}

// distinct returns bg, or when it's too close to the screen to show, bg
// moved toward toward until it stands apart.
func distinct(bg, screen, toward color.Color) color.Color {
	if contrast(bg, screen) >= minDistinct {
		return bg
	}
	lo, hi := 0.0, 1.0
	for range 24 {
		mid := (lo + hi) / 2
		if contrast(blend(bg, toward, mid), screen) >= minDistinct {
			hi = mid
		} else {
			lo = mid
		}
	}
	return blend(bg, toward, hi)
}

// blend is t of the way from a to b.
func blend(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 {
		return uint8(float64(x>>8)*(1-t) + float64(y>>8)*t + 0.5)
	}
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 0xff}
}
