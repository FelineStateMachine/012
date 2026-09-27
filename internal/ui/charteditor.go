package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// chartHost is what the chart editor and a selected chart act on. The
// model implements it. They stay in package ui: besides the charts of
// the sheet shown, they reach the grid's geometry (where a chart is on
// screen, the cell under the mouse), the pointer's shape, the chart's
// context menu, and the model's prompts, and hand the keys and clicks
// they don't take back to the grid, which takes more than a small
// interface.
type chartHost interface {
	styles() *theme.Theme
	size() (width, height int)
	sheetShown() *sheet.Sheet
	// syncChanged makes the modified flag follow the undo history.
	syncChanged()
	openOverlay(o overlay.Overlay)
	closeOverlay()
	runCommand(id string) tea.Cmd
	// selectChart selects chart i and returns the selection.
	selectChart(i int) *chartSel
	// forgetChart leaves chart commands no chart to act on but the one
	// under the active cell or the newest.
	forgetChart()
	// showChart scrolls chart i into view.
	showChart(i int)
	chartScreen(c sheet.Chart) (x, y int)
	chartAt(x, y int) int
	chartCellAt(x, y int) sheet.Addr
	setShape(shape string) tea.Cmd
	// holdsKeys reports whether keys can be held, and zoomed is chart c
	// spread over the grid, as Space held shows it (keyboard.go).
	holdsKeys() bool
	zoomed(c sheet.Chart) sheet.Chart
	showChartMenu(x, y int)
	// say puts msg on the context line.
	say(msg string)
	askText(label, initial string, done func(string), cancel func())
	pointRange(label string, done func(sheet.Rect), cancel func())
	selectRect(r sheet.Rect)
	clearSelection()
	// passKey and passMouse hand what a chart doesn't take to the grid,
	// as if nothing were selected.
	passKey(k tea.KeyPressMsg) tea.Cmd
	passMouse(e overlay.MouseEvent) tea.Cmd
	// recordChart records a chart inserted, edited, moved or resized, as
	// the command that does it answered with the chart; placed is set
	// for a move or resize without a command (chartmacro.go).
	recordChart(id string, i int, before, after sheet.Chart, placed bool)
}

// chartEditor is the chart editor: a bar on the context line, in the
// spirit of the find bar, with the type as a row of chips and the options
// as toggles. A second bar (A) holds the value axis, gridlines and
// legend, as Sheets' Customize tab does; see chartaxes.go. Changes show
// at once; Enter keeps them and Esc undoes them, removing a chart that
// was just inserted.
type chartEditor struct {
	m      chartHost // the model, through what the editor needs of it
	i      int
	start  int         // the sheet's state before editing, to undo back to
	before sheet.Chart // the chart when the editor opened, for a macro
	isNew  bool
	axes   bool // showing the axis options
}

// openChartEditor edits chart i. start is the sheet's state to return to
// on Esc.
func (m *Model) openChartEditor(i int, isNew bool, start int) *chartEditor {
	charts := m.sheet.Charts()
	if i < 0 || i >= len(charts) {
		return nil
	}
	m.charts.last = i
	e := &chartEditor{m: m, i: i, start: start, before: charts[i], isNew: isNew}
	m.openOverlay(e)
	return e
}

func (e *chartEditor) Indicator() string { return "CHART" }

func (e *chartEditor) Layout() []overlay.Box { return nil }

func (e *chartEditor) chart() sheet.Chart { return e.m.sheetShown().Charts()[e.i] }

// set applies a change to the chart as its own undo step.
func (e *chartEditor) set(fn func(c *sheet.Chart)) {
	c := e.chart()
	fn(&c)
	e.m.sheetShown().SetChart(e.i, c, "edit chart")
	e.m.syncChanged()
}

// keep closes the editor keeping the changes, and records them.
func (e *chartEditor) keep() {
	m := e.m
	m.closeOverlay()
	id := "chart.edit"
	if e.isNew {
		id = "insert.chart"
		m.say("Inserted a chart of " + e.chart().Data.String())
	}
	m.recordChart(id, e.i, e.before, e.chart(), false)
}

func (e *chartEditor) Key(k tea.KeyPressMsg) tea.Cmd {
	m := e.m
	if e.i >= len(m.sheetShown().Charts()) {
		m.closeOverlay()
		return nil
	}
	key := strings.ToLower(k.String())
	if e.axes {
		e.axisKey(key)
		return nil
	}
	switch key {
	case "enter":
		e.keep()
		m.selectChart(e.i)
	case "esc":
		s := m.sheetShown()
		for s.StateID() != e.start && s.CanUndo() {
			s.Undo()
		}
		m.syncChanged()
		m.closeOverlay()
		m.forgetChart()
	case "left", "right", "shift+tab", "tab":
		d := 1
		if key == "left" || key == "shift+tab" {
			d = -1
		}
		e.set(func(c *sheet.Chart) {
			n := len(sheet.ChartTypes)
			c.Type = sheet.ChartTypes[((int(c.Type)+d)%n+n)%n]
		})
	case editorAxes:
		e.axes = true
	default:
		e.optionKey(key)
	}
	return nil
}

// optionKey handles the keys of the main bar's options: a type's number,
// the toggles, the range and the title.
func (e *chartEditor) optionKey(key string) {
	if n, err := strconv.Atoi(key); err == nil && n >= 1 && n <= len(sheet.ChartTypes) {
		e.set(func(c *sheet.Chart) { c.Type = sheet.ChartTypes[n-1] })
		return
	}
	switch key {
	case editorSwitch:
		e.set(func(c *sheet.Chart) { c.ByRow = !c.ByRow })
	case editorHeader:
		e.set(func(c *sheet.Chart) { c.Header = !c.Header })
	case editorLabels:
		e.set(func(c *sheet.Chart) { c.Labels = !c.Labels })
	case editorStack:
		if e.chart().Type.Stackable() {
			e.set(func(c *sheet.Chart) { c.Stack = (c.Stack + 1) % 3 })
		}
	case editorTrend:
		if e.chart().Type == sheet.ChartScatter {
			e.set(func(c *sheet.Chart) { c.Trend = !c.Trend })
		}
	case editorRange:
		e.editRange()
	case editorTitle:
		e.editTitle()
	}
}

// reopen returns to the editor after a prompt.
func (e *chartEditor) reopen() { e.m.openOverlay(e) }

// editRange asks for the data range by pointing, starting from the
// current one.
func (e *chartEditor) editRange() {
	m := e.m
	m.closeOverlay()
	m.selectRect(e.chart().Data)
	m.pointRange("Chart data:", func(r sheet.Rect) {
		m.clearSelection()
		e.set(func(c *sheet.Chart) { c.Data = r })
		e.reopen()
	}, func() {
		m.clearSelection()
		e.reopen()
	})
}

func (e *chartEditor) editTitle() {
	m := e.m
	m.closeOverlay()
	m.askText("Chart title:", e.chart().Title, func(text string) {
		e.set(func(c *sheet.Chart) { c.Title = text })
		e.reopen()
	}, e.reopen)
}

// editorPart is a piece of the editor bar: a type chip or a toggle.
type editorPart struct {
	text string
	key  string // the key it stands for
}

// chip is a part drawn as a toggle, highlighted when on.
func (e *chartEditor) chip(on bool, name, key string) editorPart {
	style := e.m.styles().Muted
	if on {
		style = e.m.styles().MenuSelected
	}
	return editorPart{text: style.Render(" " + name + " "), key: key}
}

// parts returns the bar's parts, and how many of them lead it: the
// types, which are never dropped and have a wider gap after them. Compact,
// the types are one chip of the current type, which a click moves on.
func (e *chartEditor) parts(compact bool) ([]editorPart, int) {
	if e.axes {
		return e.axisParts(), 0
	}
	c := e.chart()
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
	parts = append(parts, editorPart{text: e.m.styles().Key.Render(c.Data.String()), key: editorRange})
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
func (e *chartEditor) spans() ([]editorPart, []int) {
	parts, xs := e.fit(false)
	if all, _ := e.parts(false); len(parts) < len(all) && !e.axes {
		if cp, cxs := e.fit(true); len(cp)+len(sheet.ChartTypes)-1 > len(parts) {
			return cp, cxs
		}
	}
	return parts, xs
}

// fit lays out the parts, dropping options from the right until they fit.
func (e *chartEditor) fit(compact bool) ([]editorPart, []int) {
	width, _ := e.m.size()
	parts, lead := e.parts(compact)
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
		if x-1 <= width || len(parts) <= max(lead, 1) {
			return parts, xs
		}
		parts = parts[:len(parts)-1]
	}
}

func (e *chartEditor) ContextLine() (string, string) {
	if e.i >= len(e.m.sheetShown().Charts()) {
		return "", ""
	}
	parts, xs := e.spans()
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(strings.Repeat(" ", xs[i]-ansi.StringWidth(b.String())))
		b.WriteString(p.text)
	}
	return b.String(), ""
}

func (e *chartEditor) Mouse(ev overlay.MouseEvent) tea.Cmd {
	if ev.Kind != overlay.MousePress {
		return nil
	}
	if ev.Y != contextLine {
		// A click elsewhere keeps the changes and acts as usual.
		e.keep()
		return e.m.passMouse(ev)
	}
	parts, xs := e.spans()
	for i, p := range parts {
		if ev.X >= xs[i] && ev.X < xs[i]+ansi.StringWidth(p.text) {
			return e.Key(tea.KeyPressMsg{Code: rune(p.key[0]), Text: p.key})
		}
	}
	return nil
}

func (e *chartEditor) Status() (string, string) {
	if e.axes {
		return "", e.axisStatus()
	}
	pairs := []string{"←/→", "type", "A", "axes and legend", "S", "switch rows/columns", "H", "header", "L", "labels", "R", "range", "T", "title", "Enter", "done", "Esc", "cancel"}
	if c := e.chart(); c.Type.Stackable() {
		pairs = append(pairs[:4], append([]string{"K", "stacking"}, pairs[4:]...)...)
	} else if c.Type == sheet.ChartScatter {
		pairs = append(pairs[:4], append([]string{"E", "trend line"}, pairs[4:]...)...)
	}
	width, _ := e.m.size()
	return "", fitHints(e.m.styles(), width, pairs)
}

// fitHints renders key hints, leaving out the pairs before the last two
// (Enter and Esc) from the right until they fit in width.
func fitHints(th *theme.Theme, width int, pairs []string) string {
	for {
		keys := th.KeyHints(pairs...)
		if ansi.StringWidth(keys) <= width || len(pairs) <= 4 {
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
