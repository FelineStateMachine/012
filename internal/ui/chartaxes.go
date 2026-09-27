package ui

import (
	"strconv"
	"strings"

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

func (e *chartEditor) axisParts() []editorPart {
	c := e.chart()
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

func (e *chartEditor) axisKey(key string) {
	switch key {
	case "enter", "esc":
		e.axes = false
	case axisMin:
		e.editAxisEnd("Axis minimum (blank for auto):", func(c *sheet.Chart) (*float64, *bool) { return &c.Min, &c.HasMin })
	case axisMax:
		e.editAxisEnd("Axis maximum (blank for auto):", func(c *sheet.Chart) (*float64, *bool) { return &c.Max, &c.HasMax })
	case axisLog:
		e.set(func(c *sheet.Chart) { c.Log = !c.Log })
	case axisGrid:
		e.set(func(c *sheet.Chart) { c.NoGrid = !c.NoGrid })
	case axisLegend:
		e.set(func(c *sheet.Chart) { c.Legend = (c.Legend + 1) % 3 })
	}
}

// editAxisEnd asks for an end of the value axis, as a number the way
// cells take one ("500", "$1,200", "10%"); blank makes it automatic.
func (e *chartEditor) editAxisEnd(label string, end func(*sheet.Chart) (*float64, *bool)) {
	c := e.chart()
	v, set := end(&c)
	initial := ""
	if *set {
		initial = strconv.FormatFloat(*v, 'g', -1, 64)
	}
	e.m.closeOverlay()
	e.askAxisEnd(label, label, initial, end)
}

// askAxisEnd asks with the prompt label, and asks again, saying so, until
// the answer is a number or blank.
func (e *chartEditor) askAxisEnd(label, question, initial string, end func(*sheet.Chart) (*float64, *bool)) {
	e.m.askText(label, initial, func(text string) {
		text = strings.TrimSpace(text)
		n, _, ok := sheet.ParseValue(text)
		switch {
		case text == "":
			e.set(func(c *sheet.Chart) { _, set := end(c); *set = false })
		case !ok:
			e.askAxisEnd("Not a number. "+question, question, text, end)
			return
		default:
			e.set(func(c *sheet.Chart) { v, set := end(c); *v, *set = n, true })
		}
		e.reopen()
	}, e.reopen)
}

func (e *chartEditor) axisStatus() string {
	width, _ := e.m.size()
	return fitHints(e.m.styles(), width, []string{"N", "min", "X", "max", "L", "log", "G", "grid", "P", "legend", "Esc", "back"})
}
