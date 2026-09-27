// Package rules is the side panel for a sheet's rules, as Sheets'
// Conditional formatting and Data validation sidebars: a list of the
// rules, and a form to add or edit one. It is docked at the right of
// the grid in the manner of lazygit's panels, and knows the UI only
// through Host.
package rules

import (
	"image/color"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the rules panel needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Line is the shared edit line text rows are typed in.
	Line() *lineedit.Line
	// Close closes the open overlay.
	Close()
	// Sheet is the sheet whose rules are edited.
	Sheet() *sheet.Sheet
	// Selection is the range a new rule applies to.
	Selection() sheet.Rect
	// SaveFormat and SaveValidation store rule i (-1 adds one), as the
	// commands macros record do, reporting why they couldn't.
	SaveFormat(i int, f sheet.CondFormat) error
	SaveValidation(i int, v sheet.Validation) error
	// Edited marks the file modified after a rule was removed or moved.
	Edited()
	// Slot is the terminal's color for an ANSI slot, for color scales.
	Slot(i int) color.Color
}

// kind is what the panel lists: conditional formats or validation.
type kind interface {
	title() string
	count(s *sheet.Sheet) int
	// item draws rule i of the list in w columns on base.
	item(th *theme.Theme, h Host, i, w int, sel bool) string
	// form opens rule i for editing, or a new rule over sel for -1.
	form(h Host, i int, sel sheet.Rect) form
	remove(s *sheet.Sheet, i int)
	// move moves rule i to to, and reports whether rules have an order.
	move(s *sheet.Sheet, i, to int) bool
	// ordered reports whether the rules' order matters, and listHint
	// says how rules combine, for the status line.
	ordered() bool
	listHint() string
}

// form is a rule being added or edited.
type form interface {
	rows(th *theme.Theme, h Host) []row
	save(h Host) error
	title() string
}

// Editor is the rules panel.
type Editor struct {
	h    Host
	k    kind
	f    form // the form open, or nil for the list
	sel  int  // the picked row of the form
	msg  string
	list overlay.List // over "Add rule", then the rules
	top  int          // the form's scroll
}

// ID identifies the panel's box in mouse events.
const ID = "rules"

// Formats opens the panel on the sheet's conditional formats.
func Formats(h Host) *Editor { return &Editor{h: h, k: cfKind{}} }

// Validations opens the panel on the sheet's data validation.
func Validations(h Host) *Editor { return &Editor{h: h, k: dvKind{}} }

// Add opens the form for a new rule over the selection, as Insert >
// Dropdown and a new rule do; with dropdown, a dropdown list.
func (e *Editor) Add(dropdown bool) *Editor {
	e.openForm(-1)
	if f, ok := e.f.(*dvForm); ok && dropdown {
		f.kind = int(sheet.ValidList)
	}
	return e
}

func (e *Editor) Indicator() string { return "RULES" }

func (e *Editor) openForm(i int) {
	e.f, e.msg, e.top = e.k.form(e.h, i, e.h.Selection()), "", 0
	e.sel = -1
	e.step(e.rows(), 1)
	e.enter()
}

// rows is the open form's rows.
func (e *Editor) rows() []row { return e.f.rows(e.h.Theme(), e.h) }

// enter puts the picked text row's text in the edit line.
func (e *Editor) enter() {
	if rs := e.rows(); e.sel < len(rs) && rs[e.sel].kind == rowText {
		e.h.Line().Set(*rs[e.sel].text)
	}
}

// leave keeps what was typed in the picked text row.
func (e *Editor) leave() {
	if rs := e.rows(); e.sel >= 0 && e.sel < len(rs) && rs[e.sel].kind == rowText {
		*rs[e.sel].text = e.h.Line().Text()
	}
}

// step picks the next row d away that can be picked.
func (e *Editor) step(rs []row, d int) {
	for i := e.sel + d; i >= 0 && i < len(rs); i += d {
		if rs[i].picks() {
			e.sel = i
			return
		}
	}
}

// pickRow moves the form to row i.
func (e *Editor) pickRow(i int) {
	e.leave()
	e.sel = i
	e.enter()
}

func (e *Editor) Key(k tea.KeyPressMsg) tea.Cmd {
	if e.f != nil {
		e.formKey(k)
		return nil
	}
	n := e.k.count(e.h.Sheet()) + 1
	switch key := k.String(); key {
	case "esc":
		e.h.Close()
	case "up", "ctrl+p", "shift+tab":
		e.list.Move(-1, n)
	case "down", "ctrl+n", "tab":
		e.list.Move(1, n)
	case "enter", "space":
		e.openForm(e.list.Sel - 1)
	case "a", "+":
		e.openForm(-1)
	case "delete", "backspace", "x", "-":
		if e.list.Sel > 0 {
			e.k.remove(e.h.Sheet(), e.list.Sel-1)
			e.h.Edited()
			e.list.Sel = min(e.list.Sel, e.k.count(e.h.Sheet()))
		}
	case "shift+up", "shift+down":
		e.reorder(key == "shift+down")
	}
	return nil
}

// reorder moves the highlighted rule up or down the list.
func (e *Editor) reorder(down bool) {
	i := e.list.Sel - 1
	to := i - 1
	if down {
		to = i + 1
	}
	if i < 0 || to < 0 || to >= e.k.count(e.h.Sheet()) {
		return
	}
	if e.k.move(e.h.Sheet(), i, to) {
		e.list.Sel = to + 1
		e.h.Edited()
	}
}

func (e *Editor) formKey(k tea.KeyPressMsg) {
	rs := e.rows()
	r := rs[e.sel]
	switch key := k.String(); {
	case key == "esc":
		e.f, e.msg = nil, ""
	case key == "enter":
		e.leave()
		if err := e.f.save(e.h); err != nil {
			e.msg = err.Error()
			return
		}
		e.f, e.msg = nil, ""
	case key == "up" || key == "shift+tab" || key == "ctrl+p":
		e.leave()
		e.step(rs, -1)
		e.enter()
	case key == "down" || key == "tab" || key == "ctrl+n":
		e.leave()
		e.step(rs, 1)
		e.enter()
	case r.kind == rowChoice && (key == "left" || key == "right" || key == "space"):
		r.cycle(map[bool]int{true: -1, false: 1}[key == "left"])
	case r.kind == rowCheck && key == "space":
		*r.on = !*r.on
	case r.kind == rowText:
		e.h.Line().Key(k)
		*r.text = e.h.Line().Text()
	}
}

func (e *Editor) Mouse(ev overlay.MouseEvent) tea.Cmd {
	if ev.Box != ID {
		if ev.Kind == overlay.MousePress {
			e.h.Close()
		}
		return nil
	}
	up := ev.Kind == overlay.MouseWheel && ev.Button == tea.MouseWheelUp
	down := ev.Kind == overlay.MouseWheel && ev.Button == tea.MouseWheelDown
	press := ev.Kind == overlay.MousePress && ev.Button == tea.MouseLeft
	if e.f == nil {
		n := e.k.count(e.h.Sheet()) + 1
		i := e.list.Top + ev.Row - 1
		switch {
		case up:
			e.list.Move(-1, n)
		case down:
			e.list.Move(1, n)
		case press && i >= 0 && i < n && i == e.list.Sel:
			e.openForm(i - 1)
		case press && i >= 0 && i < n:
			e.list.Sel = i
		}
		return nil
	}
	rs := e.rows()
	i := e.top + ev.Row - 1
	switch {
	case up:
		e.formKey(tea.KeyPressMsg{Code: tea.KeyUp})
	case down:
		e.formKey(tea.KeyPressMsg{Code: tea.KeyDown})
	case !press || i < 0 || i >= len(rs) || !rs[i].picks():
	case i != e.sel:
		e.pickRow(i)
	case rs[i].kind == rowChoice:
		rs[i].cycle(1)
	case rs[i].kind == rowCheck:
		*rs[i].on = !*rs[i].on
	}
	return nil
}

// box places the panel at the right of the grid.
func (e *Editor) box() (x, y, inner int) {
	width, _ := e.h.Size()
	inner = min(46, width-2)
	return max(width-inner-2, 0), GridTop, inner
}

// GridTop is the first screen row of the grid, under the control panel
// and the column headers.
const GridTop = 4

// visible is how many rows of n fit above the status line.
func (e *Editor) visible(n int) int {
	_, height := e.h.Size()
	return max(min(n, height-1-GridTop-2), 1)
}

func (e *Editor) Layout() []overlay.Box {
	th := e.h.Theme()
	x, y, inner := e.box()
	if e.f == nil {
		return []overlay.Box{{ID: ID, X: x, Y: y, Lines: th.Frame(inner, e.k.title(), e.footer(), e.listLines(inner))}}
	}
	rs := e.rows()
	n := e.visible(len(rs))
	e.top = max(e.sel-n+1, min(e.top, e.sel), 0)
	var lines []string
	for i := e.top; i < min(e.top+n, len(rs)); i++ {
		lines = append(lines, rs[i].draw(th, inner, i == e.sel, e.h.Line().Text()))
	}
	return []overlay.Box{{ID: ID, X: x, Y: y, Lines: th.Frame(inner, e.f.title(), "", lines)}}
}

func (e *Editor) footer() string {
	n := e.k.count(e.h.Sheet())
	if n == 1 {
		return "1 rule"
	}
	return strconv.Itoa(n) + " rules"
}

func (e *Editor) listLines(inner int) []string {
	th := e.h.Theme()
	n := e.k.count(e.h.Sheet()) + 1
	e.list.Sel = min(e.list.Sel, n-1)
	rows := e.visible(n)
	e.list.Show(rows)
	var lines []string
	for r := range rows {
		i := e.list.Top + r
		if i >= n {
			break
		}
		base := th.MenuBar
		if i == e.list.Sel {
			base = th.MenuSelected
		}
		if i == 0 {
			lines = append(lines, theme.Cells(base, " + Add rule", inner))
			continue
		}
		lines = append(lines, e.k.item(th, e.h, i-1, inner, i == e.list.Sel))
	}
	return lines
}

// Cursor is the caret in a text row being typed into.
func (e *Editor) Cursor() (int, int) {
	if e.f == nil {
		return -1, -1
	}
	rs := e.rows()
	if e.sel >= len(rs) || rs[e.sel].kind != rowText {
		return -1, -1
	}
	x, y, _ := e.box()
	return x + 1 + textX() + ansi.StringWidth(e.h.Line().Head()), y + 1 + e.sel - e.top
}

// Changed keeps what's typed in the picked text row.
func (e *Editor) Changed() {
	if e.f != nil {
		e.leave()
	}
}

func (e *Editor) Status() (string, string) {
	th := e.h.Theme()
	width, _ := e.h.Size()
	var desc string
	var pairs []string
	switch {
	case e.f == nil && e.list.Sel == 0:
		desc, pairs = "Add a rule over the selection", []string{"Enter", "add", "Esc", "close"}
	case e.f == nil:
		desc, pairs = e.k.listHint(), []string{"Enter", "edit", "Del", "remove", "Esc", "close"}
		if e.k.ordered() {
			pairs = []string{"Enter", "edit", "Del", "remove", "Shift+↑/↓", "move", "Esc", "close"}
		}
	default:
		r := e.rows()[e.sel]
		desc = r.hint
		pairs = []string{"Enter", "save", "Esc", "back"}
		switch r.kind {
		case rowChoice:
			pairs = append([]string{"←/→", "change"}, pairs...)
		case rowCheck:
			pairs = append([]string{"Space", "on/off"}, pairs...)
		}
	}
	if e.msg != "" {
		desc = th.Warning.Render(e.msg)
	} else {
		desc = th.Muted.Render(desc)
	}
	for {
		keys := th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= width:
			return desc, keys
		case e.msg == "" && desc != "":
			desc = ""
		case len(pairs) > 4:
			pairs = pairs[2:]
		default:
			return desc, keys
		}
	}
}
