package nbview

import (
	"strconv"
	"strings"

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

// move moves the selection for a movement key: Up and Down (j and k)
// scroll a selected output's window before they leave it, and with
// Shift (or J and K) select the cells passed over, as JupyterLab does.
func (v *View) move(key string) bool {
	n := len(v.h.Cells())
	switch key {
	case "up", "k":
		if !v.scrollOut(-1) {
			v.step(-1)
		}
	case "down", "j":
		if !v.scrollOut(1) {
			v.step(1)
		}
	case "shift+up", "K":
		v.extend(-1)
	case "shift+down", "J":
		v.extend(1)
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
// shows one, and from an output to the next cell. It selects one cell.
func (v *View) step(d int) {
	cells := v.h.Cells()
	if len(cells) == 0 {
		return
	}
	v.anchor = -1
	hasOut := func(i int) bool { return v.outHeight(cells[i]) > 0 }
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

// extend moves the active cell one cell, keeping the cells passed over
// selected with it.
func (v *View) extend(d int) {
	n := len(v.h.Cells())
	if n == 0 || v.sel+d < 0 || v.sel+d >= n {
		return
	}
	if v.anchor < 0 {
		v.anchor = v.sel
	}
	v.sel += d
	v.onOut = false
	v.clamp()
	v.follow()
}

// scrollOut scrolls the selected output's window d rows, reporting
// whether it could.
func (v *View) scrollOut(d int) bool {
	c, ok := v.Cell()
	if !ok || !v.onOut {
		return false
	}
	return v.scrollBy(c, d)
}

// scrollBy scrolls cell c's output window d rows, as far as it goes,
// reporting whether it moved.
func (v *View) scrollBy(c notebook.Cell, d int) bool {
	f := v.foldOf(c.ID)
	if f.hidden || f.whole {
		return false
	}
	limit := v.shown(c).maxScroll(false, v.outWidth())
	to := min(max(f.scroll+d, 0), limit)
	if to == f.scroll {
		return false
	}
	v.foldFor(c.ID).scroll = to
	return true
}

// selectAtTop selects the first cell whose top shows after a page
// moved the view.
func (v *View) selectAtTop() {
	cells := v.h.Cells()
	v.top = max(min(v.top, v.total()-v.height), 0)
	at := 0
	v.anchor = -1
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
	v.full, v.onOut, v.anchor = nil, false, -1
	v.edit.cancel()
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
	v.edit.cancel()
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
