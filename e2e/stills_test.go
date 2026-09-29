package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Stills are pictures of 012 for the docs, drawn from golden screens:
// each docScreen is recorded again from another screen's setup, on a
// terminal the size a page shows well, dark and light, and drawn in its
// reference palette to docs/media/<name>-dark.png and <name>-light.png.
// `make screens` records and draws them; `make e2e` fails when a
// still wasn't drawn from its golden as it is.
type docScreen struct {
	name, from string // docs/media/<name>-*.png; the screen whose setup it records
	cols, rows uint16
}

var docScreens = []docScreen{
	{name: "trace-view", from: "trace-view", cols: 80, rows: 14},
	{name: "evaluate", from: "evaluate", cols: 80, rows: 12},
	{name: "rules-bars", from: "rules-bars", cols: 80, rows: 12},
	{name: "rules-panel", from: "rules-panel", cols: 100, rows: 12},
	{name: "table", from: "table", cols: 80, rows: 13},
	{name: "layout", from: "layout", cols: 80, rows: 28},
	{name: "palette", from: "palette", cols: 80, rows: 18},
}

// stillsVersion changes when the drawing does, so every still is
// drawn again.
const stillsVersion = "1"

// addDocScreens adds the docs' screens, docs-<name> and
// docs-<name>-light, to the golden screens once every screen they're
// recorded from is listed.
func addDocScreens() {
	byName := map[string]screen{}
	for _, sc := range screens {
		byName[sc.name] = sc
	}
	for _, d := range docScreens {
		from, ok := byName[d.from]
		if !ok {
			panic("docScreens: no golden screen " + d.from)
		}
		sc := from
		sc.name, sc.opts.cols, sc.opts.rows = "docs-"+d.name, d.cols, d.rows
		light := sc
		light.name += "-light"
		light.opts.light = true
		screens = append(screens, sc, light)
	}
}

func TestStills(t *testing.T) {
	faces, err := loadFaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docScreens {
		for _, v := range []struct {
			suffix string
			p      stillPalette
		}{{"dark", darkStill}, {"light", lightStill}} {
			screen := "docs-" + d.name
			if v.suffix == "light" {
				screen += "-light"
			}
			t.Run(d.name+"-"+v.suffix, func(t *testing.T) {
				path := filepath.Join("..", "docs", "media", d.name+"-"+v.suffix+".png")
				checkStill(t, screen, int(d.cols), int(d.rows), v.p, faces, path)
			})
		}
	}
}

// checkStill draws the still of screen into path under -update, and
// otherwise checks the one there was drawn from the golden as it is.
func checkStill(t *testing.T, screen string, cols, rows int, p stillPalette, fs *stillFaces, path string) {
	src, err := os.ReadFile(filepath.Join("testdata", "screens", screen+".html"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\n%v\n%s", stillsVersion, p, src)))
	want := hex.EncodeToString(sum[:])
	if !*update {
		if got := pngText(path, "012-golden"); got != want {
			t.Errorf("%s is not drawn from the golden %s as it is; run make screens", path, screen)
		}
		return
	}
	grid, err := parseScreen(string(src), cols, rows)
	if err != nil {
		t.Fatalf("%s: %v", screen, err)
	}
	img, missing := drawStill(grid, p, fs)
	if len(missing) > 0 {
		t.Errorf("%s: no glyph for%s", screen, runeList(missing))
	}
	if err := writePNG(path, img, "012-golden", want); err != nil {
		t.Fatal(err)
	}
}

// writePNG writes img with a text chunk key=value after its header, so
// the check can tell which golden it was drawn from without drawing it
// again (drawing isn't bit for bit the same on every CPU).
func writePNG(path string, img image.Image, key, value string) error {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return err
	}
	b := buf.Bytes()
	const header = 8 + 8 + 13 + 4 // signature, then IHDR's length, type, data and CRC
	data := []byte(key + "\x00" + value)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	chunk = append(chunk, "tEXt"...)
	chunk = append(chunk, data...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	out := append(append(append([]byte{}, b[:header]...), chunk...), b[header:]...)
	return os.WriteFile(path, out, 0o644)
}

// pngText is the value of the PNG's text chunk named key, or "".
func pngText(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 8 {
		return ""
	}
	for b = b[8:]; len(b) >= 12; {
		n := int(binary.BigEndian.Uint32(b))
		if len(b) < 12+n {
			return ""
		}
		if string(b[4:8]) == "tEXt" {
			if k, v, ok := strings.Cut(string(b[8:8+n]), "\x00"); ok && k == key {
				return v
			}
		}
		b = b[12+n:]
	}
	return ""
}
