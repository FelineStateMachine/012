package fileio

import (
	"embed"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/FelineStateMachine/012/internal/chart"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// HTML pages are drawn in 012's look: the 16 ANSI colors of the
// reference palettes the golden screens use (e2e/palette_test.go),
// mapped to the roles the terminal theme gives them
// (internal/ui/theme), dark or light as the reader's system prefers,
// in IBM Plex Mono, the docs site's font, embedded so the page needs
// nothing else. One cell is htmlCellW by htmlCellH pixels, the size of
// a cell of an SVG chart, so charts sit on the grid as on the screen.

const (
	htmlCellW = chart.SVGCellW
	htmlCellH = chart.SVGCellH
)

//go:embed fonts/*.woff2
var fonts embed.FS

// htmlPalette is a reference palette and the roles drawn in it.
type htmlPalette struct {
	ansi   [16]uint32
	bg, fg uint32
	grid   uint32 // the faint lines between cells
	// ANSI slots of the roles.
	headerBg, headerFg, bar, muted, border, note, link int
	series                                             [chart.Colors]int
}

var htmlPalettes = [2]htmlPalette{{
	ansi: [16]uint32{0x1d1f21, 0xcc6666, 0xb5bd68, 0xf0c674, 0x81a2be, 0xb294bb, 0x8abeb7, 0xc5c8c6,
		0x666666, 0xd54e53, 0xb9ca4a, 0xe7c547, 0x7aa6da, 0xc397d8, 0x70c0b1, 0xeaeaea},
	bg: 0x1d1f21, fg: 0xc5c8c6, grid: 0x2c2e30,
	headerBg: 8, headerFg: 15, bar: 6, muted: 8, border: 7, note: 3, link: 12,
	series: [chart.Colors]int{6, 5, 3, 2, 12, 9},
}, {
	ansi: [16]uint32{0x1d1f21, 0xc82829, 0x718c00, 0xb58900, 0x4271ae, 0x8959a8, 0x3e999f, 0xd6d6d6,
		0x8e908c, 0xc82829, 0x718c00, 0xb58900, 0x4271ae, 0x8959a8, 0x3e999f, 0xffffff},
	bg: 0xfafafa, fg: 0x1d1f21, grid: 0xe8e8e8,
	headerBg: 7, headerFg: 0, bar: 4, muted: 0, border: 0, note: 5, link: 4,
	series: [chart.Colors]int{4, 5, 2, 1, 6, 8},
}}

// ruleSlots are the ANSI slots of sheet.Color, as the theme maps them.
var ruleSlots = [sheet.NumColors]int{0, 1, 3, 2, 6, 4, 5}

// vars writes the palette's CSS variables.
func (p htmlPalette) vars() string {
	var b strings.Builder
	hex := func(v uint32) string { return fmt.Sprintf("#%06x", v) }
	slot := func(i int) string { return hex(p.ansi[i]) }
	fmt.Fprintf(&b, "--bg:%s;--fg:%s;--grid:%s;--hbg:%s;--hfg:%s;--bar:%s;--muted:%s;--border:%s;--note:%s;--link:%s;--axis:%s;--acc:%s;--accfg:%s;",
		hex(p.bg), hex(p.fg), hex(p.grid), slot(p.headerBg), slot(p.headerFg), slot(p.bar), slot(p.muted),
		slot(p.border), slot(p.note), slot(p.link), slot(8), slot(6), slot(0))
	for i, s := range p.series {
		fmt.Fprintf(&b, "--s%d:%s;", i, slot(s))
	}
	for c := 1; c < sheet.NumColors; c++ {
		fill := p.ansi[ruleSlots[c]]
		fmt.Fprintf(&b, "--r%d:%s;--i%d:%s;", c, hex(fill), c, hex(p.ink(fill)))
	}
	return b.String()
}

// ink is black or bright white, whichever reads better on bg.
func (p htmlPalette) ink(bg uint32) uint32 {
	if contrast(p.ansi[0], bg) >= contrast(p.ansi[15], bg) {
		return p.ansi[0]
	}
	return p.ansi[15]
}

// contrast is the WCAG contrast ratio of two colors.
func contrast(a, b uint32) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

func luminance(c uint32) float64 {
	ch := func(v uint32) float64 {
		s := float64(v&0xff) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c>>16) + 0.7152*ch(c>>8) + 0.0722*ch(c)
}

// htmlCSS is the style sheet every page and view shares, fonts
// included.
var htmlCSS = sync.OnceValue(func() string {
	var b strings.Builder
	for _, f := range []struct{ file, weight, style string }{
		{"ibm-plex-mono-latin-400-normal.woff2", "400", "normal"},
		{"ibm-plex-mono-latin-600-normal.woff2", "600", "normal"},
		{"ibm-plex-mono-latin-400-italic.woff2", "400", "italic"},
	} {
		data, _ := fonts.ReadFile("fonts/" + f.file)
		fmt.Fprintf(&b, "@font-face{font-family:'IBM Plex Mono';font-weight:%s;font-style:%s;src:url(data:font/woff2;base64,%s) format('woff2')}\n",
			f.weight, f.style, base64.StdEncoding.EncodeToString(data))
	}
	dark, light := htmlPalettes[0].vars(), htmlPalettes[1].vars()
	fmt.Fprintf(&b, ":root{%s}\n@media (prefers-color-scheme:light){:root:not(.dark){%s}}\n:root.light{%s}\n", dark, light, light)
	b.WriteString(htmlRules)
	return b.String()
})

// HTMLStyle is the style sheet of 012's HTML pages, for a page built
// around HTMLGrid or HTMLChart; the root element takes the class dark
// or light to choose a palette over the reader's preference.
func HTMLStyle() string { return htmlCSS() }

// htmlRules are the page's rules, after its fonts and palettes.
const htmlRules = `*{box-sizing:border-box}
html,body{margin:0;background:var(--bg);color:var(--fg)}
body{font:15px/21px 'IBM Plex Mono',ui-monospace,'SF Mono',Menlo,Consolas,monospace;font-variant-ligatures:none}
.o12{min-width:100%;width:max-content}
.panel{position:sticky;left:0;top:0;z-index:3;background:var(--bg);white-space:pre;width:100vw;max-width:100%}
.panel>div{height:21px;display:flex;gap:9px;overflow:hidden}
.panel .book{flex:1;overflow:hidden;text-overflow:ellipsis}
.mode{background:var(--acc);color:var(--accfg);font-weight:600;padding:0 9px}
.namebox{background:var(--hbg);color:var(--hfg);min-width:90px;padding:0 9px}
.input{flex:1;overflow:hidden;text-overflow:ellipsis}
.context{color:var(--muted)}
.sheet{position:relative}
table.grid{border-collapse:collapse;table-layout:fixed;width:max-content}
.grid th{background:var(--hbg);color:var(--hfg);font-weight:400;height:21px;padding:0;text-align:center;position:sticky;z-index:1}
.grid thead th{top:63px}
.grid tbody th{left:0;text-align:right;padding-right:9px;vertical-align:bottom}
.grid thead th.corner{left:0;z-index:2}
.grid th.on{background:var(--acc);color:var(--accfg);font-weight:600}
.grid td{height:21px;padding:0 4px;white-space:pre;overflow:visible;box-shadow:inset -1px -1px 0 var(--grid);vertical-align:bottom;position:relative}
.grid td.clip{overflow:hidden}
.grid td.wrap{white-space:pre-wrap;overflow-wrap:anywhere}
.grid td.r{text-align:right}.grid td.c{text-align:center}.grid td.l{text-align:left}
.grid td.t{vertical-align:top}.grid td.m{vertical-align:middle}
.grid td.b{font-weight:600}.grid td.i{font-style:italic}
.grid td.u{text-decoration:underline}.grid td.s{text-decoration:line-through}.grid td.u.s{text-decoration:underline line-through}
.grid td.err{text-decoration:underline wavy var(--r1)}
.grid td.bad{text-decoration:underline dotted var(--r2)}
.grid td.th{font-weight:600;text-decoration:underline;color:var(--bar)}
.grid td.band{background:var(--hbg);color:var(--hfg)}
.grid td.note::after{content:'\259D';position:absolute;right:0;top:-3px;color:var(--note);font-size:12px}
.grid td.ptr{outline:2px solid var(--acc);outline-offset:-2px}
.grid a{color:var(--link)}
.grid .icon{margin-right:9px}
figure.chart{position:absolute;margin:0;border:1px solid var(--axis);background:var(--bg);padding:20px 17px;z-index:1}
figure.chart figcaption{position:absolute;top:-11px;left:9px;background:var(--bg);padding:0 9px;font-weight:600;white-space:pre}
figure.chart .range{position:absolute;bottom:-11px;right:9px;background:var(--bg);padding:0 9px;color:var(--muted);white-space:pre}
figure.solo{position:relative;display:inline-block;margin:21px 18px}
svg.chart{display:block;overflow:visible;font:15px 'IBM Plex Mono',ui-monospace,monospace}
.ct-ax{fill:var(--axis);color:var(--axis)}.ct-lb{fill:var(--muted);color:var(--muted)}.ct-mu{fill:var(--muted);color:var(--muted)}
.cs-box{stroke:currentColor;stroke-width:1}.cs-gl{stroke:var(--grid);stroke-width:1}.cs-gap{stroke:var(--bg);stroke-width:1.5}
.ct-s0,.cf-0{fill:var(--s0)}.ct-s1,.cf-1{fill:var(--s1)}.ct-s2,.cf-2{fill:var(--s2)}.ct-s3,.cf-3{fill:var(--s3)}.ct-s4,.cf-4{fill:var(--s4)}.ct-s5,.cf-5{fill:var(--s5)}
.cs-0{stroke:var(--s0)}.cs-1{stroke:var(--s1)}.cs-2{stroke:var(--s2)}.cs-3{stroke:var(--s3)}.cs-4{stroke:var(--s4)}.cs-5{stroke:var(--s5)}
.status{white-space:pre;color:var(--muted);padding:0 9px;height:21px}
`
