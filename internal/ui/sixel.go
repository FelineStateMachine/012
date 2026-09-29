package ui

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/chart"
)

// Sixel chart images, for terminals without kitty graphics. A sixel
// image is pixels the terminal draws at the cursor, which Bubble Tea's
// renderer knows nothing of: it keeps drawing cells, and a cell it
// writes over the image punches a hole in it. So:
//
//   - A chart is an image only where the whole of its plot area shows: in
//     the grid, under no chart drawn later and no floating box (a menu,
//     a dialog, formula suggestions, a note). Elsewhere it is text.
//   - Under an image the frame keeps the text chart, which the image
//     covers whole (every pixel is set). The renderer writes those cells
//     only when the chart changes, and then the image is drawn again.
//     DA1 only says a terminal parses sixel: one that lists it without
//     showing images, or a recording of the text alone (VHS records
//     xterm.js's text layer, not its image layer), shows the text chart
//     rather than an empty plot.
//   - Images are drawn after the frame: a sixelDrawMsg comes once the
//     frame has been still for sixelSettle, longer than a frame, and
//     draws each image with the cursor saved and restored around it.
//   - When an image moves, changes or goes (scrolling, resizing, an edit
//     of its data, an overlay opening over it), the screen is cleared and
//     redrawn whole, which erases the images, and they are drawn again.
//     When a row an image crosses changes, the images are drawn again.
//   - Never inside tmux: tmux answers DA1 for itself, and an image passed
//     through to the outer terminal lands where tmux doesn't know, so its
//     redraws would erase it.

// sixelRegistersQuery asks for the number of sixel color registers
// (XTSMGRAPHICS, read).
const sixelRegistersQuery = "\x1b[?1;1;0S"

// parseRegisters parses the XTSMGRAPHICS reply "CSI ? 1 ; 0 ; n S".
func parseRegisters(s string) (int, bool) {
	s, ok := strings.CutPrefix(s, "\x1b[?1;0;")
	if !ok {
		return 0, false
	}
	s, ok = strings.CutSuffix(s, "S")
	n, err := strconv.Atoi(s)
	if !ok || err != nil || n < 2 {
		return 0, false
	}
	return min(n, 256), true
}

// sixelSettle is how long the frame stays still before images are drawn
// over it: more than a frame at 60 fps, so the frame is out first.
const sixelSettle = 25 * time.Millisecond

// sixelState is what the sixel images are and where.
type sixelState struct {
	cache      map[int]sixelImage // by chart
	plan       []sixelPlace       // the images the frame keeps room for
	drawn      []sixelPlace       // the images on screen
	frame      uint64             // the rows of the latest frame the plan crosses, hashed
	drawnFrame uint64             // frame when the images were drawn
	changed    time.Time          // when frame last changed
	pending    bool               // a sixelDrawMsg is on its way
	now        func() time.Time   // the clock; time.Now when nil
	tick       func() tea.Cmd     // what sends sixelDrawMsg; sixelTick when nil
}

// sixelImage is a chart's image encoded, and where its plot area is in
// the chart's contents.
type sixelImage struct {
	key  string
	plot image.Rectangle
	seq  string // "" when there is nothing to plot
}

// sixelPlace is chart's image at screen cells at.
type sixelPlace struct {
	chart int
	at    image.Rectangle
	key   string
}

// sixelDrawMsg draws the images once the frame has settled.
type sixelDrawMsg struct{}

func sixelTick() tea.Cmd {
	return tea.Tick(sixelSettle, func(time.Time) tea.Msg { return sixelDrawMsg{} })
}

// later asks for a sixelDrawMsg.
func (s *sixelState) later() tea.Cmd {
	s.pending = true
	if s.tick != nil {
		return s.tick()
	}
	return sixelTick()
}

func (s *sixelState) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// sixelOn reports whether charts are drawn as sixel images: the
// terminal has sixel and has said its cell size, and chart-images isn't
// off.
func (t *terminal) sixelOn() bool { return t.sixel && !t.noImages && t.cellW > 0 && t.cellH > 0 }

// syncSixel follows msg, after Update has handled it: it encodes the
// charts that changed, works out where images go, clears the screen when
// one drawn moved, changed or went, and asks for the images to be drawn
// once the frame settles.
func (m *Model) syncSixel(msg tea.Msg) tea.Cmd {
	t := &m.term
	if _, ok := msg.(sixelDrawMsg); ok {
		return m.drawSixels()
	}
	var cmds []tea.Cmd
	if _, ok := msg.(tea.WindowSizeMsg); ok && t.sixel {
		cmds = append(cmds, tea.Raw(ansi.WindowOp(16))) // the font may have changed size too
	}
	if !t.sixelOn() {
		t.six.plan, t.six.cache = nil, nil
		if len(t.six.drawn) > 0 {
			t.six.drawn = nil
			cmds = append(cmds, tea.ClearScreen)
		}
		return tea.Batch(cmds...)
	}
	m.encodeSixels()
	t.six.plan = m.sixelPlan()
	if t.six.gone() {
		t.six.drawn = nil
		cmds = append(cmds, tea.ClearScreen)
	}
	if len(t.six.plan) > 0 && !t.six.pending {
		cmds = append(cmds, t.six.later())
	}
	return tea.Batch(cmds...)
}

// gone reports whether an image drawn isn't in the plan as it is.
func (s *sixelState) gone() bool {
	for _, d := range s.drawn {
		if !slices.Contains(s.plan, d) {
			return true
		}
	}
	return false
}

// drawSixels draws the planned images over the settled frame, unless
// they are drawn already over the same rows.
func (m *Model) drawSixels() tea.Cmd {
	s := &m.term.six
	s.pending = false
	if !m.term.sixelOn() || len(s.plan) == 0 || s.frame == s.drawnFrame && slices.Equal(s.drawn, s.plan) {
		return nil
	}
	if s.clock().Sub(s.changed) < sixelSettle {
		return s.later()
	}
	var b strings.Builder
	for _, p := range s.plan {
		b.WriteString(ansi.SaveCursor + ansi.CursorPosition(p.at.Min.X+1, p.at.Min.Y+1))
		b.WriteString(s.cache[p.chart].seq)
		b.WriteString(ansi.RestoreCursor)
	}
	s.drawn, s.drawnFrame = slices.Clone(s.plan), s.frame
	return tea.Raw(b.String())
}

// noteFrame records the rows of the frame content that the planned
// images cross, so images are drawn again when those rows change.
func (s *sixelState) noteFrame(content string) {
	if len(s.plan) == 0 {
		s.frame = 0
		return
	}
	h := fnv.New64a()
	lines := strings.Split(content, "\n")
	for _, p := range s.plan {
		fmt.Fprintf(h, "%d %v %s\n", p.chart, p.at, p.key)
		for y := p.at.Min.Y; y < p.at.Max.Y && y < len(lines); y++ {
			h.Write([]byte(lines[y]))
		}
	}
	if f := h.Sum64(); f != s.frame {
		s.frame, s.changed = f, s.clock()
	}
}

// encodeSixels encodes the image of each chart whose data, size, colors
// or cell size changed.
func (m *Model) encodeSixels() {
	t := &m.term
	if t.six.cache == nil {
		t.six.cache = map[int]sixelImage{}
	}
	o := t.chartOptions()
	o.Image = true
	pal, bg := t.chartPalette(&m.th), m.sixelBackground()
	charts := m.displayCharts()
	for i, c := range charts {
		if i >= maxImages {
			break
		}
		w, h := chartInner(c)
		d := m.sheet.ChartData(c)
		o.Chart = c.ChartOptions
		key := fmt.Sprintf("%v %v %d %d %v %v %v %d", c.Type, d, w, h, o, pal, bg, t.registers)
		if t.six.cache[i].key == key {
			continue
		}
		span := m.spans.Start("chart.sixel", slog.String("type", c.Type.String()), slog.Int("w", w), slog.Int("h", h))
		g := chart.Draw(c.Type, d, w, h, o)
		si := sixelImage{key: key, plot: g.Plot}
		if img := chart.Image(c.Type, d, w, h, o, pal); img != nil && !g.Plot.Empty() {
			si.seq = chart.Sixel(img, bg, t.registers)
		}
		span.End(slog.Int("bytes", len(si.seq)))
		t.six.cache[i] = si
	}
	for i := range t.six.cache {
		if i >= len(charts) || i >= maxImages {
			delete(t.six.cache, i)
		}
	}
}

// sixelBackground is the color images are composited over: the theme's
// background, or the terminal's, or black.
func (m *Model) sixelBackground() color.RGBA {
	c := m.term.bg
	if m.th.Palette != nil && m.th.Palette.Background != nil {
		c = m.th.Palette.Background
	}
	if c == nil {
		return color.RGBA{0, 0, 0, 255}
	}
	r, g, b, _ := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

// sixelPlan is where images go: the charts whose plot area shows whole
// in the grid, under no chart drawn later and no floating box.
func (m *Model) sixelPlan() []sixelPlace {
	var over []image.Rectangle
	for _, b := range m.floating() {
		if len(b.Lines) > 0 {
			over = append(over, image.Rect(b.X, b.Y, b.X+b.Width(), b.Y+b.Height()))
		}
	}
	charts := m.displayCharts()
	boxes := make([]image.Rectangle, len(charts))
	for i, c := range charts {
		x, y := m.chartScreen(c)
		boxes[i] = image.Rect(x, y, x+c.W, y+c.H)
	}
	grid := image.Rect(m.hdrW(), gridTop, m.width, gridTop+m.visibleRows())
	var plan []sixelPlace
	for i := range charts {
		img, ok := m.term.six.cache[i]
		if !ok || img.seq == "" {
			continue
		}
		at := img.plot.Add(boxes[i].Min.Add(image.Pt(2, 1))) // the border and a column of padding
		if !at.In(grid) || overlapsAny(at, over) || overlapsAny(at, boxes[i+1:]) {
			continue
		}
		plan = append(plan, sixelPlace{i, at, img.key})
	}
	return plan
}

func overlapsAny(r image.Rectangle, rs []image.Rectangle) bool {
	return slices.ContainsFunc(rs, r.Overlaps)
}
