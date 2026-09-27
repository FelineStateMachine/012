package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// The chart editor's axis bar: the ends of the value axis, typed on the
// context line (blank for automatic), a log scale, gridlines and where
// the legend goes. Enter and Esc go back to the main bar, keeping the
// changes; Esc there undoes them all.

// Axis bar keys, also the chips' mouse targets.
const (
	axisMin    = "n"
	axisMax    = "x"
	axisLog    = "l"
	axisGrid   = "g"
	axisLegend = "p"
)

func (e *chartEditor) axisParts(m *Model) []editorPart {
	c := e.chart(m)
	end := func(name string, v float64, set bool, key string) editorPart {
		text := name + " auto"
		if set {
			text = name + " " + strconv.FormatFloat(v, 'g', 10, 64)
		}
		return e.chip(set, text, key)
	}
	return []editorPart{
		end("Min", c.Min, c.HasMin, axisMin),
		end("Max", c.Max, c.HasMax, axisMax),
		e.chip(c.Log, "Log", axisLog),
		e.chip(!c.NoGrid, "Gridlines", axisGrid),
		e.chip(c.Legend != sheet.LegendNone, "Legend "+c.Legend.String(), axisLegend),
	}
}

func (e *chartEditor) axisKey(m *Model, key string) {
	switch key {
	case "enter", "esc":
		e.axes = false
	case axisMin:
		e.editAxisEnd(m, "Axis minimum (blank for auto):", func(c *sheet.Chart) (*float64, *bool) { return &c.Min, &c.HasMin })
	case axisMax:
		e.editAxisEnd(m, "Axis maximum (blank for auto):", func(c *sheet.Chart) (*float64, *bool) { return &c.Max, &c.HasMax })
	case axisLog:
		e.set(m, func(c *sheet.Chart) { c.Log = !c.Log })
	case axisGrid:
		e.set(m, func(c *sheet.Chart) { c.NoGrid = !c.NoGrid })
	case axisLegend:
		e.set(m, func(c *sheet.Chart) { c.Legend = (c.Legend + 1) % 3 })
	}
}

// editAxisEnd asks for an end of the value axis, as a number the way
// cells take one ("500", "$1,200", "10%"); blank makes it automatic.
func (e *chartEditor) editAxisEnd(m *Model, label string, end func(*sheet.Chart) (*float64, *bool)) {
	c := e.chart(m)
	v, set := end(&c)
	initial := ""
	if *set {
		initial = strconv.FormatFloat(*v, 'g', -1, 64)
	}
	m.closeOverlay()
	e.askAxisEnd(m, label, label, initial, end)
}

// askAxisEnd asks with the prompt label, and asks again, saying so, until
// the answer is a number or blank.
func (e *chartEditor) askAxisEnd(m *Model, label, question, initial string, end func(*sheet.Chart) (*float64, *bool)) {
	m.openText(label, initial, func(m *Model, text string) tea.Cmd {
		text = strings.TrimSpace(text)
		n, _, ok := sheet.ParseValue(text)
		switch {
		case text == "":
			e.set(m, func(c *sheet.Chart) { _, set := end(c); *set = false })
		case !ok:
			e.askAxisEnd(m, "Not a number. "+question, question, text, end)
			return nil
		default:
			e.set(m, func(c *sheet.Chart) { v, set := end(c); *v, *set = n, true })
		}
		e.reopen(m)
		return nil
	})
	m.prompt.onCancel = func() { e.reopen(m) }
}

func (e *chartEditor) axisStatus(m *Model) string {
	return fitHints(m, []string{"N", "min", "X", "max", "L", "log", "G", "grid", "P", "legend", "Esc", "back"})
}
