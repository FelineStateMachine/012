package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// chartTable types a small table and leaves A1 active.
func chartTable(t *testing.T, m *Model) {
	t.Helper()
	press(t, m, "Month", "<tab>", "Rent", "<tab>", "Food", "<enter>")
	press(t, m, "Jan", "<tab>", "1450", "<tab>", "600", "<enter>")
	press(t, m, "Feb", "<tab>", "1450", "<tab>", "640", "<enter>")
	press(t, m, "Mar", "<tab>", "1500", "<tab>", "612", "<enter>")
	press(t, m, "<ctrl+home>")
}

func wideModel() *Model {
	m := New(sheet.New(), "")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func TestInsertChart(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	send(m, nil)
	m.runCommand("insert.chart")
	charts := m.sheet.Charts()
	if len(charts) != 1 {
		t.Fatalf("%d charts", len(charts))
	}
	c := charts[0]
	if c.Data.String() != "A1:C4" || !c.Header || !c.Labels || c.At.String() != "D1" || c.Title != "Rent and Food" {
		t.Errorf("chart %+v", c)
	}
	if m.indicator() != "CHART" || !strings.Contains(line(m, contextLine), "Column") {
		t.Errorf("editor not open: %q / %q", m.indicator(), line(m, contextLine))
	}
	s := screen(m)
	for _, want := range []string{"Rent and Food", "1,500 ┤", "■ Rent", "A1:C4"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q:\n%s", want, s)
		}
	}
	// The editor changes the chart live; Enter keeps it selected.
	press(t, m, "<right>", "s")
	if c := m.sheet.Charts()[0]; c.Type != sheet.ChartBar || !c.ByRow {
		t.Errorf("after editing: %+v", c)
	}
	press(t, m, "<enter>")
	if _, ok := m.overlay.(*chartSel); !ok || !strings.Contains(line(m, contextLine), "Bar chart of A1:C4") {
		t.Errorf("not selected after Enter: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if m.mode != modeReady {
		t.Errorf("mode %v", m.mode)
	}
}

func TestInsertChartEscCancels(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "<right>", "<esc>")
	if n := len(m.sheet.Charts()); n != 0 || m.changed != true {
		// The table itself is unsaved; the chart is gone.
		t.Errorf("%d charts after Esc", n)
	}
	// From one blank cell far from data there's nothing to chart.
	press(t, m, "<f5>", "H20", "<enter>")
	m.runCommand("insert.chart")
	if len(m.sheet.Charts()) != 0 || !strings.Contains(line(m, contextLine), "Select the data") {
		t.Errorf("charted nothing: %q", line(m, contextLine))
	}
}

func TestChartEditorPrompts(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "t", "<ctrl+a>")
	if m.mode != modePrompt {
		t.Fatalf("title prompt not open, mode %v", m.mode)
	}
	m.buf, m.bufPos = []rune("Spend"), 5
	press(t, m, "<enter>")
	if c := m.sheet.Charts()[0]; c.Title != "Spend" || m.indicator() != "CHART" {
		t.Errorf("title %q, indicator %q", c.Title, m.indicator())
	}
	press(t, m, "r")
	if !m.pointing() {
		t.Fatal("range prompt not pointing")
	}
	press(t, m, "<shift+up>", "<enter>")
	if c := m.sheet.Charts()[0]; c.Data.String() != "A1:C3" {
		t.Errorf("data %s", c.Data)
	}
	press(t, m, "r", "<esc>", "<esc>")
	if _, ok := m.overlay.(*chartEditor); !ok {
		t.Errorf("Esc in the range prompt left the editor: %T", m.overlay)
	}
}

func TestChartMoveResizeDelete(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "<enter>")
	press(t, m, "<down>", "<right>", "<shift+right>", "<shift+down>")
	c := m.sheet.Charts()[0]
	if c.At.String() != "E2" || c.W != newChartW+2 || c.H != newChartH+1 {
		t.Errorf("after keys: %s %dx%d", c.At, c.W, c.H)
	}
	// Mouse: drag the body one column left, then the corner.
	x, y := m.chartScreen(c)
	send(m, tea.MouseClickMsg{X: x + 5, Y: y + 2, Button: tea.MouseLeft})
	send(m, tea.MouseMotionMsg{X: x + 5 - 10, Y: y + 3, Button: tea.MouseLeft})
	if got := m.sheet.Charts()[0].At; got.String() != "E2" {
		t.Errorf("moved before release: %s", got)
	}
	send(m, tea.MouseReleaseMsg{X: x - 5, Y: y + 3, Button: tea.MouseLeft})
	c = m.sheet.Charts()[0]
	if c.At.String() != "D3" {
		t.Errorf("after drag: %s", c.At)
	}
	x, y = m.chartScreen(c)
	send(m, tea.MouseClickMsg{X: x + c.W - 1, Y: y + c.H - 1, Button: tea.MouseLeft})
	send(m, tea.MouseMotionMsg{X: x + 29, Y: y + 11, Button: tea.MouseLeft})
	send(m, tea.MouseReleaseMsg{X: x + 29, Y: y + 11, Button: tea.MouseLeft})
	if c = m.sheet.Charts()[0]; c.W != 30 || c.H != 12 {
		t.Errorf("after resize: %dx%d", c.W, c.H)
	}
	press(t, m, "<ctrl+z>")
	if c = m.sheet.Charts()[0]; c.W == 30 {
		t.Error("undo didn't restore the size")
	}
	// Click the chart to select it, Del deletes it; undo brings it back.
	press(t, m, "<esc>")
	x, y = m.chartScreen(c)
	send(m, tea.MouseClickMsg{X: x + 3, Y: y + 3, Button: tea.MouseLeft})
	send(m, tea.MouseReleaseMsg{X: x + 3, Y: y + 3, Button: tea.MouseLeft})
	press(t, m, "<delete>")
	if len(m.sheet.Charts()) != 0 || m.mode != modeReady {
		t.Fatalf("not deleted: %d, mode %v", len(m.sheet.Charts()), m.mode)
	}
	press(t, m, "<ctrl+z>")
	if len(m.sheet.Charts()) != 1 {
		t.Error("undo didn't restore the chart")
	}
	// Clicking a cell outside a selected chart deselects it and lands there.
	m.runCommand("chart.select")
	send(m, tea.MouseClickMsg{X: cellX(0), Y: gridTop + 8, Button: tea.MouseLeft})
	if m.mode != modeReady || m.cur.String() != "A9" {
		t.Errorf("mode %v at %s", m.mode, m.cur)
	}
}

func TestChartFollowsData(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "<enter>", "<esc>")
	if !strings.Contains(screen(m), "1,500 ┤") {
		t.Fatalf("no axis:\n%s", screen(m))
	}
	press(t, m, "<f5>", "B4", "<enter>", "3000", "<enter>")
	if !strings.Contains(screen(m), "3,000 ┤") {
		t.Errorf("chart didn't follow the edit:\n%s", screen(m))
	}
}

func TestChartImages(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	var raw []string
	collect := func(cmd tea.Cmd) {
		for _, msg := range flatten(cmd) {
			if r, ok := msg.(tea.RawMsg); ok {
				raw = append(raw, r.Msg.(string))
			}
		}
	}
	// No kitty reply: text, and nothing sent.
	m.runCommand("insert.chart")
	press(t, m, "<enter>", "<esc>")
	if cmd := m.syncImages(); cmd != nil {
		t.Error("images sent without kitty graphics")
	}
	// The terminal answers the query: images, and placeholders in the view.
	_, cmd := m.Update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 31}, Payload: []byte("OK")})
	collect(cmd)
	if !m.term.kitty || len(raw) == 0 || !strings.Contains(strings.Join(raw, ""), "\x1b_Ga=T,q=2,f=32,o=z,") {
		t.Fatalf("no image sent: %q", raw)
	}
	if !strings.Contains(m.View().Content, "\x1b[38;5;16m\U0010EEEE\u0305\u0305") {
		t.Error("no placeholders for image 16")
	}
	// Unchanged charts aren't sent again; edits are.
	raw = nil
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	collect(cmd)
	if len(raw) != 0 {
		t.Errorf("unchanged chart sent again")
	}
	press(t, m, "<f5>", "B2", "<enter>", "99", "<enter>")
	_, cmd = m.Update(nil)
	raw = nil
	m.term.sent[firstImageID] = "stale"
	collect(m.syncImages())
	if len(raw) != 1 {
		t.Errorf("changed chart not sent")
	}
	// The old image and its placement are freed before the new one is
	// sent, so the terminal sizes it by the new placement, not the first.
	del, send := strings.Index(raw[0], "a=d,d=I,i=16"), strings.Index(raw[0], "a=T,")
	if del < 0 || send < del {
		t.Errorf("resent image doesn't free the old placement first: %q", raw[0][:min(len(raw[0]), 80)])
	}
	// Deleting the chart frees its image.
	raw = nil
	m.sheet.DeleteChart(0)
	collect(m.syncImages())
	if len(raw) != 1 || !strings.Contains(raw[0], "a=d,d=I,i=16") {
		t.Errorf("image not freed: %q", raw)
	}
}

// flatten runs a command and returns the messages of it and any batch.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, flatten(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestPaletteReply(t *testing.T) {
	tests := []struct {
		in string
		i  int
		ok bool
	}{
		{"\x1b]4;6;rgb:0000/cdcd/cdcd\x07", 6, true},
		{"\x1b]4;12;rgb:5c5c/5c5c/ffff\x1b\\", 12, true},
		{"\x1b]10;rgb:0/0/0\x07", 0, false},
		{"\x1b]4;x;rgb:0/0/0\x07", 0, false},
	}
	for _, tt := range tests {
		i, c, ok := parsePaletteReply(tt.in)
		if ok != tt.ok || ok && i != tt.i {
			t.Errorf("%q: %d %v %v", tt.in, i, c, ok)
		}
	}
	m := newModel()
	send(m, uv.UnknownOscEvent("\x1b]4;6;rgb:0000/cdcd/cdcd\x07"))
	if c := m.term.color(6); c.G != 0xcd || c.R != 0 {
		t.Errorf("palette color %v", c)
	}
}
