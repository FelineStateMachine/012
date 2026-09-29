package nbview

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// Keys. Command mode moves between cells itself (Up and Down, j and k,
// Home and End, PgUp and PgDn) and runs the commands Keys binds for the
// rest; a key it doesn't know is the UI's (menus, other sheets, Save).
// Edit mode types into the cell, with EditKeys' commands (Esc, running)
// and Tab completing.

// Editing reports whether a cell is being edited.
func (v *View) Editing() bool { return v.edit.on }

// FullOpen reports whether an output is shown full-screen.
func (v *View) FullOpen() bool { return v.full != nil }

// Pending is the first key of a pair waiting for its second: "d" or
// "0".
func (v *View) Pending() string { return v.pending }

// Key takes a key, reporting whether the notebook took it.
func (v *View) Key(k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case v.full != nil:
		if v.full.Key(k) {
			v.full = nil
		}
		return nil, true
	case v.edit.on:
		return v.editKey(k)
	}
	return v.commandKey(k)
}

func (v *View) commandKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	key := k.String()
	if p := v.pending; p != "" {
		v.pending = ""
		if id, ok := v.Keys[p+" "+key]; ok {
			return v.h.Run(id), true
		}
	}
	if key == "esc" {
		return nil, true // command mode is the notebook's outermost level
	}
	if v.move(key) {
		return nil, true
	}
	for pair := range v.Keys {
		if first, _, ok := strings.Cut(pair, " "); ok && first == key {
			v.pending = key
			return nil, true
		}
	}
	if id, ok := v.Keys[key]; ok {
		return v.h.Run(id), true
	}
	return nil, false
}

// move moves the selection for a movement key.
func (v *View) move(key string) bool {
	n := len(v.h.Cells())
	switch key {
	case "up", "k":
		v.step(-1)
	case "down", "j":
		v.step(1)
	case "home", "ctrl+home":
		v.Select(0, false)
	case "end", "ctrl+end":
		v.Select(n-1, false)
	case "pgup":
		v.top = max(v.top-v.height+1, 0)
		v.selectAtTop()
	case "pgdown":
		v.top += v.height - 1
		v.selectAtTop()
	default:
		return false
	}
	return true
}

// step moves the selection one place: from a cell to its output, if it
// shows one, and from an output to the next cell.
func (v *View) step(d int) {
	cells := v.h.Cells()
	if len(cells) == 0 {
		return
	}
	hasOut := func(i int) bool { return v.shown(cells[i]).kind != outNone }
	switch {
	case d > 0 && !v.onOut && hasOut(v.sel):
		v.onOut = true
	case d > 0 && v.sel < len(cells)-1:
		v.sel, v.onOut = v.sel+1, false
	case d < 0 && v.onOut:
		v.onOut = false
	case d < 0 && v.sel > 0:
		v.sel--
		v.onOut = hasOut(v.sel)
	}
	v.follow()
}

// selectAtTop selects the first cell whose head shows after a page
// moved the view.
func (v *View) selectAtTop() {
	cells := v.h.Cells()
	total := 0
	for i, c := range cells {
		total += v.blockOf(i, c).lines()
	}
	v.top = max(min(v.top, total-v.height), 0)
	at := 0
	for i, c := range cells {
		if at >= v.top {
			v.sel, v.onOut = i, false
			return
		}
		at += v.blockOf(i, c).lines()
	}
	v.sel, v.onOut = max(len(cells)-1, 0), false
}

func (v *View) editKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	key := k.String()
	e := &v.edit
	if len(e.comp) > 0 {
		switch key {
		case "tab", "down", "ctrl+n":
			e.compSel = (e.compSel + 1) % len(e.comp)
			return nil, true
		case "shift+tab", "up", "ctrl+p":
			e.compSel = (e.compSel + len(e.comp) - 1) % len(e.comp)
			return nil, true
		case "enter":
			v.complete()
			return v.changed(), true
		case "esc":
			e.comp = nil
			return nil, true
		}
		e.comp = nil
	}
	if id, ok := v.EditKeys[key]; ok {
		return v.h.Run(id), true
	}
	if key == "tab" {
		return v.askCompletions(), true
	}
	before := e.text()
	if !e.area.Key(k, v.content()) {
		return nil, true // a key edit mode doesn't use does nothing
	}
	v.follow()
	if e.text() != before {
		return v.changed(), true
	}
	return nil, true
}

// Paste types text into the cell being edited.
func (v *View) Paste(text string) tea.Cmd {
	if !v.edit.on {
		return nil
	}
	v.edit.area.InsertText(text)
	v.follow()
	return v.changed()
}

// StartEdit edits the selected cell, the caret at the end of its source.
func (v *View) StartEdit() tea.Cmd {
	c, ok := v.Cell()
	if !ok {
		return nil
	}
	v.full, v.onOut = nil, false
	v.edit = editor{on: true, id: c.ID}
	v.edit.area.SetText(c.Source)
	v.follow()
	return v.changed()
}

// StopEdit leaves edit mode, keeping what was typed.
func (v *View) StopEdit() {
	if !v.edit.on {
		return
	}
	id, text := v.edit.id, v.edit.text()
	v.edit = editor{}
	for _, c := range v.h.Cells() {
		if c.ID == id && c.Source != text {
			v.h.Edit(id, text)
		}
	}
	v.follow()
}

// Commit keeps what's typed as the cell's source, still editing.
func (v *View) Commit() {
	if !v.edit.on {
		return
	}
	for _, c := range v.h.Cells() {
		if c.ID == v.edit.id && c.Source != v.edit.text() {
			v.h.Edit(c.ID, v.edit.text())
		}
	}
}

// OpenFull shows the selected cell's output full-screen.
func (v *View) OpenFull() bool {
	c, ok := v.Cell()
	if !ok {
		return false
	}
	sh := v.shown(c)
	if sh.kind == outNone {
		return false
	}
	title := "Output"
	if name := c.Name(); name != "" {
		title = name
	}
	if o := v.h.Output(c.ID); o != nil && o.Count > 0 {
		title += " [" + itoa(o.Count) + "]"
	}
	v.full = newFull(title, sh, v.width, v.height)
	return true
}

// CloseFull goes back from a full-screen output.
func (v *View) CloseFull() { v.full = nil }

// Full is the output shown full-screen, or nil.
func (v *View) Full() *Full { return v.full }

// Cursor is where the terminal's caret goes in the body, while a cell
// is edited.
func (v *View) Cursor() (x, y int, ok bool) {
	if !v.edit.on {
		return 0, 0, false
	}
	cells := v.h.Cells()
	at := 0
	for i := range v.sel {
		at += v.blockOf(i, cells[i]).lines()
	}
	row, col := v.edit.caret(v.content())
	y = at + 1 + row - v.top
	if y < 0 || y >= v.height {
		return 0, 0, false
	}
	return gutter + col, y, true
}

// tap is a click, for telling a double click.
type tap struct {
	at   time.Time
	cell int
}

// Click selects what's under line y of the body, its output or the
// cell; clicking the cell again quickly edits it.
func (v *View) Click(y int) tea.Cmd {
	if v.full != nil || v.edit.on {
		return nil
	}
	cells := v.h.Cells()
	at := 0
	for i, c := range cells {
		b := v.blockOf(i, c)
		if line := v.top + y - at; line >= 0 && line < b.lines() {
			double := v.lastTap.cell == i && time.Since(v.lastTap.at) < 400*time.Millisecond
			v.lastTap = tap{at: time.Now(), cell: i}
			v.Select(i, line > b.head+b.src-1 && line < b.lines()-1 && b.out > 0)
			if double && !v.onOut {
				return v.h.Run("nb.edit")
			}
			return nil
		}
		at += b.lines()
	}
	return nil
}

// Wheel scrolls the body d lines.
func (v *View) Wheel(d int) {
	if v.full != nil {
		v.full.row += d
		v.full.settle()
		return
	}
	cells := v.h.Cells()
	total := 0
	for i, c := range cells {
		total += v.blockOf(i, c).lines()
	}
	v.top = max(min(v.top+d, total-v.height), 0)
}

// Head is what the formula bar says of the selection: the cell's name
// or number, and what it reads, or what its output holds.
func (v *View) Head() (name, text string) {
	c, ok := v.Cell()
	if !ok {
		return "empty", ""
	}
	i, _ := v.Selected()
	name = "cell " + itoa(i+1)
	if n := c.Name(); n != "" {
		name = n
	}
	if c.Kind == notebook.Note {
		return name, "Markdown"
	}
	if v.onOut {
		return name, v.outputSummary(c)
	}
	var reads []string
	for _, r := range notebook.Refs(c.Pipeline()) {
		reads = append(reads, "$"+r)
	}
	if notebook.ReadsSelection(c.Pipeline()) {
		reads = append(reads, "$selection")
	}
	if _, refs := notebook.Bind(c.Pipeline()); len(refs) > 0 {
		for _, r := range refs {
			reads = append(reads, "$sheet."+r.Ref)
		}
	}
	text = "a nushell pipeline"
	if len(reads) > 0 {
		text = "reads " + strings.Join(reads, ", ")
	}
	if n := c.Name(); n != "" {
		text += "; read as $" + n + ", nu." + n + " in formulas"
	}
	return name, text
}

// outputSummary says what an output holds.
func (v *View) outputSummary(c notebook.Cell) string {
	sh := v.shown(c)
	switch sh.kind {
	case outTable:
		return "a table: " + more(sh.total, "row") + ", " + more(len(sh.cols), "column")
	case outRecord:
		return "a record: " + more(sh.total, "field")
	case outList:
		return "a list: " + more(sh.total, "item")
	case outText:
		return "text: " + more(sh.total, "line")
	case outError:
		return "an error"
	case outUnsaved:
		return "not saved in the file; run the cell to see it"
	}
	return "a value"
}

func itoa(n int) string { return strconv.Itoa(n) }
