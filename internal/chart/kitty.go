package chart

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// Images reach the terminal through the kitty graphics protocol with
// Unicode placeholders: the image is sent once with a virtual placement,
// and the view draws placeholder characters where it should appear. The
// placeholders are ordinary text to Bubble Tea's renderer, so redraws never
// erase the image, and they survive tmux.

// Transmit returns the sequences that send img as image id, with a virtual
// placement of cols by rows cells for placeholders to show. Sending an id
// again replaces the image. wrap, if not nil, wraps each chunk, e.g. for
// tmux passthrough.
func Transmit(id int, img *image.RGBA, cols, rows int, wrap func(string) string) string {
	b := img.Bounds()
	// Straight (not premultiplied) RGBA, as the protocol expects.
	raw := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			r, g, bl, a := img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]
			if a > 0 && a < 255 {
				un := func(c uint8) uint8 { return uint8(min(int(c)*255/int(a), 255)) }
				r, g, bl = un(r), un(g), un(bl)
			}
			raw = append(raw, r, g, bl, a)
		}
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(raw)
	zw.Close()
	payload := base64.StdEncoding.EncodeToString(z.Bytes())

	if wrap == nil {
		wrap = func(s string) string { return s }
	}
	first := fmt.Sprintf("a=T,q=2,f=%d,o=z,s=%d,v=%d,i=%d,U=1,c=%d,r=%d", kitty.RGBA, b.Dx(), b.Dy(), id, cols, rows)
	var out strings.Builder
	for i := 0; i < len(payload) || i == 0; i += kitty.MaxChunkSize {
		chunk := payload[i:min(i+kitty.MaxChunkSize, len(payload))]
		more := "m=0"
		if i+kitty.MaxChunkSize < len(payload) {
			more = "m=1"
		}
		opts := "q=2," + more
		if i == 0 {
			opts = first + "," + more
		}
		out.WriteString(wrap(ansi.KittyGraphics([]byte(chunk), opts)))
	}
	return out.String()
}

// Delete returns the sequence that frees image id and its placements.
func Delete(id int, wrap func(string) string) string {
	s := ansi.KittyGraphics(nil, fmt.Sprintf("a=d,d=I,i=%d,q=2", id))
	if wrap != nil {
		return wrap(s)
	}
	return s
}

// Query is the probe for kitty graphics support: terminals that have it
// answer with an OK for image QueryID; others stay silent.
const QueryID = 31

// Query returns the probe.
func Query() string {
	return ansi.KittyGraphics([]byte("AAAA"), fmt.Sprintf("i=%d,s=1,v=1,a=q,t=d,f=24", QueryID))
}

// Placeholder returns the text of the cell at row, col of an image's
// placement. The foreground color must name the image: the 256-color
// index of its id.
func Placeholder(row, col int) string {
	return string([]rune{kitty.Placeholder, kitty.Diacritic(row), kitty.Diacritic(col)})
}
