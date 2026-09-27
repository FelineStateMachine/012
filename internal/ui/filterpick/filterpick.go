// Package filterpick is the values list and condition of one column of a
// filter, in the manner of fzf: a condition on top, then the column's
// values with checkboxes, narrowed by a fuzzy search as you type. The
// sheet's filter and a pivot table's filters open it, each saying what
// applying and cancelling do. It knows the UI only through Host.
package filterpick

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the filter picker needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Line is the shared edit line the focused field is typed in.
	Line() *lineedit.Line
	// Close closes the open overlay.
	Close()
	// Locale is how the condition's value is typed and shown.
	Locale() *locale.Locale
}

// Picker is the values list and condition for one column.
type Picker struct {
	h     Host
	title string
	x     int // where the box goes, e.g. over its column
	// OnApply gets the criteria chosen, after the picker has closed.
	OnApply func(cr sheet.Criteria)
	// OnCancel, when set, runs after Esc or a click outside closes the
	// picker.
	OnCancel     func()
	values       []sheet.FilterValue
	checked      map[string]bool
	cond         sheet.Condition
	field        int       // 0 is the search, 1 the condition's value
	fields       [2]string // the text of each field
	shown        []int     // values matching the search, best first
	overlay.List           // over the rows: "Select all", then shown
}

// ID identifies the picker's box in mouse events.
const ID = "filter"

// Rows of the box: the top border, the condition, a separator, the
// search, a separator, then the list.
const firstRow = 5

// New returns a picker titled title at screen column x over values, with
// cond as the condition, calling apply with the criteria chosen. The
// caller opens it, then calls Start.
func New(h Host, title string, x int, values []sheet.FilterValue, cond sheet.Condition, apply func(sheet.Criteria)) *Picker {
	p := &Picker{h: h, title: title, x: x, OnApply: apply, values: values, checked: map[string]bool{}, cond: cond}
	for _, v := range p.values {
		p.checked[v.Text] = v.Shown
	}
	p.fields[1] = sheet.LocalCondArg(p.cond.Op, p.cond.Arg, h.Locale())
	return p
}

// Start readies the edit line for the search, once the picker is open.
func (p *Picker) Start() {
	p.h.Line().Clear()
	p.search()
}

// close closes the picker without applying it.
func (p *Picker) close() {
	p.h.Close()
	if p.OnCancel != nil {
		p.OnCancel()
	}
}

func (p *Picker) Indicator() string { return "FILTER" }

// focus moves editing to field i, keeping the other field's text.
func (p *Picker) focus(i int) {
	line := p.h.Line()
	p.fields[p.field] = line.Text()
	p.field = i
	line.Set(p.fields[i])
}

func (p *Picker) Changed() {
	p.fields[p.field] = p.h.Line().Text()
	if p.field == 0 {
		p.search()
	}
}

// valueLabel is how a value shows in the list: as the sheet's locale
// shows it.
func valueLabel(v sheet.FilterValue) string {
	switch {
	case v.Text == "":
		return "(Blanks)"
	case v.Label != "":
		return v.Label
	}
	return v.Text
}

// search narrows the list to the values matching the search.
func (p *Picker) search() {
	p.Sel, p.Top = 0, 0
	p.shown = p.shown[:0]
	q := strings.TrimSpace(p.fields[0])
	if q == "" {
		for i := range p.values {
			p.shown = append(p.shown, i)
		}
		return
	}
	labels := make([]string, len(p.values))
	for i, v := range p.values {
		labels[i] = valueLabel(v)
	}
	for _, mt := range fuzzy.Find(q, labels) {
		p.shown = append(p.shown, mt.Index)
	}
}

// toggle checks or unchecks list row i: "Select all" (row 0) checks every
// shown value, or unchecks them if they all are.
func (p *Picker) toggle(i int) {
	if i > 0 {
		t := p.values[p.shown[i-1]].Text
		p.checked[t] = !p.checked[t]
		return
	}
	all := p.allChecked()
	for _, k := range p.shown {
		p.checked[p.values[k].Text] = !all
	}
}

func (p *Picker) allChecked() bool {
	for _, k := range p.shown {
		if !p.checked[p.values[k].Text] {
			return false
		}
	}
	return true
}

// cycle steps the condition through Sheets' list.
func (p *Picker) cycle(d int) {
	n := len(sheet.CondOps())
	p.cond.Op = sheet.CondOp((int(p.cond.Op) + d + n) % n)
}

// apply closes the picker and hands on the criteria chosen.
func (p *Picker) apply() {
	p.fields[p.field] = p.h.Line().Text()
	var cr sheet.Criteria
	for _, v := range p.values {
		if !p.checked[v.Text] {
			cr.Hidden = append(cr.Hidden, v.Text)
		}
	}
	cr.Cond = sheet.Condition{Op: p.cond.Op, Arg: sheet.CondArg(p.cond.Op, strings.TrimSpace(p.fields[1]), p.h.Locale())}
	if !cr.Cond.Op.TakesArg() {
		cr.Cond.Arg = ""
	} else if cr.Cond.Arg == "" {
		cr.Cond = sheet.Condition{}
	}
	p.h.Close()
	p.OnApply(cr)
}

// Answer applies criteria a macro recorded, as sheet.Criteria.JSON writes
// them: the values hidden, and a condition with its value in en-US's
// form, as if checked and typed.
func (p *Picker) Answer(text string) error {
	cr, err := sheet.ParseCriteria(text)
	if err != nil {
		return err
	}
	hidden := map[string]bool{}
	for _, v := range cr.Hidden {
		hidden[v] = true
	}
	for _, v := range p.values {
		p.checked[v.Text] = !hidden[v.Text]
	}
	p.cond.Op = cr.Cond.Op
	p.field = 1
	p.h.Line().Set(sheet.LocalCondArg(cr.Cond.Op, cr.Cond.Arg, p.h.Locale()))
	p.apply()
	return nil
}

func (p *Picker) Key(k tea.KeyPressMsg) tea.Cmd {
	n := len(p.shown) + 1
	switch key := k.String(); {
	case key == "esc":
		p.close()
	case key == "enter":
		p.apply()
	case key == "tab" || key == "shift+tab":
		p.focus(1 - p.field)
	case p.field == 1 && (key == "up" || key == "down"):
		if key == "up" {
			p.cycle(-1)
		} else {
			p.cycle(1)
		}
	case key == "up" || key == "ctrl+p":
		p.Move(-1, n)
	case key == "down" || key == "ctrl+n":
		p.Move(1, n)
	case key == "pgup":
		p.Sel = max(p.Sel-p.rows(), 0)
	case key == "pgdown":
		p.Sel = min(p.Sel+p.rows(), n-1)
	case p.field == 0 && key == "space":
		p.toggle(p.Sel)
	default:
		line := p.h.Line()
		before := line.Text()
		line.Key(k)
		if line.Text() != before {
			p.Changed()
		}
	}
	return nil
}

func (p *Picker) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Box != ID {
		if e.Kind == overlay.MousePress {
			p.close()
		}
		return nil
	}
	n := len(p.shown) + 1
	i := p.Top + e.Row - firstRow
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		p.Move(-1, n)
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		p.Move(1, n)
	case e.Kind != overlay.MousePress && e.Kind != overlay.MouseMotion:
	case e.Row == 1 && e.Kind == overlay.MousePress:
		if p.field != 1 {
			p.focus(1)
		}
		switch x := e.Col - 1 - len(" If "); {
		case x == 0:
			p.cycle(-1)
		case x == ansi.StringWidth(p.condChip())-1:
			p.cycle(1)
		}
	case e.Row == 3 && e.Kind == overlay.MousePress:
		p.focus(0)
	case e.Row < firstRow || i >= n || i >= p.Top+p.rows():
	case e.Kind == overlay.MouseMotion:
		p.Sel = i
	case e.Button == tea.MouseLeft:
		p.Sel = i
		p.toggle(i)
	}
	return nil
}

func (p *Picker) Status() (string, string) {
	th := p.h.Theme()
	if p.field == 1 {
		return "Rows must also meet the condition", th.KeyHints("Up/Down", "condition", "Tab", "values", "Enter", "apply", "Esc", "cancel")
	}
	width, _ := p.h.Size()
	pairs := []string{"Space", "check", "Tab", "condition", "Enter", "apply", "Esc", "cancel"}
	desc := "Type to search the values"
	for {
		keys := th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= width:
			return th.Muted.Render(desc), keys
		case desc != "":
			desc = ""
		case len(pairs) > 4:
			pairs = append(pairs[:2], pairs[4:]...)
		default:
			return "", keys
		}
	}
}

// rows is how many list rows show: all of them if they fit above the
// status line, at most twelve.
func (p *Picker) rows() int {
	_, height := p.h.Size()
	return max(min(len(p.shown)+1, 12, height-1-overlay.GridTop-firstRow-1), 1)
}

func (p *Picker) box() (x, y, inner int) {
	width, height := p.h.Size()
	w := 0
	for _, v := range p.values {
		w = max(w, ansi.StringWidth(valueLabel(v))+len(strconv.Itoa(v.Count)))
	}
	inner = min(max(w+10, 36), 48, width-2)
	h := p.rows() + firstRow + 1
	x = max(min(p.x, width-inner-2), 0)
	y = max(min(overlay.GridTop, height-h), 0)
	return x, min(y, max(height-1-h, 0)), inner
}

func (p *Picker) Cursor() (int, int) {
	x, y, _ := p.box()
	caret := ansi.StringWidth(p.h.Line().Head())
	if p.field == 1 {
		return x + 1 + len(" If ") + ansi.StringWidth(p.condChip()) + 2 + caret, y + 1
	}
	return x + 1 + ansi.StringWidth(overlay.SearchPrompt) + caret, y + 3
}

// condChip is the condition's name between arrows that change it.
func (p *Picker) condChip() string {
	return "‹ " + p.cond.Op.Title() + " ›"
}

func (p *Picker) Layout() []overlay.Box {
	th := p.h.Theme()
	x, y, inner := p.box()
	rows := p.rows()
	p.Show(rows)

	chipStyle := th.KeyChip
	if p.field == 1 {
		chipStyle = th.MenuSelected
	}
	cond := th.Muted.Render(" If ") + chipStyle.Render(p.condChip())
	if p.cond.Op.TakesArg() {
		arg := p.fields[1]
		if p.field == 1 {
			arg = p.h.Line().Text()
		}
		if arg == "" && p.field != 1 {
			arg = th.Muted.Render("value")
		}
		cond += "  " + arg
	}
	search := p.fields[0]
	if p.field == 0 {
		search = p.h.Line().Text()
	}
	input := th.Title.Render(overlay.SearchPrompt) + search
	if search == "" {
		input += th.Muted.Render("Search values")
	}
	lines := []string{theme.Cells(th.MenuBar, cond, inner), theme.SepRow, theme.Cells(th.MenuBar, input, inner), theme.SepRow}
	for r := range rows {
		lines = append(lines, p.row(th, p.Top+r, inner))
	}
	footer := strconv.Itoa(len(p.shown)) + " of " + strconv.Itoa(len(p.values))
	return []overlay.Box{{ID: ID, X: x, Y: y, Lines: th.Frame(inner, p.title, footer, lines)}}
}

// row draws list row i: "Select all", then the values shown.
func (p *Picker) row(th *theme.Theme, i, inner int) string {
	if i > len(p.shown) {
		return theme.Cells(th.MenuBar, "", inner)
	}
	label, count, checked := "Select all", "", p.allChecked()
	dim := false
	if i > 0 {
		v := p.values[p.shown[i-1]]
		label, count, checked, dim = valueLabel(v), strconv.Itoa(v.Count), p.checked[v.Text], v.Text == ""
	}
	box := "[ ] "
	if checked {
		box = "[x] "
	}
	base, muted := th.MenuBar, th.Muted
	if i == p.Sel {
		base, muted = th.MenuSelected, th.MenuSelected
	}
	labelStyle := base
	if dim {
		labelStyle = muted
	}
	text := base.Render(" "+box) + theme.Cells(labelStyle, label, inner-6-len(count)-1)
	text += muted.Render(theme.PadLeft(count, len(count)+1)) + base.Render(" ")
	return ansi.Truncate(text, inner, "")
}
