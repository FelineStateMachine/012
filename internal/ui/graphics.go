package ui

import (
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/chart"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// terminal is what 012 knows about the terminal it runs in, learned from
// replies to queries sent at startup. It survives File New and Open.
//
// Charts become images when the terminal answers a kitty graphics query
// (kitty, Ghostty, WezTerm and others); everywhere else they stay text.
// Images use the terminal's own palette, asked for with OSC 4 once
// graphics are known to work, so bars match the text legend.
type terminal struct {
	kitty        bool // answers kitty graphics queries
	tmux         bool // inside tmux: sequences for the outer terminal need passthrough
	cellW, cellH int  // cell size in pixels, 0 until reported
	palette      map[int]color.RGBA
	sent         map[int]string // image id -> what was sent, to send only changes
	blurred      bool           // the terminal window doesn't have focus
}

func newTerminal() terminal {
	return terminal{tmux: os.Getenv("TMUX") != "", palette: map[int]color.RGBA{}, sent: map[int]string{}}
}

// wrap prepares a sequence for the outer terminal.
func (t *terminal) wrap(seq string) string {
	if t.tmux {
		return ansi.TmuxPassthrough(seq)
	}
	return seq
}

// probes are the startup queries: kitty graphics support, and live
// light and dark changes (mode 2031).
func (m *Model) probes() tea.Cmd {
	return tea.Raw(m.term.wrap(chart.Query()) + ansi.SetModeLightDark)
}

// firstImageID is the image id of the first chart. Placeholders name
// their image by its 256-color index, and indexes below 16 would be sent
// as basic colors, which don't carry an id.
const firstImageID = 16

// maxImages caps how many charts become images; the rest stay text.
const maxImages = 256 - firstImageID

// handleTerminal handles replies to queries and terminal events.
func (m *Model) handleTerminal(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID == chart.QueryID && string(msg.Payload) == "OK" && !m.term.kitty {
			m.term.kitty = true
			// Ask for the cell size (for image proportions) and the
			// palette colors charts draw with.
			var q strings.Builder
			q.WriteString(ansi.WindowOp(ansi.RequestCellSizeWinOp))
			for i := range 16 {
				fmt.Fprintf(&q, "\x1b]4;%d;?\x07", i)
			}
			return tea.Raw(q.String())
		}
	case uv.CellSizeEvent:
		if msg.Width > 0 && msg.Height > 0 {
			m.term.cellW, m.term.cellH = msg.Width, msg.Height
		}
	case uv.UnknownOscEvent:
		if i, c, ok := parsePaletteReply(string(msg)); ok {
			m.term.palette[i] = c
		}
	case uv.DarkColorSchemeEvent, uv.LightColorSchemeEvent:
		// The system switched between light and dark. The terminal's
		// background decides the theme, so ask for it again.
		return tea.RequestBackgroundColor
	case tea.FocusMsg:
		m.term.blurred = false
	case tea.BlurMsg:
		m.term.blurred = true
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

// chartPalette is the theme's series colors as the terminal draws them.
func (m *Model) chartPalette() chart.Palette {
	var p chart.Palette
	for i, idx := range m.th.seriesANSI {
		p.Series[i] = m.term.color(idx)
	}
	p.Grid = m.term.color(8) // bright black, like the text axes
	p.Grid.A = 90
	return p
}

// chartOptions are the drawing options for charts on this terminal.
func (m *Model) chartOptions() chart.Options {
	return chart.Options{Image: m.term.kitty, CellW: m.term.cellW, CellH: m.term.cellH}
}

// syncImages sends the images of charts that changed since they were last
// sent, and frees those of deleted charts. Update calls it after every
// message, so images follow edits, recalculation, resizing and the theme.
func (m *Model) syncImages() tea.Cmd {
	if !m.term.kitty {
		return nil
	}
	var out strings.Builder
	charts := m.displayCharts()
	o := m.chartOptions()
	pal := m.chartPalette()
	live := map[int]bool{}
	for i, c := range charts {
		if i >= maxImages {
			break
		}
		id := firstImageID + i
		live[id] = true
		w, h := chartInner(c)
		d := m.sheet.ChartData(c)
		key := fmt.Sprintf("%v %v %d %d %v %v", c.Type, d, w, h, o, pal)
		prev, resent := m.term.sent[id]
		if prev == key {
			continue
		}
		m.term.sent[id] = key
		if resent {
			// Free the old image and its placement first: Ghostty keeps
			// every virtual placement of an id and sizes the image by the
			// oldest, so a chart that grew would draw at its old size.
			out.WriteString(chart.Delete(id, m.term.wrap))
		}
		span := telemetry.Start("chart.image", slog.String("type", c.Type.String()), slog.Int("w", w), slog.Int("h", h))
		g := chart.Draw(c.Type, d, w, h, o)
		img := chart.Image(c.Type, d, w, h, o, pal)
		if img == nil || g.Plot.Empty() {
			span.End()
			out.WriteString(chart.Delete(id, m.term.wrap))
			continue
		}
		sent := chart.Transmit(id, img, g.Plot.Dx(), g.Plot.Dy(), m.term.wrap)
		span.End(slog.Int("bytes", len(sent)))
		out.WriteString(sent)
	}
	for id := range m.term.sent {
		if !live[id] {
			delete(m.term.sent, id)
			out.WriteString(chart.Delete(id, m.term.wrap))
		}
	}
	if out.Len() == 0 {
		return nil
	}
	return tea.Raw(out.String())
}

// releaseTerminal undoes the startup modes and frees the chart images.
func (m *Model) releaseTerminal() string {
	var b strings.Builder
	b.WriteString(ansi.ResetModeLightDark)
	for id := range m.term.sent {
		b.WriteString(chart.Delete(id, m.term.wrap))
	}
	return b.String()
}

// notifyDone tells the user a long job finished, with a desktop
// notification (OSC 9) when the terminal window isn't focused. When it is,
// the screen already shows the result.
func (m *Model) notifyDone(msg string) tea.Cmd {
	if !m.term.blurred {
		return nil
	}
	msg = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || r >= 0x80 && r < 0xa0 {
			return -1
		}
		return r
	}, msg)
	return tea.Raw(m.term.wrap(ansi.Notify("012: " + msg)))
}
