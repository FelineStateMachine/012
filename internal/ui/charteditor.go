package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// chartEditor is the chart editor: a bar on the context line, in the
// spirit of the find bar, with the type as a row of chips and the options
// as toggles. Changes show at once; Enter keeps them and Esc undoes them,
// removing a chart that was just inserted.
type chartEditor struct {
	i     int
	start int // the sheet's state before editing, to undo back to
	isNew bool
}

// openChartEditor edits chart i. start is the sheet's state to return to
// on Esc.
func (m *Model) openChartEditor(i int, isNew bool, start int) {
	if i < 0 || i >= len(m.sheet.Charts()) {
		return
	}
	m.charts.last = i
	m.openOverlay(&chartEditor{i: i, start: start, isNew: isNew})
}

func (e *chartEditor) indicator() string { return "CHART" }

func (e *chartEditor) layout(*Model) []box { return nil }

func (e *chartEditor) chart(m *Model) sheet.Chart { return m.sheet.Charts()[e.i] }

// set applies a change to the chart as its own undo step.
func (e *chartEditor) set(m *Model, fn func(c *sheet.Chart)) {
	c := e.chart(m)
	fn(&c)
	m.sheet.SetChart(e.i, c, "edit chart")
	m.changed = m.sheet.StateID() != m.saved
}

func (e *chartEditor) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	if e.i >= len(m.sheet.Charts()) {
		m.closeOverlay()
		return nil
	}
	switch key := strings.ToLower(k.String()); key {
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
	case "1", "2", "3", "4":
		e.set(m, func(c *sheet.Chart) { c.Type = sheet.ChartTypes[key[0]-'1'] })
	case editorSwitch:
		e.set(m, func(c *sheet.Chart) { c.ByRow = !c.ByRow })
	case editorHeader:
		e.set(m, func(c *sheet.Chart) { c.Header = !c.Header })
	case editorLabels:
		e.set(m, func(c *sheet.Chart) { c.Labels = !c.Labels })
	case editorRange:
		e.editRange(m)
	case editorTitle:
		e.editTitle(m)
	}
	return nil
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
	m.prompt.onCancel = func(m *Model) {
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
	m.prompt.onCancel = e.reopen
}

// editorPart is a piece of the editor bar: a type chip or a toggle.
type editorPart struct {
	text string
	key  string // the key it stands for
}

func (e *chartEditor) parts(m *Model) []editorPart {
	c := e.chart(m)
	chip := func(on bool, name, key string) editorPart {
		style := m.th.Muted
		if on {
			style = m.th.MenuSelected
		}
		return editorPart{text: style.Render(" " + name + " "), key: key}
	}
	var parts []editorPart
	for i, t := range sheet.ChartTypes {
		parts = append(parts, chip(t == c.Type, t.Title(), strconv.Itoa(i+1)))
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
		editorPart{text: m.th.Key.Render(c.Data.String()), key: editorRange},
		chip(true, series, editorSwitch),
		chip(c.Header, header, editorHeader),
		chip(c.Labels, labels, editorLabels))
}

// spans lays the parts out on the context line, dropping options from the
// right on narrow screens, and returns each part's x.
func (e *chartEditor) spans(m *Model) ([]editorPart, []int) {
	parts := e.parts(m)
	for {
		xs := make([]int, len(parts))
		x := 0
		for i, p := range parts {
			if i == len(sheet.ChartTypes) {
				x += 2 // a wider gap between the types and the options
			}
			xs[i] = x
			x += ansi.StringWidth(p.text) + 1
		}
		if x-1 <= m.width || len(parts) <= len(sheet.ChartTypes) {
			return parts, xs
		}
		parts = parts[:len(parts)-1]
	}
}

func (e *chartEditor) contextLine(m *Model) (string, string) {
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

func (e *chartEditor) mouse(m *Model, ev mouseEvent) tea.Cmd {
	if ev.kind != mousePress {
		return nil
	}
	if ev.y != contextLine {
		// A click elsewhere keeps the changes and acts as usual.
		m.closeOverlay()
		return m.handlePress(tea.Mouse{X: ev.x, Y: ev.y, Button: ev.button})
	}
	parts, xs := e.spans(m)
	for i, p := range parts {
		if ev.x >= xs[i] && ev.x < xs[i]+ansi.StringWidth(p.text) {
			return e.key(m, tea.KeyPressMsg{Code: rune(p.key[0]), Text: p.key})
		}
	}
	return nil
}

func (e *chartEditor) status(m *Model) (string, string) {
	pairs := []string{"←/→", "type", "S", "switch rows/columns", "H", "header", "L", "labels", "R", "range", "T", "title", "Enter", "done", "Esc", "cancel"}
	for {
		keys := m.th.KeyHints(pairs...)
		if ansi.StringWidth(keys) <= m.width || len(pairs) <= 4 {
			return "", keys
		}
		pairs = append(pairs[:len(pairs)-6], pairs[len(pairs)-4:]...) // keep Enter and Esc
	}
}

// contextLiner is an overlay drawn on the context line, like the chart
// editor.
type contextLiner interface {
	contextLine(m *Model) (left, right string)
}

// Editor keys, also the toggles' mouse targets.
const (
	editorSwitch = "s"
	editorHeader = "h"
	editorLabels = "l"
	editorRange  = "r"
	editorTitle  = "t"
)
