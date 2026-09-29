// Package nbview draws a notebook tab and takes its keys: the cells one
// under another, each a head line (its run count, name and state), its
// source (a pipeline, soft-wrapped so none of it is ever hidden, or
// Markdown drawn as text) and a code cell's output under it. Like
// Jupyter it has two modes: in command mode keys act on cells (move,
// add, delete, run), in edit mode they type into the selected cell. It
// knows the UI only through Host: the cells and outputs, the state of
// runs, and running the registered commands every action is.
package nbview

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the notebook needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Locale() *locale.Locale
	// Cells are the notebook's cells, and Output a cell's output.
	Cells() []notebook.Cell
	Output(id int) *notebook.Output
	// State is how a cell's run stands.
	State(id int) State
	// Run runs a registered command, as its key would.
	Run(command string) tea.Cmd
	// Edit keeps a cell's new source, as one undo step.
	Edit(id int, source string)
}

// State is how a cell's run stands, as the runner knows it.
type State struct {
	Waiting, Running bool
	Started          time.Time // when it started running
	Stale            bool      // what it read, or its source, changed since it ran
	// Problem says why it can't run as it is: its name is taken.
	Problem string
}

// View is a notebook tab's view and its state: the selected cell, the
// scroll position, the mode, and what's worked out from the cells.
type View struct {
	h Host
	// Keys bind command mode's keys to commands, EditKeys edit mode's;
	// "d d" is d pressed twice.
	Keys, EditKeys map[string]string
	Providers      Providers

	sel   int  // the selected cell
	onOut bool // the selection is the cell's output
	top   int  // the first line of the cells shown
	// width and height are the body's, as last drawn.
	width, height int
	pending       string // the first key of a pair: "d" of "d d"

	edit    editor // the cell being edited, if any
	full    *Full  // an output shown full-screen, if any
	expand  map[int]bool
	outs    map[*notebook.Output]*shown
	syntax  syntaxCache
	lastTap tap // the last click, for double clicks
}

// New returns a view of the notebook h shows.
func New(h Host) *View {
	return &View{h: h, width: 80, height: 16, expand: map[int]bool{}, outs: map[*notebook.Output]*shown{}, syntax: syntaxCache{spans: map[string][]Span{}}}
}

// gutter is the columns left of every line: a mark and a space.
const gutter = 2

// Selected is the selected cell's index, and whether its output is
// selected rather than the cell.
func (v *View) Selected() (int, bool) {
	v.clamp()
	return v.sel, v.onOut
}

// Select selects cell i, or its output with out.
func (v *View) Select(i int, out bool) {
	v.sel, v.onOut = i, out
	v.clamp()
	v.follow()
}

// Cell is the selected cell, if there is one.
func (v *View) Cell() (notebook.Cell, bool) {
	cells := v.h.Cells()
	v.clamp()
	if len(cells) == 0 {
		return notebook.Cell{}, false
	}
	return cells[v.sel], true
}

// clamp keeps the selection on a cell there is, and on an output only
// where one shows.
func (v *View) clamp() {
	cells := v.h.Cells()
	v.sel = max(min(v.sel, len(cells)-1), 0)
	if v.onOut && (len(cells) == 0 || v.shown(cells[v.sel]).kind == outNone) {
		v.onOut = false
	}
}

// shown is a cell's output as it shows, parsed once.
func (v *View) shown(c notebook.Cell) *shown {
	o := v.h.Output(c.ID)
	if c.Kind != notebook.Code || o == nil {
		return &shown{}
	}
	sh := v.outs[o]
	if sh == nil {
		if len(v.outs) > 256 {
			clear(v.outs) // outputs replaced by runs since
		}
		sh = parse(o)
		v.outs[o] = sh
	}
	return sh
}

// Expanded reports whether a cell's output shows whole.
func (v *View) Expanded(id int) bool { return v.expand[id] }

// Toggle shows the selected cell's output whole, or its window again.
func (v *View) Toggle() {
	if c, ok := v.Cell(); ok {
		v.expand[c.ID] = !v.expand[c.ID]
		v.follow()
	}
}

// content is how wide a cell's source and output are drawn.
func (v *View) content() int { return max(v.width-gutter-1, 8) }

// block is what a cell takes on the screen, in lines: its head, its
// source, its output, and a blank line after.
type block struct {
	head, src, out int
}

func (b block) lines() int { return b.head + b.src + b.out + 1 }

// blockOf lays out cell c.
func (v *View) blockOf(i int, c notebook.Cell) block {
	b := block{head: 1}
	switch {
	case v.edit.on && i == v.sel:
		b.src = len(v.edit.rows(v.content()))
	case c.Kind == notebook.Note:
		b.src = len(v.noteLines(c))
	default:
		b.src = len(lineedit.Wrap([]rune(c.Source), v.content()))
	}
	if c.Kind == notebook.Code {
		b.out = v.shown(c).height(v.expand[c.ID], v.content())
	}
	return b
}

// noteLines are a note cell's lines as drawn.
func (v *View) noteLines(c notebook.Cell) []string {
	if strings.TrimSpace(c.Source) == "" {
		return []string{v.h.Theme().Muted.Render("An empty note: Enter writes it, in Markdown")}
	}
	return markdown(v.h.Theme(), c.Source, v.content())
}

// Resize sets the body's size: the lines under the tab's bar and above
// the status line.
func (v *View) Resize(width, height int) {
	height = max(height, 1)
	if width == v.width && height == v.height {
		return
	}
	v.width, v.height = width, height
	if v.full != nil {
		v.full.resize(width, height)
		v.full.settle()
	}
	v.follow()
}

// follow scrolls so the selection shows: the selected cell's head, or
// its output's first lines, or the caret while editing.
func (v *View) follow() {
	cells := v.h.Cells()
	at := 0
	for i := range cells {
		if i == v.sel {
			break
		}
		at += v.blockOf(i, cells[i]).lines()
	}
	if len(cells) == 0 {
		v.top = 0
		return
	}
	b := v.blockOf(v.sel, cells[v.sel])
	first, last := at, at+min(b.lines()-1, v.height-1)
	switch {
	case v.edit.on:
		row, _ := v.edit.caret(v.content())
		first = at + 1 + row
		last = first
	case v.onOut:
		first = at + b.head + b.src
		last = min(first+b.out-1, first+v.height-1)
	}
	if first < v.top {
		v.top = first
	}
	if last >= v.top+v.height {
		v.top = last - v.height + 1
	}
	v.top = max(v.top, 0)
}

// Lines draws the body: height lines, each width wide or less.
func (v *View) Lines() []string {
	if v.full != nil {
		return v.full.lines(v.h.Theme(), v.h.Locale())
	}
	v.clamp()
	out := make([]string, 0, v.height)
	cells := v.h.Cells()
	if len(cells) == 0 {
		th := v.h.Theme()
		out = append(out, "", "  "+th.Muted.Render("An empty notebook: ")+th.KeyHints("b", "add a code cell", "m", "make it a note"))
	}
	at := 0
	for i, c := range cells {
		b := v.blockOf(i, c)
		if at+b.lines() <= v.top {
			at += b.lines()
			continue
		}
		for k := range b.lines() {
			if n := at + k; n >= v.top && len(out) < v.height {
				out = append(out, v.blockLine(i, c, b, k))
			}
		}
		at += b.lines()
		if len(out) >= v.height {
			break
		}
	}
	for len(out) < v.height {
		out = append(out, "")
	}
	return out
}

// blockLine draws line k of cell i's block.
func (v *View) blockLine(i int, c notebook.Cell, b block, k int) string {
	th := v.h.Theme()
	switch {
	case k == 0:
		return v.headLine(i, c)
	case k <= b.src:
		return th.Border.Render("│") + " " + v.sourceLine(i, c, k-1)
	case k <= b.src+b.out:
		mark := "  "
		if i == v.sel && v.onOut {
			mark = th.Selection.Render("▌") + " "
		}
		return mark + v.shown(c).line(th, v.h.Locale(), k-1-b.src, v.expand[c.ID], v.content())
	}
	return ""
}

// headLine is a cell's head: its run count and name, and at the right
// its state and how long it took.
func (v *View) headLine(i int, c notebook.Cell) string {
	th := v.h.Theme()
	left, right := v.headText(c)
	w := max(v.width-1, 1)
	left = ansi.Truncate(left, w, "…") // a long name on a narrow screen
	right = ansi.Truncate(right, max(w-ansi.StringWidth(left)-2, 0), "…")
	text := left + strings.Repeat(" ", max(w-ansi.StringWidth(left)-ansi.StringWidth(right), 1)) + right
	switch {
	case i == v.sel && v.edit.on:
		return th.Pointer.Render(text)
	case i == v.sel && !v.onOut:
		return th.Selection.Render(text)
	}
	st := v.h.State(c.ID)
	if st.Stale && !st.Running && !st.Waiting {
		k := strings.LastIndex(text, "stale")
		return th.CellHead.Render(text[:k]) + th.Stale.Render("stale") + th.CellHead.Render(text[k+len("stale"):])
	}
	return th.CellHead.Render(text)
}

// headText is what a cell's head says: [3] sales at the left, and its
// state at the right.
func (v *View) headText(c notebook.Cell) (string, string) {
	if c.Kind == notebook.Note {
		return "  Markdown", ""
	}
	st := v.h.State(c.ID)
	o := v.h.Output(c.ID)
	count := "[ ]"
	switch {
	case st.Running:
		count = "[*]"
	case o != nil && o.Count > 0:
		count = "[" + strconv.Itoa(o.Count) + "]"
	}
	left := "  " + count
	if name := c.Name(); name != "" {
		left += " " + name
	}
	var right string
	switch {
	case st.Problem != "":
		right = st.Problem
	case st.Running:
		right = running(time.Since(st.Started))
	case st.Waiting:
		right = "waiting"
	case o == nil:
		right = "not run"
	case o.Failed():
		right = "failed" + took(o)
	case st.Stale:
		right = "stale" + took(o)
	case o.Count == 0:
		right = "saved"
	default:
		right = "✓" + took(o)
	}
	return left, right + " "
}

// spinner turns while a cell runs.
var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// running is what a running cell's head says: after a few seconds, a
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
