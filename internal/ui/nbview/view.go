// Package nbview draws a notebook tab and takes its keys and clicks, as
// Jupyter does: cells one under another, a code cell's pipeline in a box
// with its [n]: prompt and a ▶ that runs it, its output under it after
// Out[n]:, and a note cell's Markdown drawn as text. The active cell has
// a bar at its left. Like Jupyter it has two modes: in command mode keys
// act on cells (move, select several, add, delete, move them, run), in
// edit mode they type into the active cell. A toolbar over the cells
// runs the same commands. It knows the UI only through Host: the cells
// and outputs, the state of runs, and running the registered commands
// every action is.
package nbview

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the notebook needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Locale() *locale.Locale
	// Cells are the notebook's cells, and Output a cell's output.
	Cells() []notebook.Cell
	Output(id int) *notebook.Output
	// State is how a cell's run stands, and Kernel how running stands.
	State(id int) State
	Kernel() Kernel
	// Run runs a registered command, as its key would.
	Run(command string) tea.Cmd
	// Edit keeps a cell's new source, as one undo step.
	Edit(id int, source string)
	// Grid draws cell id's output, a table or record, as the UI's
	// grid: data is the grid's table as NUON (a record's fields as
	// rows), of rows rows.
	Grid(id int, data []byte, rows int) Grid
}

// State is how a cell's run stands, as the runner knows it.
type State struct {
	Waiting, Running bool
	Started          time.Time // when it started running
	// Live is set while it runs as a stream, and Rows counts the rows
	// the stream has printed.
	Live  bool
	Rows  int
	Stale bool // what it read, or its source, changed since it ran
	// Problem says why it can't run as it is: its name is taken.
	Problem string
}

// Kernel is how running cells stands, for the toolbar: Jupyter's
// kernel, which here is nu started for each cell.
type Kernel struct {
	Busy     bool   // a cell is running
	Waiting  int    // cells waiting to run
	Live     int    // cells running as streams
	Off      string // why cells don't run here, or ""
	Reactive bool   // cells reading a cell run again when it runs
	Clip     int    // cells copied, for Paste
}

// View is a notebook tab's view and its state: the selected cells, the
// scroll position, the mode, and what's worked out from the cells.
type View struct {
	h Host
	// Keys bind command mode's keys to commands, EditKeys edit mode's;
	// "d d" is d pressed twice.
	Keys, EditKeys map[string]string
	Providers      Providers
	// After is the clock typing's pauses are timed on: a command that
	// gives msg once d has passed, tea.Tick's when nil. Tests give one
	// that hands them msg, to deliver when they want the pause over.
	After func(d time.Duration, msg tea.Msg) tea.Cmd

	sel    int  // the active cell
	anchor int  // the other end of the cells selected with it, or -1
	onOut  bool // the selection is the active cell's output
	top    int  // the first line of the cells shown
	// width and height are the body's, as last drawn.
	width, height int
	pending       string // the first key of a pair: "d" of "d d"

	edit    editor // the cell being edited, if any
	full    *Full  // an output shown full-screen, if any
	folds   map[int]*fold
	outs    map[*notebook.Output]*shown
	syntax  syntaxCache
	lastTap tap // the last click, for double clicks
}

// New returns a view of the notebook h shows.
func New(h Host) *View {
	return &View{h: h, width: 80, height: 16, anchor: -1, folds: map[int]*fold{}, outs: map[*notebook.Output]*shown{},
		syntax: syntaxCache{spans: map[string][]Span{}}}
}

// Selected is the active cell's index, and whether its output is
// selected rather than the cell.
func (v *View) Selected() (int, bool) {
	v.clamp()
	return v.sel, v.onOut
}

// Range is the selected cells, first and last: the active cell alone,
// or the run of cells Shift+Up and Down selected with it.
func (v *View) Range() (from, to int) {
	v.clamp()
	if v.anchor < 0 {
		return v.sel, v.sel
	}
	return min(v.sel, v.anchor), max(v.sel, v.anchor)
}

// Select makes cell i active and selects it alone, or its output with
// out.
func (v *View) Select(i int, out bool) {
	v.sel, v.onOut, v.anchor = i, out, -1
	v.clamp()
	v.follow()
}

// SelectRange selects cells from to to, the last one active.
func (v *View) SelectRange(from, to int) {
	v.sel, v.anchor, v.onOut = to, from, false
	if from == to {
		v.anchor = -1
	}
	v.clamp()
	v.follow()
}

// Cell is the active cell, if there is one.
func (v *View) Cell() (notebook.Cell, bool) {
	cells := v.h.Cells()
	v.clamp()
	if len(cells) == 0 {
		return notebook.Cell{}, false
	}
	return cells[v.sel], true
}

// clamp keeps the selection on cells there are, and on an output only
// where one shows.
func (v *View) clamp() {
	cells := v.h.Cells()
	v.sel = max(min(v.sel, len(cells)-1), 0)
	if v.anchor >= len(cells) || v.anchor == v.sel {
		v.anchor = -1
	}
	if v.onOut && (len(cells) == 0 || v.outHeight(cells[v.sel]) == 0) {
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
		if sh.data != nil {
			sh.grid = v.h.Grid(c.ID, sh.data, sh.total)
		}
		v.outs[o] = sh
	}
	return sh
}

// foldOf is how cell id's output shows.
func (v *View) foldOf(id int) fold {
	if f := v.folds[id]; f != nil {
		return *f
	}
	return fold{}
}

// foldFor is cell id's fold, to change.
func (v *View) foldFor(id int) *fold {
	f := v.folds[id]
	if f == nil {
		f = &fold{}
		v.folds[id] = f
	}
	return f
}

// Hidden reports whether cell id's output is folded to a line, and
// Whole whether it shows every row rather than a window.
func (v *View) Hidden(id int) bool { return v.foldOf(id).hidden }
func (v *View) Whole(id int) bool  { return v.foldOf(id).whole }

// ToggleHidden folds the selected cells' outputs to a line, or shows
// them again, as Jupyter's o.
func (v *View) ToggleHidden() {
	v.eachFold(func(f *fold, on bool) { f.hidden = !on }, (*fold).isHidden)
}

// ToggleWhole shows the selected cells' outputs whole, or in their
// windows again, as Jupyter's Shift+O toggles scrolling.
func (v *View) ToggleWhole() {
	v.eachFold(func(f *fold, on bool) { f.whole, f.hidden = !on, false }, (*fold).isWhole)
}

func (f *fold) isHidden() bool { return f.hidden }
func (f *fold) isWhole() bool  { return f.whole }

// eachFold sets the selected cells' folds, all as the active cell's is
// turned.
func (v *View) eachFold(set func(f *fold, on bool), get func(*fold) bool) {
	cells := v.h.Cells()
	if len(cells) == 0 {
		return
	}
	from, to := v.Range()
	on := get(v.foldFor(cells[v.sel].ID))
	for i := from; i <= to; i++ {
		set(v.foldFor(cells[i].ID), on)
	}
	v.clamp()
	v.follow()
}

// Resize sets the body's size: the lines under the toolbar and above
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

// startOf is the line cell i's block starts at.
func (v *View) startOf(i int) int {
	cells := v.h.Cells()
	at := 0
	for k := 0; k < i && k < len(cells); k++ {
		at += v.blockOf(k, cells[k]).lines()
	}
	return at
}

// total is how many lines the cells take.
func (v *View) total() int { return v.startOf(len(v.h.Cells())) }

// follow scrolls so the selection shows: the active cell's box, or its
// output's first lines, or the caret while editing.
func (v *View) follow() {
	cells := v.h.Cells()
	if len(cells) == 0 {
		v.top = 0
		return
	}
	at := v.startOf(v.sel)
	b := v.blockOf(v.sel, cells[v.sel])
	first, last := at, at+min(b.lines()-1, v.height-1)
	switch {
	case v.edit.on:
		row, _ := v.edit.caret(v.content())
		first = at + b.box + row
		last = first + b.box // the box's bottom too
	case v.onOut:
		first = at + 2*b.box + b.src
		last = min(first+b.out-1, first+v.height-1)
	}
	v.reveal(first, last)
}

// reveal scrolls so lines first to last show, first when they can't all.
func (v *View) reveal(first, last int) {
	if last >= v.top+v.height {
		v.top = last - v.height + 1
	}
	if first < v.top {
		v.top = first
	}
	v.top = max(v.top, 0)
}

// Reveal scrolls so cell id's block shows, as far as it fits, without
// selecting it: the cell running while every cell runs.
func (v *View) Reveal(id int) {
	for i, c := range v.h.Cells() {
		if c.ID == id {
			at := v.startOf(i)
			v.reveal(at, at+min(v.blockOf(i, c).lines()-2, v.height-1))
			return
		}
	}
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
		out = append(out, "", strings.Repeat(" ", textX)+th.Muted.Render("An empty notebook: ")+th.KeyHints("b", "add a code cell", "m", "make it a note"))
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
