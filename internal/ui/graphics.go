package ui

import (
	"fmt"
	"image/color"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/chart"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// terminal is what 012 knows about the terminal it runs in, learned from
// replies to queries sent at startup. It survives File New and Open.
//
// Charts become images when the terminal answers a kitty graphics query
// (kitty, Ghostty, WezTerm and others), or else when its primary device
// attributes (DA1) list sixel graphics (foot, xterm, mlterm, Windows
// Terminal; see sixel.go); everywhere else they stay text. Images use
// the terminal's own palette, asked for with OSC 4 at startup, so bars
// match the text legend.
type terminal struct {
	noImages     bool // chart-images = false in the config
	noNotify     bool // notifications = false in the config
	kitty        bool // answers kitty graphics queries
	sixel        bool // lists sixel in DA1, without kitty graphics, outside tmux
	registers    int  // sixel color registers, as XTSMGRAPHICS reports them
	tmux         bool // inside tmux: sequences for the outer terminal need passthrough
	cellW, cellH int  // cell size in pixels, 0 until reported
	palette      map[int]color.RGBA
	bg           color.Color    // the terminal's background, nil until reported
	sent         map[int]string // image id -> what was sent, to send only changes
	six          sixelState     // the sixel images drawn and to draw
	blurred      bool           // the terminal window doesn't have focus
}

// newTerminal starts with what getenv, the terminal's environment, says:
// the process's own locally, the client's when served over SSH.
func newTerminal(getenv func(string) string) terminal {
	return terminal{tmux: getenv("TMUX") != "", palette: map[int]color.RGBA{}, sent: map[int]string{}, registers: 256}
}

// wrap prepares a sequence for the outer terminal.
func (t *terminal) wrap(seq string) string {
	if t.tmux {
		return ansi.TmuxPassthrough(seq)
	}
	return seq
}

// probes are the startup queries: kitty graphics support, live light
// and dark changes (mode 2031), the 16 palette colors (OSC 4), which
// chart images and color scales draw with, the sixel color registers
// (XTSMGRAPHICS) and, last, the primary device attributes. Terminals
// answer in order, so the kitty and XTSMGRAPHICS replies, if any, come
// before DA1's, which then decides between kitty images, sixel and text.
func (t *terminal) probes() tea.Cmd {
	return tea.Raw(t.wrap(chart.Query()) + ansi.SetModeLightDark + paletteQuery() + sixelRegistersQuery + ansi.RequestPrimaryDeviceAttributes)
}

// paletteQuery asks for the 16 ANSI colors; terminals that don't answer
// leave xtermColors in use.
func paletteQuery() string {
	var q strings.Builder
	for i := range 16 {
		fmt.Fprintf(&q, "\x1b]4;%d;?\x07", i)
	}
	return q.String()
}

// firstImageID is the image id of the first chart. Placeholders name
// their image by its 256-color index, and indexes below 16 would be sent
// as basic colors, which don't carry an id.
const firstImageID = 16

// maxImages caps how many charts become images; the rest stay text.
const maxImages = 256 - firstImageID

// handle handles replies to queries and terminal events.
func (t *terminal) handle(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID == chart.QueryID && string(msg.Payload) == "OK" && !t.kitty {
			t.kitty = true
			// Ask for the cell size, for image proportions; the palette
			// was asked for at startup.
			return tea.Raw(ansi.WindowOp(16)) // 16: report the cell size in pixels
		}
	case uv.PrimaryDeviceAttributesEvent:
		if slices.Contains(msg, 4) && !t.kitty && !t.tmux && !t.sixel {
			t.sixel = true
			return tea.Raw(ansi.WindowOp(16)) // sixel images are drawn at the cell size
		}
	case uv.UnknownCsiEvent:
		if n, ok := parseRegisters(string(msg)); ok {
			t.registers = n
		}
	case uv.CellSizeEvent:
		if msg.Width > 0 && msg.Height > 0 {
			t.cellW, t.cellH = msg.Width, msg.Height
		}
	case uv.UnknownOscEvent:
		if i, c, ok := parsePaletteReply(string(msg)); ok {
			t.palette[i] = c
		}
	case uv.DarkColorSchemeEvent, uv.LightColorSchemeEvent:
		// The system switched between light and dark. The terminal's
		// background decides the theme, so ask for it again.
		return tea.RequestBackgroundColor
	case tea.FocusMsg:
		t.blurred = false
	case tea.BlurMsg:
		t.blurred = true
	}
	return nil
}

// parsePaletteReply parses an OSC 4 reply, "ESC ] 4 ; 6 ; rgb:0000/cdcd/cdcd".
func parsePaletteReply(s string) (int, color.RGBA, bool) {
	s = strings.TrimPrefix(s, "\x1b]")
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "\x07"), "\x1b\\"), "\x9c")
	parts := strings.Split(s, ";")
	if len(parts) != 3 || parts[0] != "4" {
		return 0, color.RGBA{}, false
	}
	i, err := strconv.Atoi(parts[1])
	c := ansi.XParseColor(parts[2])
	if err != nil || c == nil || i < 0 || i > 255 {
		return 0, color.RGBA{}, false
	}
	r, g, b, _ := c.RGBA()
	return i, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}, true
}

// xtermColors are the 16 colors assumed when the terminal doesn't report
// its palette.
var xtermColors = [16]color.RGBA{
	{0, 0, 0, 255}, {205, 49, 49, 255}, {13, 188, 121, 255}, {229, 229, 16, 255},
	{36, 114, 200, 255}, {188, 63, 188, 255}, {17, 168, 205, 255}, {229, 229, 229, 255},
	{102, 102, 102, 255}, {241, 76, 76, 255}, {35, 209, 139, 255}, {245, 245, 67, 255},
	{59, 142, 234, 255}, {214, 112, 214, 255}, {41, 184, 219, 255}, {255, 255, 255, 255},
}

func (t *terminal) color(i int) color.RGBA {
	if c, ok := t.palette[i]; ok {
		return c
	}
	return xtermColors[i%16]
}

// chartPalette is the theme's series colors as the terminal draws them,
// or as the theme's color scheme has them.
func (t *terminal) chartPalette(th *theme.Theme) chart.Palette {
	col := t.color
	if th.Palette != nil {
		col = func(i int) color.RGBA {
			r, g, b, _ := th.Palette.ANSI[i%16].RGBA()
			return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
		}
	}
	var p chart.Palette
	for i, idx := range th.SeriesANSI {
		p.Series[i] = col(idx)
	}
	p.Grid = col(8) // bright black, like the text axes
	p.Grid.A = 90
	return p
}

// images reports whether charts are drawn as kitty images.
func (t *terminal) images() bool { return t.kitty && !t.noImages }

// chartOptions are the drawing options for charts on this terminal. For
// sixel, whether a chart is an image depends on where it is: see
// Model.drawChart.
func (t *terminal) chartOptions() chart.Options {
	return chart.Options{Image: t.images(), CellW: t.cellW, CellH: t.cellH}
}

// syncImages sends the images of charts that changed since they were last
// sent, and frees those of deleted charts; shown gives the charts as
// drawn, asked for only on terminals that show images. Update calls it
// after every message, so images follow edits, recalculation, resizing
// and the theme.
func (t *terminal) syncImages(s *sheet.Sheet, shown func() []sheet.Chart, th *theme.Theme, spans *telemetry.Trace) tea.Cmd {
	if !t.images() {
		return t.freeImages()
	}
	var out strings.Builder
	o := t.chartOptions()
	pal := t.chartPalette(th)
	live := map[int]bool{}
	for i, c := range shown() {
		if i >= maxImages {
			break
		}
		id := firstImageID + i
		live[id] = true
		w, h := chartInner(c)
		d := s.ChartData(c)
		o.Chart = c.ChartOptions
		key := fmt.Sprintf("%v %v %d %d %v %v", c.Type, d, w, h, o, pal)
		prev, resent := t.sent[id]
		if prev == key {
			continue
		}
		t.sent[id] = key
		if resent {
			// Free the old image and its placement first: Ghostty keeps
			// every virtual placement of an id and sizes the image by the
			// oldest, so a chart that grew would draw at its old size.
			out.WriteString(chart.Delete(id, t.wrap))
		}
		span := spans.Start("chart.image", slog.String("type", c.Type.String()), slog.Int("w", w), slog.Int("h", h))
		g := chart.Draw(c.Type, d, w, h, o)
		img := chart.Image(c.Type, d, w, h, o, pal)
		if img == nil || g.Plot.Empty() {
			span.End()
			out.WriteString(chart.Delete(id, t.wrap))
			continue
		}
		sent := chart.Transmit(id, img, g.Plot.Dx(), g.Plot.Dy(), t.wrap)
		span.End(slog.Int("bytes", len(sent)))
		out.WriteString(sent)
	}
	for id := range t.sent {
		if !live[id] {
			delete(t.sent, id)
			out.WriteString(chart.Delete(id, t.wrap))
		}
	}
	if out.Len() == 0 {
		return nil
	}
	return tea.Raw(out.String())
}

// freeImages frees every image sent, when images are turned off.
func (t *terminal) freeImages() tea.Cmd {
	if len(t.sent) == 0 {
		return nil
	}
	var b strings.Builder
	for id := range t.sent {
		b.WriteString(chart.Delete(id, t.wrap))
		delete(t.sent, id)
	}
	return tea.Raw(b.String())
}

// release undoes the startup modes and frees the chart images.
func (t *terminal) release() string {
	var b strings.Builder
	b.WriteString(ansi.ResetModeLightDark)
	for id := range t.sent {
		b.WriteString(chart.Delete(id, t.wrap))
	}
	return b.String()
}

// notify tells the user a long job finished, with a desktop
// notification (OSC 9) when the terminal window isn't focused. When it is,
// the screen already shows the result.
func (t *terminal) notify(msg string) tea.Cmd {
	if !t.blurred || t.noNotify {
		return nil
	}
	msg = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || r >= 0x80 && r < 0xa0 {
			return -1
		}
		return r
	}, msg)
	return tea.Raw(t.wrap(ansi.Notify("012: " + msg)))
}
