package e2e

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Drawing a screen's cells as a picture of a terminal: text in
// JetBrains Mono, the font of the recordings and the site (testdata/fonts,
// so every machine draws the same letters), with Go Mono for the few
// characters it has none of, and box drawing, blocks, braille and the
// symbols 012 draws its chrome with drawn as shapes, as terminals draw
// them, so lines join across cells.

const (
	stillFontSize = 26 // pixels; stills are drawn at twice the size they show
	stillPad      = 28 // around the screen, inside the window
	stillRadius   = 18 // the window's corners
)

// stillFaces are the font's four styles at the still's size, regular,
// bold, italic and bold italic, and Go Mono's for what the font lacks.
type stillFaces struct {
	faces, fallback [4]font.Face
	cw, ch, ascent  int
}

// fontFiles are the styles' files in testdata/fonts.
var fontFiles = [4]string{"JetBrainsMono-Regular.ttf", "JetBrainsMono-Bold.ttf", "JetBrainsMono-Italic.ttf", "JetBrainsMono-BoldItalic.ttf"}

func loadFaces() (*stillFaces, error) {
	var fs stillFaces
	goMono := [4][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF}
	for i, name := range fontFiles {
		ttf, err := os.ReadFile(filepath.Join("testdata", "fonts", name))
		if err != nil {
			return nil, err
		}
		if fs.faces[i], err = newFace(ttf); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if fs.fallback[i], err = newFace(goMono[i]); err != nil {
			return nil, err
		}
	}
	adv, _ := fs.faces[0].GlyphAdvance('0')
	m := fs.faces[0].Metrics()
	fs.cw = adv.Round()
	fs.ch = int(math.Round(stillFontSize * 1.3))
	fs.ascent = (fs.ch-(m.Ascent+m.Descent).Round())/2 + m.Ascent.Round()
	return &fs, nil
}

func newFace(ttf []byte) (font.Face, error) {
	parsed, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{Size: stillFontSize, DPI: 72, Hinting: font.HintingNone})
}

// face is the face c's rune is drawn in: its style in the font, or in Go
// Mono when the font has no glyph for it; nil when neither has.
func (fs *stillFaces) face(c stillCell) font.Face {
	i := 0
	if c.bold {
		i++
	}
	if c.italic {
		i += 2
	}
	for _, f := range []font.Face{fs.faces[i], fs.fallback[i]} {
		if _, ok := f.GlyphAdvance(c.r); ok {
			return f
		}
	}
	return nil
}

// drawStill draws grid as a terminal window in palette p. It returns the
// runes it had no way to draw.
func drawStill(grid [][]stillCell, p stillPalette, fs *stillFaces) (*image.NRGBA, []rune) {
	cols, rows := len(grid[0]), len(grid)
	w, h := cols*fs.cw+2*stillPad, rows*fs.ch+2*stillPad
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	window(img, p)
	var missing []rune
	for y, row := range grid {
		for x := range row {
			origin := image.Pt(stillPad+x*fs.cw, stillPad+y*fs.ch)
			if !drawCell(img, row, x, origin, p, fs) {
				missing = append(missing, row[x].r)
			}
		}
	}
	return img, missing
}

// window fills a rounded rectangle with the terminal's background, with
// a hairline border so the window reads on a page of the same color.
func window(img *image.NRGBA, p stillPalette) {
	b := img.Bounds()
	w, h := float32(b.Dx()), float32(b.Dy())
	border := blend(rgba(p.bg), rgba(p.fg), 0.18)
	fillShape(img, b, border, func(s *shape) { s.roundRect(0, 0, w, h, stillRadius) })
	fillShape(img, b, rgba(p.bg), func(s *shape) { s.roundRect(2, 2, w-2, h-2, stillRadius-2) })
}

// drawCell draws cell x of row at origin; false when its rune has no
// glyph.
func drawCell(img *image.NRGBA, row []stillCell, x int, origin image.Point, p stillPalette, fs *stillFaces) bool {
	c := row[x]
	span := 1
	if c.wide {
		span = 2
	}
	r := image.Rect(origin.X, origin.Y, origin.X+span*fs.cw, origin.Y+fs.ch)
	if c.r == 0 && c.filled {
		return true // the second half of a wide character, drawn with the first
	}
	fg, bg := p.inks(c)
	if c.bg.kind != inkNone {
		draw.Draw(img, image.Rect(origin.X, origin.Y, origin.X+fs.cw, origin.Y+fs.ch), image.NewUniform(bg), image.Point{}, draw.Src)
	}
	ok := true
	if c.r != 0 && c.r != ' ' && c.r != ' ' {
		ok = drawGlyph(img, c, r, fg, fs)
	}
	lines(img, c, r, fg, bg, p, fs)
	return ok
}

// drawGlyph draws c's rune: as a shape when it's one terminals draw
// themselves, else from the font.
func drawGlyph(img *image.NRGBA, c stillCell, r image.Rectangle, fg color.RGBA, fs *stillFaces) bool {
	if drawn := drawSpecial(img, c.r, r, fg, fs); drawn {
		return true
	}
	face := fs.face(c)
	if face == nil {
		return false
	}
	adv, _ := face.GlyphAdvance(c.r)
	d := font.Drawer{Dst: img, Src: image.NewUniform(fg), Face: face}
	d.Dot = fixed.P(r.Min.X, r.Min.Y+fs.ascent)
	d.Dot.X += (fixed.I(r.Dx()) - adv) / 2
	d.DrawString(string(c.r))
	return true
}

// lines draws c's underline and strikethrough.
func lines(img *image.NRGBA, c stillCell, r image.Rectangle, fg, bg color.RGBA, p stillPalette, fs *stillFaces) {
	t := lineWidth(fs.cw)
	if c.strike {
		y := r.Min.Y + fs.ascent - stillFontSize*3/10
		fill(img, image.Rect(r.Min.X, y, r.Max.X, y+t), fg)
	}
	if c.underline == ulNone {
		return
	}
	col := fg
	if c.ulInk.kind != inkNone {
		col = p.color(c.ulInk, inkFg)
		if c.ulInk.kind == inkPalette {
			col = readable(col, bg, rgba(p.fg), 3)
		}
	}
	y := r.Min.Y + fs.ascent + t*2
	switch c.underline {
	case ulDouble:
		fill(img, image.Rect(r.Min.X, y-t, r.Max.X, y), col)
		fill(img, image.Rect(r.Min.X, y+t, r.Max.X, y+2*t), col)
	case ulWavy:
		wavy(img, r, y, t, col)
	case ulDotted:
		for x := r.Min.X; x < r.Max.X; x += 2 * t {
			fill(img, image.Rect(x, y, x+t, y+t), col)
		}
	case ulDashed:
		for x := r.Min.X; x < r.Max.X; x += r.Dx() / 2 {
			fill(img, image.Rect(x, y, x+r.Dx()*3/10, y+t), col)
		}
	default:
		fill(img, image.Rect(r.Min.X, y, r.Max.X, y+t), col)
	}
}

// wavy draws a curly underline: one wave a cell, in phase across cells.
func wavy(img *image.NRGBA, r image.Rectangle, y, t int, col color.RGBA) {
	amp := float32(t) * 1.2
	fillShape(img, image.Rect(r.Min.X, y-2*t-2, r.Max.X, y+2*t+2), col, func(s *shape) {
		var pts [][2]float32
		w := float32(r.Dx())
		for i := 0; i <= 12; i++ {
			fx := w * float32(i) / 12
			pts = append(pts, [2]float32{fx, float32(2*t+2) + amp*float32(math.Sin(2*math.Pi*float64(i)/12))})
		}
		s.stroke(pts, float32(t))
	})
}

// lineWidth is a light line's width for cells cw wide.
func lineWidth(cw int) int { return max(1, (cw+4)/8) }

func fill(img *image.NRGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Over)
}

// blend mixes a toward b by t.
func blend(a, b color.RGBA, t float64) color.RGBA {
	mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x)*(1-t) + float64(y)*t)) }
	return color.RGBA{mix(a.R, b.R), mix(a.G, b.G), mix(a.B, b.B), 0xff}
}

// fillShape rasterizes the shape build makes, in r's coordinates, and
// fills it with c.
func fillShape(img *image.NRGBA, r image.Rectangle, c color.Color, build func(*shape)) {
	s := &shape{z: vector.NewRasterizer(r.Dx(), r.Dy())}
	s.z.DrawOp = draw.Over
	build(s)
	s.z.Draw(img, r, image.NewUniform(c), image.Point{})
}

func runeList(rs []rune) string {
	seen := map[rune]bool{}
	var out string
	for _, r := range rs {
		if !seen[r] {
			seen[r] = true
			out += fmt.Sprintf(" %q", r)
		}
	}
	return out
}
