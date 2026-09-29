package chart

import (
	"image"
	"image/color"
	"slices"
	"strconv"
	"strings"
)

// Sixel is the other way images reach a terminal, for terminals without
// kitty graphics (foot, xterm, mlterm, Windows Terminal and others). A
// sixel image is pixels drawn at the cursor, not text: the UI draws it
// after the frame, over the text chart the cells hold (see the ui
// package's sixel.go), so every pixel is set and the text never shows
// through. The image is sent whole each time, in at most as many colors
// as the terminal has color registers, run-length encoded.

// Sixel returns the DCS sequence that draws img at the cursor, every
// pixel composited over bg, in at most registers colors (at least 2, at
// most 256).
func Sixel(img *image.RGBA, bg color.RGBA, registers int) string {
	registers = min(max(registers, 2), 256)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}
	idx, pal := quantize(img, bg, registers)
	var out strings.Builder
	// P2 = 1: pixels not drawn keep what's under them, which only the
	// empty sixels past the image's last row are. The raster attributes
	// set square pixels and the image's size.
	out.WriteString("\x1bP0;1;0q\"1;1;")
	out.WriteString(strconv.Itoa(w) + ";" + strconv.Itoa(h))
	for i, c := range pal {
		out.WriteString("#" + strconv.Itoa(i) + ";2;" + percent(c.R) + ";" + percent(c.G) + ";" + percent(c.B))
	}
	bands := sixelBands(idx, w, h, len(pal))
	out.WriteString(bands)
	out.WriteString("\x1b\\")
	return out.String()
}

// percent is a color channel as sixel's 0 to 100.
func percent(v uint8) string { return strconv.Itoa((int(v)*100 + 127) / 255) }

// sixelBands encodes the image of register indexes, six
// rows at a time: for each color in the band, its row of sixels,
// run-length encoded, then "$" to go back for the next color, and "-"
// between bands.
func sixelBands(idx []int16, w, h, colors int) string {
	var out strings.Builder
	b := bands{w: w, rows: make([][]byte, colors), band: make([]int, colors)}
	for y0 := 0; y0 < h; y0 += 6 {
		if y0 > 0 {
			out.WriteByte('-')
		}
		b.gather(idx[y0*w:min(y0+6, h)*w], y0)
		for i, c := range b.used {
			if i > 0 {
				out.WriteByte('$')
			}
			out.WriteString("#" + strconv.Itoa(int(c)))
			writeRuns(&out, b.rows[c])
		}
	}
	return out.String()
}

// bands holds the rows of sixels of one band of six pixel rows.
type bands struct {
	w    int
	rows [][]byte // each color's row of sixels, made when first used
	band []int    // the band each color was last used in, plus one
	used []int16  // the colors of the band, in order
}

// gather makes the rows of the band at y0 from its pixels' registers.
func (b *bands) gather(px []int16, y0 int) {
	b.used = b.used[:0]
	for i, c := range px {
		if b.band[c] != y0+1 {
			b.band[c] = y0 + 1
			b.used = append(b.used, c)
			if b.rows[c] == nil {
				b.rows[c] = make([]byte, b.w)
			}
			for x := range b.rows[c] {
				b.rows[c][x] = '?'
			}
		}
		b.rows[c][i%b.w] += 1 << (i / b.w) // each pixel has one color, so each bit is added once
	}
	slices.Sort(b.used)
}

// writeRuns writes a row of sixels, runs of four or more as "!n", and
// leaves out the empty sixels at its end.
func writeRuns(out *strings.Builder, row []byte) {
	end := len(row)
	for end > 0 && row[end-1] == '?' {
		end--
	}
	for i := 0; i < end; {
		j := i + 1
		for j < end && row[j] == row[i] {
			j++
		}
		if n := j - i; n >= 4 {
			out.WriteString("!" + strconv.Itoa(n))
			out.WriteByte(row[i])
		} else {
			for range n {
				out.WriteByte(row[i])
			}
		}
		i = j
	}
}

// quantize maps each pixel of img, composited over bg, to a register,
// and returns the registers' colors.
// Colors are kept exactly while there are at most registers of them;
// past that, their low bits are dropped until they fit, and each
// register is the average of the colors it stands for. Registers are
// numbered in the order their colors first appear, so the same image
// always encodes the same way.
func quantize(img *image.RGBA, bg color.RGBA, registers int) ([]int16, []color.RGBA) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	px := make([]uint32, w*h) // 1<<24 | RGB
	for y := range h {
		for x := range w {
			c := over(img, b.Min.X+x, b.Min.Y+y, bg)
			px[y*w+x] = 1<<24 | uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
		}
	}
	shift := uint(0)
	for ; shift < 8; shift++ {
		if distinct(px, shift, registers) <= registers {
			break
		}
	}
	reg := map[uint32]int16{}
	var sums [][4]int // R, G, B sums and count per register
	idx := make([]int16, w*h)
	last, lastReg := uint32(0), int16(-1) // runs of one color are common: look each up once
	for i, p := range px {
		r := lastReg
		if p != last {
			k := bucket(p, shift)
			var ok bool
			if r, ok = reg[k]; !ok {
				r = int16(len(sums))
				reg[k] = r
				sums = append(sums, [4]int{})
			}
			last, lastReg = p, r
		}
		idx[i] = r
		s := &sums[r]
		s[0] += int(p >> 16 & 0xff)
		s[1] += int(p >> 8 & 0xff)
		s[2] += int(p & 0xff)
		s[3]++
	}
	pal := make([]color.RGBA, len(sums))
	for i, s := range sums {
		pal[i] = color.RGBA{uint8(s[0] / s[3]), uint8(s[1] / s[3]), uint8(s[2] / s[3]), 255}
	}
	return idx, pal
}

// bucket is the key of pixel p with shift low bits of each channel
// dropped.
func bucket(p uint32, shift uint) uint32 {
	m := uint32(0xff>>shift<<shift) * 0x010101
	return p & (1<<24 | m)
}

// distinct counts the buckets of px at shift, stopping past limit.
func distinct(px []uint32, shift uint, limit int) int {
	seen := map[uint32]struct{}{}
	last := uint32(0)
	for _, p := range px {
		if p == last {
			continue
		}
		last = p
		seen[bucket(p, shift)] = struct{}{}
		if len(seen) > limit {
			break
		}
	}
	return len(seen)
}

// over is the pixel at x, y of the premultiplied img composited over
// bg, opaque.
func over(img *image.RGBA, x, y int, bg color.RGBA) color.RGBA {
	i := img.PixOffset(x, y)
	p := img.Pix[i : i+4 : i+4]
	a := int(p[3])
	return color.RGBA{mix(p[0], bg.R, a), mix(p[1], bg.G, a), mix(p[2], bg.B, a), 255}
}

// mix is the premultiplied channel c of coverage a over under.
func mix(c, under uint8, a int) uint8 { return uint8(min(int(c)+int(under)*(255-a)/255, 255)) }
