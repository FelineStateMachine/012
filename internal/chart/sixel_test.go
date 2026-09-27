package chart

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// The encoder's bytes for a small image: a register per color in the
// order they appear, runs of four or more as "!n", trailing empty
// sixels left out, "$" between the colors of a band and "-" between
// bands, a pixel with half coverage composited over the background and
// a transparent one left unset.
func TestSixelGolden(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 7))
	red := color.RGBA{255, 0, 0, 255}
	for x := range 4 {
		img.SetRGBA(x, 0, red)
	}
	img.SetRGBA(1, 1, color.RGBA{0, 128, 0, 128}) // premultiplied: half green
	img.SetRGBA(0, 2, red)
	img.SetRGBA(1, 2, red)
	img.SetRGBA(2, 2, red)
	img.SetRGBA(0, 6, color.RGBA{0, 0, 255, 255})
	got := Sixel(img, color.RGBA{0, 0, 0, 255}, 256)
	want := "\x1bP0;1;0q\"1;1;4;7" +
		"#0;2;100;0;0#1;2;0;50;0#2;2;0;0;100" +
		"#0DDD@$#1?A" + // red: rows 0 and 2 (1+4) three times, then row 0 alone
		"-#2@" +
		"\x1b\\"
	if got != want {
		t.Errorf("Sixel =\n%q\nwant\n%q", got, want)
	}
	img.SetRGBA(3, 2, red)
	if got := Sixel(img, color.RGBA{0, 0, 0, 255}, 256); !strings.Contains(got, "#0!4D$") {
		t.Errorf("a run of four isn't run-length encoded: %q", got)
	}
}

// Past the terminal's registers, colors are merged until they fit, and
// every pixel still names a register that exists.
func TestSixelQuantize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := range 30 {
		for x := range 40 {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 6), uint8(y * 8), uint8((x + y) * 3), 255})
		}
	}
	for _, regs := range []int{16, 256} {
		idx, pal := quantize(img, color.RGBA{}, regs)
		if len(pal) > regs {
			t.Errorf("%d registers: %d colors", regs, len(pal))
		}
		for _, i := range idx {
			if i < 0 || int(i) >= len(pal) {
				t.Fatalf("%d registers: pixel names register %d of %d", regs, i, len(pal))
			}
		}
	}
	if s := Sixel(img, color.RGBA{}, 16); strings.Count(s, ";2;") > 16 {
		t.Errorf("more than 16 colors defined")
	}
}

// A chart's image encodes, and an empty image draws nothing.
func TestSixelChart(t *testing.T) {
	img := Image(sheet.ChartColumn, single, 40, 12, Options{CellW: 8, CellH: 16}, testPalette)
	if img == nil {
		t.Fatal("no image")
	}
	s := Sixel(img, color.RGBA{0, 0, 0, 255}, 256)
	if !strings.HasPrefix(s, "\x1bP0;1;0q\"1;1;") || !strings.HasSuffix(s, "\x1b\\") {
		t.Errorf("not a sixel sequence: %.40q", s)
	}
	if Sixel(image.NewRGBA(image.Rect(0, 0, 0, 0)), color.RGBA{}, 256) != "" {
		t.Error("an empty image drew something")
	}
}
