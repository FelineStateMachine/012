package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// chartEditor is the chart editor: a bar on the context line, in the
// spirit of the find bar, with the type as a row of chips and the options
// as toggles. A second bar (A) holds the value axis, gridlines and
// legend, as Sheets' Customize tab does; see chartaxes.go. Changes show
// at once; Enter keeps them and Esc undoes them, removing a chart that
// was just inserted.
type chartEditor struct {
	m     *Model // the model it acts on
	i     int
	start int // the sheet's state before editing, to undo back to
	isNew bool
	axes  bool // showing the axis options
}

// openChartEditor edits chart i. start is the sheet's state to return to
// on Esc.
func (m *Model) openChartEditor(i int, isNew bool, start int) {
	if i < 0 || i >= len(m.sheet.Charts()) {
		return
	}
	m.charts.last = i
	m.openOverlay(&chartEditor{m: m, i: i, start: start, isNew: isNew})
}

func (e *chartEditor) Indicator() string { return "CHART" }

func (e *chartEditor) Layout() []overlay.Box { return nil }

func (e *chartEditor) chart(m *Model) sheet.Chart { return m.sheet.Charts()[e.i] }

// set applies a change to the chart as its own undo step.
func (e *chartEditor) set(m *Model, fn func(c *sheet.Chart)) {
	c := e.chart(m)
	fn(&c)
	m.sheet.SetChart(e.i, c, "edit chart")
	m.changed = m.sheet.StateID() != m.saved
}

func (e *chartEditor) Key(k tea.KeyPressMsg) tea.Cmd {
	m := e.m
	if e.i >= len(m.sheet.Charts()) {
		m.closeOverlay()
		return nil
	}
	key := strings.ToLower(k.String())
	if e.axes {
		e.axisKey(m, key)
		return nil
	}
	switch key {
	case "enter":
		m.selectChart(e.i)
		if e.isNew {
			m.note = "Inserted a chart of " + e.chart(m).Data.String()
		}
	case "esc":
		for m.sheet.StateID() != e.start && m.sheet.CanUndo() {
			m.sheet.Undo()
		}
		m.changed = m.sheet.StateID() != m.saved
		m.closeOverlay()
		m.charts.last = -1
	case "left", "right", "shift+tab", "tab":
		d := 1
		if key == "left" || key == "shift+tab" {
			d = -1
		}
		e.set(m, func(c *sheet.Chart) {
			n := len(sheet.ChartTypes)
			c.Type = sheet.ChartTypes[((int(c.Type)+d)%n+n)%n]
		})
	case editorAxes:
		e.axes = true
	default:
		e.optionKey(m, key)
	}
	return nil
}

// optionKey handles the keys of the main bar's options: a type's number,
// the toggles, the range and the title.
func (e *chartEditor) optionKey(m *Model, key string) {
	if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(sheet.ChartTypes) {
		e.set(m, func(c *sheet.Chart) { c.Type = sheet.ChartTypes[n-1] })
		return
	}
	switch key {
	case editorSwitch:
		e.set(m, func(c *sheet.Chart) { c.ByRow = !c.ByRow })
	case editorHeader:
		e.set(m, func(c *sheet.Chart) { c.Header = !c.Header })
	case editorLabels:
		e.set(m, func(c *sheet.Chart) { c.Labels = !c.Labels })
	case editorStack:
		if e.chart(m).Type.Stackable() {
			e.set(m, func(c *sheet.Chart) { c.Stack = (c.Stack + 1) % 3 })
		}
	case editorTrend:
		if e.chart(m).Type == sheet.ChartScatter {
			e.set(m, func(c *sheet.Chart) { c.Trend = !c.Trend })
		}
	case editorRange:
		e.editRange(m)
	case editorTitle:
		e.editTitle(m)
	}
}

// reopen returns to the editor after a prompt.
func (e *chartEditor) reopen(m *Model) {
	m.openOverlay(e)
}

// editRange asks for the data range by pointing, starting from the
// current one.
func (e *chartEditor) editRange(m *Model) {
	d := e.chart(m).Data
	m.closeOverlay()
	m.cur, m.ext, m.selecting, m.whole = d.From, d.To, true, wholeNone
	m.openRange("Chart data:", func(m *Model, r sheet.Rect) tea.Cmd {
		m.clearSelection()
		e.set(m, func(c *sheet.Chart) { c.Data = r })
		e.reopen(m)
		return nil
	})
	m.prompt.onCancel = func() {
		m.clearSelection()
		e.reopen(m)
	}
}

func (e *chartEditor) editTitle(m *Model) {
	title := e.chart(m).Title
	m.closeOverlay()
	m.openText("Chart title:", title, func(m *Model, text string) tea.Cmd {
		e.set(m, func(c *sheet.Chart) { c.Title = text })
		e.reopen(m)
		return nil
	})
	m.prompt.onCancel = func() { e.reopen(m) }
}

// editorPart is a piece of the editor bar: a type chip or a toggle.
type editorPart struct {
	text string
	key  string // the key it stands for
}

// chip is a part drawn as a toggle, highlighted when on.
func (e *chartEditor) chip(on bool, name, key string) editorPart {
	style := e.m.th.Muted
	if on {
		style = e.m.th.MenuSelected
	}
	return editorPart{text: style.Render(" " + name + " "), key: key}
}

// parts returns the bar's parts, and how many of them lead it: the
// types, which are never dropped and have a wider gap after them. Compact,
// the types are one chip of the current type, which a click moves on.
func (e *chartEditor) parts(m *Model, compact bool) ([]editorPart, int) {
	if e.axes {
		return e.axisParts(m), 0
	}
	c := e.chart(m)
	var parts []editorPart
	for i, t := range sheet.ChartTypes {
		switch {
		case !compact:
			parts = append(parts, e.chip(t == c.Type, t.Title(), strconv.Itoa(i+1)))
		case t == c.Type:
			next := (i+1)%len(sheet.ChartTypes) + 1
			parts = append(parts, e.chip(true, "◂ "+t.Title()+" ▸", strconv.Itoa(next)))
		}
	}
	lead := len(parts)
	parts = append(parts, editorPart{text: m.th.Key.Render(c.Data.String()), key: editorRange})
	switch {
	case c.Type.Stackable():
		parts = append(parts, e.chip(c.Stack != sheet.StackNone, c.Stack.Title(), editorStack))
	case c.Type == sheet.ChartScatter:
		parts = append(parts, e.chip(c.Trend, "Trend line", editorTrend))
	}
	series := "Series in columns"
	if c.ByRow {
		series = "Series in rows"
	}
	header, labels := "Header row", "Labels in column"
	if c.ByRow {
		header, labels = "Header column", "Labels in row"
	}
	return append(parts,
		e.chip(true, series, editorSwitch),
		e.chip(c.Header, header, editorHeader),
		e.chip(c.Labels, labels, editorLabels)), lead
}

// spans lays the parts out on the context line and returns each part's
// x. On narrow screens the types become one chip when that leaves room
// for more options, and options that still don't fit are dropped from the
// right.
func (e *chartEditor) spans(m *Model) ([]editorPart, []int) {
	parts, xs := e.fit(m, false)
	if all, _ := e.parts(m, false); len(parts) < len(all) && !e.axes {
		if cp, cxs := e.fit(m, true); len(cp)+len(sheet.ChartTypes)-1 > len(parts) {
			return cp, cxs
		}
	}
	return parts, xs
}

// fit lays out the parts, dropping options from the right until they fit.
func (e *chartEditor) fit(m *Model, compact bool) ([]editorPart, []int) {
	parts, lead := e.parts(m, compact)
	for {
		xs := make([]int, len(parts))
		x := 0
		for i, p := range parts {
			if i == lead && lead > 0 {
				x += 2 // a wider gap between the types and the options
			}
			xs[i] = x
			x += ansi.StringWidth(p.text) + 1
		}
		if x-1 <= m.width || len(parts) <= max(lead, 1) {
			return parts, xs
		}
		parts = parts[:len(parts)-1]
	}
}

func (e *chartEditor) ContextLine() (string, string) {
	m := e.m
	if e.i >= len(m.sheet.Charts()) {
		return "", ""
	}
	parts, xs := e.spans(m)
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(strings.Repeat(" ", xs[i]-ansi.StringWidth(b.String())))
		b.WriteString(p.text)
	}
	return b.String(), ""
}

func (e *chartEditor) Mouse(ev overlay.MouseEvent) tea.Cmd {
	m := e.m
	if ev.Kind != overlay.MousePress {
		return nil
	}
	if ev.Y != contextLine {
		// A click elsewhere keeps the changes and acts as usual.
		m.closeOverlay()
		return m.handlePress(tea.Mouse{X: ev.X, Y: ev.Y, Button: ev.Button})
	}
	parts, xs := e.spans(m)
	for i, p := range parts {
		if ev.X >= xs[i] && ev.X < xs[i]+ansi.StringWidth(p.text) {
			return e.Key(tea.KeyPressMsg{Code: rune(p.key[0]), Text: p.key})
		}
	}
	return nil
}

func (e *chartEditor) Status() (string, string) {
	m := e.m
	if e.axes {
		return "", e.axisStatus(m)
	}
	pairs := []string{"←/→", "type", "A", "axes and legend", "S", "switch rows/columns", "H", "header", "L", "labels", "R", "range", "T", "title", "Enter", "done", "Esc", "cancel"}
	if c := e.chart(m); c.Type.Stackable() {
		pairs = append(pairs[:4], append([]string{"K", "stacking"}, pairs[4:]...)...)
	} else if c.Type == sheet.ChartScatter {
		pairs = append(pairs[:4], append([]string{"E", "trend line"}, pairs[4:]...)...)
	}
	return "", fitHints(m, pairs)
}

// fitHints renders key hints, leaving out the pairs before the last two
// (Enter and Esc) from the right until they fit.
func fitHints(m *Model, pairs []string) string {
	for {
		keys := m.th.KeyHints(pairs...)
		if ansi.StringWidth(keys) <= m.width || len(pairs) <= 4 {
			return keys
		}
		pairs = append(pairs[:len(pairs)-6], pairs[len(pairs)-4:]...)
	}
}

// Editor keys, also the toggles' mouse targets.
const (
	editorSwitch = "s"
	editorHeader = "h"
	editorLabels = "l"
	editorRange  = "r"
	editorTitle  = "t"
	editorStack  = "k"
	editorTrend  = "e"
	editorAxes   = "a"
)
