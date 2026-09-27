package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

func TestChartEditorStackingAndTrend(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	if _, keys := m.overlay.Status(); !strings.Contains(line(m, contextLine), "Not stacked") || !strings.Contains(keys, "stacking") {
		t.Errorf("no stacking chip: %q / %q", line(m, contextLine), keys)
	}
	press(t, m, "k")
	if c := m.sheet.Charts()[0]; c.Stack != sheet.StackNormal || !strings.Contains(line(m, contextLine), "Stacked") {
		t.Errorf("stack %v: %q", c.Stack, line(m, contextLine))
	}
	press(t, m, "k")
	if c := m.sheet.Charts()[0]; c.Stack != sheet.StackPercent || !strings.Contains(screen(m), "100% ┤") {
		t.Errorf("100%% stacking %v:\n%s", c.Stack, screen(m))
	}
	// Scatter has a trend line instead; K does nothing there.
	press(t, m, "6")
	if !strings.Contains(line(m, contextLine), "Trend line") || strings.Contains(line(m, contextLine), "stacked") {
		t.Errorf("scatter bar: %q", line(m, contextLine))
	}
	press(t, m, "e", "k")
	if c := m.sheet.Charts()[0]; c.Type != sheet.ChartScatter || !c.Trend || c.Stack != sheet.StackPercent {
		t.Errorf("after E and K: %+v", c)
	}
	// Esc undoes every change, removing the new chart.
	press(t, m, "<esc>")
	if n := len(m.sheet.Charts()); n != 0 {
		t.Errorf("%d charts after Esc", n)
	}
}

func TestChartEditorAxes(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "a")
	ctx := line(m, contextLine)
	for _, want := range []string{"Min auto", "Max auto", "Log", "Gridlines", "Legend bottom"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("axis bar lacks %q: %q", want, ctx)
		}
	}
	// The minimum is typed as a cell takes a number.
	press(t, m, "n")
	if m.mode != modePrompt {
		t.Fatalf("minimum prompt not open, mode %v", m.mode)
	}
	m.line.Buf, m.line.Pos = []rune("$500"), 4
	press(t, m, "<enter>")
	if c := m.sheet.Charts()[0]; !c.HasMin || c.Min != 500 || !strings.Contains(line(m, contextLine), "Min 500") {
		t.Errorf("min %v %v: %q", c.HasMin, c.Min, line(m, contextLine))
	}
	// Not a number: asked again, and Esc leaves the axis as it was.
	press(t, m, "x")
	m.line.Buf, m.line.Pos = []rune("lots"), 4
	press(t, m, "<enter>")
	if m.mode != modePrompt || !strings.Contains(line(m, contextLine), "Not a number. Axis maximum") {
		t.Errorf("not asked again: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if c := m.sheet.Charts()[0]; c.HasMax || !strings.Contains(line(m, contextLine), "Max auto") {
		t.Errorf("max %v %v: %q", c.HasMax, c.Max, line(m, contextLine))
	}
	press(t, m, "l", "g", "p")
	c := m.sheet.Charts()[0]
	if !c.Log || !c.NoGrid || c.Legend != sheet.LegendRight {
		t.Errorf("toggles: %+v", c.ChartOptions)
	}
	if !strings.Contains(screen(m), "■ Rent") {
		t.Errorf("right legend missing:\n%s", screen(m))
	}
	press(t, m, "p")
	if c := m.sheet.Charts()[0]; c.Legend != sheet.LegendNone || strings.Contains(screen(m), "■ Rent") {
		t.Errorf("legend none: %v", c.Legend)
	}
	// A blank minimum is automatic again.
	press(t, m, "n")
	m.line.Buf, m.line.Pos = nil, 0
	press(t, m, "<enter>")
	if c := m.sheet.Charts()[0]; c.HasMin {
		t.Errorf("min still set: %v", c.Min)
	}
	// Esc goes back to the main bar, keeping the changes.
	press(t, m, "<esc>")
	if _, ok := m.overlay.(*chartEditor); !ok || !strings.Contains(line(m, contextLine), "Column") {
		t.Fatalf("not back on the main bar: %T %q", m.overlay, line(m, contextLine))
	}
	press(t, m, "<enter>")
	if c := m.sheet.Charts()[0]; !c.Log || c.Legend != sheet.LegendNone {
		t.Errorf("changes lost: %+v", c.ChartOptions)
	}
	// Undo takes back the last change.
	press(t, m, "<esc>", "<ctrl+z>")
	if c := m.sheet.Charts()[0]; !c.HasMin {
		t.Errorf("undo: %+v", c.ChartOptions)
	}
}

func TestChartEditorFitsNarrow(t *testing.T) {
	m := wideModel()
	chartTable(t, m)
	m.runCommand("insert.chart")
	press(t, m, "a")
	m.Update(teaSize(60, 16))
	if ctx := line(m, contextLine); !strings.Contains(ctx, "Legend bottom") {
		t.Errorf("axis bar clipped at 60 columns: %q", ctx)
	}
	if _, keys := m.overlay.Status(); !strings.Contains(keys, "grid") || !strings.Contains(keys, "legend") || ansi.StringWidth(keys) > 60 {
		t.Errorf("axis keys at 60 columns: %q", ansi.Strip(keys))
	}
	press(t, m, "<esc>")
	for _, want := range []string{"◂ Column ▸", "Series in columns"} {
		if ctx := line(m, contextLine); !strings.Contains(ctx, want) {
			t.Errorf("main bar lacks %q at 60 columns: %q", want, ctx)
		}
	}
	// Clicking the one type chip moves on to the next type.
	leftClick(m, 3, contextLine)
	if c := m.sheet.Charts()[0]; c.Type != sheet.ChartBar || !strings.Contains(line(m, contextLine), "◂ Bar ▸") {
		t.Errorf("after a click: %v %q", c.Type, line(m, contextLine))
	}
}
