package e2e

import (
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// Reference palettes make screen snapshots deterministic and give review
// screenshots a realistic look. They follow common Ghostty defaults.
var (
	darkANSI = [16]uint32{
		0x1d1f21, 0xcc6666, 0xb5bd68, 0xf0c674, 0x81a2be, 0xb294bb, 0x8abeb7, 0xc5c8c6,
		0x666666, 0xd54e53, 0xb9ca4a, 0xe7c547, 0x7aa6da, 0xc397d8, 0x70c0b1, 0xeaeaea,
	}
	lightANSI = [16]uint32{
		0x1d1f21, 0xc82829, 0x718c00, 0xb58900, 0x4271ae, 0x8959a8, 0x3e999f, 0xd6d6d6,
		0x8e908c, 0xc82829, 0x718c00, 0xb58900, 0x4271ae, 0x8959a8, 0x3e999f, 0xffffff,
	}
)

func rgb(v uint32) ghostty.ColorRGB {
	return ghostty.ColorRGB{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}
}

func applyPalette(t *testing.T, vt *ghostty.Terminal, light bool) {
	t.Helper()
	ansi, bg, fg := darkANSI, rgb(0x1d1f21), rgb(0xc5c8c6)
	if light {
		ansi, bg, fg = lightANSI, rgb(0xfafafa), rgb(0x1d1f21)
	}
	p, err := vt.ColorPaletteDefault()
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range ansi {
		p[i] = rgb(c)
	}
	if err := vt.SetColorPalette(p); err != nil {
		t.Fatal(err)
	}
	if err := vt.SetColorBackground(&bg); err != nil {
		t.Fatal(err)
	}
	if err := vt.SetColorForeground(&fg); err != nil {
		t.Fatal(err)
	}
}
