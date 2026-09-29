package nbview

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
)

// Drawing a cell's block line by line (layout.go says where).

// blockLine draws line k of cell i's block.
func (v *View) blockLine(i int, c notebook.Cell, b block, k int) string {
	kind, r := b.row(k)
	if kind == rowGap {
		return ""
	}
	lead := v.bar(i, kind) + v.prompt(c, kind, r) + " "
	switch kind {
	case rowTop:
		return lead + v.boxTop(i, c)
	case rowBottom:
		return lead + v.boxEdge(i, "╰", "╯", "┗", "┛")
	case rowOut:
		return lead + "  " + v.outLine(c, r)
	}
	if b.box == 0 {
		return lead + "  " + v.sourceLine(i, c, r)
	}
	side := v.boxStyle(i).Render(v.edges(i, "│", "┃"))
	text := v.sourceLine(i, c, r)
	text += strings.Repeat(" ", max(v.content()-ansi.StringWidth(text), 0))
	line := lead + side + " " + text + " " + side
	if r == 0 {
		line += " " + v.runButton(i, c)
	}
	return line
}

// bar is the column left of a cell: ▌ by the active cell, blue in
// command mode and green in edit mode, and ▎ by the other cells
// selected with it. With the active cell's output selected, the bar is
// by the output alone.
func (v *View) bar(i int, kind rowKind) string {
	from, to := v.Range()
	if i < from || i > to || kind == rowGap || i == v.sel && v.onOut && kind != rowOut {
		return " "
	}
	th := v.h.Theme()
	style := th.CellBar
	if v.edit.on || i == v.sel && v.onOut && v.shown(v.h.Cells()[i]).entered() {
		style = th.CellBarEdit // a grid entered takes keys as a cell edited does
	}
	if i != v.sel {
		return style.Render("▎")
	}
	return style.Render("▌")
}

// prompt is the prompt column: [n]: by a code cell's first line, Out[n]:
// by its output's, [*]: while it runs and [ ]: before it has.
func (v *View) prompt(c notebook.Cell, kind rowKind, r int) string {
	text := ""
	if c.Kind == notebook.Code && r == 0 {
		o := v.h.Output(c.ID)
		switch {
		case kind == rowSrc && v.h.State(c.ID).Running:
			text = "[*]:"
		case kind == rowSrc:
			text = "[" + count(o, " ") + "]:"
		case kind == rowOut && v.shown(c).kind != outError:
			text = "Out[" + count(o, " ") + "]:"
			if len(text) > promptW {
				text = "[" + count(o, " ") + "]:"
			}
		}
	}
	return v.h.Theme().CellHead.Render(strings.Repeat(" ", max(promptW-len(text), 0)) + text)
}

// count is the output's run number, or blank for none or one read from
// the file.
func count(o *notebook.Output, blank string) string {
	if o == nil || o.Count == 0 {
		return blank
	}
	return strconv.Itoa(o.Count)
}

// edges picks a box's line: light, or heavy for the cell being edited,
// which reads without color.
func (v *View) edges(i int, light, heavy string) string {
	if v.edit.on && i == v.sel {
		return heavy
	}
	return light
}

// boxStyle is a box's lines: in the bar's color for the active cell.
func (v *View) boxStyle(i int) lipgloss.Style {
	th := v.h.Theme()
	switch {
	case v.edit.on && i == v.sel:
		return th.CellBarEdit
	case i == v.sel && !v.onOut:
		return th.CellBar
	}
	return th.Border
}

// boxTop is a box's top border, with the cell's name at the left and
// how its run stands at the right.
func (v *View) boxTop(i int, c notebook.Cell) string {
	th := v.h.Theme()
	style := v.boxStyle(i)
	h := v.edges(i, "─", "━")
	w := v.content() + 2 // between the corners
	title := c.Name()
	if c.Kind == notebook.Note {
		title = "Markdown"
	}
	state, stateStyle := v.state(c)
	room := w - 2
	if state != "" {
		state = ansi.Truncate(state, max(room/2, 8), "…")
		room -= ansi.StringWidth(state) + 3
	}
	var left, right string
	used := 0
	if title != "" && room > 4 {
		title = ansi.Truncate(title, room-2, "…")
		left = style.Render(h+" ") + th.CellHead.Render(title) + style.Render(" ")
		used += ansi.StringWidth(title) + 3
	}
	if state != "" {
		right = style.Render(" ") + stateStyle.Render(state) + style.Render(" "+h)
		used += ansi.StringWidth(state) + 3
	}
	fill := style.Render(strings.Repeat(h, max(w-used, 0)))
	return style.Render(v.edges(i, "╭", "┏")) + left + fill + right + style.Render(v.edges(i, "╮", "┓"))
}

// boxEdge is a box's bottom border.
func (v *View) boxEdge(i int, left, right, heavyLeft, heavyRight string) string {
	return v.boxStyle(i).Render(v.edges(i, left, heavyLeft) + strings.Repeat(v.edges(i, "─", "━"), v.content()+2) + v.edges(i, right, heavyRight))
}

// runButton is ▶, which runs the cell, or ■ while it runs, which stops
// it.
func (v *View) runButton(i int, c notebook.Cell) string {
	th := v.h.Theme()
	style := th.CellHead
	if i == v.sel {
		style = v.boxStyle(i)
	}
	if v.h.State(c.ID).Running {
		return style.Render("■")
	}
	return style.Render("▶")
}

// state is what a cell's box says of its run, and in what style: empty
// before it has run.
func (v *View) state(c notebook.Cell) (string, lipgloss.Style) {
	th := v.h.Theme()
	if c.Kind != notebook.Code {
		return "", th.CellHead
	}
	st := v.h.State(c.ID)
	o := v.h.Output(c.ID)
	switch {
	case st.Problem != "":
		return st.Problem, th.Warning
	case st.Running:
		return running(time.Since(st.Started)), th.CellHead
	case st.Waiting:
		return "waiting", th.CellHead
	case o == nil:
		return "", th.CellHead
	case o.Failed():
		return "failed" + took(o), th.Warning
	case st.Stale:
		return "stale" + took(o), th.Stale
	case o.Count == 0:
		return "saved", th.CellHead
	}
	return "✓" + took(o), th.CellHead
}

// outLine is line r of cell c's output: its window, or one line saying
// it's folded.
func (v *View) outLine(c notebook.Cell, r int) string {
	th := v.h.Theme()
	f := v.foldOf(c.ID)
	if f.hidden {
		return th.Muted.Render("⋯ output hidden  (o shows it)")
	}
	return v.shown(c).line(th, v.h.Locale(), r, f, v.outWidth())
}

// sourceLine is row r of a cell's source: the editor's while it's
// edited, a note's Markdown, or a pipeline highlighted.
func (v *View) sourceLine(i int, c notebook.Cell, r int) string {
	if v.edit.on && i == v.sel {
		return v.edit.line(v.h.Theme(), r, v.content(), v.spansFor(v.edit.text(), c.Kind, true))
	}
	if c.Kind == notebook.Note {
		lines := v.noteLines(c)
		if r < len(lines) {
			return lines[r]
		}
		return ""
	}
	buf := []rune(c.Source)
	rows := lineedit.Wrap(buf, v.content())
	if r >= len(rows) {
		return ""
	}
	return drawRow(v.h.Theme(), buf, rows[r], v.spansFor(c.Source, c.Kind, false), nil)
}

// spinner turns while a cell runs.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// running is what a running cell's box says: after a few seconds, a
// spinner and how long it has run.
func running(d time.Duration) string {
	if d < 3*time.Second {
		return "running"
	}
	return spinner[int(d/(100*time.Millisecond))%len(spinner)] + " running " + duration(d)
}

func took(o *notebook.Output) string {
	if o.Took <= 0 {
		return ""
	}
	return " " + duration(o.Took)
}

// duration is a run's length: under a second as <1s, so a fast cell
// always reads the same, then seconds, then minutes.
func duration(d time.Duration) string {
	switch {
	case d < time.Second:
		return "<1s"
	case d < 10*time.Second:
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	}
	return strconv.Itoa(int(d.Minutes())) + "m" + pad2(int(d.Seconds())%60) + "s"
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
