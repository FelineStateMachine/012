package theme

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Fill draws a rendered line on a band's colors out to width columns:
// every cell without a background of its own gets the band's background,
// and every cell without a text color its foreground. Styled parts (key
// chips, the mode indicator) keep their colors. A band with no colors
// returns the line unchanged.
//
// It works on the escape sequences, so it costs one pass over the line:
// the band's colors are set at the start and again after every SGR that
// resets them.
func Fill(line string, width int, band lipgloss.Style) string {
	bg, fg := band.GetBackground(), band.GetForeground()
	hasBg, hasFg := !isNoColor(bg), !isNoColor(fg)
	if !hasBg && !hasFg {
		return line
	}
	set := func(needBg, needFg bool) string {
		st := ansi.NewStyle()
		if needBg && hasBg {
			st = st.BackgroundColor(bg)
		}
		if needFg && hasFg {
			st = st.ForegroundColor(fg)
		}
		if len(st) == 0 {
			return ""
		}
		return st.String()
	}
	var b strings.Builder
	b.Grow(len(line) + 64)
	b.WriteString(set(true, true))
	p := ansi.NewParser()
	var state byte
	w := 0
	for s := line; len(s) > 0; {
		seq, sw, n, newState := ansi.DecodeSequence(s, state, p)
		state, s = newState, s[n:]
		b.WriteString(seq)
		w += sw
		if sw == 0 && isSGR(seq, p) {
			resetBg, resetFg := sgrResets(p.Params())
			b.WriteString(set(resetBg, resetFg))
		}
	}
	if w < width {
		b.WriteString("\x1b[m" + set(true, true) + strings.Repeat(" ", width-w))
	}
	b.WriteString("\x1b[m")
	return b.String()
}

// isSGR reports whether seq, just decoded by p, is Select Graphic
// Rendition (ESC [ ... m).
func isSGR(seq string, p *ansi.Parser) bool {
	if !ansi.HasCsiPrefix(seq) {
		return false
	}
	c := ansi.Cmd(p.Command())
	return c.Final() == 'm' && c.Prefix() == 0 && c.Intermediate() == 0
}

// sgrResets reports whether an SGR's parameters leave the background and
// the foreground at the terminal's default, so the band's must be set
// again.
func sgrResets(params ansi.Params) (bg, fg bool) {
	if len(params) == 0 {
		return true, true
	}
	for i := 0; i < len(params); i++ {
		switch v := params[i].Param(0); {
		case v == 0:
			bg, fg = true, true
		case v == 39:
			fg = true
		case v == 49:
			bg = true
		case v >= 30 && v <= 37, v >= 90 && v <= 97:
			fg = false
		case v >= 40 && v <= 47, v >= 100 && v <= 107:
			bg = false
		case v == 38 || v == 48 || v == 58:
			var c color.Color
			n := ansi.ReadStyleColor(params[i:], &c)
			if v == 38 {
				fg = false
			} else if v == 48 {
				bg = false
			}
			i += max(n, 1) - 1
		}
	}
	return bg, fg
}
